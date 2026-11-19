package main

import (
	"bytes"
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
		Env:    map[string]string{"LANG": "en_US.UTF-8", "TERM": "xterm-256color"},
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
		{name: "pie", args: []string{"--type", "pie"}, want: "not implemented yet"},
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
	for _, flag := range []string{"--label", "--columns", "--rename", "--sort", "--filter"} {
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
