package ptg_test

import (
	"testing"
	"time"

	"github.com/ppowo/rfs/internal/rfs"
	"github.com/ppowo/rfs/internal/sources/ptg"
)

func TestFlowDropsLiveThreadAndKeepsSupersededThreads(t *testing.T) {
	doc := parseHTML(t, `
<html><body>
<article class="clearfix thread">
  <article class="post doc_id_128749327 post_is_op has_image" id="109236748" data-board="g">
    <h2 class="post_title">/ptg/ - Private Trackers General</h2>
    <time datetime="2026-07-09T21:58:03+00:00">Thu 09 Jul 2026 21:58:03</time>
    <div class="text">RED edition<br /><br /><span class="greentext">&gt;Not sure what private trackers are all about?</span><br />A private tracker is an invite-only torrent website. <a href="https://example.com/faq">FAQ</a></div>
  </article>
  <article class="post" id="109237167" data-board="g">
    <h2 class="post_title">reply</h2>
    <div class="text">This reply is not a thread.</div>
  </article>
</article>
<article class="post post_is_op" id="109238334" data-board="g">
  <h2 class="post_title">/ptg/ - Private Trackers General</h2>
  <time datetime="2026-07-10T08:00:00Z">Fri 10 Jul 2026 08:00:00</time>
  <div class="text">Blue edition</div>
</article>
</body></html>`)

	items, err := (ptg.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 superseded thread (live dropped), got %d: %#v", len(items), items)
	}

	// The newest thread (2026-07-10, "Blue edition") is the live one and is
	// dropped; the older "RED edition" thread has been superseded and remains.
	item := items[0]
	if item.GUID != "ptg:109236748" {
		t.Fatalf("unexpected GUID: %q", item.GUID)
	}
	if item.Title != "/ptg/ - Private Trackers General — RED edition" {
		t.Fatalf("unexpected title: %q", item.Title)
	}
	if item.Link != "https://desuarchive.org/g/thread/109236748/#109236748" {
		t.Fatalf("unexpected link: %q", item.Link)
	}
	if item.Description != "RED edition\n\n>Not sure what private trackers are all about?\nA private tracker is an invite-only torrent website. FAQ" {
		t.Fatalf("unexpected description: %q", item.Description)
	}
	wantDate := time.Date(2026, 7, 9, 21, 58, 3, 0, time.UTC)
	if item.PubDate == nil || !item.PubDate.Equal(wantDate) {
		t.Fatalf("unexpected pubDate: %#v", item.PubDate)
	}
}

func TestFlowErrorsWhenOnlyTheLiveThreadIsPresent(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<article class="post post_is_op" id="109238334" data-board="g">
  <h2 class="post_title">/ptg/ - Private Trackers General</h2>
  <time datetime="2026-07-10T08:00:00Z">Fri 10 Jul 2026 08:00:00</time>
  <div class="text">Blue edition</div>
</article>
</body></html>`)

	_, err := (ptg.Flow{}).Extract(doc)
	if err == nil {
		t.Fatal("expected an error when only the live thread is present")
	}
}

func TestFlowRejectsPageWithoutValidOpeningPosts(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<article class="post post_is_op" id="" data-board="g"><div class="text">missing id</div></article>
<article class="post post_is_op" id="109236748" data-board="g"><div class="text">missing title and date</div></article>
</body></html>`)

	_, err := (ptg.Flow{}).Extract(doc)
	if err == nil {
		t.Fatal("expected an error for a page without valid opening posts")
	}
}

func parseHTML(t *testing.T, body string) rfs.Page {
	t.Helper()
	page := rfs.Page(body)
	if _, err := rfs.ParseHTML(page); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return page
}
