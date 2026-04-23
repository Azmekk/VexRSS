# VexRSS

A small, self-hosted RSS reader. One Go binary, one SQLite file, and a glassy card UI that looks good on your phone.

- **Shuffle, sort, filter** news across all your feeds, or just one at a time.
- **Card-based reading.** Each story renders as a rounded card with its image, a blurred version of that image as the backdrop, and a glass text panel on top.
- **Clock + weather** in the header. Weather uses [Open-Meteo](https://open-meteo.com/), so no API key — just allow location once.
- **Cross-source dedup.** If two feeds link to the same article, you see it once.
- **Works on mobile.** Responsive layout, no hover-only interactions.

## Quick start — Docker Compose

```bash
curl -O https://raw.githubusercontent.com/Azmekk/VexRSS/main/docker-compose.yml.example
mv docker-compose.yml.example docker-compose.yml
docker compose up -d
```

Open <http://localhost:8080>. Click the gear icon (top-left) to add your first feed — try `https://hnrss.org/frontpage` or `https://xkcd.com/rss.xml` to get going.

The SQLite database is stored in `./data/vexrss.db` next to your compose file.

## Docker (without Compose)

```bash
docker run -d \
  --name vexrss \
  -p 8080:8080 \
  -v "$(pwd)/data:/data" \
  --restart unless-stopped \
  ghcr.io/azmekk/vexrss:latest
```

## Configuration

Override via flags on the container command:

| Flag | Default | What it does |
|---|---|---|
| `-addr` | `:8080` | Listen address |
| `-db` | `/data/vexrss.db` | Path to the SQLite file |
| `-poll` | `15m` | How often feeds are refreshed (Go duration) |
| `-log` | `info` | `debug` \| `info` \| `warn` \| `error` |

## Building from source

Requires Go 1.26+. No CGo or external tools needed at runtime — everything is embedded into the binary.

```bash
git clone https://github.com/Azmekk/VexRSS.git
cd VexRSS/src
go build -o ../bin/vexrss .
../bin/vexrss -addr :8080 -db ./vexrss.db
```

## License

Not yet licensed. All rights reserved for now — open an issue if you have a specific use case in mind.
