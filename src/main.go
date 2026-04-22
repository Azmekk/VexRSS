package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"database/sql"

	_ "modernc.org/sqlite"

	"github.com/martinjordanov/vexrss/db"
	dbq "github.com/martinjordanov/vexrss/db/gen"
	"github.com/martinjordanov/vexrss/internal/feed"
	"github.com/martinjordanov/vexrss/internal/server"
	"github.com/martinjordanov/vexrss/internal/weather"
	"github.com/martinjordanov/vexrss/web"
)

func main() {
	var (
		addr     = flag.String("addr", ":8080", "listen address")
		dbPath   = flag.String("db", "vexrss.db", "path to SQLite database file")
		pollStr  = flag.String("poll", "15m", "feed poll interval (Go duration, e.g. 15m)")
		logLevel = flag.String("log", "info", "log level: debug, info, warn, error")
	)
	flag.Parse()

	logger := newLogger(*logLevel)
	slog.SetDefault(logger)

	pollEvery, err := time.ParseDuration(*pollStr)
	if err != nil {
		logger.Error("invalid -poll duration", "err", err)
		os.Exit(2)
	}

	sqlDB, err := openDB(*dbPath)
	if err != nil {
		logger.Error("open db", "err", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	if _, err := sqlDB.Exec(db.Schema); err != nil {
		logger.Error("apply schema", "err", err)
		os.Exit(1)
	}

	queries := dbq.New(sqlDB)
	weatherCli := weather.New()
	fetcher := feed.New(queries, pollEvery, logger)

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		logger.Error("static FS", "err", err)
		os.Exit(1)
	}
	templates, err := server.ParseTemplates(web.Templates)
	if err != nil {
		logger.Error("parse templates", "err", err)
		os.Exit(1)
	}

	srv := server.New(server.Config{
		Queries:   queries,
		Fetcher:   fetcher,
		Weather:   weatherCli,
		Templates: templates,
		StaticFS:  staticFS,
		Logger:    logger,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go fetcher.Run(ctx)

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("vexrss listening", "addr", *addr, "db", *dbPath, "poll", pollEvery.String())
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server", "err", err)
			cancel()
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
}

func openDB(path string) (*sql.DB, error) {
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // modernc.org/sqlite recommends serializing writes; plenty for our throughput
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	h := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})
	return slog.New(h)
}
