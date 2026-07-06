package rfs

import (
	"context"
	"testing"
	"time"
)

func TestSQLiteStoreSavesAndLoadsSnapshotAndFetchCache(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInMemorySQLiteStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	cache := FetchCache{ETag: `"v1"`, LastModified: "Wed, 07 Apr 1982 00:00:00 GMT"}
	if err := store.SaveFetchCache(ctx, "meltzer", cache); err != nil {
		t.Fatalf("save fetch cache: %v", err)
	}
	loadedCache, err := store.LoadFetchCache(ctx, "meltzer")
	if err != nil {
		t.Fatalf("load fetch cache: %v", err)
	}
	if loadedCache != cache {
		t.Fatalf("cache = %#v, want %#v", loadedCache, cache)
	}

	oldItem := Item{GUID: "old", Title: "Old", Link: "https://example.com/old", PubDate: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
	newItem := Item{GUID: "new", Title: "New", Link: "https://example.com/new", PubDate: time.Date(1982, 4, 7, 0, 0, 0, 0, time.UTC)}
	if err := store.SaveSnapshot(ctx, "meltzer", []Item{oldItem, newItem}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	items, err := store.LoadSnapshot(ctx, "meltzer")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(items) != 2 || items[0].GUID != "new" || items[1].GUID != "old" {
		t.Fatalf("items not loaded newest-first: %#v", items)
	}

	if err := store.SaveSnapshot(ctx, "meltzer", []Item{newItem}); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	items, err = store.LoadSnapshot(ctx, "meltzer")
	if err != nil {
		t.Fatalf("load replaced snapshot: %v", err)
	}
	if len(items) != 1 || items[0].GUID != "new" {
		t.Fatalf("snapshot was not replaced: %#v", items)
	}
}

func TestSQLiteStoreFirstSeenIsStablePerSourceAndGUID(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInMemorySQLiteStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	first := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	later := first.Add(24 * time.Hour)

	got, err := store.FirstSeen(ctx, "meltzer", "guid-1", first)
	if err != nil {
		t.Fatalf("first FirstSeen: %v", err)
	}
	if !got.Equal(first) {
		t.Fatalf("first seen = %s, want %s", got, first)
	}

	got, err = store.FirstSeen(ctx, "meltzer", "guid-1", later)
	if err != nil {
		t.Fatalf("second FirstSeen: %v", err)
	}
	if !got.Equal(first) {
		t.Fatalf("first seen changed: got %s want %s", got, first)
	}

	otherSource, err := store.FirstSeen(ctx, "other", "guid-1", later)
	if err != nil {
		t.Fatalf("other source FirstSeen: %v", err)
	}
	if !otherSource.Equal(later) {
		t.Fatalf("first seen should be source-scoped: got %s want %s", otherSource, later)
	}
}
