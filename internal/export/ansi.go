package export

import (
	"image/color"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"

	"github.com/sazardev/plot/internal/termtheme"
)

const (
	ansiBlack = iota
	ansiRed
	ansiGreen
	ansiYellow
	ansiBlue
	ansiMagenta
	ansiCyan
	ansiWhite
	ansiBrightBlack
	ansiBrightRed
	ansiBrightGreen
	ansiBrightYellow
	ansiBrightBlue
	ansiBrightMagenta
	ansiBrightCyan
	ansiBrightWhite
)

type styledRune struct {
	r     rune
	fg    color.RGBA
	width int
}

type ansiState struct {
	fg color.RGBA
}

// parseLine converts one rendered line into styled terminal cells. It keeps
// spaces so column positions survive, and understands the SGR subset emitted
// by pl (16-color, 256-color and truecolor foregrounds).
func parseLine(line string, palette termtheme.Palette) []styledRune {
	state := ansiState{fg: palette.Foreground}
	cells := make([]styledRune, 0, len(line))
	for index := 0; index < len(line); {
		if line[index] == 0x1b && index+1 < len(line) && line[index+1] == '[' {
			end := index + 2
			for end < len(line) && (line[end] < 0x40 || line[end] > 0x7e) {
				end++
			}
			if end >= len(line) {
				break
			}
			if line[end] == 'm' {
				applySGR(&state, line[index+2:end], palette)
			}
			index = end + 1
			continue
		}
		r, size := utf8.DecodeRuneInString(line[index:])
		if r == utf8.RuneError && size == 1 {
			r = '?'
		}
		if r == '\r' || r == '\n' {
			index += size
			continue
		}
		width := runewidth.RuneWidth(r)
		if width < 1 {
			width = 1
		}
		cells = append(cells, styledRune{r: r, fg: state.fg, width: width})
		index += size
	}
	return cells
}

func applySGR(state *ansiState, params string, palette termtheme.Palette) {
	if params == "" {
		params = "0"
	}
	values := strings.Split(params, ";")
	for index := 0; index < len(values); index++ {
		value, err := strconv.Atoi(values[index])
		if err != nil {
			continue
		}
		switch {
		case value == 0:
			state.fg = palette.Foreground
		case value == 39:
			state.fg = palette.Foreground
		case value >= 30 && value <= 37:
			state.fg = palette.ANSI[ansiBlack+value-30]
		case value >= 90 && value <= 97:
			state.fg = palette.ANSI[ansiBrightBlack+value-90]
		case value == 38 && index+1 < len(values):
			kind, err := strconv.Atoi(values[index+1])
			if err != nil {
				continue
			}
			switch kind {
			case 5:
				if index+2 < len(values) {
					if n, err := strconv.Atoi(values[index+2]); err == nil {
						state.fg = ansi256(n, palette)
						index += 2
					}
				}
			case 2:
				if index+4 < len(values) {
					r, errR := strconv.Atoi(values[index+2])
					g, errG := strconv.Atoi(values[index+3])
					b, errB := strconv.Atoi(values[index+4])
					if errR == nil && errG == nil && errB == nil {
						state.fg = color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
						index += 4
					}
				}
			}
		}
	}
}

func ansi256(n int, palette termtheme.Palette) color.RGBA {
	if n < 0 {
		return palette.Foreground
	}
	if n < 16 {
		return palette.ANSI[n]
	}
	if n >= 232 {
		level := uint8(8 + (n-232)*10)
		return color.RGBA{R: level, G: level, B: level, A: 255}
	}
	n -= 16
	levels := [6]uint8{0, 95, 135, 175, 215, 255}
	return color.RGBA{
		R: levels[(n/36)%6],
		G: levels[(n/6)%6],
		B: levels[n%6],
		A: 255,
	}
}
