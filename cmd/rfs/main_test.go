package main

import (
	"testing"

	"github.com/ppowo/rfs/internal/rfs"
)

func TestShouldEnableSelfUpdate(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		flagEnabled bool
		want        bool
	}{
		{"release build enabled by default", "2026.709.201956", true, true},
		{"release-like local build can opt out", "2026.709.201956", false, false},
		{"dev build is disabled even when flag is true", "dev", true, false},
		{"dev build is disabled when flag is false", "dev", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldEnableSelfUpdate(tt.version, tt.flagEnabled)
			if got != tt.want {
				t.Fatalf("shouldEnableSelfUpdate(%q, %v) = %v, want %v", tt.version, tt.flagEnabled, got, tt.want)
			}
		})
	}
}

func TestPollCycleDetails(t *testing.T) {
	sources := []rfs.Source{{ID: "meltzer-5-star-matches"}}
	if got := pollCycleDetails(sources); got != "source poll meltzer-5-star-matches" {
		t.Fatalf("pollCycleDetails(one source) = %q", got)
	}
	if got := pollCycleDetails(nil); got != "0 source polls" {
		t.Fatalf("pollCycleDetails(no sources) = %q", got)
	}
}
