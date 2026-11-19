package main

import (
	"os"
	"time"

	"github.com/sazardev/plot/internal/export"
	"github.com/sazardev/plot/internal/termtheme"
)

// resolveExportOptions inherits the terminal palette and the system monospace
// font when the process has a terminal. Non-TTY/test runs keep deterministic
// embedded defaults.
func resolveExportOptions(stream appIO) export.Options {
	options := export.DefaultOptions()
	if !stream.InTTY && !stream.OutTTY {
		return options
	}
	options.FontPath = termtheme.ResolveFont(stream.Env["PL_EXPORT_FONT"])
	options.FontSize = termtheme.FontSize(stream.Env["PL_EXPORT_FONT_SIZE"], options.FontSize)
	if !stream.InTTY || !stream.OutTTY {
		return options
	}
	input, inputOK := stream.In.(*os.File)
	output, outputOK := stream.Out.(*os.File)
	if !inputOK || !outputOK {
		return options
	}
	if palette, ok := termtheme.Detect(input, output, 120*time.Millisecond); ok {
		options.Palette = palette
	}
	return options
}
