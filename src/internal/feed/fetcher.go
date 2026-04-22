package feed

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"

	dbq "github.com/martinjordanov/vexrss/db/gen"
)

// Fetcher polls RSS sources on an interval and upserts new items.
type Fetcher struct {
	Queries  *dbq.Queries
	Interval time.Duration
	Timeout  time.Duration
	Logger   *slog.Logger
}

func New(q *dbq.Queries, interval time.Duration, logger *slog.Logger) *Fetcher {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Fetcher{
		Queries:  q,
		Interval: interval,
		Timeout:  15 * time.Second,
		Logger:   logger,
	}
}

// Run blocks, polling sources on each tick until ctx is cancelled. The first
// pass runs immediately on startup.
func (f *Fetcher) Run(ctx context.Context) {
	f.RunOnce(ctx)
	t := time.NewTicker(f.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.RunOnce(ctx)
		}
	}
}

// RunOnce fetches every known source once, in parallel.
func (f *Fetcher) RunOnce(ctx context.Context) {
	sources, err := f.Queries.ListSources(ctx)
	if err != nil {
		f.Logger.Error("list sources failed", "err", err)
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, s := range sources {
		s := s
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := f.FetchSource(ctx, s); err != nil {
				f.Logger.Warn("fetch source failed", "source", s.Title, "err", err)
			}
		}()
	}
	wg.Wait()
}

// FetchSource parses a single source's feed and upserts its items.
func (f *Fetcher) FetchSource(ctx context.Context, src dbq.Source) error {
	cctx, cancel := context.WithTimeout(ctx, f.Timeout)
	defer cancel()

	fp := gofeed.NewParser()
	fp.UserAgent = "vexrss/0.1 (+https://github.com/martinjordanov/vexrss)"
	feed, err := fp.ParseURLWithContext(src.FeedUrl, cctx)
	if err != nil {
		return err
	}

	for _, it := range feed.Items {
		if it == nil || it.Link == "" {
			continue
		}
		guid := it.GUID
		if guid == "" {
			guid = it.Link
		}
		var pub sql.NullTime
		if it.PublishedParsed != nil {
			pub = sql.NullTime{Time: *it.PublishedParsed, Valid: true}
		} else if it.UpdatedParsed != nil {
			pub = sql.NullTime{Time: *it.UpdatedParsed, Valid: true}
		}
		params := dbq.UpsertItemParams{
			SourceID:    src.ID,
			Guid:        guid,
			Title:       firstNonEmpty(it.Title, "(untitled)"),
			Url:         it.Link,
			UrlNorm:     NormalizeURL(it.Link),
			Description: cleanDescription(it.Description),
			ImageUrl:    PickImage(it),
			PublishedAt: pub,
		}
		if err := f.Queries.UpsertItem(cctx, params); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			f.Logger.Debug("upsert item failed", "source", src.Title, "url", it.Link, "err", err)
		}
	}

	if err := f.Queries.TouchSourceFetch(cctx, src.ID); err != nil {
		f.Logger.Debug("touch source failed", "source", src.Title, "err", err)
	}
	return nil
}

// ResolveFeedMeta parses the URL once to extract a title & site URL, for use
// when adding a new source interactively.
func ResolveFeedMeta(ctx context.Context, feedURL string) (title, siteURL string, err error) {
	fp := gofeed.NewParser()
	fp.UserAgent = "vexrss/0.1 (+https://github.com/martinjordanov/vexrss)"
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	feed, err := fp.ParseURLWithContext(feedURL, cctx)
	if err != nil {
		return "", "", err
	}
	return firstNonEmpty(feed.Title, feedURL), feed.Link, nil
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// cleanDescription strips HTML tags from a feed description and trims it for
// display. It's cheap and good enough for a card blurb; we don't try to render
// full article HTML.
func cleanDescription(body string) string {
	if body == "" {
		return ""
	}
	text := stripTags(body)
	text = collapseSpace(text)
	const max = 280
	if len(text) > max {
		text = text[:max] + "…"
	}
	return text
}
