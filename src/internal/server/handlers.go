package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	dbq "github.com/martinjordanov/vexrss/db/gen"
	"github.com/martinjordanov/vexrss/internal/feed"
)

const defaultCardLimit = 24

// Card is the unified view model the card template renders. All list queries
// are projected into this shape.
type Card struct {
	ID          int64
	Title       string
	URL         string
	Description string
	ImageURL    string
	PublishedAt time.Time
	SourceTitle string
	SourceSite  string
	Seen        bool
}

// SourceRow is the view model for the sources list.
type SourceRow struct {
	ID        int64
	Title     string
	FeedURL   string
	SiteURL   string
	LastFetch time.Time
}

// IndexData is passed to the full-page index template.
type IndexData struct {
	PageName  string
	PageTitle string
	Cards     []Card
	Sources   []SourceRow
	Filter    FilterState
}

// SettingsData is passed to the full-page settings template.
type SettingsData struct {
	PageName       string
	PageTitle      string
	Sources        []SourceRow
	RetentionDays  int64
	RetentionSaved bool
}

// CardsData is the partial render payload for #cards swaps.
type CardsData struct {
	Cards  []Card
	Filter FilterState
}

// FilterState captures the currently-applied sort + source filter, used to
// round-trip selection state in the template.
type FilterState struct {
	Sort       string // "new" | "old" | "shuffle"
	Source     string // "all" or numeric id as string
	FreshOnly  bool   // hide items no longer present in the latest fetch
	UnseenOnly bool   // hide items the user has already clicked through
	WithImage  bool   // hide items with no image
}

func (f FilterState) IsSort(s string) bool   { return f.Sort == s }
func (f FilterState) IsSource(s string) bool { return f.Source == s }

func boolToInt64(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	filter := parseFilter(r)
	cards, err := s.loadCards(r.Context(), filter, defaultCardLimit, 0)
	if err != nil {
		s.Logger.Error("load cards", "err", err)
		http.Error(w, "could not load feed", http.StatusInternalServerError)
		return
	}
	sources, err := s.loadSources(r.Context())
	if err != nil {
		s.Logger.Error("load sources", "err", err)
		http.Error(w, "could not load sources", http.StatusInternalServerError)
		return
	}
	data := IndexData{
		PageName: "index",
		Cards:    cards,
		Sources:  sources,
		Filter:   filter,
	}
	if err := s.Templates.RenderPage(w, "index", data); err != nil {
		s.Logger.Error("render index", "err", err)
	}
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	sources, err := s.loadSources(r.Context())
	if err != nil {
		s.Logger.Error("load sources", "err", err)
		http.Error(w, "could not load sources", http.StatusInternalServerError)
		return
	}
	settings, err := s.Queries.GetSettings(r.Context())
	if err != nil {
		s.Logger.Error("load settings", "err", err)
		http.Error(w, "could not load settings", http.StatusInternalServerError)
		return
	}
	data := SettingsData{
		PageName:       "settings",
		PageTitle:      "Settings",
		Sources:        sources,
		RetentionDays:  settings.RetentionDays,
		RetentionSaved: r.URL.Query().Get("saved") == "retention",
	}
	if err := s.Templates.RenderPage(w, "settings", data); err != nil {
		s.Logger.Error("render settings", "err", err)
	}
}

// handleUpdateRetention sets the global retention-days value. Accepts 0
// (never prune) through 3650 days (~10 years).
func (s *Server) handleUpdateRetention(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	raw := strings.TrimSpace(r.Form.Get("retention_days"))
	days, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || days < 0 || days > 3650 {
		http.Error(w, "retention_days must be an integer between 0 and 3650", http.StatusBadRequest)
		return
	}
	if err := s.Queries.UpdateRetention(r.Context(), days); err != nil {
		s.Logger.Error("update retention", "err", err)
		http.Error(w, "could not save retention", http.StatusInternalServerError)
		return
	}
	// htmx swap target is the retention form itself — re-render with saved flash.
	settings, err := s.Queries.GetSettings(r.Context())
	if err != nil {
		s.Logger.Error("reload settings", "err", err)
		http.Error(w, "could not reload settings", http.StatusInternalServerError)
		return
	}
	if err := s.Templates.RenderPartial(w, "retention_form", retentionFormData{
		RetentionDays: settings.RetentionDays,
		Saved:         true,
	}); err != nil {
		s.Logger.Error("render retention_form", "err", err)
	}
}

type retentionFormData struct {
	RetentionDays int64
	Saved         bool
}

