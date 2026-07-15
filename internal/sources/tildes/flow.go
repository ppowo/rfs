package tildes

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/ppowo/rfs/internal/rfs"
)

const (
	baseURL = "https://tildes.net"
	// PageURL is the ~comp topic listing sorted by votes over the trailing year.
	PageURL = baseURL + "/~comp?order=votes&period=365d"
	group   = "~comp"

	// topicIDPrefix is the id="..." prefix Tildes gives every topic article on
	// the listing (e.g. id="topic-1su4"). The suffix is a stable, unique topic
	// id that becomes the item GUID.
	topicIDPrefix = "topic-"
)

// ExtractVersion is the derivation version for the Tildes ~comp Flow. Bump it
// whenever Extract's output can change for a fixed listing page.
const ExtractVersion = 1

type Flow struct{}

// Version reports the Tildes ~comp extraction version.
func (Flow) Version() int { return ExtractVersion }

// Extract turns each valid topic in the ~comp topic listing into one RSS item.
// Listing membership alone defines the Source's current Items: topic age and
// position do not suppress an Item, while a topic that leaves the trailing-year
// listing naturally leaves the feed.
func (Flow) Extract(page rfs.Page) ([]rfs.ExtractedItem, error) {
	doc, err := rfs.ParseHTML(page)
	if err != nil {
		return nil, fmt.Errorf("tildes: parse page: %w", err)
	}

	listing := findElementByClass(doc, "topic-listing")
	if listing == nil {
		return nil, errors.New("tildes: topic listing not found")
	}

	var items []rfs.ExtractedItem
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "article" && strings.HasPrefix(attribute(n, "id"), topicIDPrefix) {
			if item, ok := extractTopic(n); ok {
				items = append(items, item)
			}
			return // never descend into a topic article
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(listing)

	if len(items) == 0 {
		return nil, errors.New("tildes: no valid topics")
	}

	return items, nil
}

// extractTopic parses a single <article id="topic-..."> into an item. A topic
// is emitted only when it has a stable id, a non-empty title, and a parseable
// posted timestamp. The remaining metadata (votes, comments, type, source
// domain, author) is best-effort and omitted from the description when absent.
func extractTopic(article *html.Node) (rfs.ExtractedItem, bool) {
	id := strings.TrimPrefix(attribute(article, "id"), topicIDPrefix)
	if id == "" {
		return rfs.ExtractedItem{}, false
	}

	titleNode := findElementByClass(article, "topic-title")
	timeNode := findElement(article, "time")
	if titleNode == nil || timeNode == nil {
		return rfs.ExtractedItem{}, false
	}

	title := textContent(titleNode)
	datetime := attribute(timeNode, "datetime")
	if title == "" || datetime == "" {
		return rfs.ExtractedItem{}, false
	}
	pubDate, err := time.Parse(time.RFC3339Nano, datetime)
	if err != nil {
		return rfs.ExtractedItem{}, false
	}

	// The discussion link is consistent across Tildes content types. Only a
	// site-relative ~comp path may replace the safe, slugless fallback.
	link := baseURL + "/" + group + "/" + id
	commentsNode := findElementByClass(article, "topic-info-comments")
	if commentsNode != nil {
		if a := findElement(commentsNode, "a"); a != nil {
			if href := attribute(a, "href"); strings.HasPrefix(href, "/"+group+"/") {
				link = baseURL + href
			}
		}
	}

	author := attribute(article, "data-topic-posted-by")
	votes := textContent(findElementByClass(article, "topic-voting-votes"))
	commentsText := textContent(commentsNode)
	contentType := textContent(findElementByClass(article, "topic-content-type"))
	domain := ""
	if source := findElementByClass(article, "topic-info-source"); source != nil {
		// For link/article topics Tildes sets the title attribute to the linked
		// site's domain (e.g. "theregister.com"); text/ask topics carry the
		// author here instead and have no title attribute.
		domain = attribute(source, "title")
	}

	return rfs.ExtractedItem{
		GUID:        "tildes:" + id,
		Title:       title,
		Link:        link,
		Description: buildDescription(votes, commentsText, contentType, domain, author),
		PubDate:     &pubDate,
	}, true
}

// buildDescription joins the available topic metadata with a middle dot, in the
// order a reader scanning a top-of-the-year feed cares about: votes first
// (what the listing is sorted by), then engagement, the content type, the
// linked site's domain (when the topic is a link), and finally the poster.
func buildDescription(votes, comments, contentType, domain, author string) string {
	var parts []string
	if votes != "" {
		parts = append(parts, votes+" votes")
	}
	if comments != "" {
		parts = append(parts, comments)
	}
	if contentType != "" {
		parts = append(parts, contentType)
	}
	if domain != "" {
		parts = append(parts, domain)
	}
	if author != "" {
		parts = append(parts, "posted by "+author)
	}
	return strings.Join(parts, " · ")
}

func findElementByClass(root *html.Node, class string) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && hasClass(n, class) {
			found = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func findElement(root *html.Node, name string) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == name {
			found = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func attribute(root *html.Node, name string) string {
	for _, attr := range root.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func hasClass(root *html.Node, class string) bool {
	for _, part := range strings.Fields(attribute(root, "class")) {
		if part == class {
			return true
		}
	}
	return false
}

func textContent(root *html.Node) string {
	if root == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	// Concatenated text nodes carry whatever indentation/line breaks the
	// markup had, so collapse runs of whitespace into single spaces and trim —
	// "45 comments", not "  45\n  comments ".
	return strings.Join(strings.Fields(b.String()), " ")
}
