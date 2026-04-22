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
UPDATE sources SET last_fetch = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpsertItem :exec
INSERT INTO items (source_id, guid, title, url, url_norm, description, image_url, published_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(source_id, guid) DO UPDATE SET
  title = excluded.title,
  url = excluded.url,
  url_norm = excluded.url_norm,
  description = excluded.description,
  image_url = excluded.image_url,
  published_at = excluded.published_at;

-- name: ListItemsNewest :many
WITH ranked AS (
  SELECT
    i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
    i.published_at, i.fetched_at,
    s.title AS source_title, s.site_url AS source_site,
    ROW_NUMBER() OVER (
      PARTITION BY i.url_norm
      ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id ASC
    ) AS rn
  FROM items i
  JOIN sources s ON s.id = i.source_id
)
SELECT id, source_id, guid, title, url, description, image_url,
       published_at, fetched_at, source_title, source_site
FROM ranked
WHERE rn = 1
ORDER BY COALESCE(published_at, fetched_at) DESC, id DESC
LIMIT ? OFFSET ?;

-- name: ListItemsOldest :many
WITH ranked AS (
  SELECT
    i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
    i.published_at, i.fetched_at,
    s.title AS source_title, s.site_url AS source_site,
    ROW_NUMBER() OVER (
      PARTITION BY i.url_norm
      ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id ASC
    ) AS rn
  FROM items i
  JOIN sources s ON s.id = i.source_id
)
SELECT id, source_id, guid, title, url, description, image_url,
       published_at, fetched_at, source_title, source_site
FROM ranked
WHERE rn = 1
ORDER BY COALESCE(published_at, fetched_at) ASC, id ASC
LIMIT ? OFFSET ?;

-- name: ListItemsRandom :many
WITH ranked AS (
  SELECT
    i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
    i.published_at, i.fetched_at,
    s.title AS source_title, s.site_url AS source_site,
    ROW_NUMBER() OVER (
      PARTITION BY i.url_norm
      ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id ASC
    ) AS rn
  FROM items i
  JOIN sources s ON s.id = i.source_id
)
SELECT id, source_id, guid, title, url, description, image_url,
       published_at, fetched_at, source_title, source_site
FROM ranked
WHERE rn = 1
ORDER BY RANDOM()
LIMIT ? OFFSET ?;

-- name: ListItemsNewestBySource :many
SELECT
  i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
  i.published_at, i.fetched_at,
  s.title AS source_title, s.site_url AS source_site
FROM items i
JOIN sources s ON s.id = i.source_id
WHERE i.source_id = ?
ORDER BY COALESCE(i.published_at, i.fetched_at) DESC, i.id DESC
LIMIT ? OFFSET ?;

-- name: ListItemsOldestBySource :many
SELECT
  i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
  i.published_at, i.fetched_at,
  s.title AS source_title, s.site_url AS source_site
FROM items i
JOIN sources s ON s.id = i.source_id
WHERE i.source_id = ?
ORDER BY COALESCE(i.published_at, i.fetched_at) ASC, i.id ASC
LIMIT ? OFFSET ?;

-- name: ListItemsRandomBySource :many
SELECT
  i.id, i.source_id, i.guid, i.title, i.url, i.description, i.image_url,
  i.published_at, i.fetched_at,
  s.title AS source_title, s.site_url AS source_site
FROM items i
JOIN sources s ON s.id = i.source_id
WHERE i.source_id = ?
ORDER BY RANDOM()
LIMIT ? OFFSET ?;
