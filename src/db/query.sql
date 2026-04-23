-- name: CreateSource :one
INSERT INTO sources (title, feed_url, site_url, icon_url)
VALUES (?, ?, ?, ?)
ON CONFLICT(feed_url) DO UPDATE SET
  site_url = excluded.site_url,
  icon_url = excluded.icon_url
RETURNING id, title, feed_url, site_url, icon_url, created_at, last_fetch;

-- name: UpdateSource :one
UPDATE sources
SET title = ?
WHERE id = ?
RETURNING id, title, feed_url, site_url, icon_url, created_at, last_fetch;

-- name: ListSources :many
SELECT id, title, feed_url, site_url, icon_url, created_at, last_fetch
FROM sources
ORDER BY title COLLATE NOCASE;

-- name: GetSource :one
SELECT id, title, feed_url, site_url, icon_url, created_at, last_fetch
FROM sources WHERE id = ?;

-- name: DeleteSource :exec
DELETE FROM sources WHERE id = ?;

-- name: TouchSourceFetch :exec
UPDATE sources SET last_fetch = ? WHERE id = ?;

-- name: UpsertItem :exec
INSERT INTO items (source_id, guid, title, url, url_norm, description, image_url, published_at, last_seen_in_feed)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_id, guid) DO UPDATE SET
  title = excluded.title,
  url = excluded.url,
  url_norm = excluded.url_norm,
  description = excluded.description,
  image_url = excluded.image_url,
  published_at = excluded.published_at,
  last_seen_in_feed = excluded.last_seen_in_feed;

-- name: MarkItemSeen :exec
UPDATE items SET viewed_at = CURRENT_TIMESTAMP
WHERE id = ? AND viewed_at IS NULL;

-- name: GetSettings :one
SELECT id, retention_days FROM settings WHERE id = 1;

-- name: UpdateRetention :exec
UPDATE settings SET retention_days = ? WHERE id = 1;

-- name: PruneOldItems :exec
DELETE FROM items WHERE last_seen_in_feed < ?;

-- name: ListItemsNewest :many
WITH ranked AS (
  SELECT
    i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
    i.published_at, i.fetched_at, i.last_seen_in_feed, i.viewed_at,
    s.title AS source_title, s.site_url AS source_site, s.last_fetch AS source_last_fetch,
    ROW_NUMBER() OVER (
      PARTITION BY i.url_norm
      ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id ASC
    ) AS rn
  FROM items i
  JOIN sources s ON s.id = i.source_id
  WHERE ( CAST(?1 AS INTEGER) = 0 OR i.image_url != '' )
    AND ( CAST(?2 AS INTEGER) = 0 OR (s.last_fetch IS NOT NULL AND i.last_seen_in_feed >= s.last_fetch) )
    AND ( CAST(?3 AS INTEGER) = 0 OR i.viewed_at IS NULL )
)
SELECT id, source_id, guid, title, url, description, image_url,
       published_at, fetched_at, viewed_at, source_title, source_site
FROM ranked
WHERE rn = 1
ORDER BY COALESCE(published_at, fetched_at) DESC, id DESC
LIMIT ?4 OFFSET ?5;

-- name: ListItemsOldest :many
WITH ranked AS (
  SELECT
    i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
    i.published_at, i.fetched_at, i.last_seen_in_feed, i.viewed_at,
    s.title AS source_title, s.site_url AS source_site, s.last_fetch AS source_last_fetch,
    ROW_NUMBER() OVER (
      PARTITION BY i.url_norm
      ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id ASC
    ) AS rn
  FROM items i
  JOIN sources s ON s.id = i.source_id
  WHERE ( CAST(?1 AS INTEGER) = 0 OR i.image_url != '' )
    AND ( CAST(?2 AS INTEGER) = 0 OR (s.last_fetch IS NOT NULL AND i.last_seen_in_feed >= s.last_fetch) )
    AND ( CAST(?3 AS INTEGER) = 0 OR i.viewed_at IS NULL )
)
SELECT id, source_id, guid, title, url, description, image_url,
       published_at, fetched_at, viewed_at, source_title, source_site
