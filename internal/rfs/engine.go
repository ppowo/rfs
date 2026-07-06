package rfs

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/net/html"
)

type DocumentFetcher interface {
	Fetch(context.Context, string, FetchCache) (FetchResult, error)
}

type SnapshotStore interface {
	LoadFetchCache(context.Context, string) (FetchCache, error)
	SaveFetchCache(context.Context, string, FetchCache) error
	SaveSnapshot(context.Context, string, []Item) error
	FirstSeen(context.Context, string, string, time.Time) (time.Time, error)
}

type Clock interface {
	Now() time.Time
}

type Poller struct {
	Fetcher DocumentFetcher
	Store   SnapshotStore
	Clock   Clock
}

type PollStatus int

const (
	PollUpdated PollStatus = iota
	PollUnchanged
	PollThrottled
)

type PollResult struct {
	Status     PollStatus
	RetryAfter time.Duration
}

func (p Poller) Poll(ctx context.Context, source Source) (PollResult, error) {
	if p.Fetcher == nil {
		return PollResult{}, fmt.Errorf("poll %s: missing fetcher", source.ID)
	}
	if p.Store == nil {
		return PollResult{}, fmt.Errorf("poll %s: missing store", source.ID)
	}
	if source.Flow == nil {
		return PollResult{}, fmt.Errorf("poll %s: missing flow", source.ID)
	}

	cache, err := p.Store.LoadFetchCache(ctx, source.ID)
	if err != nil {
		return PollResult{}, err
	}

	fetchResult, err := p.Fetcher.Fetch(ctx, source.URL, cache)
	if err != nil {
		return PollResult{}, err
	}

	switch fetchResult.Status {
	case FetchNotModified:
		if err := p.Store.SaveFetchCache(ctx, source.ID, fetchResult.Cache); err != nil {
			return PollResult{}, err
		}
		return PollResult{Status: PollUnchanged}, nil
	case FetchThrottled:
		return PollResult{Status: PollThrottled, RetryAfter: fetchResult.RetryAfter}, nil
	case FetchModified:
		if fetchResult.Document == nil {
			return PollResult{}, fmt.Errorf("poll %s: modified fetch returned no document", source.ID)
		}
	default:
		return PollResult{}, fmt.Errorf("poll %s: unknown fetch status %d", source.ID, fetchResult.Status)
	}

	extracted, err := source.Flow.Extract(fetchResult.Document)
	if err != nil {
		return PollResult{}, err
	}

	items := make([]Item, 0, len(extracted))
	seenGUIDs := map[string]struct{}{}
	for _, item := range extracted {
		if item.GUID == "" {
			continue
		}
		if _, seen := seenGUIDs[item.GUID]; seen {
			continue
		}
		seenGUIDs[item.GUID] = struct{}{}
		pubDate := p.now()
		if item.PubDate != nil {
			pubDate = *item.PubDate
		} else {
			seenAt, err := p.Store.FirstSeen(ctx, source.ID, item.GUID, pubDate)
			if err != nil {
				return PollResult{}, err
			}
			pubDate = seenAt
		}
		items = append(items, Item{
			GUID:        item.GUID,
			Title:       item.Title,
			Link:        item.Link,
			Description: item.Description,
			PubDate:     pubDate,
		})
	}

	if err := p.Store.SaveSnapshot(ctx, source.ID, items); err != nil {
		return PollResult{}, err
	}
	if err := p.Store.SaveFetchCache(ctx, source.ID, fetchResult.Cache); err != nil {
		return PollResult{}, err
	}
	return PollResult{Status: PollUpdated}, nil
}

func (p Poller) now() time.Time {
	if p.Clock == nil {
		return time.Now().UTC()
	}
	return p.Clock.Now()
}

var _ Flow = flowFunc(nil)

type flowFunc func(*html.Node) ([]ExtractedItem, error)

func (f flowFunc) Extract(doc *html.Node) ([]ExtractedItem, error) { return f(doc) }
