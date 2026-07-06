package rfs

import (
	"context"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
)

func TestPollerStoresExtractedSnapshotAndAppliesFirstSeenFallback(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInMemorySQLiteStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	sourceDate := time.Date(1982, 4, 7, 0, 0, 0, 0, time.UTC)
	fetcher := &fakeFetcher{result: FetchResult{Status: FetchModified, Document: emptyDoc(t), Cache: FetchCache{ETag: `"v1"`}}}
	flow := &fakeFlow{items: []ExtractedItem{
		{GUID: "with-source-date", Title: "With source date", Link: "https://example.com/a", PubDate: &sourceDate},
		{GUID: "needs-first-seen", Title: "Needs first seen", Link: "https://example.com/b"},
	}}
	poller := Poller{Fetcher: fetcher, Store: store, Clock: fixedClock{now: now}}

	result, err := poller.Poll(ctx, Source{ID: "meltzer", URL: "https://example.com", Flow: flow})
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if result.Status != PollUpdated {
		t.Fatalf("expected updated status, got %v", result.Status)
	}
	if fetcher.cache != (FetchCache{}) {
		t.Fatalf("expected first fetch to use empty cache, got %#v", fetcher.cache)
	}

	items, err := store.LoadSnapshot(ctx, "meltzer")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 stored items, got %#v", items)
	}
	byGUID := map[string]Item{}
	for _, item := range items {
		byGUID[item.GUID] = item
	}
	if !byGUID["with-source-date"].PubDate.Equal(sourceDate) {
		t.Fatalf("source-derived pubDate was not kept: %#v", byGUID["with-source-date"])
	}
	if !byGUID["needs-first-seen"].PubDate.Equal(now) {
		t.Fatalf("first-seen fallback was not applied: %#v", byGUID["needs-first-seen"])
	}

	cache, err := store.LoadFetchCache(ctx, "meltzer")
	if err != nil {
		t.Fatalf("load fetch cache: %v", err)
	}
	if cache.ETag != `"v1"` {
		t.Fatalf("fetch cache was not saved: %#v", cache)
	}
}

func TestPollerDeduplicatesExtractedGUIDsBeforeSavingSnapshot(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInMemorySQLiteStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	pubDate := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	fetcher := &fakeFetcher{result: FetchResult{Status: FetchModified, Document: emptyDoc(t)}}
	flow := &fakeFlow{items: []ExtractedItem{
		{GUID: "duplicate", Title: "First", Link: "https://example.com/a", PubDate: &pubDate},
		{GUID: "duplicate", Title: "Second", Link: "https://example.com/b", PubDate: &pubDate},
	}}
	poller := Poller{Fetcher: fetcher, Store: store, Clock: fixedClock{now: pubDate}}

	_, err = poller.Poll(ctx, Source{ID: "meltzer", URL: "https://example.com", Flow: flow})
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}

	items, err := store.LoadSnapshot(ctx, "meltzer")
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if len(items) != 1 || items[0].GUID != "duplicate" || items[0].Title != "First" {
		t.Fatalf("snapshot was not deduplicated keeping first item: %#v", items)
	}
}

func TestPollerSkipsFlowWhenSourceNotModified(t *testing.T) {
	ctx := context.Background()
	store, err := OpenInMemorySQLiteStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	if err := store.SaveFetchCache(ctx, "meltzer", FetchCache{ETag: `"v1"`}); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	flow := &fakeFlow{}
	fetcher := &fakeFetcher{result: FetchResult{Status: FetchNotModified, Cache: FetchCache{ETag: `"v1"`}}}
	poller := Poller{Fetcher: fetcher, Store: store, Clock: fixedClock{now: time.Now()}}

	result, err := poller.Poll(ctx, Source{ID: "meltzer", URL: "https://example.com", Flow: flow})
	if err != nil {
		t.Fatalf("Poll returned error: %v", err)
	}
	if result.Status != PollUnchanged {
		t.Fatalf("expected unchanged status, got %v", result.Status)
	}
	if flow.called {
		t.Fatal("flow should not be called for not-modified source")
	}
	if fetcher.cache.ETag != `"v1"` {
		t.Fatalf("poller did not pass stored fetch cache: %#v", fetcher.cache)
	}
}

type fakeFetcher struct {
	result FetchResult
	cache  FetchCache
}

func (f *fakeFetcher) Fetch(ctx context.Context, url string, cache FetchCache) (FetchResult, error) {
	f.cache = cache
	return f.result, nil
}

type fakeFlow struct {
	items  []ExtractedItem
	called bool
}

func (f *fakeFlow) Extract(doc *html.Node) ([]ExtractedItem, error) {
	f.called = true
	return f.items, nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

func emptyDoc(t *testing.T) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(`<html><body></body></html>`))
	if err != nil {
		t.Fatalf("parse empty doc: %v", err)
	}
	return doc
}
