package storage_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/storage"
)

func cacheDataset(t *testing.T) *domain.Dataset {
	t.Helper()
	ds := &domain.Dataset{
		Columns: []domain.Column{
			{Name: "month", OriginalName: "month", Index: 0},
			{Name: "revenue", OriginalName: "revenue", Index: 1},
		},
		Rows: [][]domain.Cell{
			{domain.TextCell("Jan"), domain.TextCell("10")},
			{domain.TextCell("Feb"), domain.TextCell("20")},
		},
		SourceKind:  domain.SourceFile,
		SourceID:    "sales.csv",
		SourceCount: 1,
		RowCount:    2,
	}
	require.NoError(t, domain.Infer(ds))
	return ds
}

func TestCachePutGetRoundTrip(t *testing.T) {
	t.Parallel()

	cache := storage.NewCache(t.TempDir(), time.Hour, 1<<20)
	t.Cleanup(func() { _ = cache.Close() })
	key := storage.CacheKey("test", "one")
	ds := cacheDataset(t)

	require.NoError(t, cache.Put(context.Background(), key, ds))
	got, ok := cache.Get(context.Background(), key)
	require.True(t, ok)
	require.NotNil(t, got)
	assert.Equal(t, ds.RowCount, got.RowCount)
	assert.Equal(t, ds.Columns[1].Name, got.Columns[1].Name)
	assert.Equal(t, domain.CellNumber, got.Rows[0][1].Kind)
}

func TestCacheReadDoesNotCreateDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := storage.NewCache(dir, time.Hour, 1<<20)
	_, ok := cache.Get(context.Background(), "missing")
	assert.False(t, ok)
	_, err := os.Stat(filepath.Join(dir, "cache.db"))
	assert.True(t, os.IsNotExist(err))
}

func TestCacheTTLAndSize(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := storage.NewCache(dir, time.Millisecond, 1<<20)
	t.Cleanup(func() { _ = cache.Close() })
	key := storage.CacheKey("ttl")
	require.NoError(t, cache.Put(context.Background(), key, cacheDataset(t)))
	time.Sleep(3 * time.Millisecond)
	_, ok := cache.Get(context.Background(), key)
	assert.False(t, ok)

	small := storage.NewCache(t.TempDir(), time.Hour, 8)
	t.Cleanup(func() { _ = small.Close() })
	require.NoError(t, small.Put(context.Background(), "key", cacheDataset(t)))
	_, ok = small.Get(context.Background(), "key")
	assert.False(t, ok)
}

func TestCacheCorruptPayloadIsDiscarded(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := storage.NewCache(dir, time.Hour, 1<<20)
	t.Cleanup(func() { _ = cache.Close() })
	key := storage.CacheKey("corrupt")
	require.NoError(t, cache.Put(context.Background(), key, cacheDataset(t)))

	database, err := sql.Open("sqlite", filepath.Join(dir, "cache.db"))
	require.NoError(t, err)
	_, err = database.Exec(`update cache_entries set payload = x'0102' where key = ?`, key)
	require.NoError(t, err)
	require.NoError(t, database.Close())

	_, ok := cache.Get(context.Background(), key)
	assert.False(t, ok)
}

func TestCachePermissions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := storage.NewCache(dir, time.Hour, 1<<20)
	t.Cleanup(func() { _ = cache.Close() })
	require.NoError(t, cache.Put(context.Background(), "key", cacheDataset(t)))

	info, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	info, err = os.Stat(cache.Path())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	for _, suffix := range []string{"-wal", "-shm"} {
		if info, err := os.Stat(cache.Path() + suffix); err == nil {
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), suffix)
		}
	}
}

func TestCacheKeysChangeWithContentAndOptions(t *testing.T) {
	t.Parallel()

	base := storage.StdinCacheKey([]byte("a,b\n1,2\n"), storage.CSVOptions{})
	assert.NotEqual(t, base, storage.StdinCacheKey([]byte("a,b\n1,3\n"), storage.CSVOptions{}))
	assert.NotEqual(t, base, storage.StdinCacheKey([]byte("a,b\n1,2\n"), storage.CSVOptions{HeaderMode: storage.HeaderPresent}))

	filesA := storage.FilesCacheKey([]storage.FileInput{{Path: "a.csv", Data: []byte("x")}}, storage.CSVOptions{})
	filesB := storage.FilesCacheKey([]storage.FileInput{{Path: "b.csv", Data: []byte("x")}}, storage.CSVOptions{})
	assert.NotEqual(t, filesA, filesB)

	dbA := storage.DBCacheKey("metrics.db", "select 1")
	dbB := storage.DBCacheKey("metrics.db", "select 2")
	assert.NotEqual(t, dbA, dbB)
}

