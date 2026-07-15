package film

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"github.com/ppowo/rfs/internal/rfs"
)

const (
	archiveURL = "https://archive.4plebs.org"
	board      = "tv"
	// %2Ffilm%2F is the URL-encoded subject "/film/"; the text qualifier
	// restricts the search to opening posts containing "arthouse".
	PageURL = archiveURL + "/tv/search/subject/%2Ffilm%2F/text/arthouse/type/op/"
)

// ExtractVersion is the derivation version for the /film/ Flow. Bump it when
// Extract's output can change for a fixed search-results page.
const ExtractVersion = 1

type Flow struct{}

// Version reports the /film/ extraction version.
func (Flow) Version() int { return ExtractVersion }

// Extract turns each opening-post result on the 4plebs search page into one
// RSS item. The search is restricted to OP posts, but checking the post_is_op
// class here keeps the Flow correct if the page includes other article types
// in the future.
//
// The currently-live /film/ thread is still filling up, so it is dropped: a
// thread is surfaced only once a newer one exists (i.e. it has been
// superseded — filled up or archived). The live thread is the one with the
// newest pubDate, found by date rather than search-result position so the
// rule holds regardless of how 4plebs orders the page.
func (Flow) Extract(page rfs.Page) ([]rfs.ExtractedItem, error) {
	doc, err := rfs.ParseHTML(page)
	if err != nil {
		return nil, fmt.Errorf("film: parse page: %w", err)
	}

	var items []rfs.ExtractedItem
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "article" && hasClass(n, "post_is_op") {
			if item, ok := extractPost(n); ok {
				items = append(items, item)
			}
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)

	if len(items) == 0 {
		return nil, errors.New("film: no valid opening-post search results")
	}

	// Every valid item carries a parsed pubDate (extractPost rejects posts
	// without one), so the maximum is well-defined. The newest thread is the
	// live one and is dropped; the rest have been superseded.
	live := 0
	for i := 1; i < len(items); i++ {
		if items[i].PubDate.After(*items[live].PubDate) {
			live = i
		}
	}
	items = append(items[:live], items[live+1:]...)
	if len(items) == 0 {
		return nil, errors.New("film: only the live thread is present")
	}

	return items, nil
}

func extractPost(post *html.Node) (rfs.ExtractedItem, bool) {
	id := attribute(post, "id")
	if attribute(post, "data-board") != board {
		return rfs.ExtractedItem{}, false
	}
	if _, err := strconv.ParseUint(id, 10, 64); err != nil {
		return rfs.ExtractedItem{}, false
	}

	subjectNode := findElementByClass(post, "post_title")
	timeNode := findElement(post, "time")
	textNode := findElementByClass(post, "text")
	if subjectNode == nil || timeNode == nil || textNode == nil {
		return rfs.ExtractedItem{}, false
	}

	subject := textContent(subjectNode)
	dateValue := attribute(timeNode, "datetime")
	pubDate, err := time.Parse(time.RFC3339Nano, dateValue)
	if subject == "" || dateValue == "" || err != nil {
		return rfs.ExtractedItem{}, false
	}

	link := archiveURL + "/" + board + "/thread/" + id + "/#" + id

	description := textContent(textNode)
	title := subject
	if firstLine := strings.SplitN(description, "\n", 2)[0]; firstLine != "" {
		title += " — " + firstLine
	}

	return rfs.ExtractedItem{
		GUID:        "film:" + id,
		Title:       title,
		Link:        link,
		Description: description,
		PubDate:     &pubDate,
	}, true
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
	var raw strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			raw.WriteString(n.Data)
			return
		}
		if n.Type == html.ElementNode && n.Data == "br" {
			raw.WriteByte('\n')
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return normalizeText(raw.String())
}

func normalizeText(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	raw = strings.ReplaceAll(raw, "\u00a0", " ")

	var lines []string
	blank := false
	for _, line := range strings.Split(raw, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if len(lines) > 0 && !blank {
				lines = append(lines, "")
				blank = true
			}
			continue
		}
		lines = append(lines, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
