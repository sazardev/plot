package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var update = flag.Bool("update", false, "update golden files")

// AssertGolden compares got with a golden file. Run tests with -update to
// regenerate the file intentionally.
func AssertGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, got, 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), string(got))
}