func TestCacheSchemaMismatchIsMissAndWriteMigrates(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := storage.NewCache(dir, time.Hour, 1<<20)
	key := storage.CacheKey("schema")
	require.NoError(t, cache.Put(context.Background(), key, cacheDataset(t)))
	require.NoError(t, cache.Close())

	database, err := sql.Open("sqlite", filepath.Join(dir, "cache.db"))
	require.NoError(t, err)
	_, err = database.Exec("PRAGMA user_version = 99")
	require.NoError(t, err)
	require.NoError(t, database.Close())

	reopened := storage.NewCache(dir, time.Hour, 1<<20)
	t.Cleanup(func() { _ = reopened.Close() })
	_, ok := reopened.Get(context.Background(), key)
	assert.False(t, ok, "a future schema version must be a miss")

	require.NoError(t, reopened.Put(context.Background(), key, cacheDataset(t)))
	_, ok = reopened.Get(context.Background(), key)
	assert.True(t, ok, "the write path must recreate the schema")
}

func TestCacheCorruptDatabaseIsRecreatedOnWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cache.db"), []byte("not a database"), 0o600))
	cache := storage.NewCache(dir, time.Hour, 1<<20)
	t.Cleanup(func() { _ = cache.Close() })

	key := storage.CacheKey("corrupt-db")
	require.NoError(t, cache.Put(context.Background(), key, cacheDataset(t)))
	_, ok := cache.Get(context.Background(), key)
	assert.True(t, ok)
}

func TestCacheInvalidDatasetIsDiscarded(t *testing.T) {
	t.Parallel()

	cache := storage.NewCache(t.TempDir(), time.Hour, 1<<20)
	t.Cleanup(func() { _ = cache.Close() })
	key := storage.CacheKey("invalid-dataset")
	bad := cacheDataset(t)
	bad.RowCount = 99
	require.NoError(t, cache.Put(context.Background(), key, bad))

	_, ok := cache.Get(context.Background(), key)
	assert.False(t, ok)
}

func TestFilesCacheKeyIncludesFileIdentity(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "data.csv")
	require.NoError(t, os.WriteFile(path, []byte("a,b\n1,2\n"), 0o600))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	first := storage.FilesCacheKey([]storage.FileInput{{Path: path, Data: data}}, storage.CSVOptions{})

	require.NoError(t, os.Chtimes(path, time.Unix(1000, 0), time.Unix(1000, 0)))
	second := storage.FilesCacheKey([]storage.FileInput{{Path: path, Data: data}}, storage.CSVOptions{})
	assert.NotEqual(t, first, second)
}

func TestCacheBusyLockSkipsWithoutDeleting(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cache := storage.NewCache(dir, time.Hour, 1<<20)
	key := storage.CacheKey("busy")
	require.NoError(t, cache.Put(context.Background(), key, cacheDataset(t)))
	require.NoError(t, cache.Close())

	holder, err := sql.Open("sqlite", filepath.Join(dir, "cache.db"))
	require.NoError(t, err)
	defer func() { _ = holder.Close() }()
	_, err = holder.Exec("PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	_, err = holder.Exec("BEGIN IMMEDIATE")
	require.NoError(t, err)

	contended := storage.NewCache(dir, time.Hour, 1<<20)
	t.Cleanup(func() { _ = contended.Close() })
	start := time.Now()
	_, _ = contended.Get(context.Background(), key)
	_ = contended.Put(context.Background(), key, cacheDataset(t))
	assert.Less(t, time.Since(start), 3*time.Second, "a busy cache must not stall the frame")
	_, err = os.Stat(filepath.Join(dir, "cache.db"))
	assert.NoError(t, err, "a busy cache must not be deleted")

	_, err = holder.Exec("ROLLBACK")
	require.NoError(t, err)
	require.NoError(t, contended.Put(context.Background(), key, cacheDataset(t)))
	_, ok := contended.Get(context.Background(), key)
	assert.True(t, ok)
}
