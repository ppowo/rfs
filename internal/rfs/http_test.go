package rfs

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHTTPHandlerServesStoredSnapshotAsRSS(t *testing.T) {
	store, err := OpenInMemorySQLiteStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	pubDate := time.Date(1982, 4, 7, 0, 0, 0, 0, time.UTC)
	if err := store.SaveSnapshot(t.Context(), "meltzer", []Item{{
		GUID:        "meltzer:1982-04-07:ric-flair-vs-butch-reed:miami-beach-show",
		Title:       "Ric Flair vs. Butch Reed — Miami Beach show",
		Link:        "https://example.com/meltzer",
		Description: "CWF · 5 stars",
		PubDate:     pubDate,
	}}); err != nil {
		t.Fatalf("save snapshot: %v", err)
	}

	handler := NewHTTPHandler(store, []Source{{
		ID: "meltzer",
		Meta: SourceMeta{
			Title:       "Meltzer 5-star matches",
			Description: "Matches rated 5 or more stars by Dave Meltzer.",
			Link:        "https://example.com/meltzer",
		},
	}})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/feeds/meltzer.xml", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/rss+xml") {
		t.Fatalf("Content-Type = %q", got)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `<title>Meltzer 5-star matches</title>`) {
		t.Fatalf("missing channel title:\n%s", body)
	}
	if !strings.Contains(body, `<guid isPermaLink="false">meltzer:1982-04-07:ric-flair-vs-butch-reed:miami-beach-show</guid>`) {
		t.Fatalf("missing item GUID:\n%s", body)
	}
}

func TestHTTPHandlerReturns404ForUnknownFeed(t *testing.T) {
	store, err := OpenInMemorySQLiteStore()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	handler := NewHTTPHandler(store, []Source{{ID: "meltzer", Meta: SourceMeta{Title: "Meltzer"}}})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/feeds/unknown.xml", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
