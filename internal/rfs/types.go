package rfs

import (
	"time"

	"golang.org/x/net/html"
)

// SourceMeta describes the RSS channel served for a Source.
type SourceMeta struct {
	Title       string
	Description string
	Link        string
}

// Flow extracts source-specific items from a fetched HTML page.
type Flow interface {
	Extract(*html.Node) ([]ExtractedItem, error)

	// Version is bumped whenever Extract's output can change for a fixed page.
	// rfs persists the version that produced each stored snapshot and, on a
	// mismatch, forces a full re-fetch+re-extraction rather than trusting an
	// HTTP 304 (which only proves the page bytes are unchanged, not that the
	// parser is). See docs/adr/0003-extract-version-invalidates-snapshot.md.
	Version() int
}

// Source wires a hardcoded web page to the Flow and metadata used to serve it.
type Source struct {
	ID       string
	URL      string
	Meta     SourceMeta
	Flow     Flow
	Interval time.Duration
}

// Item is a single entry in a Source's RSS feed.
type Item struct {
	GUID        string
	Title       string
	Link        string
	Description string
	PubDate     time.Time
}

// ExtractedItem is an Item emitted by a Flow before rfs has applied fallback
// values such as first-seen pubDate.
type ExtractedItem struct {
	GUID        string
	Title       string
	Link        string
	Description string
	PubDate     *time.Time
}
