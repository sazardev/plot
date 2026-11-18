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

func TestRunRejectsMultipleFilesForNow(t *testing.T) {
	t.Parallel()

	stream, _, errOut := testIO("", false, false)
	code := run([]string{"a.csv", "b.csv"}, stream)

	assert.Equal(t, exitUsage, code)
	assert.Contains(t, errOut.String(), "multiple files")
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
