package tildes_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/ppowo/rfs/internal/sources/tildes"
)

func TestFlowExtractsTextAndLinkTopicsAsItems(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<ol class="topic-listing">
  <li>
    <article id="topic-1su4" class="topic topic-with-excerpt" data-topic-posted-by="JCAPER">
      <header><h1 class="topic-title"><a href="/~comp/1su4/i_switched_my_gaming_pc_to_linux">I switched my gaming PC to Linux</a></h1></header>
      <div class="topic-metadata"><span class="topic-content-type">Text</span></div>
      <footer class="topic-info">
        <div class="topic-info-comments"><a href="/~comp/1su4/i_switched_my_gaming_pc_to_linux"><span>45 comments</span></a></div>
        <div class="topic-info-source"><a href="/user/JCAPER" class="link-user">JCAPER</a></div>
        <div><time class="time-responsive" datetime="2026-02-22T23:09:34Z">February 22</time></div>
      </footer>
      <div class="topic-voting"><span class="topic-voting-votes">85</span><span class="topic-voting-label">votes</span></div>
    </article>
  </li>
  <li>
    <article id="topic-1p7l" class="topic" data-topic-posted-by="asteroid">
      <header><h1 class="topic-title"><a href="https://www.theregister.com/2025/07/21/windows_11_productivity_sink/">If you're forced to use Windows 11, here's how to steal some of your time back</a></h1></header>
      <div class="topic-metadata"><span class="topic-content-type">Article</span></div>
      <footer class="topic-info">
        <div class="topic-info-comments"><a href="/~comp/1p7l/if_youre_forced_to_use_windows_11"><span>35 comments</span></a></div>
        <div class="topic-info-source" title="theregister.com"><div class="topic-icon"></div>theregister.com</div>
        <div><time class="time-responsive" datetime="2025-07-22T00:53:38Z">July 22, 2025</time></div>
      </footer>
      <div class="topic-voting"><span class="topic-voting-votes">68</span><span class="topic-voting-label">votes</span></div>
    </article>
  </li>
</ol>
</body></html>`)

	items, err := (tildes.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %#v", len(items), items)
	}

	// A Text topic: author came from data-topic-posted-by, the link is the
	// Tildes discussion URL (not an external site), and the source-domain
	// field is absent because there is no linked site.
	text := items[0]
	if text.GUID != "tildes:1su4" {
		t.Fatalf("unexpected GUID: %q", text.GUID)
	}
	if text.Title != "I switched my gaming PC to Linux" {
		t.Fatalf("unexpected title: %q", text.Title)
	}
	if text.Link != "https://tildes.net/~comp/1su4/i_switched_my_gaming_pc_to_linux" {
		t.Fatalf("unexpected link: %q", text.Link)
	}
	if text.Description != "85 votes · 45 comments · Text · posted by JCAPER" {
		t.Fatalf("unexpected description: %q", text.Description)
	}
	wantText := time.Date(2026, 2, 22, 23, 9, 34, 0, time.UTC)
	if text.PubDate == nil || !text.PubDate.Equal(wantText) {
		t.Fatalf("unexpected pubDate: %#v", text.PubDate)
	}

	// An Article (link) topic: the title anchor points off-site, but the item
	// link still resolves to the Tildes discussion; the linked site's domain
	// shows up in the description.
	link := items[1]
	if link.GUID != "tildes:1p7l" {
		t.Fatalf("unexpected GUID: %q", link.GUID)
	}
	if link.Title != "If you're forced to use Windows 11, here's how to steal some of your time back" {
		t.Fatalf("unexpected title: %q", link.Title)
	}
	if link.Link != "https://tildes.net/~comp/1p7l/if_youre_forced_to_use_windows_11" {
		t.Fatalf("unexpected link: %q", link.Link)
	}
	if link.Description != "68 votes · 35 comments · Article · theregister.com · posted by asteroid" {
		t.Fatalf("unexpected description: %q", link.Description)
	}
	wantLink := time.Date(2025, 7, 22, 0, 53, 38, 0, time.UTC)
	if link.PubDate == nil || !link.PubDate.Equal(wantLink) {
		t.Fatalf("unexpected pubDate: %#v", link.PubDate)
	}
}

func TestFlowKeepsTopicWhenOptionalMetadataIsMissing(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<ol class="topic-listing">
  <li><article id="topic-sparse" data-topic-posted-by="sparse">
    <h1 class="topic-title"><a href="/~comp/sparse/x">Sparse topic</a></h1>
    <time datetime="2026-04-05T00:00:00Z">Apr 5</time>
  </article></li>
</ol>
</body></html>`)

	items, err := (tildes.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected sparse topic to be kept, got %d: %#v", len(items), items)
	}
	if items[0].Description != "posted by sparse" {
		t.Fatalf("unexpected sparse description: %q", items[0].Description)
	}
}

