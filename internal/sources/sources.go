package sources

import (
	"github.com/ppowo/rfs/internal/rfs"
	"github.com/ppowo/rfs/internal/sources/meltzer"
)

func All() []rfs.Source {
	return []rfs.Source{
		{
			ID:  "meltzer-5-star-matches",
			URL: meltzer.PageURL,
			Meta: rfs.SourceMeta{
				Title:       "Dave Meltzer 5-star wrestling matches",
				Description: "Professional wrestling matches rated 5 or more stars by Dave Meltzer.",
				Link:        meltzer.PageURL,
			},
			Flow: meltzer.Flow{},
		},
	}
}
