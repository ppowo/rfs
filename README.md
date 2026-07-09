# rfs

rfs watches a set of hardcoded web pages and serves each one as its own RSS 2.0 feed, so you can subscribe to sites that publish no feed of their own.

## Run

```sh
go run ./cmd/rfs
```

By default the server listens on `:14298` and polls every hour. Feeds are served at:

- `/` — HTML index listing every source
- `/feeds/meltzer-5-star-matches.xml` — RSS 2.0 feed
- `/feeds/meltzer-5-star-matches.html` — HTML view of the feed

State is stored in a SQLite database under the OS user cache directory by default:

- Linux: `$XDG_CACHE_HOME/rfs/rfs.sqlite`, or `~/.cache/rfs/rfs.sqlite`
- macOS: `~/Library/Caches/rfs/rfs.sqlite`
- Windows: `%LocalAppData%\\rfs\\rfs.sqlite`

Use `-db :memory:` for a throwaway in-memory database, or `-db /path/to/rfs.sqlite` to choose a specific file. Run `go run ./cmd/rfs -h` for all flags (`-addr`, `-interval`, `-self-update`, `-self-update-interval`, `-self-update-timeout`, `-version`). Self-update checks run independently of source polling every hour by default, with a 30-second deadline per check.