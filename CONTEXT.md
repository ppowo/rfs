# rfs

rfs watches a set of hardcoded web resources and serves each one as its own RSS feed, so you can subscribe to publishers that provide no feed of their own.

## Language

**Source**:
A specific upstream web resource rfs watches for changes; each source maps to exactly one RSS feed. Its fetched representation may be HTML or JSON, while its metadata links to a human-facing page when one exists.
_Avoid_: thing, site, endpoint

**Flow**:
The per-source recipe that extracts feed items from fetched source content and gives each a stable identity. What counts as an item is the Flow's decision, not rfs's; rfs does the fetching.
_Avoid_: adapter, scraper, connector, pipeline

**Item**:
A single entry a Flow emits into its source's RSS feed. What counts as one item is defined by that source's Flow; rfs treats items opaquely except for a stable identity that becomes the item's RSS GUID (by which readers detect additions).
_Avoid_: entry, record, row, post
