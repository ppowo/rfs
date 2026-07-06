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
