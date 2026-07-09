# rfs self-updates in place via exec, draining on shutdown

rfs runs on a server its operator cannot reach with root and cannot easily update by hand, so a manual `git pull && go install && restart` per release is a recurring hassle — and any ad-hoc "auto-update" script kept beside the binary drifts from the binary over time. To remove both, rfs self-updates: a background tick compares the running build's version (baked in via `-ldflags -X`) to the latest GitHub release, and when a newer release exists it downloads the matching `GOOS/GOARCH` asset, verifies it against the published SHA-256 checksums file over HTTPS, atomically renames it over the running binary, and `syscall.Exec`s the new image in place — same PID, no supervisor, no separate updater to rot. Because the updater is compiled into the binary it ships with, the two cannot drift apart.

`exec` replaces the process image instantly; Go goroutines do not get to finish. So a self-update (like a SIGTERM) goes through rfs's shutdown path: it drains an in-flight poll before exec — the HTTP fetch completes and the snapshot save commits — rather than cutting it. Each poll runs under a per-poll timeout on a context independent of the shutdown signal, so a healthy poll finishes naturally (drain) while a stuck upstream is cut by its own timeout and shutdown always terminates.

## Considered options

- **A separate supervisor/launcher (rejected for this host)**: a second process stops the worker, swaps the binary, restarts it — robust and cross-platform, and the natural place for rollback. Rejected here because it needs a process manager the no-root host doesn't provide, and because the drift problem (updater ≠ binary) is what we set out to eliminate. Revisit if rfs ships to hosts with systemd/launchd.
- **A shell update script beside the binary (rejected)**: the classic auto-update script that rots over time — a separate artifact that drifts from the binary it updates. Rejected because the updater must ship inside the binary so they evolve together.
- **Drop conditional GET / always full re-fetch on update (n/a)**: self-update re-execs the process; it does not change the per-poll fetch strategy. Noted only to head off the confusion.

## Consequences

- **No automatic rollback.** A release that won't boot leaves the box dark until the operator SSHes in and restores the previous binary by hand. Accepted: the operator is reachable enough for manual recovery, and releases are kept boring. The old binary is renamed aside before the swap so that restoration is a one-line `mv`.
- **Verification is SHA-256 checksums over HTTPS**, not a signing key. Adequate for a personal repo; revisit minisign/cosign if rfs is ever distributed to untrusted hosts.
- **A release pipeline ships as part of this feature**: a GitHub Actions workflow builds `rfs_<ver>_<os>_<arch>.tar.gz` plus a `checksums.txt` (SHA-256) per tag and attaches them to the Release — the self-update tick downloads and verifies these exact artifacts. Without it the tick has nothing to fetch.
- **The build bakes in a version string** (`-ldflags -X main.version`) so the running binary can compare itself to `releases/latest`.
- **Shutdown drains the poll loop.** `Loop.Run` waits for an in-flight poll to finish before returning, and `main` waits for the loop before exiting, so a SIGTERM and a self-update exec both let the snapshot save commit. Polls run on a per-poll timeout (default 2m) on a context independent of the shutdown signal, so a healthy poll drains while a stuck upstream is cut by its own timeout — shutdown always terminates. (Implemented first, as the seam the exec path depends on.)
