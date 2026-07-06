package rfs

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func OpenInMemorySQLiteStore() (*SQLiteStore, error) {
	return OpenSQLiteStore(":memory:")
}

func OpenSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &SQLiteStore{db: db}
	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) init(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS snapshots (
			source_id TEXT NOT NULL,
			guid TEXT NOT NULL,
			title TEXT NOT NULL,
			link TEXT NOT NULL,
			description TEXT NOT NULL,
			pub_date TEXT NOT NULL,
			PRIMARY KEY (source_id, guid)
		)`,
		`CREATE TABLE IF NOT EXISTS first_seen (
			source_id TEXT NOT NULL,
			guid TEXT NOT NULL,
			seen_at TEXT NOT NULL,
			PRIMARY KEY (source_id, guid)
		)`,
		`CREATE TABLE IF NOT EXISTS fetch_cache (
			source_id TEXT PRIMARY KEY,
			etag TEXT NOT NULL DEFAULT '',
			last_modified TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) SaveSnapshot(ctx context.Context, sourceID string, items []Item) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer rollbackUnlessCommitted(tx)

	if _, err := tx.ExecContext(ctx, `DELETE FROM snapshots WHERE source_id = ?`, sourceID); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO snapshots (source_id, guid, title, link, description, pub_date) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		if _, err := stmt.ExecContext(ctx, sourceID, item.GUID, item.Title, item.Link, item.Description, formatStoreTime(item.PubDate)); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) LoadSnapshot(ctx context.Context, sourceID string) ([]Item, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT guid, title, link, description, pub_date FROM snapshots WHERE source_id = ? ORDER BY pub_date DESC, guid ASC`, sourceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		var pubDate string
		if err := rows.Scan(&item.GUID, &item.Title, &item.Link, &item.Description, &pubDate); err != nil {
			return nil, err
		}
		parsed, err := parseStoreTime(pubDate)
		if err != nil {
			return nil, fmt.Errorf("parse stored pubDate for %s: %w", item.GUID, err)
		}
		item.PubDate = parsed
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) FirstSeen(ctx context.Context, sourceID, guid string, discoveredAt time.Time) (time.Time, error) {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO first_seen (source_id, guid, seen_at) VALUES (?, ?, ?)`, sourceID, guid, formatStoreTime(discoveredAt))
	if err != nil {
		return time.Time{}, err
	}

	var seenAt string
	if err := s.db.QueryRowContext(ctx, `SELECT seen_at FROM first_seen WHERE source_id = ? AND guid = ?`, sourceID, guid).Scan(&seenAt); err != nil {
		return time.Time{}, err
	}
	return parseStoreTime(seenAt)
}

func (s *SQLiteStore) SaveFetchCache(ctx context.Context, sourceID string, cache FetchCache) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO fetch_cache (source_id, etag, last_modified) VALUES (?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET etag = excluded.etag, last_modified = excluded.last_modified`, sourceID, cache.ETag, cache.LastModified)
	return err
}

func (s *SQLiteStore) LoadFetchCache(ctx context.Context, sourceID string) (FetchCache, error) {
	var cache FetchCache
	err := s.db.QueryRowContext(ctx, `SELECT etag, last_modified FROM fetch_cache WHERE source_id = ?`, sourceID).Scan(&cache.ETag, &cache.LastModified)
	if err == sql.ErrNoRows {
		return FetchCache{}, nil
	}
	return cache, err
}

func rollbackUnlessCommitted(tx *sql.Tx) {
	_ = tx.Rollback()
}

func formatStoreTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseStoreTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}
