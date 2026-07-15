package film_test

import (
	"testing"
	"time"

	"github.com/ppowo/rfs/internal/rfs"
	"github.com/ppowo/rfs/internal/sources/film"
)

func TestFlowDropsLiveThreadAndKeepsSupersededThreads(t *testing.T) {
	doc := parseHTML(t, `
<html><body>
<article class="clearfix thread">
  <article class="post doc_id_214846968 post_is_op has_image" id="221573858" data-board="tv">
    <h2 class="post_title">/film/</h2>
    <time datetime="2026-07-06T10:59:11+00:00">Mon 06 Jul 2026 10:59:11</time>
    <div class="text">Thread for the discussion of arthouse and classic cinema.<br /><br />Don Carlo edition<br /><span class="greentext">&gt;QOTD</span><br />What did you watch this week? <a href="https://example.com/chart">chart</a></div>
  </article>
  <article class="post" id="221574000" data-board="tv">
    <h2 class="post_title">reply</h2>
    <div class="text">This reply is not a thread.</div>
  </article>
</article>
<article class="post post_is_op" id="221663746" data-board="tv">
  <h2 class="post_title">/film/</h2>
  <time datetime="2026-07-10T13:12:49+00:00">Fri 10 Jul 2026 13:12:49</time>
  <div class="text">Thread for the discussion of arthouse and classic cinema.<br /><br />New edition</div>
</article>
</body></html>`)

	items, err := (film.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 superseded thread (live dropped), got %d: %#v", len(items), items)
	}

	// The newest thread (2026-07-10, "New edition") is the live one and is
	// dropped; the older "Don Carlo edition" thread has been superseded and
	// remains.
	item := items[0]
	if item.GUID != "film:221573858" {
		t.Fatalf("unexpected GUID: %q", item.GUID)
	}
	if item.Title != "/film/ — Thread for the discussion of arthouse and classic cinema." {
		t.Fatalf("unexpected title: %q", item.Title)
	}
	if item.Link != "https://archive.4plebs.org/tv/thread/221573858/#221573858" {
		t.Fatalf("unexpected link: %q", item.Link)
	}
	if item.Description != "Thread for the discussion of arthouse and classic cinema.\n\nDon Carlo edition\n>QOTD\nWhat did you watch this week? chart" {
		t.Fatalf("unexpected description: %q", item.Description)
	}
	wantDate := time.Date(2026, 7, 6, 10, 59, 11, 0, time.UTC)
	if item.PubDate == nil || !item.PubDate.Equal(wantDate) {
		t.Fatalf("unexpected pubDate: %#v", item.PubDate)
	}
}

func TestFlowErrorsWhenOnlyTheLiveThreadIsPresent(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<article class="post post_is_op" id="221663746" data-board="tv">
  <h2 class="post_title">/film/</h2>
  <time datetime="2026-07-10T13:12:49+00:00">Fri 10 Jul 2026 13:12:49</time>
  <div class="text">Thread for the discussion of arthouse and classic cinema.</div>
</article>
</body></html>`)

	_, err := (film.Flow{}).Extract(doc)
	if err == nil {
		t.Fatal("expected an error when only the live thread is present")
	}
}

func TestFlowRejectsPageWithoutValidOpeningPosts(t *testing.T) {
	doc := parseHTML(t, `<html><body>
<article class="post post_is_op" id="" data-board="tv"><div class="text">missing id</div></article>
<article class="post post_is_op" id="221573858" data-board="tv"><div class="text">missing title and date</div></article>
</body></html>`)

	_, err := (film.Flow{}).Extract(doc)
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
