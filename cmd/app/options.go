package main

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/storage"
)

func resolveLocale(env map[string]string) string {
	for _, key := range []string{"PL_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
		value := strings.TrimSpace(env[key])
		if value == "" {
			continue
		}
		if i := strings.IndexAny(value, ".@"); i >= 0 {
			value = value[:i]
		}
		value = strings.ReplaceAll(value, "_", "-")
		switch strings.ToLower(value) {
		case "c", "posix", "":
			return "en"
		default:
			return value
		}
	}
	return "en"
}

func localeIsUTF8(env map[string]string) bool {
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		value := strings.ToLower(strings.TrimSpace(env[key]))
		if value == "" {
			continue
		}
		return strings.Contains(value, "utf-8") || strings.Contains(value, "utf8")
	}
	return true
}

func resolveASCII(opts cliOptions, env map[string]string) bool {
	if opts.ASCII {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(env["TERM"]), "dumb") {
		return true
	}
	return !localeIsUTF8(env)
}

func resolveColor(value string, outTTY bool, env map[string]string) (domain.ColorMode, bool, *appError) {
	mode := domain.ColorMode(strings.ToLower(strings.TrimSpace(value)))
	if mode == "" {
		mode = domain.ColorAuto
	}
	switch mode {
	case domain.ColorAlways:
		return mode, true, nil
	case domain.ColorNever:
		return mode, false, nil
	case domain.ColorAuto:
		enabled := outTTY &&
			env["NO_COLOR"] == "" &&
			!strings.EqualFold(strings.TrimSpace(env["TERM"]), "dumb")
		return mode, enabled, nil
	default:
		return "", false, usageError("cli.error.invalid_color", map[string]any{"Value": value})
	}
}

func resolveDelimiter(value string) (rune, *appError) {
	if value == "" {
		return 0, nil
	}
	if value == "\t" {
		return '\t', nil
	}
	if value == " " {
		return ' ', nil
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return 0, nil
	case "comma":
		return ',', nil
	case "tab":
		return '\t', nil
	case "semicolon":
		return ';', nil
	case "pipe":
		return '|', nil
	case "\\t":
		return '\t', nil
	}
	runes := []rune(value)
	if len(runes) != 1 || !utf8.ValidString(value) {
		return 0, usageError("cli.error.invalid_delimiter", map[string]any{"Value": value})
	}
	delim := runes[0]
	switch delim {
	case 0, '\r', '\n', '"':
		return 0, usageError("cli.error.invalid_delimiter", map[string]any{"Value": value})
	default:
		return delim, nil
	}
}

func resolveHeaderMode(header, noHeader bool) (storage.HeaderMode, *appError) {
	if header && noHeader {
		return storage.HeaderAuto, usageError("cli.error.header_conflict", nil)
	}
	if header {
		return storage.HeaderPresent, nil
	}
	if noHeader {
		return storage.HeaderAbsent, nil
	}
	return storage.HeaderAuto, nil
}

func resolveSize(stream appIO, opts cliOptions) (int, int, *appError) {
	width, height := 80, 24
	if stream.OutTTY {
		if stream.Width > 0 {
			width = minInt(stream.Width, 100)
		}
		if stream.Height > 0 {
			height = minInt(stream.Height, 40)
		}
	}
	if opts.Width != 0 {
		if opts.Width < 20 || opts.Width > 500 {
			return 0, 0, usageError("cli.error.width", map[string]any{"Min": "20", "Max": "500"})
		}
		width = opts.Width
	}
	if opts.Height != 0 {
		if opts.Height < 8 || opts.Height > 200 {
			return 0, 0, usageError("cli.error.height", map[string]any{"Min": "8", "Max": "200"})
		}
		height = opts.Height
	}
	return width, height, nil
}

func maxInputBytes(env map[string]string) int64 {
	value := strings.TrimSpace(env["PL_MAX_INPUT_BYTES"])
	if value == "" {
		return 0
	}
	size, err := strconv.ParseInt(value, 10, 64)
	if err != nil || size <= 0 {
		return 0
	}
	return size
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
