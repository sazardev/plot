package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/sazar/plot/internal/db"
	"github.com/sazar/plot/internal/domain"
)

const (
	cacheSchemaVersion = 1
	// A cache hit may touch last_used_at, but lock contention must not delay
	// the frame; the regular write path keeps the 250 ms busy timeout.
	cacheTouchTimeout = 25 * time.Millisecond
)

// cacheMigrations are applied in order on the write path. The read path only
// gets the source schema from the generated schema_version column, as required
// by FR-13: it must never create or migrate the cache database.
var cacheMigrations = [][]string{
	{
		`CREATE TABLE IF NOT EXISTS cache_entries (
			key            TEXT    PRIMARY KEY,
			schema_version INTEGER NOT NULL,
			source_kind    TEXT    NOT NULL,
			source_id      TEXT    NOT NULL,
			created_at     INTEGER NOT NULL,
			last_used_at   INTEGER NOT NULL,
			size_bytes     INTEGER NOT NULL,
			payload        BLOB    NOT NULL
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS cache_entries_last_used_at
			ON cache_entries (last_used_at)`,
	},
}

var errCacheMiss = errors.New("cache miss")

// Cache is a disposable parsed-dataset cache. All methods are best-effort at
// the call site: a cache failure must never fail a render.
type Cache struct {
	path    string
	ttl     time.Duration
	maxSize int64

	db *sql.DB
	q  *db.Queries
}

// DefaultCacheDir is os.UserCacheDir()/pl.
func DefaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pl")
}

// NewCache prepares a cache handle without opening the database.
func NewCache(dir string, ttl time.Duration, maxSize int64) *Cache {
	if dir == "" {
		dir = DefaultCacheDir()
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	if maxSize <= 0 {
		maxSize = 64 << 20
	}
	if dir == "" {
		return nil
	}
	return &Cache{
		path:    filepath.Join(dir, "cache.db"),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Path is the cache database location.
func (c *Cache) Path() string {
	if c == nil {
		return ""
	}
	return c.path
}

// Get returns a decoded dataset on a hit. Every error is a miss.
func (c *Cache) Get(ctx context.Context, key string) (*domain.Dataset, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	if err := c.open(false); err != nil {
		if !errors.Is(err, errCacheMiss) {
			slog.Debug("cache read unavailable", "error", err)
		}
		return nil, false
	}
	entry, err := c.q.GetCacheEntry(ctx, key)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Debug("cache read failed", "error", err)
		}
		return nil, false
	}
	if entry.SchemaVersion != cacheSchemaVersion ||
		entry.SizeBytes <= 0 ||
		int64(len(entry.Payload)) > c.maxSize {
		c.discardEntry(ctx, key, "invalid metadata")
		return nil, false
	}
	if time.Since(time.Unix(entry.CreatedAt, 0)) > c.ttl {
		c.discardEntry(ctx, key, "expired")
		return nil, false
	}
	var dataset domain.Dataset
	if err := gob.NewDecoder(bytes.NewReader(entry.Payload)).Decode(&dataset); err != nil {
		c.discardEntry(ctx, key, "decode failed")
		return nil, false
	}
	if !validCachedDataset(&dataset) {
		c.discardEntry(ctx, key, "invalid dataset")
		return nil, false
	}
	now := time.Now().Unix()
	touchCtx, cancel := context.WithTimeout(ctx, cacheTouchTimeout)
	defer cancel()
	if err := c.q.TouchCacheEntry(touchCtx, db.TouchCacheEntryParams{LastUsedAt: now, Key: key}); err != nil {
		slog.Debug("cache touch failed", "error", err)
	}
	return &dataset, true
}

// Put stores a dataset and applies TTL/LRU eviction.
func (c *Cache) Put(ctx context.Context, key string, dataset *domain.Dataset) error {
	if c == nil || key == "" || dataset == nil {
		return nil
	}
	var payload bytes.Buffer
	if err := gob.NewEncoder(&payload).Encode(dataset); err != nil {
		return err
	}
	if int64(payload.Len()) > c.maxSize {
		return nil
	}
	if err := c.open(true); err != nil {
		return err
	}
	now := time.Now().Unix()
	if err := c.q.UpsertCacheEntry(ctx, db.UpsertCacheEntryParams{
		Key:           key,
		SchemaVersion: cacheSchemaVersion,
		SourceKind:    string(dataset.SourceKind),
		SourceID:      dataset.SourceID,
		CreatedAt:     now,
		LastUsedAt:    now,
		SizeBytes:     int64(payload.Len()),
		Payload:       payload.Bytes(),
	}); err != nil {
		return err
	}
	if err := c.q.DeleteStaleCacheEntries(ctx, time.Now().Add(-c.ttl).Unix()); err != nil {
		slog.Debug("cache expiry failed", "error", err)
	}
	err := c.evict(ctx)
	c.secureFiles()
	return err
}

func (c *Cache) discardEntry(ctx context.Context, key, reason string) {
	slog.Debug("discarding cache entry", "reason", reason)
	if err := c.q.DeleteCacheEntry(ctx, key); err != nil {
		slog.Debug("cache entry delete failed", "error", err)
	}
}

func validCachedDataset(dataset *domain.Dataset) bool {
	if dataset == nil || len(dataset.Columns) == 0 {
		return false
	}
	if dataset.RowCount != len(dataset.Rows) {
		return false
	}
	for _, row := range dataset.Rows {
		if len(row) != len(dataset.Columns) {
			return false
		}
	}
	return true
}

func (c *Cache) evict(ctx context.Context) error {
	for {
		total, err := c.q.CacheTotalSize(ctx)
		if err != nil {
			return err
		}
		if total <= c.maxSize {
			return nil
		}
		oldest, err := c.q.OldestCacheEntries(ctx, 32)
		if err != nil {
			return err
		}
		if len(oldest) == 0 {
			return nil
		}
		for _, entry := range oldest {
			if err := c.q.DeleteCacheEntry(ctx, entry.Key); err != nil {
				return err
			}
		}
	}
}

func (c *Cache) open(write bool) error {
	if c == nil {
		return errCacheMiss
	}
	if c.db != nil {
		return nil
	}
	if write {
		if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
			return err
		}
		_ = os.Chmod(filepath.Dir(c.path), 0o700)
	} else if _, err := os.Stat(c.path); err != nil {
		return errCacheMiss
	}

	err := c.connect(write)
	if err == nil {
		return nil
	}
	if !write || !isCorrupt(err) {
		return err
	}
	slog.Debug("cache database is corrupt; recreating", "error", err)
	c.removeFiles()
	return c.connect(write)
}

