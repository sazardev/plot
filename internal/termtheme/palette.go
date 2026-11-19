package termtheme

import (
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Palette is the terminal's foreground, background and 16 ANSI colors. It is
// used for exported screenshots so they inherit the terminal's color scheme
// instead of a hard-coded product palette.
type Palette struct {
	Foreground color.RGBA
	Background color.RGBA
	ANSI       [16]color.RGBA
}

// DefaultPalette is a conservative dark-terminal fallback for non-TTY runs and
// for terminals that do not answer OSC color queries.
func DefaultPalette() Palette {
	return Palette{
		Foreground: color.RGBA{R: 233, G: 237, B: 223, A: 255},
		Background: color.RGBA{R: 23, G: 24, B: 36, A: 255},
		ANSI: [16]color.RGBA{
			{R: 32, G: 36, B: 48, A: 255},
			{R: 255, G: 107, B: 107, A: 255},
			{R: 110, G: 231, B: 168, A: 255},
			{R: 230, G: 217, B: 106, A: 255},
			{R: 110, G: 168, B: 255, A: 255},
			{R: 211, G: 139, B: 255, A: 255},
			{R: 90, G: 215, B: 230, A: 255},
			{R: 242, G: 244, B: 233, A: 255},
			{R: 107, G: 114, B: 128, A: 255},
			{R: 255, G: 143, B: 143, A: 255},
			{R: 143, G: 245, B: 192, A: 255},
			{R: 247, G: 235, B: 138, A: 255},
			{R: 143, G: 192, B: 255, A: 255},
			{R: 229, G: 170, B: 255, A: 255},
			{R: 126, G: 234, B: 245, A: 255},
			{R: 255, G: 255, B: 255, A: 255},
		},
	}
}

// ResolveFont returns a font file for screenshots. An explicit file path or
// font family wins; otherwise the system monospace font is resolved through
// fontconfig. Terminals usually use that same family. An empty result means the
// exporter should use its embedded fallback.
func ResolveFont(override string) string {
	override = strings.TrimSpace(override)
	if override != "" {
		if info, err := os.Stat(override); err == nil && !info.IsDir() {
			return override
		}
		if path := fontconfigMatch(override); path != "" {
			return path
		}
	}
	return fontconfigMatch("monospace")
}

// FontSize parses the optional export font-size override. Terminal font sizes
// are not exposed through standard terminal queries, so the caller provides a
// default and users can override it with PL_EXPORT_FONT_SIZE.
func FontSize(value string, fallback float64) float64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	size, err := strconv.ParseFloat(value, 64)
	if err != nil || size < 6 || size > 96 {
		return fallback
	}
	return size
}

func fontconfigMatch(pattern string) string {
	binary, err := exec.LookPath("fc-match")
	if err != nil {
		return ""
	}
	output, err := exec.Command(binary, "-f", "%{file}\n", pattern).Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(output), "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ttf", ".otf":
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				return path
			}
		}
	}
	return ""
}
