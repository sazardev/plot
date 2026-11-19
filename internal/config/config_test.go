package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazardev/plot/internal/config"
)

func TestLoadResolvesPathsAndParsesValues(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(`
[database]
path = "metrics.db"
query = "select 1"
timeout = "5s"

[display]
color = "never"
ascii = true
width = 60
height = 20

[cache]
enabled = false
dir = "cache"
ttl = "24h"
max_size = "16MiB"
`), 0o600))

	cfg, err := config.Load(path, true)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "metrics.db"), cfg.Database.Path)
	assert.Equal(t, filepath.Join(dir, "cache"), cfg.Cache.Dir)
	assert.Equal(t, "never", cfg.Display.Color)
	assert.True(t, cfg.Display.ASCII)
	assert.Equal(t, 60, cfg.Display.Width)

	timeout, err := cfg.Timeout(time.Second)
	require.NoError(t, err)
	assert.Equal(t, 5*time.Second, timeout)

	ttl, err := cfg.CacheTTL(time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, ttl)

	size, err := cfg.CacheMaxSize(1 << 20)
	require.NoError(t, err)
	assert.Equal(t, int64(16<<20), size)
}

func TestMissingDefaultConfigIsOK(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join(t.TempDir(), "missing.toml"), false)
	require.NoError(t, err)
	assert.Empty(t, cfg.Database.Path)
}

func TestMissingExplicitConfigIsAnError(t *testing.T) {
	t.Parallel()

	_, err := config.Load(filepath.Join(t.TempDir(), "missing.toml"), true)
	require.Error(t, err)
}

func TestUnknownConfigKeyIsRejected(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte("[display]\ncolour = \"red\"\n"), 0o600))
	_, err := config.Load(path, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown key")
}

func TestParseSize(t *testing.T) {
	t.Parallel()

	tests := map[string]int64{
		"1024":  1024,
		"1KiB":  1 << 10,
		"64MiB": 64 << 20,
		"1GiB":  1 << 30,
	}
	for input, want := range tests {
		got, err := config.ParseSize(input)
		require.NoError(t, err, input)
		assert.Equal(t, want, got, input)
	}
}
