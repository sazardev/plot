package main

import (
	"bytes"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testIO(input string, inTTY, outTTY bool) (appIO, *bytes.Buffer, *bytes.Buffer) {
	out := &bytes.Buffer{}
	errOut := &bytes.Buffer{}
	return appIO{
		In:     strings.NewReader(input),
		Out:    out,
		Err:    errOut,
		InTTY:  inTTY,
		OutTTY: outTTY,
		Env:    map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm-256color", "PL_CONFIG": os.DevNull, "PL_CACHE_DISABLE": "1"},
		Width:  0,
		Height: 0,
	}, out, errOut
}

func TestRunRendersCSVFromStdin(t *testing.T) {
	t.Parallel()

	stream, out, errOut := testIO("day,signups\nMon,3\nTue,5\n", false, false)
	code := run(nil, stream)

	require.Equal(t, 0, code)
	require.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "stdin — bar")
	assert.Contains(t, out.String(), "Mon")
	assert.Contains(t, out.String(), "Tue")
	assert.NotContains(t, out.String(), "\x1b[")
}

func TestRunNoInputOnTTYIsUsageError(t *testing.T) {
	t.Parallel()

	stream, out, errOut := testIO("", true, true)
	code := run(nil, stream)

	assert.Equal(t, exitUsage, code)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "no input")
	assert.Contains(t, errOut.String(), "run 'pl --help'")
}

func TestRunEmptyStdinIsDataError(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", false, false)
	code := run(nil, stream)

	assert.Equal(t, exitData, code)
	assert.Contains(t, errOut.String(), "no data on stdin")
}

func TestRunMissingFileIsSourceError(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", false, false)
	code := run([]string{"/does/not/exist.csv"}, stream)

	assert.Equal(t, exitSource, code)
	assert.Contains(t, errOut.String(), "cannot read")
	assert.Contains(t, errOut.String(), "/does/not/exist.csv")
}

func TestRunReadsAFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sales.csv")
	require.NoError(t, os.WriteFile(path, []byte("month,revenue\nJan,10\nFeb,12\n"), 0o600))

	stream, out, errOut := testIO("", false, false)
	code := run([]string{path}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "sales.csv")
	assert.Contains(t, out.String(), "Jan")
}

func TestRunHelpDoesNotReadStdin(t *testing.T) {
	t.Parallel()

	stream, out, errOut := testIO("", true, true)
	stream.In = failingReader{t: t}
	code := run([]string{"--help"}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "Usage")
	assert.Contains(t, out.String(), "--type")
	assert.Contains(t, out.String(), "Exit codes")
}

func TestRunVersionDoesNotReadStdin(t *testing.T) {
	t.Parallel()

	stream, out, errOut := testIO("", true, true)
	stream.In = failingReader{t: t}
	code := run([]string{"--version"}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Equal(t, "pl dev\n", out.String())
}

func TestRunColorAlwaysEmitsANSI(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, false)
	code := run([]string{"--color=always"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "\x1b[")
}

func TestRunNoColorIsDefaultOffTTY(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, false)
	code := run(nil, stream)

	require.Equal(t, 0, code)
	assert.NotContains(t, out.String(), "\x1b[")
}

func TestRunASCII(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, false)
	code := run([]string{"--ascii"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "stdin - line")
	assert.NotContains(t, out.String(), "●")
	assert.NotContains(t, out.String(), "█")
}

func TestRunTitleOverride(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, false)
	code := run([]string{"--title", "My chart"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "My chart")
}

func TestRunInvalidFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "width", args: []string{"--width", "10"}, want: "--width must be between"},
		{name: "height", args: []string{"--height", "2"}, want: "--height must be between"},
		{name: "color", args: []string{"--color", "rainbow"}, want: "invalid color mode"},
		{name: "delimiter", args: []string{"--delimiter", ";;"}, want: "invalid delimiter"},
		{name: "header conflict", args: []string{"--header", "--no-header"}, want: "use only one"},
		{name: "unknown type", args: []string{"--type", "scatter"}, want: "unknown chart type"},
		{name: "pie needs slices", args: []string{"--type", "pie"}, want: "at least two non-zero slices"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, _, errOut := testIO("a,b\n1,2\n", false, false)
			code := run(tt.args, stream)
			assert.Equal(t, exitUsage, code)
			assert.Contains(t, errOut.String(), tt.want)
		})
	}
}

