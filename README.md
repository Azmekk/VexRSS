# VexRSS

A single-binary, self-hostable Go RSS reader with a glassy, mobile-first UI. It shuffles news across your feeds, shows the current time and weather, and deduplicates cross-source stories that share the same URL.

- **One process, one file.** Everything — templates, static assets, schema — is embedded into a single Go binary. State lives in a SQLite file you point it at.
- **Zero external services.** No API keys. Weather comes from Open-Meteo (free, no key), geocoding falls back to the same service.
- **Designed for mobile.** Sticky compact header, single-column card grid on small screens, 44px+ tap targets, no hover-only interactions.
- **htmx, not a SPA.** Server-rendered `html/template` with small htmx swaps for shuffle/filter/add-source. No JS build step.

## Features

- **Shuffle / Newest / Oldest** sort modes, plus per-source filtering.
- **Glass cards**: each story renders as a rounded rectangle with its own image, the same image blurred & horizontally flipped behind it, and text on a semi-transparent glass panel.
- **Cross-source dedup**: `url_norm` normalises feed links (strips `utm_*`, `fbclid`, `www.`, trailing slashes, lowercases host) and a SQL `ROW_NUMBER() PARTITION BY url_norm` window collapses duplicates at query time.
- **Current time & weather** in the header. Location comes from `navigator.geolocation` (cached in `localStorage`); a city-name fallback is available when permission is denied.
- **Dedicated `/settings` page** for adding, renaming, refreshing and removing sources — inline-edit the source name, and it flows through to the card chips and the source-filter dropdown.
- **Custom dropdowns** that match the dark theme (no jarring native OS popups).

## Stack

- `github.com/go-chi/chi/v5` — HTTP router
- `modernc.org/sqlite` — pure-Go SQLite driver (no CGo; builds statically on any platform)
- `sqlc` v2 — type-safe generated query bindings, added as a Go tool dependency (`go tool sqlc …`)
- `github.com/mmcdole/gofeed` — RSS / Atom / JSON feed parser
- `golang.org/x/net/html` — fallback image extraction from feed item HTML
- `html/template` + [htmx](https://htmx.org/) — server-rendered UI with partial-swap interactivity
- [Open-Meteo](https://open-meteo.com/) — weather + geocoding, proxied through the server with a 10-minute in-memory cache

## Running

### With Docker Compose (recommended)

```bash
cp docker-compose.yml.example docker-compose.yml
docker compose up -d
```

Then open <http://localhost:8080>. Data persists in `./data/vexrss.db`.

### With Docker directly

```bash
docker build -t vexrss .
docker run -d \
  --name vexrss \
  -p 8080:8080 \
  -v "$(pwd)/data:/data" \
  --restart unless-stopped \
  vexrss
```

### From source

Requires Go 1.25+ (no CGo needed). All commands run from `src/`:

```bash
cd src
go tool sqlc generate              # only if you changed db/schema.sql or db/query.sql
go build -o ../bin/vexrss .        # ../bin/vexrss.exe on Windows
../bin/vexrss -addr :8080 -db ./vexrss.db
```

For development without producing a binary:

```bash
cd src && go run . -addr :8080 -db ./vexrss.db
```

### Flags

| Flag | Default | Purpose |
|---|---|---|
| `-addr` | `:8080` | Listen address |
| `-db` | `vexrss.db` | Path to the SQLite database file |
| `-poll` | `15m` | Feed refresh interval (Go `time.Duration`) |
| `-log` | `info` | Log level: `debug` \| `info` \| `warn` \| `error` |

## First use

Open the app and click the **gear icon** (top-left of the header) to go to `/settings`. Paste a feed URL — any RSS, Atom, or JSON feed works. Try:

- `https://hnrss.org/frontpage` (Hacker News)
- `https://www.theverge.com/rss/index.xml`
- `https://xkcd.com/rss.xml`

The first fetch runs inline, so cards show up within a second or two of adding a source.

## Repo layout

```
bin/                    Compiled binaries (gitignored per-file; dir is tracked)
Dockerfile              Multi-stage build → distroless/static image
docker-compose.yml.example
src/                    The Go module
├── main.go               Entrypoint: flags, DB open, fetcher, server
├── db/
│   ├── schema.sql          Applied on startup; embedded into the binary
│   ├── query.sql           sqlc source queries
│   └── gen/                sqlc output (checked in so Docker builds don't need sqlc)
├── internal/
│   ├── feed/               gofeed wrapper + URL normaliser + image picker + HTML strip
│   ├── server/             chi router, handlers, html/template loader
│   └── weather/            Open-Meteo client + 10-min cache
├── web/
│   ├── templates/          layout, index, settings, partials (card, cards, sources_list…)
│   └── static/             app.css, app.js, favicon.svg
├── go.mod, go.sum
└── sqlc.yaml
```

## Development notes

- **sqlc comments are ASCII-only.** Multi-line `--` comments with em-dashes or other non-ASCII characters corrupt the generated output. Stick to a single `-- name: … :one|:many|:exec` line per query.
- **Schema migrations.** Every boot runs `db/schema.sql` with `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`, so additive changes are automatic. Destructive changes need a manual migration for now.
- **Embedding.** `src/web/embed.go` embeds `templates/**/*.html` and `static/*`; `src/db/embed.go` embeds `schema.sql`. No runtime filesystem dependency.
- **Tests.** None yet. Contributions welcome.

## License

Not yet licensed. Code is **all rights reserved** for now — open an issue if you'd like to use it for something specific.
