package sources_test

import (
	"testing"

	"github.com/ppowo/rfs/internal/sources"
	"github.com/ppowo/rfs/internal/sources/ptg"
)

func TestAllIncludesPTGSource(t *testing.T) {
	var found bool
	for _, source := range sources.All() {
		if source.ID != "ptg" {
			continue
		}
		found = true
		if source.URL != ptg.PageURL {
			t.Fatalf("ptg source URL = %q, want %q", source.URL, ptg.PageURL)
		}
		if source.Meta.Title != "/ptg/ - Private Trackers General" {
			t.Fatalf("ptg source title = %q", source.Meta.Title)
		}
		if source.Meta.Link != ptg.PageURL {
			t.Fatalf("ptg source link = %q, want %q", source.Meta.Link, ptg.PageURL)
		}
		if source.Flow.Version() != ptg.ExtractVersion {
			t.Fatalf("ptg source flow version = %d, want %d", source.Flow.Version(), ptg.ExtractVersion)
		}
	}
	if !found {
		t.Fatal("sources.All does not include the ptg source")
	}
}