func TestRunHeaderOnlyIsDataError(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("a,b\n", false, false)
	code := run(nil, stream)

	assert.Equal(t, exitData, code)
	assert.Contains(t, errOut.String(), "no data rows")
}

func TestRunRaggedRowWarnsOnStderr(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("a,b\n1\n2,3\n", false, false)
	code := run(nil, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, errOut.String(), "record 1")
	assert.Contains(t, errOut.String(), "expected 2 fields")
}

func TestRunDashRequiresPipedStdin(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", true, true)
	code := run([]string{"-"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "no input")
}

type failingReader struct {
	t *testing.T
}

func (r failingReader) Read([]byte) (int, error) {
	r.t.Fatal("stdin must not be read")
	return 0, io.EOF
}

func TestRunNoColorEnvironment(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, true)
	stream.Env["NO_COLOR"] = "1"
	code := run(nil, stream)

	require.Equal(t, 0, code)
	assert.NotContains(t, out.String(), "\x1b[")
}

func TestRunDumbTerminalUsesASCIINoColor(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, true)
	stream.Env["TERM"] = "dumb"
	code := run(nil, stream)

	require.Equal(t, 0, code)
	assert.NotContains(t, out.String(), "\x1b[")
	assert.Contains(t, out.String(), "stdin - line")
}

func TestRunNonUTF8LocaleUsesASCII(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, false)
	stream.Env["LANG"] = "C"
	code := run(nil, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "stdin - line")
	assert.NotContains(t, out.String(), "·")
}

func TestRunExplicitColorWinsNoColor(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, true)
	stream.Env["NO_COLOR"] = "1"
	code := run([]string{"--color=always"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "\x1b[")
}

func TestRunDashReadsPipedStdin(t *testing.T) {
	t.Parallel()

	stream, out, errOut := testIO("a,b\n1,2\n3,4\n", false, false)
	code := run([]string{"-"}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "stdin")
}

func TestRunTTYSizeIsUsed(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, true)
	stream.Width = 40
	stream.Height = 12
	stream.Env["NO_COLOR"] = "1"
	code := run(nil, stream)

	require.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	assert.LessOrEqual(t, len(lines), 12)
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(line)), 40, "line: %q", line)
	}
}

func TestRunDefaultNonTTYSize(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("a,b\n1,2\n3,4\n", false, false)
	code := run(nil, stream)

	require.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	assert.LessOrEqual(t, len(lines), 24)
	for _, line := range lines {
		assert.LessOrEqual(t, len([]rune(line)), 80, "line: %q", line)
	}
}

