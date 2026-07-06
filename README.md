# rfs

rfs is a Go service that periodically fetches hardcoded web pages and serves each source as its own RSS 2.0 feed.

## Run

```sh
go run ./cmd/rfs
```

Feeds are served at:

- `http://localhost:8080/feeds/meltzer-5-star-matches.xml`

State is stored in a SQLite database under the OS user cache directory by default:

- Linux: `$XDG_CACHE_HOME/rfs/rfs.sqlite`, or `~/.cache/rfs/rfs.sqlite`
- macOS: `~/Library/Caches/rfs/rfs.sqlite`
- Windows: `%LocalAppData%\\rfs\\rfs.sqlite`

Use `-db :memory:` for a throwaway in-memory database, or `-db /path/to/rfs.sqlite` to choose a specific file.