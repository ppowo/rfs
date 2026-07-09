package main

// version is the running build's derivation version, injected at release time
// via -ldflags "-X main.version=<tag>" (see .goreleaser.yaml). "dev" for local
// and untagged builds. The self-update tick (see docs/adr/0004-…) compares it
// to the latest GitHub release to decide whether a self-update is needed.
var version = "dev"
