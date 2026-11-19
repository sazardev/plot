package main

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/sazardev/plot/internal/storage"
)

// setupLogging sends debug output to os.UserCacheDir()/pl/debug.log when DEBUG
// is set. Without DEBUG every log record is discarded: stdout and stderr
// belong to the chart and user-facing errors (SPEC §9.5, NFR-7).
func setupLogging(stream appIO) func() {
	if strings.TrimSpace(stream.Env["DEBUG"]) == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return func() {}
	}
	dir := storage.DefaultCacheDir()
	if dir == "" {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return func() {}
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return func() {}
	}
	_ = os.Chmod(dir, 0o700)
	path := filepath.Join(dir, "debug.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
		return func() {}
	}
	_ = file.Chmod(0o600)
	slog.SetDefault(slog.New(slog.NewTextHandler(file, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))
	return func() { _ = file.Close() }
}
