-- +goose Up
CREATE TABLE IF NOT EXISTS cache_entries (
  key            TEXT    PRIMARY KEY,
  schema_version INTEGER NOT NULL,
  source_kind    TEXT    NOT NULL,
  source_id      TEXT    NOT NULL,
  created_at     INTEGER NOT NULL,
  last_used_at   INTEGER NOT NULL,
  size_bytes     INTEGER NOT NULL,
  payload        BLOB    NOT NULL
) STRICT;

CREATE INDEX IF NOT EXISTS cache_entries_last_used_at
  ON cache_entries (last_used_at);

-- +goose Down
DROP TABLE cache_entries;
