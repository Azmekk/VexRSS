CREATE TABLE IF NOT EXISTS sources (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  title       TEXT NOT NULL,
  feed_url    TEXT NOT NULL UNIQUE,
  site_url    TEXT NOT NULL DEFAULT '',
  icon_url    TEXT NOT NULL DEFAULT '',
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_fetch  DATETIME
);

CREATE TABLE IF NOT EXISTS items (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id          INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  guid               TEXT NOT NULL,
  title              TEXT NOT NULL,
  url                TEXT NOT NULL,
  url_norm           TEXT NOT NULL,
  description        TEXT NOT NULL DEFAULT '',
  image_url          TEXT NOT NULL DEFAULT '',
  published_at       DATETIME,
  fetched_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_seen_in_feed  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  viewed_at          DATETIME,
  UNIQUE(source_id, guid)
);

CREATE TABLE IF NOT EXISTS settings (
  id              INTEGER PRIMARY KEY CHECK (id = 1),
  retention_days  INTEGER NOT NULL DEFAULT 30
);
INSERT OR IGNORE INTO settings (id, retention_days) VALUES (1, 30);

CREATE INDEX IF NOT EXISTS items_published_idx ON items(published_at DESC);
CREATE INDEX IF NOT EXISTS items_source_idx    ON items(source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS items_url_norm_idx  ON items(url_norm);
CREATE INDEX IF NOT EXISTS items_last_seen_idx ON items(last_seen_in_feed DESC);
CREATE INDEX IF NOT EXISTS items_viewed_idx    ON items(viewed_at) WHERE viewed_at IS NULL;