func (c *Cache) connect(write bool) error {
	options := "mode=rwc"
	if !write {
		options = "mode=rw"
	}
	dsn := "file:" + filepath.ToSlash(c.path) + "?" + options
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		_ = database.Close()
		return err
	}
	pragmas := []string{
		"PRAGMA busy_timeout=250",
		"PRAGMA foreign_keys=ON",
	}
	if write {
		pragmas = append(pragmas, "PRAGMA journal_mode=WAL")
	}
	for _, pragma := range pragmas {
		if _, err := database.Exec(pragma); err != nil {
			_ = database.Close()
			return err
		}
	}
	if write {
		if err := migrateCache(database); err != nil {
			_ = database.Close()
			return err
		}
	} else {
		version, err := cacheVersion(database)
		if err != nil {
			_ = database.Close()
			return err
		}
		if version != cacheSchemaVersion {
			_ = database.Close()
			return errCacheMiss
		}
	}
	c.db = database
	c.q = db.New(database)
	c.secureFiles()
	return nil
}

func cacheVersion(database *sql.DB) (int, error) {
	var version int
	if err := database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func migrateCache(database *sql.DB) error {
	version, err := cacheVersion(database)
	if err != nil {
		return err
	}
	if version > len(cacheMigrations) {
		if _, err := database.Exec("DROP TABLE IF EXISTS cache_entries"); err != nil {
			return err
		}
		version = 0
	}
	for i := version; i < len(cacheMigrations); i++ {
		for _, statement := range cacheMigrations[i] {
			if _, err := database.Exec(statement); err != nil {
				return err
			}
		}
		if _, err := database.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			return err
		}
	}
	return nil
}

func isCorrupt(err error) bool {
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	switch sqliteErr.Code() {
	case sqlite3.SQLITE_CORRUPT, sqlite3.SQLITE_NOTADB:
		return true
	default:
		return false
	}
}

func (c *Cache) secureFiles() {
	for _, path := range []string{c.path, c.path + "-wal", c.path + "-shm"} {
		_ = os.Chmod(path, 0o600)
	}
}

// Close releases the cache database.
func (c *Cache) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	err := c.db.Close()
	c.db = nil
	c.q = nil
	return err
}

// CacheKey hashes source identity and parse options. Length prefixes prevent
// ambiguity between adjacent fields.
func CacheKey(parts ...string) string {
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d:", cacheSchemaVersion)
	for _, part := range parts {
		_, _ = fmt.Fprintf(hash, "%d:%s", len(part), part)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

// StdinCacheKey includes the raw stdin bytes.
func StdinCacheKey(data []byte, opts CSVOptions) string {
	return CacheKey("stdin", contentHash(data), parseKey(opts))
}

// FileInput pairs a path with the bytes used for parsing.
type FileInput struct {
	Path string
	Data []byte
}

// FilesCacheKey includes every path, file identity and content hash in order.
func FilesCacheKey(inputs []FileInput, opts CSVOptions) string {
	parts := []string{"files"}
	for _, input := range inputs {
		absolute, err := filepath.Abs(input.Path)
		if err != nil {
			absolute = input.Path
		}
		parts = append(parts, absolute, fileIdentity(input.Path), contentHash(input.Data))
	}
	parts = append(parts, parseKey(opts))
	return CacheKey(parts...)
}

// DBCacheKey includes the database file identity and query text hash.
func DBCacheKey(path, query string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		absolute = path
	}
	return CacheKey("db", absolute, fileIdentity(path), contentHash([]byte(query)))
}

func fileIdentity(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "0:0"
	}
	return strconv.FormatInt(info.Size(), 10) + ":" +
		strconv.FormatInt(info.ModTime().UnixNano(), 10)
}

func parseKey(opts CSVOptions) string {
	return string(opts.Delimiter) + ":" + strconv.Itoa(int(opts.HeaderMode))
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c *Cache) removeFiles() {
	if c == nil {
		return
	}
	if c.db != nil {
		_ = c.db.Close()
		c.db = nil
		c.q = nil
	}
	for _, path := range []string{c.path, c.path + "-wal", c.path + "-shm"} {
		_ = os.Remove(path)
	}
}
