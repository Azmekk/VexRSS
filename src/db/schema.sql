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
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  source_id     INTEGER NOT NULL REFERENCES sources(id) ON DELETE CASCADE,
  guid          TEXT NOT NULL,
  title         TEXT NOT NULL,
  url           TEXT NOT NULL,
  url_norm      TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',
  image_url     TEXT NOT NULL DEFAULT '',
  published_at  DATETIME,
  fetched_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(source_id, guid)
);

CREATE INDEX IF NOT EXISTS items_published_idx ON items(published_at DESC);
CREATE INDEX IF NOT EXISTS items_source_idx    ON items(source_id, published_at DESC);
CREATE INDEX IF NOT EXISTS items_url_norm_idx  ON items(url_norm);