FROM ranked
WHERE rn = 1
ORDER BY COALESCE(published_at, fetched_at) ASC, id ASC
LIMIT ?4 OFFSET ?5;

-- name: ListItemsRandom :many
WITH ranked AS (
  SELECT
    i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
    i.published_at, i.fetched_at, i.last_seen_in_feed, i.viewed_at,
    s.title AS source_title, s.site_url AS source_site, s.last_fetch AS source_last_fetch,
    ROW_NUMBER() OVER (
      PARTITION BY i.url_norm
      ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id ASC
    ) AS rn
  FROM items i
  JOIN sources s ON s.id = i.source_id
  WHERE ( CAST(?1 AS INTEGER) = 0 OR i.image_url != '' )
    AND ( CAST(?2 AS INTEGER) = 0 OR (s.last_fetch IS NOT NULL AND i.last_seen_in_feed >= s.last_fetch) )
    AND ( CAST(?3 AS INTEGER) = 0 OR i.viewed_at IS NULL )
)
SELECT id, source_id, guid, title, url, description, image_url,
       published_at, fetched_at, viewed_at, source_title, source_site
FROM ranked
WHERE rn = 1
ORDER BY
  CASE
    WHEN viewed_at IS NULL AND source_last_fetch IS NOT NULL AND last_seen_in_feed >= source_last_fetch THEN 0
    WHEN viewed_at IS NULL THEN 1
    ELSE 2
  END,
  RANDOM()
LIMIT ?4 OFFSET ?5;

-- name: ListItemsNewestBySource :many
SELECT
  i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
  i.published_at, i.fetched_at, i.viewed_at,
  s.title AS source_title, s.site_url AS source_site
FROM items i
JOIN sources s ON s.id = i.source_id
WHERE i.source_id = ?1
  AND ( CAST(?2 AS INTEGER) = 0 OR i.image_url != '' )
  AND ( CAST(?3 AS INTEGER) = 0 OR (s.last_fetch IS NOT NULL AND i.last_seen_in_feed >= s.last_fetch) )
  AND ( CAST(?4 AS INTEGER) = 0 OR i.viewed_at IS NULL )
ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id DESC
LIMIT ?5 OFFSET ?6;

-- name: ListItemsOldestBySource :many
SELECT
  i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
  i.published_at, i.fetched_at, i.viewed_at,
  s.title AS source_title, s.site_url AS source_site
FROM items i
JOIN sources s ON s.id = i.source_id
WHERE i.source_id = ?1
  AND ( CAST(?2 AS INTEGER) = 0 OR i.image_url != '' )
  AND ( CAST(?3 AS INTEGER) = 0 OR (s.last_fetch IS NOT NULL AND i.last_seen_in_feed >= s.last_fetch) )
  AND ( CAST(?4 AS INTEGER) = 0 OR i.viewed_at IS NULL )
ORDER BY COALESCE(i.published_at, i.fetched_at) ASC, i.id ASC
LIMIT ?5 OFFSET ?6;

-- name: ListItemsRandomBySource :many
SELECT
  i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
  i.published_at, i.fetched_at, i.viewed_at,
  s.title AS source_title, s.site_url AS source_site
FROM items i
JOIN sources s ON s.id = i.source_id
WHERE i.source_id = ?1
  AND ( CAST(?2 AS INTEGER) = 0 OR i.image_url != '' )
  AND ( CAST(?3 AS INTEGER) = 0 OR (s.last_fetch IS NOT NULL AND i.last_seen_in_feed >= s.last_fetch) )
  AND ( CAST(?4 AS INTEGER) = 0 OR i.viewed_at IS NULL )
ORDER BY
  CASE
    WHEN i.viewed_at IS NULL AND s.last_fetch IS NOT NULL AND i.last_seen_in_feed >= s.last_fetch THEN 0
    WHEN i.viewed_at IS NULL THEN 1
    ELSE 2
  END,
  RANDOM()
LIMIT ?5 OFFSET ?6;