func TestFlowIgnoresTopicArticlesOutsideTheListing(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<aside>
  <article id="topic-sidebar" data-topic-posted-by="sidebar">
    <h1 class="topic-title">Sidebar topic</h1>
    <time datetime="2026-04-06T00:00:00Z">Apr 6</time>
  </article>
</aside>
<ol class="topic-listing">
  <li><article id="topic-listed" data-topic-posted-by="listed">
    <h1 class="topic-title">Listed topic</h1>
    <time datetime="2026-04-05T00:00:00Z">Apr 5</time>
  </article></li>
</ol>
</body></html>`)

	items, err := (tildes.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 1 || items[0].GUID != "tildes:listed" {
		t.Fatalf("expected only the listed topic, got %#v", items)
	}
}

func TestFlowDoesNotUseAnExternalCommentsLink(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<ol class="topic-listing">
  <li><article id="topic-safe" data-topic-posted-by="safe">
    <h1 class="topic-title">Safe topic</h1>
    <div class="topic-info-comments"><a href="https://example.com/not-tildes">3 comments</a></div>
    <time datetime="2026-04-05T00:00:00Z">Apr 5</time>
  </article></li>
</ol>
</body></html>`)

	items, err := (tildes.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item, got %d: %#v", len(items), items)
	}
	if items[0].Link != "https://tildes.net/~comp/safe" {
		t.Fatalf("external comments link escaped Tildes: %q", items[0].Link)
	}
}

func TestFlowKeepsEveryTopicAndDropsNothing(t *testing.T) {
	// Listing membership defines the Items, so recency does not suppress the
	// newer topic or any other valid topic on the page.
	doc := parseHTML(t, `<html><body>
<ol class="topic-listing">
<li><article id="topic-old" data-topic-posted-by="a">
  <h1 class="topic-title"><a href="/~comp/old/x">Older topic</a></h1>
  <div class="topic-info-comments"><a href="/~comp/old/x"><span>1 comment</span></a></div>
  <span class="topic-content-type">Text</span>
  <span class="topic-voting-votes">12</span>
  <time datetime="2025-08-01T00:00:00Z">Aug 1</time>
</article></li>
<li><article id="topic-new" data-topic-posted-by="b">
  <h1 class="topic-title"><a href="/~comp/new/x">Newer topic</a></h1>
  <div class="topic-info-comments"><a href="/~comp/new/x"><span>2 comments</span></a></div>
  <span class="topic-content-type">Text</span>
  <span class="topic-voting-votes">99</span>
  <time datetime="2026-07-10T00:00:00Z">Jul 10</time>
</article></li>
</ol>
</body></html>`)

	items, err := (tildes.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected both topics kept (none dropped), got %d: %#v", len(items), items)
	}
	guids := map[string]bool{items[0].GUID: true, items[1].GUID: true}
	if !guids["tildes:old"] || !guids["tildes:new"] {
		t.Fatalf("expected old and new topics both present, got %v", guids)
	}
}

func TestFlowSkipsTopicsMissingRequiredFields(t *testing.T) {
	// Each broken topic exercises a different rejection path:
	//   - sidebar widget: id lacks the "topic-" prefix, so the walk skips it;
	//   - topic-            : id prefix present but suffix empty (no stable identity);
	//   - topic-notime      : no <time> element;
	//   - topic-badtime      : <time> datetime does not parse;
	//   - topic-notitle      : no .topic-title.
	// Only topic-good has id, title, and timestamp all present.
	doc := parseHTML(t, `<html><body>
<ol class="topic-listing">
<article id="sidebar-1" data-topic-posted-by="a"><h1 class="topic-title"><a href="/x">sidebar widget</a></h1><time datetime="2026-01-01T00:00:00Z"></time></article>
<article id="topic-" data-topic-posted-by="a"><h1 class="topic-title"><a href="/x">empty id suffix</a></h1><time datetime="2026-01-01T00:00:00Z"></time></article>
<article id="topic-notime" data-topic-posted-by="b"><h1 class="topic-title"><a href="/x">no time</a></h1><span class="topic-voting-votes">5</span></article>
<article id="topic-badtime" data-topic-posted-by="c"><h1 class="topic-title"><a href="/x">bad time</a></h1><time datetime="not-a-date"></time><span class="topic-voting-votes">5</span></article>
<article id="topic-notitle" data-topic-posted-by="d"><span class="topic-voting-votes">5</span><time datetime="2026-01-01T00:00:00Z"></time></article>
<article id="topic-good" data-topic-posted-by="e"><h1 class="topic-title"><a href="/~comp/good/x">kept</a></h1><span class="topic-content-type">Text</span><span class="topic-voting-votes">7</span><time datetime="2026-03-03T00:00:00Z"></time></article>
</ol>
</body></html>`)

	items, err := (tildes.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected only the complete topic to survive, got %d: %#v", len(items), items)
	}
	if items[0].GUID != "tildes:good" {
		t.Fatalf("unexpected survivor: %q", items[0].GUID)
	}
}