// handleMarkSeen is the landing point for the navigator.sendBeacon fired when
// a user clicks a card. Idempotent — repeat POSTs are no-ops.
func (s *Server) handleMarkSeen(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.Queries.MarkItemSeen(r.Context(), id); err != nil {
		s.Logger.Debug("mark seen", "id", id, "err", err)
		// still 204 — the beacon doesn't read the response, and we don't want
		// to spam error logs when the DB is momentarily busy.
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCards(w http.ResponseWriter, r *http.Request) {
	filter := parseFilter(r)
	limit, offset := parsePaging(r)
	cards, err := s.loadCards(r.Context(), filter, limit, offset)
	if err != nil {
		s.Logger.Error("load cards", "err", err)
		http.Error(w, "could not load feed", http.StatusInternalServerError)
		return
	}
	if err := s.Templates.RenderPartial(w, "cards", CardsData{Cards: cards, Filter: filter}); err != nil {
		s.Logger.Error("render cards partial", "err", err)
	}
}

func (s *Server) handleAddSource(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	feedURL := strings.TrimSpace(r.Form.Get("feed_url"))
	if feedURL == "" {
		http.Error(w, "feed_url is required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(feedURL, "http://") && !strings.HasPrefix(feedURL, "https://") {
		feedURL = "https://" + feedURL
	}
	customTitle := strings.TrimSpace(r.Form.Get("title"))
	if len(customTitle) > 80 {
		customTitle = customTitle[:80]
	}

	parsedTitle, siteURL, err := feed.ResolveFeedMeta(r.Context(), feedURL)
	if err != nil {
		s.Logger.Warn("resolve feed meta failed", "url", feedURL, "err", err)
		http.Error(w, "could not parse that feed — check the URL", http.StatusBadRequest)
		return
	}

	title := customTitle
	if title == "" {
		title = parsedTitle
	}

	created, err := s.Queries.CreateSource(r.Context(), dbq.CreateSourceParams{
		Title:   title,
		FeedUrl: feedURL,
		SiteUrl: siteURL,
	})
	if err != nil {
		s.Logger.Error("create source", "err", err)
		http.Error(w, "could not save source", http.StatusInternalServerError)
		return
	}

	// Fetch once synchronously so the user sees cards immediately.
	if err := s.Fetcher.FetchSource(r.Context(), created); err != nil {
		s.Logger.Warn("initial fetch after add failed", "source", created.Title, "err", err)
	}

	// Trigger an htmx event so the cards area re-fetches itself.
	w.Header().Set("HX-Trigger", "sources-changed")
	sources, err := s.loadSources(r.Context())
	if err != nil {
		s.Logger.Error("load sources", "err", err)
		http.Error(w, "could not reload sources", http.StatusInternalServerError)
		return
	}
	if err := s.Templates.RenderPartial(w, "sources_list", sources); err != nil {
		s.Logger.Error("render sources_list", "err", err)
	}
}

func (s *Server) handleDeleteSource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.Queries.DeleteSource(r.Context(), id); err != nil {
		s.Logger.Error("delete source", "err", err)
		http.Error(w, "could not delete source", http.StatusInternalServerError)
		return
	}
	w.Header().Set("HX-Trigger", "sources-changed")
	sources, err := s.loadSources(r.Context())
	if err != nil {
		s.Logger.Error("load sources", "err", err)
		http.Error(w, "could not reload sources", http.StatusInternalServerError)
		return
	}
	if err := s.Templates.RenderPartial(w, "sources_list", sources); err != nil {
		s.Logger.Error("render sources_list", "err", err)
	}
}

func (s *Server) handleUpdateSource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	title := strings.TrimSpace(r.Form.Get("title"))
	if title == "" {
		http.Error(w, "title cannot be empty", http.StatusBadRequest)
		return
	}
	if len(title) > 80 {
		title = title[:80]
	}
	updated, err := s.Queries.UpdateSource(r.Context(), dbq.UpdateSourceParams{
		Title: title,
		ID:    id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		s.Logger.Error("update source", "err", err)
		http.Error(w, "could not update source", http.StatusInternalServerError)
		return
	}
	row := SourceRow{
		ID:      updated.ID,
		Title:   updated.Title,
		FeedURL: updated.FeedUrl,
		SiteURL: updated.SiteUrl,
	}
	if updated.LastFetch.Valid {
		row.LastFetch = updated.LastFetch.Time
	}
	w.Header().Set("HX-Trigger", "sources-changed")
	if err := s.Templates.RenderPartial(w, "source_row", row); err != nil {
		s.Logger.Error("render source_row", "err", err)
	}
}

func (s *Server) handleRefreshSource(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	src, err := s.Queries.GetSource(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "db error", http.StatusInternalServerError)
		return
	}
	if err := s.Fetcher.FetchSource(r.Context(), src); err != nil {
		s.Logger.Warn("manual refresh failed", "source", src.Title, "err", err)
	}
	w.Header().Set("HX-Trigger", "sources-changed")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleWeather(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, lerr := strconv.ParseFloat(q.Get("lat"), 64)
	lon, oerr := strconv.ParseFloat(q.Get("lon"), 64)
	if lerr != nil || oerr != nil {
		http.Error(w, "lat and lon are required floats", http.StatusBadRequest)
		return
	}
	cur, err := s.Weather.Fetch(r.Context(), lat, lon)
	if err != nil {
		s.Logger.Warn("weather fetch", "err", err)
		http.Error(w, "weather lookup failed", http.StatusBadGateway)
		return
	}
	if label := strings.TrimSpace(q.Get("label")); label != "" {
		cur.Label = label
	}
	writeJSON(w, http.StatusOK, cur)
}

func (s *Server) handleGeocode(w http.ResponseWriter, r *http.Request) {
	place := strings.TrimSpace(r.URL.Query().Get("q"))
	if place == "" {
		http.Error(w, "q is required", http.StatusBadRequest)
		return
	}
	lat, lon, label, err := s.Weather.Geocode(r.Context(), place)
	if err != nil {
		http.Error(w, "could not geocode "+place, http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lat":   lat,
		"lon":   lon,
		"label": label,
	})
}

// loadCards dispatches to the right sqlc query for the given filter and
// projects the result into the unified Card type.
func (s *Server) loadCards(ctx context.Context, f FilterState, limit, offset int) ([]Card, error) {
	if limit <= 0 {
		limit = defaultCardLimit
	}
	lim := int64(limit)
	off := int64(offset)

	imgFlag := boolToInt64(f.WithImage)
	freshFlag := boolToInt64(f.FreshOnly)
	unseenFlag := boolToInt64(f.UnseenOnly)

	if srcID, ok := parseSourceFilter(f.Source); ok {
		switch f.Sort {
		case "old":
			rows, err := s.Queries.ListItemsOldestBySource(ctx, dbq.ListItemsOldestBySourceParams{
				SourceID: srcID, Column2: imgFlag, Column3: freshFlag, Column4: unseenFlag,
				Limit: lim, Offset: off,
			})
			if err != nil {
				return nil, err
			}
			out := make([]Card, 0, len(rows))
			for _, r := range rows {
				out = append(out, cardFromOldestBySource(r))
			}
			return out, nil
		case "shuffle":
			rows, err := s.Queries.ListItemsRandomBySource(ctx, dbq.ListItemsRandomBySourceParams{
				SourceID: srcID, Column2: imgFlag, Column3: freshFlag, Column4: unseenFlag,
				Limit: lim, Offset: off,
			})
			if err != nil {
				return nil, err
			}
			out := make([]Card, 0, len(rows))
			for _, r := range rows {
				out = append(out, cardFromRandomBySource(r))
			}
			return out, nil
		default:
			rows, err := s.Queries.ListItemsNewestBySource(ctx, dbq.ListItemsNewestBySourceParams{
				SourceID: srcID, Column2: imgFlag, Column3: freshFlag, Column4: unseenFlag,
				Limit: lim, Offset: off,
			})
			if err != nil {
				return nil, err
			}
			out := make([]Card, 0, len(rows))
			for _, r := range rows {
				out = append(out, cardFromNewestBySource(r))
			}
			return out, nil
		}
	}

	switch f.Sort {
	case "old":
		rows, err := s.Queries.ListItemsOldest(ctx, dbq.ListItemsOldestParams{
			Column1: imgFlag, Column2: freshFlag, Column3: unseenFlag, Limit: lim, Offset: off,
		})
		if err != nil {
			return nil, err
		}
		out := make([]Card, 0, len(rows))
		for _, r := range rows {
			out = append(out, cardFromOldest(r))
		}
		return out, nil
	case "shuffle":
		rows, err := s.Queries.ListItemsRandom(ctx, dbq.ListItemsRandomParams{
			Column1: imgFlag, Column2: freshFlag, Column3: unseenFlag, Limit: lim, Offset: off,
		})
		if err != nil {
			return nil, err
		}
		out := make([]Card, 0, len(rows))
		for _, r := range rows {
			out = append(out, cardFromRandom(r))
		}
		return out, nil
	default:
		rows, err := s.Queries.ListItemsNewest(ctx, dbq.ListItemsNewestParams{
			Column1: imgFlag, Column2: freshFlag, Column3: unseenFlag, Limit: lim, Offset: off,
		})
		if err != nil {
			return nil, err
		}
		out := make([]Card, 0, len(rows))
		for _, r := range rows {
			out = append(out, cardFromNewest(r))
		}
		return out, nil
	}
}

func (s *Server) loadSources(ctx context.Context) ([]SourceRow, error) {
	rows, err := s.Queries.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]SourceRow, 0, len(rows))
	for _, r := range rows {
		var last time.Time
		if r.LastFetch.Valid {
			last = r.LastFetch.Time
		}
		out = append(out, SourceRow{
			ID:        r.ID,
			Title:     r.Title,
			FeedURL:   r.FeedUrl,
			SiteURL:   r.SiteUrl,
			LastFetch: last,
		})
	}
	return out, nil
}

