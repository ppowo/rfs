package rfs

// BuildInfo describes the running rfs binary: a release version (the timestamp
// the self-update gate compares) plus the commit, commit/build dates, and Go
// toolchain/platform used to produce it. It is the single source for how a
// build is described in text — the -version flag, the startup log, and the HTML
// page footer all render the same String — so an operator sees the same
// identifier in every place. See docs/adr/0004-self-updates-in-place-via-exec.md.
type BuildInfo struct {
	Version    string
	Commit     string
	CommitDate string
	BuildDate  string
	GoVersion  string
	GOOS       string
	GOARCH     string
}

// String renders BuildInfo as one verbose line: the identifier the -version
// flag prints and the startup log records, reused verbatim in the HTML footer.
func (b BuildInfo) String() string {
	return "rfs " + b.Version + " (commit " + b.Commit + ", committed " + b.CommitDate +
		", built " + b.BuildDate + ", " + b.GoVersion + " " + b.GOOS + "/" + b.GOARCH + ")"
}
