package rfs

import "testing"

// TestBuildInfoString locks the canonical build-identifier line: the -version
// flag, the startup log, and the HTML page footer all render this exact shape,
// so an operator sees the same string in every place a build is described.
func TestBuildInfoString(t *testing.T) {
	got := BuildInfo{
		Version:    "2026.709.210339",
		Commit:     "abc1234",
		CommitDate: "2026-07-09",
		BuildDate:  "2026-07-09",
		GoVersion:  "go1.24.5",
		GOOS:       "linux",
		GOARCH:     "amd64",
	}.String()
	want := "rfs 2026.709.210339 (commit abc1234, committed 2026-07-09, built 2026-07-09, go1.24.5 linux/amd64)"
	if got != want {
		t.Fatalf("BuildInfo.String() = %q, want %q", got, want)
	}
}
