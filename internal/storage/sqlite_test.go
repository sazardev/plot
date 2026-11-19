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

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/storage"
)

func createMetricsDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`create table metrics (
		day text not null,
		signups integer not null,
		rate real,
		note text,
		blob blob
	)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into metrics values
		('2024-01-01', 3, 1.5, 'first', x'0102'),
		('2024-01-02', 5, null, null, null),
		('2024-01-03', 4, 2.5, 'third', null)`)
	require.NoError(t, err)
	return path
}

func TestReadSQLiteReturnsColumnsAndRows(t *testing.T) {
	t.Parallel()

	path := createMetricsDB(t)
	ds, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
		Path:  path,
		Query: `select day, signups, rate, note, blob from metrics order by day`,
	})
	require.NoError(t, err)

	assert.Equal(t, domain.SourceDB, ds.SourceKind)
	assert.Equal(t, path, ds.SourceID)
	assert.Equal(t, 3, ds.RowCount)
	require.Len(t, ds.Columns, 5)
	assert.Equal(t, "day", ds.Columns[0].Name)
	assert.Equal(t, domain.CellText, ds.Rows[0][0].Kind)
	assert.Equal(t, domain.CellNumber, ds.Rows[0][1].Kind)
	assert.InDelta(t, 3, ds.Rows[0][1].Number, 0.0001)
	assert.Equal(t, domain.CellNumber, ds.Rows[0][2].Kind)
	assert.True(t, ds.Rows[1][2].IsNull())
	assert.True(t, ds.Rows[1][3].IsNull())
	assert.Equal(t, domain.CellBlob, ds.Rows[0][4].Kind)
	assert.Equal(t, "<blob 2 bytes>", ds.Rows[0][4].Text)
}

func TestReadSQLiteIsReadOnly(t *testing.T) {
	t.Parallel()

	path := createMetricsDB(t)
	_, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
		Path:  path,
		Query: `delete from metrics`,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSource)

	var dbErr *domain.DatabaseError
	require.ErrorAs(t, err, &dbErr)
	assert.Contains(t, dbErr.Error(), "readonly")
}

func TestReadSQLiteReportsDriverErrorsVerbatim(t *testing.T) {
	t.Parallel()

	path := createMetricsDB(t)
	_, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
		Path:  path,
		Query: `select * from missing_table`,
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSource)

	var dbErr *domain.DatabaseError
	require.ErrorAs(t, err, &dbErr)
	assert.Contains(t, dbErr.Error(), "no such table")
}

func TestReadSQLiteDisambiguatesDuplicateColumns(t *testing.T) {
	t.Parallel()

	path := createMetricsDB(t)
	ds, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
		Path:  path,
		Query: `select 1 as value, 2 as value`,
	})
	require.NoError(t, err)

	require.Len(t, ds.Columns, 2)
	assert.Equal(t, "value", ds.Columns[0].Name)
	assert.Equal(t, "value_2", ds.Columns[1].Name)
	require.NotEmpty(t, ds.Warnings)
	assert.Equal(t, "cli.warn.duplicate_header", ds.Warnings[0].ID)
}

func TestReadSQLiteQueryFile(t *testing.T) {
	t.Parallel()

	path := createMetricsDB(t)
	queryPath := filepath.Join(t.TempDir(), "query.sql")
	require.NoError(t, os.WriteFile(queryPath, []byte("select day, signups from metrics order by day"), 0o600))

	ds, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
		Path:      path,
		QueryFile: queryPath,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, ds.RowCount)
}

func TestReadSQLiteRejectsMissingSourceParameters(t *testing.T) {
	t.Parallel()

	_, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{Query: "select 1"})
	assert.ErrorIs(t, err, domain.ErrUsage)

	path := createMetricsDB(t)
	_, err = storage.ReadSQLite(context.Background(), storage.SQLiteOptions{Path: path})
	assert.ErrorIs(t, err, domain.ErrUsage)
}

func TestReadSQLiteMissingFileIsDatabaseError(t *testing.T) {
	t.Parallel()

	_, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
		Path:  filepath.Join(t.TempDir(), "missing.db"),
		Query: "select 1",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSource)
}

func TestReadSQLiteTimeout(t *testing.T) {
	t.Parallel()

	path := createMetricsDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	_, err := storage.ReadSQLite(ctx, storage.SQLiteOptions{
		Path:  path,
		Query: "select day from metrics",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSource)
}