func TestFlowErrorsOnNilDocument(t *testing.T) {
	if _, err := (tildes.Flow{}).Extract(nil); err == nil {
		t.Fatal("expected an error for a nil document")
	}
}

func TestFlowErrorsWhenTopicListingIsMissing(t *testing.T) {
	doc := parseHTML(t, `<html><body><p>Tildes maintenance page, no listing.</p></body></html>`)
	if _, err := (tildes.Flow{}).Extract(doc); err == nil {
		t.Fatal("expected an error when the topic listing is missing")
	}
}

func TestFlowErrorsWhenNoValidTopics(t *testing.T) {
	doc := parseHTML(t, `<html><body><ol class="topic-listing"></ol></body></html>`)
	if _, err := (tildes.Flow{}).Extract(doc); err == nil {
		t.Fatal("expected an error when the listing has no valid topics")
	}
}

// TestFlowParsesEveryTopicOnRealListing verifies the real ~comp listing
// (testdata/comp.html) parses end to end with no dropped topics: every one of
// the 50 topics the page surfaces becomes an item with a stable GUID, a
// Tildes discussion link, a parseable pubDate, and a votes-bearing description.
func TestFlowParsesEveryTopicOnRealListing(t *testing.T) {
	raw, err := os.ReadFile("testdata/comp.html")
	if err != nil {
		t.Fatalf("read testdata/comp.html: %v", err)
	}
	doc := parseHTML(t, string(raw))

	items, err := (tildes.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}

	wantTopics := countTopicArticles(t, doc)
	if wantTopics != 50 {
		t.Fatalf("fixture topic count = %d, want 50", wantTopics)
	}
	if len(items) != wantTopics {
		t.Fatalf("expected every topic article to produce an item; got %d items for %d articles", len(items), wantTopics)
	}

	seen := map[string]bool{}
	for _, item := range items {
		if !strings.HasPrefix(item.GUID, "tildes:") {
			t.Fatalf("unexpected GUID prefix: %q", item.GUID)
		}
		if seen[item.GUID] {
			t.Fatalf("duplicate GUID: %q", item.GUID)
		}
		seen[item.GUID] = true

		if !strings.HasPrefix(item.Link, "https://tildes.net/~comp/") {
			t.Fatalf("unexpected link for %q: %q", item.GUID, item.Link)
		}
		if item.PubDate == nil || item.PubDate.IsZero() {
			t.Fatalf("expected parsed pubDate for %q: %#v", item.GUID, item.PubDate)
		}
		if !strings.Contains(item.Description, "votes") {
			t.Fatalf("expected votes in description for %q: %q", item.GUID, item.Description)
		}
	}

	// A Text topic has no source-domain segment.
	first := items[0]
	if !strings.Contains(first.Title, "I switched my gaming PC to Linux") {
		t.Fatalf("unexpected top-voted title: %q", first.Title)
	}
	if first.Description != "85 votes · 45 comments · Text · posted by JCAPER" {
		t.Fatalf("unexpected Text-topic description: %q", first.Description)
	}

	// An Article topic includes its external source domain while still linking
	// to the Tildes discussion.
	articleDescription := ""
	for _, item := range items {
		if item.GUID == "tildes:1p7l" {
			articleDescription = item.Description
			break
		}
	}
	if articleDescription == "" {
		t.Fatal("fixture is missing Article topic tildes:1p7l")
	}
	if articleDescription != "68 votes · 35 comments · Article · theregister.com · posted by asteroid" {
		t.Fatalf("unexpected Article-topic description: %q", articleDescription)
	}
}

func parseHTML(t *testing.T, body string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return doc
}

func countTopicArticles(t *testing.T, doc *html.Node) int {
	t.Helper()
	count := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "article" {
			for _, attr := range n.Attr {
				if attr.Key == "id" && strings.HasPrefix(attr.Val, "topic-") {
					count++
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return count
}
