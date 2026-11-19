package export

import (
	"image/color"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
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

var defaultForeground = color.RGBA{R: 233, G: 237, B: 223, A: 255}

var ansiPalette = [16]color.RGBA{
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
}

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
func parseLine(line string) []styledRune {
	state := ansiState{fg: defaultForeground}
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
				applySGR(&state, line[index+2:end])
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

func applySGR(state *ansiState, params string) {
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
			state.fg = defaultForeground
		case value == 39:
			state.fg = defaultForeground
		case value >= 30 && value <= 37:
			state.fg = ansiPalette[ansiBlack+value-30]
		case value >= 90 && value <= 97:
			state.fg = ansiPalette[ansiBrightBlack+value-90]
		case value == 38 && index+1 < len(values):
			kind, err := strconv.Atoi(values[index+1])
			if err != nil {
				continue
			}
			switch kind {
			case 5:
				if index+2 < len(values) {
					if n, err := strconv.Atoi(values[index+2]); err == nil {
						state.fg = ansi256(n)
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

func ansi256(n int) color.RGBA {
	if n < 0 {
		return defaultForeground
	}
	if n < 16 {
		return ansiPalette[n]
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
