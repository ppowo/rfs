# rfs

rfs watches a set of hardcoded web pages and serves each one as its own RSS feed, so you can subscribe to sites that publish no feed of their own.

## Language

**Source**:
A specific web page rfs watches for changes; each source maps to exactly one RSS feed. Typically a static page that changes occasionally (e.g. a Wikipedia list that grows over time).
_Avoid_: thing, site, endpoint

**Flow**:
The per-source recipe that extracts feed items from a fetched page and gives each a stable identity. What counts as an item is the Flow's decision, not rfs's; rfs does the fetching.
_Avoid_: adapter, scraper, connector, pipeline

**Item**:
A single entry a Flow emits into its source's RSS feed. What counts as one item is defined by that source's Flow; rfs treats items opaquely except for a stable identity that becomes the item's RSS GUID (by which readers detect additions).
_Avoid_: entry, record, row, post
