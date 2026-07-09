package main

import "runtime"

// Build metadata for the running rfs binary. version/commit/commitDate/buildDate
// are injected at release time via -ldflags (see .goreleaser.yaml); the go
// version, OS, and arch come from the runtime, so they need no injection.
// Defaults make local `go run`/`go install` builds self-identify as "dev" so
// they skip self-update (see ADR 0004).
var (
	version    = "dev"
	commit     = "none"
	commitDate = "unknown"
	buildDate  = "unknown"
)

// versionString returns a single verbose line describing this build, used by
// the -version flag and the startup log so auto-incremented builds can be told
// apart on the server. The self-update gate uses only `version` (the leading
// timestamp), not this whole string.
func versionString() string {
	return "rfs " + version + " (commit " + commit + ", committed " + commitDate +
		", built " + buildDate + ", " + runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH + ")"
}
