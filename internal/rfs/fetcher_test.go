package rfs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPFetcherFetchesModifiedPageAndCachesValidators(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Wed, 07 Apr 1982 00:00:00 GMT")
		_, _ = w.Write([]byte(`<html><body><p>Hello</p></body></html>`))
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(server.Client())
	result, err := fetcher.Fetch(context.Background(), server.URL, FetchCache{})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Status != FetchModified {
		t.Fatalf("expected modified status, got %v", result.Status)
	}
	if got := string(result.Page); got != `<html><body><p>Hello</p></body></html>` {
		t.Fatalf("unexpected fetched page: %q", got)
	}
	if result.Cache.ETag != `"v1"` {
		t.Fatalf("unexpected ETag: %q", result.Cache.ETag)
	}
	if result.Cache.LastModified != "Wed, 07 Apr 1982 00:00:00 GMT" {
		t.Fatalf("unexpected Last-Modified: %q", result.Cache.LastModified)
	}
}

func TestHTTPFetcherUsesConditionalHeadersAndReportsNotModified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-None-Match"); got != `"v1"` {
			t.Fatalf("If-None-Match = %q", got)
		}
		if got := r.Header.Get("If-Modified-Since"); got != "Wed, 07 Apr 1982 00:00:00 GMT" {
			t.Fatalf("If-Modified-Since = %q", got)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(server.Client())
	result, err := fetcher.Fetch(context.Background(), server.URL, FetchCache{
		ETag:         `"v1"`,
		LastModified: "Wed, 07 Apr 1982 00:00:00 GMT",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Status != FetchNotModified {
		t.Fatalf("expected not-modified status, got %v", result.Status)
	}
	if result.Page != nil {
		t.Fatal("expected no page for 304")
	}
}

func TestHTTPFetcherReportsThrottlingWithRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	fetcher := NewHTTPFetcher(server.Client())
	result, err := fetcher.Fetch(context.Background(), server.URL, FetchCache{})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if result.Status != FetchThrottled {
		t.Fatalf("expected throttled status, got %v", result.Status)
	}
	if result.RetryAfter != 2*time.Minute {
		t.Fatalf("unexpected retry-after: %s", result.RetryAfter)
	}
}
