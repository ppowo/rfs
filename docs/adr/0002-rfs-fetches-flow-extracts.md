# rfs fetches; Flows are pure extractors

rfs owns a single shared HTTP client that fetches source content — with conditional GETs (ETag / If-Modified-Since), retries, a User-Agent, and rate limiting — and hands the response body to each source's Flow. A Flow's only job is to turn fetched content into items: `(page) → []Item`. Flows do not fetch.

We chose this so all source-politeness and HTTP concerns live in one place, and so Flows are pure functions of source content — trivially testable against saved response fixtures with no network. For "hopefully static" sources the shared conditional-GET fetcher is especially cheap: most polls return 304 Not Modified.

## Considered options

- **Each Flow fetches itself** (rejected): a Flow would be fully autonomous (custom auth, multi-request, pagination) at the cost of duplicating HTTP/politeness logic per Flow and making Flows network-dependent and hard to unit-test. Rejected because the current sources are static resources that don't need per-Flow fetching; if a future source needs pagination or custom auth, the contract can be extended without rebuilding the common case.

## Consequences

- A Flow that genuinely needs multi-request fetching or custom auth is constrained by the shared fetcher until the contract is extended. Accepted as a YAGNI trade-off: don't pre-build that until a real source demands it.
- Flows are unit-tested with saved fixtures (response body in, items out); the shared fetcher is tested separately. No network in Flow tests.