func TestRunColumnsAndLabel(t *testing.T) {
	t.Parallel()

	input := "month,revenue,cost\nJan,10,4\nFeb,12,5\n"
	stream, out, errOut := testIO(input, false, false)
	code := run([]string{"--label", "month", "--columns", "cost,revenue"}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	text := out.String()
	assert.Contains(t, text, "cost")
	assert.Contains(t, text, "revenue")
	assert.Less(t, strings.Index(text, "cost"), strings.Index(text, "revenue"))
}

func TestRunRenameIsDisplayed(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,10\nFeb,12\n"
	stream, out, _ := testIO(input, false, false)
	code := run([]string{"--label", "month", "--columns", "revenue", "--rename", "revenue=Revenue"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "Revenue")
	assert.NotContains(t, out.String(), " revenue")
}

func TestRunFilterAndSort(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,10\nFeb,30\nMar,20\n"
	stream, out, errOut := testIO(input, false, false)
	code := run([]string{"--label", "month", "--columns", "revenue", "--filter", "revenue>=20", "--sort", "revenue:desc"}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	text := out.String()
	assert.Contains(t, text, "Feb")
	assert.Contains(t, text, "Mar")
	assert.NotContains(t, text, "Jan")
}

func TestRunFilterNoRows(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,10\nFeb,30\n"
	stream, out, errOut := testIO(input, false, false)
	code := run([]string{"--filter", "revenue>100"}, stream)

	assert.Equal(t, exitData, code)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "no rows after filter")
}

func TestRunMissingColumnListsAvailableColumns(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,10\n"
	stream, _, errOut := testIO(input, false, false)
	code := run([]string{"--columns", "missing"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "column missing not found")
	assert.Contains(t, errOut.String(), "month, revenue")
}

func TestRunInvalidFilter(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,10\n"
	stream, _, errOut := testIO(input, false, false)
	code := run([]string{"--filter", "revenue~10"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "invalid filter")
}

func TestRunColumnsRejectsNonNumericColumn(t *testing.T) {
	t.Parallel()

	input := "region,month,revenue\nEU,Jan,10\nEU,Feb,12\n"
	stream, _, errOut := testIO(input, false, false)
	code := run([]string{"--columns", "month"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "not numeric")
}

func TestRunLabelAndColumnsConflict(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,10\n"
	stream, _, errOut := testIO(input, false, false)
	code := run([]string{"--label", "month", "--columns", "month,revenue"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "is the label")
}

func TestRunDuplicateRename(t *testing.T) {
	t.Parallel()

	input := "month,revenue,cost\nJan,10,4\nFeb,12,5\n"
	stream, _, errOut := testIO(input, false, false)
	code := run([]string{"--rename", "revenue=Total", "--rename", "cost=Total"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "duplicate display label")
}

func TestRunHelpListsTransformFlags(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("", true, true)
	code := run([]string{"--help"}, stream)

	require.Equal(t, 0, code)
	for _, flag := range []string{"--label", "--columns", "--rename", "--sort", "--filter", "--db", "--query", "--timeout", "--tui", "--no-motion", "--no-cache", "--cache-dir", "--config"} {
		assert.Contains(t, out.String(), flag)
	}
}

func TestRunMultipleFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "one.csv")
	second := filepath.Join(dir, "two.csv")
	require.NoError(t, os.WriteFile(first, []byte("month,revenue\nJan,10\nFeb,12\n"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("month,revenue\nMar,14\n"), 0o600))

	stream, out, errOut := testIO("", false, false)
	code := run([]string{first, second}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "one.csv (+1 more)")
	assert.Contains(t, out.String(), "Mar")
}

func TestRunMultipleFilesSchemaMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "one.csv")
	second := filepath.Join(dir, "two.csv")
	require.NoError(t, os.WriteFile(first, []byte("month,revenue\nJan,10\n"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("month,cost\nFeb,12\n"), 0o600))

	stream, _, errOut := testIO("", false, false)
	code := run([]string{first, second}, stream)

	assert.Equal(t, exitSource, code)
	assert.Contains(t, errOut.String(), "cannot read")
	assert.Contains(t, errOut.String(), "expected column")
}

func TestRunStdinCannotBeCombinedWithMultipleFiles(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("a,b\n1,2\n", false, false)
	code := run([]string{"-", "two.csv"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "stdin cannot be combined")
}

func TestRunInvalidSortDirection(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("month,revenue\nJan,10\n", false, false)
	code := run([]string{"--sort", "revenue:sideways"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "invalid sort")
}

func TestRunInvalidRename(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("month,revenue\nJan,10\n", false, false)
	code := run([]string{"--rename", "revenue"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "invalid rename")
}

func TestRunEmptySelectionFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "label", args: []string{"--label", ""}, want: "--label needs"},
		{name: "columns", args: []string{"--columns", ""}, want: "--columns needs"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, _, errOut := testIO("month,revenue\nJan,10\n", false, false)
			code := run(tt.args, stream)
			assert.Equal(t, exitUsage, code)
			assert.Contains(t, errOut.String(), tt.want)
		})
	}
}

func TestRunTooManyExplicitSeries(t *testing.T) {
	t.Parallel()

	header := "month"
	row := "Jan"
	for i := 0; i < 9; i++ {
		header += ",v" + string(rune('a'+i))
		row += ",1"
	}
	stream, _, errOut := testIO(header+"\n"+row+"\n", false, false)
	code := run([]string{"--columns", strings.Join(strings.Split(header, ",")[1:], ",")}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "maximum is 8")
}

func TestRunPieAndDonut(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,30\nFeb,20\nMar,50\n"
	for _, chartType := range []string{"pie", "donut"} {
		chartType := chartType
		t.Run(chartType, func(t *testing.T) {
			t.Parallel()
			stream, out, errOut := testIO(input, false, false)
			code := run([]string{"--type", chartType}, stream)
			require.Equal(t, 0, code)
			assert.Empty(t, errOut.String())
			assert.Contains(t, out.String(), "Jan")
			assert.Contains(t, out.String(), "30.0%")
		})
	}
}

func TestRunPieRejectsNegativeValues(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,30\nFeb,-20\n"
	stream, out, errOut := testIO(input, false, false)
	code := run([]string{"--type", "pie"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "non-negative")
}

func TestRunPieDropsZeroSlicesWithWarning(t *testing.T) {
	t.Parallel()

	input := "month,revenue\nJan,30\nFeb,0\nMar,70\n"
	stream, out, errOut := testIO(input, false, false)
	code := run([]string{"--type", "pie"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "Jan")
	assert.Contains(t, out.String(), "Mar")
	assert.NotContains(t, out.String(), "Feb")
	assert.Contains(t, errOut.String(), "zero-valued slice")
}

func createSQLiteFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`create table metrics (day text, signups integer)`)
	require.NoError(t, err)
	_, err = db.Exec(`insert into metrics values ('2024-01-01', 3), ('2024-01-02', 5), ('2024-01-03', 4)`)
	require.NoError(t, err)
	return path
}

func TestRunSQLiteSource(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	stream, out, errOut := testIO("ignored stdin", false, false)
	stream.Env["LANG"] = "C.UTF-8"
	code := run([]string{"--db", path, "--query", "select day, signups from metrics order by day"}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "metrics.db")
	assert.Contains(t, out.String(), "2024-01-01")
	assert.Contains(t, out.String(), "signups")
}

func TestRunSQLiteQueryFile(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	queryPath := filepath.Join(t.TempDir(), "query.sql")
	require.NoError(t, os.WriteFile(queryPath, []byte("select day, signups from metrics"), 0o600))

	stream, out, errOut := testIO("", false, false)
	code := run([]string{"--db", path, "--query-file", queryPath}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}

func TestRunSQLiteMissingQueryOrDatabase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "query without db", args: []string{"--query", "select 1"}, want: "no database configured"},
		{name: "db without query", args: []string{"--db", "metrics.db"}, want: "no query"},
		{name: "both queries", args: []string{"--db", "metrics.db", "--query", "select 1", "--query-file", "q.sql"}, want: "only one"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			stream, _, errOut := testIO("", false, false)
			code := run(tt.args, stream)
			assert.Equal(t, exitUsage, code)
			assert.Contains(t, errOut.String(), tt.want)
		})
	}
}

func TestRunSQLiteSourceConflictWithFile(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	stream, _, errOut := testIO("", false, false)
	code := run([]string{"--db", path, "--query", "select 1", "data.csv"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "either a database source or file")
}

func TestRunSQLiteDriverErrorIsVerbatim(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	stream, out, errOut := testIO("", false, false)
	code := run([]string{"--db", path, "--query", "select * from missing_table"}, stream)

	assert.Equal(t, exitSource, code)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "no such table: missing_table")
}

func TestRunSQLiteReadOnly(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	stream, _, errOut := testIO("", false, false)
	code := run([]string{"--db", path, "--query", "delete from metrics"}, stream)

	assert.Equal(t, exitSource, code)
	assert.Contains(t, errOut.String(), "readonly")
}

func TestRunSQLiteEnvironmentSource(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	stream, out, errOut := testIO("", false, false)
	stream.Env["PL_DB"] = path
	stream.Env["PL_QUERY"] = "select day, signups from metrics"
	code := run(nil, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}

func TestRunSQLiteResultWithNoRows(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	stream, _, errOut := testIO("", false, false)
	code := run([]string{"--db", path, "--query", "select day, signups from metrics where 0"}, stream)

	assert.Equal(t, exitData, code)
	assert.Contains(t, errOut.String(), "no data rows")
}

func TestRunPositionalFileWinsOverDatabaseEnvironment(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "sales.csv")
	require.NoError(t, os.WriteFile(csvPath, []byte("month,revenue\nJan,10\nFeb,20\n"), 0o600))

	stream, out, errOut := testIO("", false, false)
	stream.Env["PL_DB"] = path
	stream.Env["PL_QUERY"] = "select day, signups from metrics"
	code := run([]string{csvPath}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "sales.csv")
	assert.Contains(t, out.String(), "Jan")
}

func TestRunExplicitDatabaseQueryConflictsWithFile(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", false, false)
	stream.Env["PL_DB"] = createSQLiteFixture(t)
	code := run([]string{"--query", "select 1", "data.csv"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "either a database source or file")
}

func TestRunTUIFallsBackWhenNotATTY(t *testing.T) {
	t.Parallel()

	stream, out, errOut := testIO("month,revenue\nJan,10\nFeb,20\n", false, false)
	code := run([]string{"--tui"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "Jan")
	assert.Contains(t, errOut.String(), "TUI needs a terminal")
}

func TestRunTUIRejectsCLISizeFlags(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", true, true)
	code := run([]string{"--tui", "--width", "40"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "--tui uses the terminal size")
}

func TestRunTUIRequiresASource(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", true, true)
	code := run([]string{"--tui"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "no input")
}

func TestRunTUIRejectsStdinDash(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", true, true)
	code := run([]string{"--tui", "-"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "no input")
}

func TestRunHelpListsTUIFlags(t *testing.T) {
	t.Parallel()

	stream, out, _ := testIO("", true, true)
	code := run([]string{"--help"}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "--tui")
	assert.Contains(t, out.String(), "--no-motion")
}

func TestRunTUIFallsBackWhenStdoutRedirected(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.csv")
	require.NoError(t, os.WriteFile(path, []byte("month,revenue\nJan,10\nFeb,20\n"), 0o600))

	stream, out, errOut := testIO("", true, false)
	code := run([]string{"--tui", path}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "Jan")
	assert.Contains(t, errOut.String(), "TUI needs a terminal")
}

func TestRunSanitizesControlCharactersInLabels(t *testing.T) {
	t.Parallel()

	input := "month,revenue\n\"Ja\x1b[31mn\",10\nFeb,20\n"
	stream, out, errOut := testIO(input, false, false)
	code := run(nil, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.NotContains(t, out.String(), "\x1b")
	assert.Contains(t, out.String(), "Ja [31mn")
}

func TestRunMultipleFilesWithTransforms(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := filepath.Join(dir, "one.csv")
	second := filepath.Join(dir, "two.csv")
	require.NoError(t, os.WriteFile(first, []byte("month,revenue,cost\nJan,10,4\nFeb,30,8\n"), 0o600))
	require.NoError(t, os.WriteFile(second, []byte("month,revenue,cost\nMar,20,2\n"), 0o600))

	stream, out, errOut := testIO("", false, false)
	code := run([]string{
		first, second,
		"--label", "month",
		"--columns", "revenue",
		"--filter", "revenue>=20",
		"--sort", "revenue:desc",
		"--rename", "revenue=Revenue",
	}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "one.csv (+1 more)")
	assert.Contains(t, out.String(), "Revenue")
	assert.Contains(t, out.String(), "Feb")
	assert.Contains(t, out.String(), "Mar")
	assert.NotContains(t, out.String(), "Jan")
}

func TestRunSQLiteBLOBandFilter(t *testing.T) {
	t.Parallel()

	path := createSQLiteFixture(t)
	stream, out, errOut := testIO("", false, false)
	code := run([]string{
		"--db", path,
		"--query", "select x'0102' as payload, day, signups from metrics",
		"--label", "payload",
		"--columns", "signups",
		"--filter", "signups>=4",
	}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "<blob 2 byt")
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

func TestRunUsesConfigDatabase(t *testing.T) {
	t.Parallel()

	dbPath := createSQLiteFixture(t)
	configPath := writeConfig(t, "[database]\npath = \""+dbPath+"\"\nquery = \"select day, signups from metrics\"\n")
	stream, out, errOut := testIO("", false, false)
	code := run([]string{"--config", configPath}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}

func TestRunFlagsOverrideConfig(t *testing.T) {
	t.Parallel()

	configPath := writeConfig(t, "[database]\npath = \"missing.db\"\nquery = \"select * from missing\"\n")
	dbPath := createSQLiteFixture(t)
	stream, out, errOut := testIO("", false, false)
	code := run([]string{
		"--config", configPath,
		"--db", dbPath,
		"--query", "select day, signups from metrics",
	}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}

func TestRunEnvironmentOverridesConfig(t *testing.T) {
	t.Parallel()

	configPath := writeConfig(t, "[database]\npath = \"missing.db\"\nquery = \"select * from missing\"\n")
	dbPath := createSQLiteFixture(t)
	stream, out, errOut := testIO("", false, false)
	stream.Env["PL_DB"] = dbPath
	stream.Env["PL_QUERY"] = "select day, signups from metrics"
	code := run([]string{"--config", configPath}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}

func TestRunMissingConfigIsUsageError(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", false, false)
	code := run([]string{"--config", filepath.Join(t.TempDir(), "missing.toml")}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "invalid config")
}

func TestRunUnknownConfigKeyIsUsageError(t *testing.T) {
	t.Parallel()

	configPath := writeConfig(t, "[display]\ncolour = \"red\"\n")
	stream, _, errOut := testIO("", false, false)
	code := run([]string{"--config", configPath}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "unknown key")
}

func TestRunConfigDisplayAndRelativeDatabasePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "metrics.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`create table metrics(day text, signups integer); insert into metrics values ('Mon',3),('Tue',5)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(`
[database]
path = "metrics.db"
query = "select day, signups from metrics"
[display]
color = "never"
ascii = true
`), 0o600))

	stream, out, errOut := testIO("", false, false)
	code := run([]string{"--config", configPath}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "Mon")
	assert.NotContains(t, out.String(), "\x1b[")
	assert.Contains(t, out.String(), "metrics.db - bar")
}

func TestRunCacheStdinIsStable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := "month,revenue\nJan,10\nFeb,20\n"
	runOnce := func() string {
		stream, out, errOut := testIO(input, false, false)
		delete(stream.Env, "PL_CACHE_DISABLE")
		code := run([]string{"--cache-dir", dir}, stream)
		require.Equal(t, 0, code)
		assert.Empty(t, errOut.String())
		return out.String()
	}
	first := runOnce()
	second := runOnce()
	assert.Equal(t, first, second)
	_, err := os.Stat(filepath.Join(dir, "cache.db"))
	require.NoError(t, err)
}

func TestRunCacheHitsDatabaseResult(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := createSQLiteFixture(t)
	query := "select day, abs(random() % 100) as value from metrics"
	runOnce := func() string {
		stream, out, errOut := testIO("", false, false)
		delete(stream.Env, "PL_CACHE_DISABLE")
		code := run([]string{"--cache-dir", dir, "--db", path, "--query", query}, stream)
		require.Equal(t, 0, code)
		assert.Empty(t, errOut.String())
		return out.String()
	}
	first := runOnce()
	second := runOnce()
	assert.Equal(t, first, second)
}

func TestRunNoCacheDoesNotCreateDatabase(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stream, _, _ := testIO("month,revenue\nJan,10\n", false, false)
	code := run([]string{"--cache-dir", dir, "--no-cache"}, stream)
	require.Equal(t, 0, code)
	_, err := os.Stat(filepath.Join(dir, "cache.db"))
	assert.True(t, os.IsNotExist(err))
}

func TestRunUnwritableCacheStillRenders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blocker := filepath.Join(dir, "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))
	stream, out, errOut := testIO("month,revenue\nJan,10\n", false, false)
	delete(stream.Env, "PL_CACHE_DISABLE")
	code := run([]string{"--cache-dir", blocker}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "Jan")
}

func TestRunConfigCanDisableCache(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	configPath := writeConfig(t, "[cache]\nenabled = false\ndir = \""+dir+"\"\n")
	stream, _, _ := testIO("month,revenue\nJan,10\n", false, false)
	delete(stream.Env, "PL_CACHE_DISABLE")
	code := run([]string{"--config", configPath}, stream)
	require.Equal(t, 0, code)
	_, err := os.Stat(filepath.Join(dir, "cache.db"))
	assert.True(t, os.IsNotExist(err))
}

func TestRunDebugLogWritesOnlyToFile(t *testing.T) {
	cacheHome := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cacheHome)

	cacheDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(cacheDir, "cache.db"), []byte("not a database"), 0o600))

	stream, out, errOut := testIO("month,revenue\nJan,10\n", false, false)
	delete(stream.Env, "PL_CACHE_DISABLE")
	stream.Env["DEBUG"] = "1"
	code := run([]string{"--cache-dir", cacheDir}, stream)

	require.Equal(t, 0, code)
	assert.Contains(t, out.String(), "Jan")
	assert.Empty(t, errOut.String())

	debugPath := filepath.Join(cacheHome, "pl", "debug.log")
	info, err := os.Stat(debugPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	content, err := os.ReadFile(debugPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), "cache read unavailable")
}

func TestRunEnvironmentCacheDirectoryAndDisable(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stream, out, errOut := testIO("month,revenue\nJan,10\n", false, false)
	delete(stream.Env, "PL_CACHE_DISABLE")
	stream.Env["PL_CACHE_DIR"] = dir
	code := run(nil, stream)
	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "Jan")
	_, err := os.Stat(filepath.Join(dir, "cache.db"))
	require.NoError(t, err)

	disabledDir := t.TempDir()
	stream, _, _ = testIO("month,revenue\nJan,10\n", false, false)
	stream.Env["PL_CACHE_DIR"] = disabledDir
	stream.Env["PL_CACHE_DISABLE"] = "1"
	code = run(nil, stream)
	require.Equal(t, 0, code)
	_, err = os.Stat(filepath.Join(disabledDir, "cache.db"))
	assert.True(t, os.IsNotExist(err))
}

func TestRunConfigFlagWinsOverEnvironment(t *testing.T) {
	t.Parallel()

	dbPath := createSQLiteFixture(t)
	configPath := writeConfig(t, "[database]\npath = \""+dbPath+"\"\nquery = \"select day, signups from metrics\"\n")
	stream, out, errOut := testIO("", false, false)
	stream.Env["PL_CONFIG"] = filepath.Join(t.TempDir(), "missing.toml")
	code := run([]string{"--config", configPath}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}

func TestRunCacheStoresUntransformedDataset(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input := "month,revenue\nJan,10\nFeb,20\nMar,30\n"

	stream, _, errOut := testIO(input, false, false)
	delete(stream.Env, "PL_CACHE_DISABLE")
	code := run([]string{"--cache-dir", dir, "--filter", "revenue>=20"}, stream)
	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())

	stream, out, errOut := testIO(input, false, false)
	delete(stream.Env, "PL_CACHE_DISABLE")
	code = run([]string{"--cache-dir", dir}, stream)
	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "Jan", "the cache must not store a filtered dataset")
}

func TestRunQueryFlagOverridesConfigQueryFile(t *testing.T) {
	t.Parallel()

	dbPath := createSQLiteFixture(t)
	dir := t.TempDir()
	queryFile := filepath.Join(dir, "query.sql")
	require.NoError(t, os.WriteFile(queryFile, []byte("select * from missing"), 0o600))
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[database]\npath = \""+dbPath+"\"\nquery_file = \"query.sql\"\n"), 0o600))

	stream, out, errOut := testIO("", false, false)
	code := run([]string{"--config", configPath, "--query", "select day, signups from metrics"}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}

func TestRunEnvironmentQueryOverridesConfigQueryFile(t *testing.T) {
	t.Parallel()

	dbPath := createSQLiteFixture(t)
	dir := t.TempDir()
	queryFile := filepath.Join(dir, "query.sql")
	require.NoError(t, os.WriteFile(queryFile, []byte("select * from missing"), 0o600))
	configPath := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte("[database]\npath = \""+dbPath+"\"\nquery_file = \"query.sql\"\n"), 0o600))

	stream, out, errOut := testIO("", false, false)
	stream.Env["PL_QUERY"] = "select day, signups from metrics"
	code := run([]string{"--config", configPath}, stream)

	require.Equal(t, 0, code)
	assert.Empty(t, errOut.String())
	assert.Contains(t, out.String(), "2024-01-01")
}
