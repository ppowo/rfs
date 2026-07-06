package meltzer_test

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/ppowo/rfs/internal/sources/meltzer"
)

func TestFlowExtractsMatchRowsAsItems(t *testing.T) {
	doc := parseHTML(t, `
<html><body>
<table class="wikitable">
  <tr>
    <th>Date</th><th>Match</th><th>Promotion</th><th>Event</th><th>Rating</th><th>Notes</th><th>Ref.</th>
  </tr>
  <tr>
    <td>April 7, 1982</td>
    <td><a href="/wiki/Ric_Flair">Ric Flair</a> vs. <a href="/wiki/Butch_Reed">Butch Reed</a><sup class="reference">[a]</sup></td>
    <td><a href="/wiki/Championship_Wrestling_from_Florida">CWF</a></td>
    <td>Miami Beach show</td>
    <td>5</td>
    <td>for the NWA World Heavyweight Championship</td>
    <td>[2]</td>
  </tr>
</table>
</body></html>`)

	items, err := (meltzer.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d: %#v", len(items), items)
	}

	item := items[0]
	if item.GUID != "meltzer:1982-04-07:ric-flair-vs-butch-reed:miami-beach-show" {
		t.Fatalf("unexpected GUID: %q", item.GUID)
	}
	if item.Title != "Ric Flair vs. Butch Reed — Miami Beach show" {
		t.Fatalf("unexpected title: %q", item.Title)
	}
	if item.Link != meltzer.PageURL {
		t.Fatalf("unexpected link: %q", item.Link)
	}
	if item.Description != "CWF · 5 stars · for the NWA World Heavyweight Championship" {
		t.Fatalf("unexpected description: %q", item.Description)
	}
	if item.PubDate == nil {
		t.Fatal("expected source-derived pubDate")
	}
	wantDate := time.Date(1982, 4, 7, 0, 0, 0, 0, time.UTC)
	if !item.PubDate.Equal(wantDate) {
		t.Fatalf("unexpected pubDate: got %s want %s", item.PubDate, wantDate)
	}
}

func TestFlowHandlesWikipediaRowspanDates(t *testing.T) {
	doc := parseHTML(t, `
<html><body>
<table class="wikitable sortable">
  <tr>
    <th></th><th></th><th>Date</th><th>Match</th><th>Promotion</th><th>Event</th><th>Rating</th><th>Notes</th><th>Ref.</th>
  </tr>
  <tr>
    <td>162</td><td>64</td><td rowspan="2">August 31, 2019</td>
    <td>Walter vs. Tyler Bate</td><td>WWE</td><td>NXT UK TakeOver: Cardiff</td><td>5.25</td><td>for the WWE United Kingdom Championship</td><td>163</td>
  </tr>
  <tr>
    <td>163</td><td>65</td>
    <td>Lucha Brothers (Pentagón Jr. and Rey Fenix) vs. The Young Bucks (Matt Jackson and Nick Jackson)</td><td>AEW</td><td>All Out</td><td>5.25</td><td>Ladder match for the AAA World Tag Team Championship</td><td>164</td>
  </tr>
</table>
</body></html>`)

	items, err := (meltzer.Flow{}).Extract(doc)
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %#v", len(items), items)
	}

	second := items[1]
	if second.Title != "Lucha Brothers (Pentagón Jr. and Rey Fenix) vs. The Young Bucks (Matt Jackson and Nick Jackson) — All Out" {
		t.Fatalf("rowspan row shifted columns, title = %q", second.Title)
	}
	if second.Description != "AEW · 5.25 stars · Ladder match for the AAA World Tag Team Championship" {
		t.Fatalf("rowspan row shifted columns, description = %q", second.Description)
	}
	wantDate := time.Date(2019, 8, 31, 0, 0, 0, 0, time.UTC)
	if second.PubDate == nil || !second.PubDate.Equal(wantDate) {
		t.Fatalf("rowspan date not carried forward: %#v", second.PubDate)
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
