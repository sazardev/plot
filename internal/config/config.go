package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config mirrors the TOML file. Durations and sizes stay strings so a bad
// value can be reported with its key rather than silently becoming zero.
type Config struct {
	Database Database `toml:"database"`
	Display  Display  `toml:"display"`
	Cache    Cache    `toml:"cache"`
}

type Database struct {
	Path      string `toml:"path"`
	Query     string `toml:"query"`
	QueryFile string `toml:"query_file"`
	Timeout   string `toml:"timeout"`
}

type Display struct {
	Color  string `toml:"color"`
	ASCII  bool   `toml:"ascii"`
	Width  int    `toml:"width"`
	Height int    `toml:"height"`
}

type Cache struct {
	Enabled *bool  `toml:"enabled"`
	Dir     string `toml:"dir"`
	TTL     string `toml:"ttl"`
	MaxSize string `toml:"max_size"`
}

// Loaded carries the parsed config and the directory used to resolve its
// relative paths.
type Loaded struct {
	Config
	Path    string
	BaseDir string
}

// DefaultPath follows os.UserConfigDir()/pl/config.toml.
func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pl", "config.toml")
}

// Load reads path. A missing default config is fine; a missing explicitly
// requested file is an error. Unknown keys are rejected.
func Load(path string, explicit bool) (*Loaded, error) {
	if path == "" {
		path = DefaultPath()
	}
	if path == "" {
		return &Loaded{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return &Loaded{Path: path}, nil
		}
		return nil, err
	}
	var cfg Config
	metadata, err := toml.Decode(string(data), &cfg)
	if err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	if undecoded := metadata.Undecoded(); len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return nil, fmt.Errorf("unknown key(s): %s", strings.Join(keys, ", "))
	}
	if strings.TrimSpace(cfg.Database.Query) != "" && strings.TrimSpace(cfg.Database.QueryFile) != "" {
		return nil, fmt.Errorf("database.query and database.query_file are mutually exclusive")
	}
	loaded := &Loaded{Config: cfg, Path: path, BaseDir: filepath.Dir(path)}
	loaded.resolvePaths()
	return loaded, nil
}

func (c *Loaded) resolvePaths() {
	c.Database.Path = c.resolve(c.Database.Path)
	c.Database.QueryFile = c.resolve(c.Database.QueryFile)
	c.Cache.Dir = c.resolve(c.Cache.Dir)
}

func (c *Loaded) resolve(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.BaseDir, path)
}

// Timeout parses the database timeout, with fallback for an absent value.
func (c *Loaded) Timeout(fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(c.Database.Timeout) == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(c.Database.Timeout)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("database.timeout must be a positive duration")
	}
	return duration, nil
}

// CacheTTL parses cache.ttl.
func (c *Loaded) CacheTTL(fallback time.Duration) (time.Duration, error) {
	if strings.TrimSpace(c.Cache.TTL) == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(c.Cache.TTL)
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("cache.ttl must be a positive duration")
	}
	return duration, nil
}

// CacheMaxSize parses cache.max_size (bytes, KiB, MiB or GiB suffix).
func (c *Loaded) CacheMaxSize(fallback int64) (int64, error) {
	value := strings.TrimSpace(c.Cache.MaxSize)
	if value == "" {
		return fallback, nil
	}
	size, err := ParseSize(value)
	if err != nil || size <= 0 {
		return 0, fmt.Errorf("cache.max_size must be a positive size")
	}
	return size, nil
}

// ParseSize accepts 1048576, 1024KiB, 64MiB or 1GiB.
func ParseSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("empty size")
	}
	multiplier := int64(1)
	upper := strings.ToUpper(value)
	switch {
	case strings.HasSuffix(upper, "KIB"):
		multiplier = 1 << 10
		value = value[:len(value)-3]
	case strings.HasSuffix(upper, "MIB"):
		multiplier = 1 << 20
		value = value[:len(value)-3]
	case strings.HasSuffix(upper, "GIB"):
		multiplier = 1 << 30
		value = value[:len(value)-3]
	case strings.HasSuffix(upper, "KB"):
		multiplier = 1000
		value = value[:len(value)-2]
	case strings.HasSuffix(upper, "MB"):
		multiplier = 1000 * 1000
		value = value[:len(value)-2]
	case strings.HasSuffix(upper, "GB"):
		multiplier = 1000 * 1000 * 1000
		value = value[:len(value)-2]
	}
	number := strings.TrimSpace(value)
	parsed, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return 0, err
	}
	return parsed * multiplier, nil
}
