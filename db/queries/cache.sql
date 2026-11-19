-- name: GetCacheEntry :one
SELECT key, schema_version, source_kind, source_id, created_at, last_used_at, size_bytes, payload
FROM cache_entries
WHERE key = ?;

-- name: UpsertCacheEntry :exec
INSERT INTO cache_entries (
  key, schema_version, source_kind, source_id, created_at, last_used_at, size_bytes, payload
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET
  schema_version = excluded.schema_version,
  source_kind = excluded.source_kind,
  source_id = excluded.source_id,
  created_at = excluded.created_at,
  last_used_at = excluded.last_used_at,
  size_bytes = excluded.size_bytes,
  payload = excluded.payload;

-- name: TouchCacheEntry :exec
UPDATE cache_entries SET last_used_at = ? WHERE key = ?;

-- name: DeleteCacheEntry :exec
DELETE FROM cache_entries WHERE key = ?;

-- name: DeleteStaleCacheEntries :exec
DELETE FROM cache_entries WHERE created_at < ?;

-- name: CacheTotalSize :one
SELECT CAST(COALESCE(SUM(size_bytes), 0) AS INTEGER) AS total FROM cache_entries;

-- name: OldestCacheEntries :many
SELECT key, size_bytes FROM cache_entries ORDER BY last_used_at ASC LIMIT ?;
