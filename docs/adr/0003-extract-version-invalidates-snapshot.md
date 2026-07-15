# Derived snapshots are invalidated by an extract version, not only by HTTP 304

rfs serves items from a per-source **snapshot** stored in SQLite, not by re-running a Flow on every request. A snapshot is derived data: a pure function of `(upstream page bytes, extraction code)`. Until now the only invalidation signal was the HTTP conditional cache (ETag / If-Modified-Since): when the source answered 304 Not Modified, `Poll` left the stored snapshot untouched and skipped `Extract` entirely.

That is correct for unchanged code — identical page bytes re-derived by the same Flow yield the same items — but it breaks the day the Flow's extraction logic changes. A 304 means "the page bytes are the same," not "the derivation is still valid," so a deployed Flow change never re-ran against the identical page, and stale snapshots (e.g. all-decades items still served after the Flow was narrowed to the 2020s) survived indefinitely until the upstream page itself changed or the database was cleared.

We fix this by giving every Flow a derivation **version** (`Flow.Version() int`, bumped whenever `Extract`'s output can change for a fixed page) and persisting, alongside the fetch cache, the version that produced each stored snapshot. On each poll `Poll` compares the running Flow's version to the stored one; on a mismatch it drops the conditional headers for that fetch — forcing a full 200 + re-extraction that overwrites the stale snapshot — then records the new version. When the versions match, the cheap 304 fast-path is preserved.

## Considered options

- **Cache the last fetched page and re-derive on a version mismatch (rejected)**: on a 304 with a changed version, re-run `Extract` against the cached page bytes instead of refetching. Correct and zero extra traffic, but it requires persisting a response-body blob per source (new storage, new size concerns) for a benefit — one skipped fetch right after each deploy — that is negligible for an hourly small-source poller. Rejected as YAGNI; revisit if a source's page is large or fetched very frequently.
- **Drop conditional GET entirely and always re-fetch + re-extract (rejected)**: simplest, and the existing throttle/Retry-After handling already covers politeness. But it throws away the 304 optimisation for the common case (unchanged page + unchanged code — i.e. every poll except the first one after a deploy). Rejected because the version check recovers that optimisation for free in the common case while still detecting code-driven staleness.
- **Operational: clear the SQLite DB on deploy (rejected)**: leaves the staleness class latent for the next contributor who changes `Extract` and forgets to clear state. Rejected because correctness after a code change should not depend on a manual, out-of-band step.

## Consequences

- A Flow whose extraction output can change **must** bump its `ExtractVersion`. Forgetting to bump reintroduces latent staleness; that cost is the same as before this decision, so it is not a regression, but it is now the documented contract and the `Flow.Version()` method makes it visible at the interface.
- The `fetch_cache` table gains an `extract_version` column, populated for new databases and migrated (idempotent `ALTER TABLE ADD COLUMN`) for existing ones. Existing rows default to version 0, so the first poll after upgrading rfs with a bumped Flow version forces exactly one full re-derivation — the deploy-time refresh we want, automatic.
- `FetchCache` now carries `ExtractVersion` alongside the HTTP validators; the HTTP fetcher ignores it, and the poller pins it to the running Flow's version on every save so a 304 (which echoes no useful version) cannot clobber it back to 0.
- `Poll` retains fast-path behaviour: matching version + 304 still skips `Extract` and leaves the snapshot untouched.
