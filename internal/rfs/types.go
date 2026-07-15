package rfs

import "time"

// SourceMeta describes the RSS channel served for a Source.
type SourceMeta struct {
	Title       string
	Description string
	Link        string
}

// Page is the response body fetched for a Source. A Flow decides how to decode
// it; HTML and JSON are both source-specific representations.
type Page []byte

// Flow extracts source-specific items from a fetched Page.
type Flow interface {
	Extract(Page) ([]ExtractedItem, error)

	// Version is bumped whenever Extract's output can change for a fixed Page.
	// rfs persists the version that produced each stored snapshot and, on a
	// mismatch, forces a full re-fetch+re-extraction rather than trusting an
	// HTTP 304 (which only proves the page bytes are unchanged, not that the
	// parser is). See docs/adr/0003-extract-version-invalidates-snapshot.md.
	Version() int
}

// Source wires a hardcoded upstream resource to the Flow and feed metadata.
type Source struct {
	ID   string
	URL  string
	Meta SourceMeta
	Flow Flow

	// Interval overrides the process's default poll interval when positive.
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