// --- projection helpers ---

func cardFromNewest(r dbq.ListItemsNewestRow) Card {
	return Card{
		ID: r.ID, Title: r.Title, URL: r.Url, Description: r.Description, ImageURL: r.ImageUrl,
		PublishedAt: nullTime(r.PublishedAt, r.FetchedAt),
		SourceTitle: r.SourceTitle, SourceSite: r.SourceSite,
		Seen: r.ViewedAt.Valid,
	}
}
func cardFromOldest(r dbq.ListItemsOldestRow) Card {
	return Card{
		ID: r.ID, Title: r.Title, URL: r.Url, Description: r.Description, ImageURL: r.ImageUrl,
		PublishedAt: nullTime(r.PublishedAt, r.FetchedAt),
		SourceTitle: r.SourceTitle, SourceSite: r.SourceSite,
		Seen: r.ViewedAt.Valid,
	}
}
func cardFromRandom(r dbq.ListItemsRandomRow) Card {
	return Card{
		ID: r.ID, Title: r.Title, URL: r.Url, Description: r.Description, ImageURL: r.ImageUrl,
		PublishedAt: nullTime(r.PublishedAt, r.FetchedAt),
		SourceTitle: r.SourceTitle, SourceSite: r.SourceSite,
		Seen: r.ViewedAt.Valid,
	}
}
func cardFromNewestBySource(r dbq.ListItemsNewestBySourceRow) Card {
	return Card{
		ID: r.ID, Title: r.Title, URL: r.Url, Description: r.Description, ImageURL: r.ImageUrl,
		PublishedAt: nullTime(r.PublishedAt, r.FetchedAt),
		SourceTitle: r.SourceTitle, SourceSite: r.SourceSite,
		Seen: r.ViewedAt.Valid,
	}
}
func cardFromOldestBySource(r dbq.ListItemsOldestBySourceRow) Card {
	return Card{
		ID: r.ID, Title: r.Title, URL: r.Url, Description: r.Description, ImageURL: r.ImageUrl,
		PublishedAt: nullTime(r.PublishedAt, r.FetchedAt),
		SourceTitle: r.SourceTitle, SourceSite: r.SourceSite,
		Seen: r.ViewedAt.Valid,
	}
}
func cardFromRandomBySource(r dbq.ListItemsRandomBySourceRow) Card {
	return Card{
		ID: r.ID, Title: r.Title, URL: r.Url, Description: r.Description, ImageURL: r.ImageUrl,
		PublishedAt: nullTime(r.PublishedAt, r.FetchedAt),
		SourceTitle: r.SourceTitle, SourceSite: r.SourceSite,
		Seen: r.ViewedAt.Valid,
	}
}

