package storage_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/storage"
)

func TestReadCSVCommaWithHeader(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("day,signups\nMon,3\nTue,5\n"), storage.CSVOptions{})
	require.NoError(t, err)

	require.Len(t, ds.Columns, 2)
	assert.Equal(t, "day", ds.Columns[0].Name)
	assert.Equal(t, "signups", ds.Columns[1].Name)
	assert.Equal(t, 2, ds.RowCount)
	assert.Equal(t, domain.CellText, ds.Rows[0][0].Kind)
	assert.Equal(t, "Mon", ds.Rows[0][0].Text)
}

func TestReadCSVDetectsDelimiters(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "comma", input: "a,b\n1,2\n"},
		{name: "tab", input: "a\tb\n1\t2\n"},
		{name: "semicolon", input: "a;b\n1;2\n"},
		{name: "pipe", input: "a|b\n1|2\n"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ds, err := storage.ReadCSV(strings.NewReader(tt.input), storage.CSVOptions{})
			require.NoError(t, err)
			require.Len(t, ds.Columns, 2)
			assert.Equal(t, "a", ds.Columns[0].Name)
			assert.Equal(t, "b", ds.Columns[1].Name)
		})
	}
}

func TestReadCSVHeaderlessNumericInput(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("1,2\n3,4\n"), storage.CSVOptions{})
	require.NoError(t, err)

	assert.Equal(t, "column_1", ds.Columns[0].Name)
	assert.Equal(t, "column_2", ds.Columns[1].Name)
	assert.Equal(t, 2, ds.RowCount)
}

func TestReadCSVForcedHeaderModes(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("1,2\n3,4\n"), storage.CSVOptions{HeaderMode: storage.HeaderPresent})
	require.NoError(t, err)
	assert.Equal(t, "1", ds.Columns[0].Name)
	assert.Equal(t, 1, ds.RowCount)

	ds, err = storage.ReadCSV(strings.NewReader("a,b\n1,2\n"), storage.CSVOptions{HeaderMode: storage.HeaderAbsent})
	require.NoError(t, err)
	assert.Equal(t, "column_1", ds.Columns[0].Name)
	assert.Equal(t, 2, ds.RowCount)
}

func TestReadCSVDuplicateHeadersAreDisambiguated(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("a,a\n1,2\n"), storage.CSVOptions{})
	require.NoError(t, err)

	assert.Equal(t, "a", ds.Columns[0].Name)
	assert.Equal(t, "a_2", ds.Columns[1].Name)
	require.Len(t, ds.Warnings, 1)
	assert.Equal(t, "cli.warn.duplicate_header", ds.Warnings[0].ID)
}

func TestReadCSVRaggedRowsPadAndTruncate(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("a,b\n1\n2,3,4\n"), storage.CSVOptions{})
	require.NoError(t, err)

	require.Len(t, ds.Rows, 2)
	assert.Equal(t, domain.CellText, ds.Rows[0][0].Kind)
	assert.True(t, ds.Rows[0][1].IsNull())
	assert.Equal(t, "2", ds.Rows[1][0].Text)
	assert.Equal(t, "3", ds.Rows[1][1].Text)
	require.Len(t, ds.Warnings, 1)
	assert.Equal(t, "cli.warn.ragged_row", ds.Warnings[0].ID)
	assert.Equal(t, "1", ds.Warnings[0].Data["Line"])
}

func TestReadCSVStripsBOMAndHandlesQuotes(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("\ufeffa,b\n\"x,y\",2\n"), storage.CSVOptions{})
	require.NoError(t, err)

	assert.Equal(t, "a", ds.Columns[0].Name)
	assert.Equal(t, "x,y", ds.Rows[0][0].Text)
}

func TestReadCSVHeaderOnly(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("a,b\n"), storage.CSVOptions{})
	require.NoError(t, err)
	assert.Equal(t, 0, ds.RowCount)
}

func TestReadCSVEmptyInput(t *testing.T) {
	t.Parallel()

	_, err := storage.ReadCSV(strings.NewReader("  \n"), storage.CSVOptions{})
	assert.ErrorIs(t, err, domain.ErrNoData)
}

func TestReadCSVInputTooLarge(t *testing.T) {
	t.Parallel()

	_, err := storage.ReadCSV(strings.NewReader("a,b\n1,2\n"), storage.CSVOptions{MaxBytes: 3})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrInputTooLarge)
}

func TestReadFileMissingIsASourceError(t *testing.T) {
	t.Parallel()

	_, err := storage.ReadFile("/does/not/exist.csv", storage.CSVOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSource)

	var sourceErr *domain.SourceError
	require.ErrorAs(t, err, &sourceErr)
	assert.Equal(t, "/does/not/exist.csv", sourceErr.Path)
}

func TestReadCSVExplicitDelimiter(t *testing.T) {
	t.Parallel()

	ds, err := storage.ReadCSV(strings.NewReader("a;b\n1;2\n"), storage.CSVOptions{Delimiter: ';'})
	require.NoError(t, err)
	require.Len(t, ds.Columns, 2)
}

func TestReadCSVInvalidQuotingIsAParseError(t *testing.T) {
	t.Parallel()

	_, err := storage.ReadCSV(strings.NewReader("a,b\n\"unterminated,2\n"), storage.CSVOptions{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, domain.ErrInvalidData))
}

func TestReadFilesConcatenatesMatchingSchemas(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "one.csv")
	second := filepath.Join(dir, "two.csv")
	require.NoError(t, os.WriteFile(first, []byte("month,revenue\nJan,10\nFeb,12\n"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("month,revenue\nMar,14\n"), 0o600))

	ds, err := storage.ReadFiles([]string{first, second}, storage.CSVOptions{})
	require.NoError(t, err)

	assert.Equal(t, 3, ds.RowCount)
	assert.Equal(t, 2, ds.SourceCount)
	assert.Equal(t, first, ds.SourceID)
	assert.Equal(t, "Jan", ds.Rows[0][0].Text)
	assert.Equal(t, "Mar", ds.Rows[2][0].Text)
}

func TestReadFilesRejectsSchemaMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "one.csv")
	second := filepath.Join(dir, "two.csv")
	require.NoError(t, os.WriteFile(first, []byte("month,revenue\nJan,10\n"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("month,cost\nFeb,12\n"), 0o600))

	_, err := storage.ReadFiles([]string{first, second}, storage.CSVOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSource)
	assert.Contains(t, err.Error(), "expected column")
}

func TestReadFilesMissingFileIsSourceError(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "one.csv")
	require.NoError(t, os.WriteFile(first, []byte("month,revenue\nJan,10\n"), 0o600))

	_, err := storage.ReadFiles([]string{first, filepath.Join(dir, "missing.csv")}, storage.CSVOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSource)
}
