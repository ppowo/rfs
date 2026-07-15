# Flows decode fetched page bytes

rfs originally parsed every successful response as HTML before calling a Flow. That made the Flow interface specific to one representation even though the fetcher's responsibility is HTTP and some useful Sources, such as the Will Codex Reset forecast, publish their usable state as JSON.

The fetcher now returns the response body as `Page` bytes. Each Flow decodes the representation its Source publishes: HTML-backed Flows use the shared `rfs.ParseHTML` helper, while JSON-backed Flows use `encoding/json`. The existing rule from ADR 0002 remains unchanged: rfs fetches and Flows extract; a Flow never performs network requests.

We chose this seam because it keeps one small Flow interface, preserves conditional requests and domain throttling in one place, and keeps every Flow testable from a saved response fixture.

## Considered options

- **Add separate HTML and JSON Flow interfaces (rejected)**: avoids migrating existing Flows, but makes the poller select extractors at runtime and grows another interface for every future representation.
- **Let the JSON Flow fetch its own API data (rejected)**: violates ADR 0002, duplicates HTTP policy, and makes Flow tests network-dependent.
- **Parse JSON through the HTML parser and recover its text (rejected)**: relies on HTML parsing preserving arbitrary JSON strings and silently corrupts data containing HTML-significant text.

## Consequences

- HTML parsing moves from the HTTP fetcher into the HTML-backed Flow implementation via one shared helper.
- A Flow's saved fixture is the raw response body, whether HTML or JSON.
- Successful response bodies are buffered in memory before extraction. Current Sources are small; body-size limits can be added in the fetcher if a future Source requires them.
