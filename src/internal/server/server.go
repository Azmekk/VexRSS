package server

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	dbq "github.com/martinjordanov/vexrss/db/gen"
	"github.com/martinjordanov/vexrss/internal/feed"
	"github.com/martinjordanov/vexrss/internal/weather"
)

type Server struct {
	Queries   *dbq.Queries
	Fetcher   *feed.Fetcher
	Weather   *weather.Client
	Templates *Templates
	Static    http.FileSystem
	Logger    *slog.Logger
}

type Config struct {
	Queries   *dbq.Queries
	Fetcher   *feed.Fetcher
	Weather   *weather.Client
	Templates *Templates
	StaticFS  fs.FS // rooted at "static"
	Logger    *slog.Logger
}

func New(c Config) *Server {
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	return &Server{
		Queries:   c.Queries,
		Fetcher:   c.Fetcher,
		Weather:   c.Weather,
		Templates: c.Templates,
		Static:    http.FS(c.StaticFS),
		Logger:    c.Logger,
	}
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(slogRequest(s.Logger))

	r.Get("/", s.handleIndex)
	r.Get("/cards", s.handleCards)
	r.Get("/settings", s.handleSettings)

	r.Post("/sources", s.handleAddSource)
	r.Patch("/sources/{id}", s.handleUpdateSource)
	r.Delete("/sources/{id}", s.handleDeleteSource)
	r.Post("/sources/{id}/refresh", s.handleRefreshSource)

	r.Post("/settings/retention", s.handleUpdateRetention)
	r.Post("/items/{id}/seen", s.handleMarkSeen)

	r.Get("/api/weather", s.handleWeather)
	r.Get("/api/geocode", s.handleGeocode)

	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(s.Static)))

	// Serve the SVG favicon at the legacy /favicon.ico path too — browsers
	// request it unconditionally even when we've linked an SVG explicitly.
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		f, err := s.Static.Open("favicon.svg")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = io.Copy(w, f)
	})

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return r
}

// slogRequest is a minimal access log middleware built on slog. We use it
// instead of chi's default Logger to keep log format consistent.
func slogRequest(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			logger.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"dur_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