// --- request helpers ---

func parseFilter(r *http.Request) FilterState {
	q := r.URL.Query()

	sort := q.Get("sort")
	switch sort {
	case "new", "old", "shuffle":
	default:
		sort = "new"
	}
	src := q.Get("source")
	if src == "" {
		src = "all"
	}

	// The form emits a hidden `filtered=1` every time it's submitted. Its
	// presence distinguishes "user unticked a checkbox" (checkbox absent from
	// query string, but filtered=1) from "first page load, no form submission
	// yet" (no query params at all) — in the latter case we apply sensible
	// defaults: fresh=on, unseen=on, with_image=off.
	fs := FilterState{Sort: sort, Source: src}
	if q.Has("filtered") {
		fs.FreshOnly = q.Get("fresh") == "1"
		fs.UnseenOnly = q.Get("unseen") == "1"
		fs.WithImage = q.Get("with_image") == "1"
	} else {
		fs.FreshOnly = true
		fs.UnseenOnly = true
		fs.WithImage = false
	}
	return fs
}

func parsePaging(r *http.Request) (limit, offset int) {
	q := r.URL.Query()
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 && n <= 100 {
		limit = n
	} else {
		limit = defaultCardLimit
	}
	if n, err := strconv.Atoi(q.Get("offset")); err == nil && n >= 0 {
		offset = n
	}
	return
}

func parseSourceFilter(s string) (int64, bool) {
	if s == "" || s == "all" {
		return 0, false
	}
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func nullTime(nt sql.NullTime, fallback time.Time) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return fallback
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

