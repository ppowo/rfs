package rfs

import (
	"strings"
	"testing"
	"time"
)

func TestRenderRSSIncludesChannelAndItems(t *testing.T) {
	pubDate := time.Date(1982, 4, 7, 0, 0, 0, 0, time.UTC)

	xml, err := RenderRSS(SourceMeta{
		Title:       "Meltzer 5-star matches",
		Description: "Professional wrestling matches rated 5 or more stars by Dave Meltzer.",
		Link:        "https://en.wikipedia.org/wiki/List_of_professional_wrestling_matches_rated_5_or_more_stars_by_Dave_Meltzer",
	}, []Item{{
		GUID:        "meltzer:ric-flair-vs-butch-reed-1982-04-07",
		Title:       "Ric Flair vs. Butch Reed — Miami Beach show",
		Link:        "https://en.wikipedia.org/wiki/List_of_professional_wrestling_matches_rated_5_or_more_stars_by_Dave_Meltzer",
		Description: "CWF · 5 stars · for the NWA World Heavyweight Championship",
		PubDate:     pubDate,
	}})
	if err != nil {
		t.Fatalf("RenderRSS returned error: %v", err)
	}

	got := string(xml)
	wantParts := []string{
		`<rss version="2.0">`,
		`<title>Meltzer 5-star matches</title>`,
		`<description>Professional wrestling matches rated 5 or more stars by Dave Meltzer.</description>`,
		`<guid isPermaLink="false">meltzer:ric-flair-vs-butch-reed-1982-04-07</guid>`,
		`<pubDate>Wed, 07 Apr 1982 00:00:00 +0000</pubDate>`,
	}
	for _, want := range wantParts {
		if !strings.Contains(got, want) {
			t.Fatalf("RSS output did not contain %q:\n%s", want, got)
		}
	}
}
