package export

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"strings"

	"github.com/mattn/go-runewidth"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/sazardev/plot/internal/termtheme"
)

const (
	exportPadding = 26
	footerHeight  = 28
)

// RenderFrame rasterizes a chart frame with the embedded fallback palette and
// font. Use RenderFrameWithOptions to inherit a terminal palette and font.
func RenderFrame(frame string) (*image.RGBA, error) {
	return RenderFrameWithOptions(frame, DefaultOptions())
}

// RenderFrameWithOptions rasterizes the same terminal frame that pl prints.
// The result is a self-contained image with the chart's colors, braille and
// block glyphs, so CLI and TUI exports look like the view the user just saw.
func RenderFrameWithOptions(frame string, options Options) (*image.RGBA, error) {
	if strings.TrimSpace(frame) == "" {
		return nil, fmt.Errorf("empty chart frame")
	}
	palette := options.Palette
	if palette.Foreground.A == 0 && palette.Background.A == 0 {
		palette = termtheme.DefaultPalette()
	}
	fontSize := options.FontSize
	if fontSize <= 0 {
		fontSize = 16
	}
	lines := strings.Split(strings.TrimRight(frame, "\n"), "\n")
	parsed := make([][]styledRune, len(lines))
	maxColumns := 0
	for row, line := range lines {
		parsed[row] = parseLine(line, palette)
		columns := 0
		for _, cell := range parsed[row] {
			columns += cell.width
		}
		if columns > maxColumns {
			maxColumns = columns
		}
	}
	if maxColumns == 0 {
		return nil, fmt.Errorf("chart frame has no visible cells")
	}

	face, err := newFace(fontSize, options.FontPath)
	if err != nil {
		return nil, err
	}
	smallFace, err := newFace(math.Max(8, fontSize*0.62), options.FontPath)
	if err != nil {
		return nil, err
	}
	cellWidth := int(math.Ceil(float64(font.MeasureString(face, "M")) / 64))
	if cellWidth < 6 {
		cellWidth = 6
	}
	cellWidth++
	cellHeight := face.Metrics().Height.Ceil() + 3
	width := maxColumns*cellWidth + exportPadding*2
	height := len(lines)*cellHeight + exportPadding*2 + footerHeight
	if width < 120 {
		width = 120
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(palette.Background), image.Point{}, draw.Src)

	footer := "pl · terminal charts"
	footerDot := fixed.P(exportPadding, height-exportPadding)
	muted := palette.ANSI[ansiBrightBlack]
	drawer := font.Drawer{Dst: img, Src: image.NewUniform(muted), Face: smallFace, Dot: footerDot}
	drawer.DrawString(footer)

	for row, cells := range parsed {
		column := 0
		for _, cell := range cells {
			x := exportPadding + column*cellWidth
			y := exportPadding + row*cellHeight
			drawCell(img, face, cell, x, y, cellWidth, cellHeight)
			column += cell.width
		}
	}
	return img, nil
}

func newFace(size float64, fontPath string) (font.Face, error) {
	parsed, err := parseFont(fontPath)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("create exported font: %w", err)
	}
	return face, nil
}

func parseFont(path string) (*opentype.Font, error) {
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if parsed, parseErr := opentype.Parse(data); parseErr == nil {
				return parsed, nil
			}
		}
	}
	parsed, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse exported font: %w", err)
	}
	return parsed, nil
}

func drawCell(dst *image.RGBA, face font.Face, cell styledRune, x, y, cellWidth, cellHeight int) {
	if cell.r == ' ' || cell.r == '\t' {
		return
	}
	switch {
	case cell.r >= 0x2800 && cell.r <= 0x28ff:
		drawBraille(dst, cell.r, cell.fg, x, y, cellWidth, cellHeight)
	case cell.r >= 0x2500 && cell.r <= 0x257f:
		drawBox(dst, cell.r, cell.fg, x, y, cellWidth, cellHeight)
	case cell.r >= 0x2580 && cell.r <= 0x259f:
		fillRect(dst, x, y, cellWidth, cellHeight, cell.fg)
	default:
		if drawMarker(dst, cell.r, cell.fg, x, y, cellWidth, cellHeight) {
			return
		}
		drawText(dst, face, cell.r, cell.fg, x, y, cellWidth, cellHeight)
	}
}

func drawText(dst *image.RGBA, face font.Face, r rune, fg color.RGBA, x, y, cellWidth, cellHeight int) {
	text := r
	if !hasGlyph(face, text) {
		switch r {
		case 'µ':
			text = 'u'
		case '×':
			drawCross(dst, fg, x, y, cellWidth, cellHeight)
			return
		case '—', '–':
			fillRect(dst, x+1, y+cellHeight/2, cellWidth-2, 1, fg)
			return
		case '…':
			dotRadius := maxInt(1, cellWidth/12)
			for i := 0; i < 3; i++ {
				fillCircle(dst, x+cellWidth/4+i*cellWidth/4, y+cellHeight/2, dotRadius, fg)
			}
			return
		default:
			text = '?'
		}
	}
	ascent := face.Metrics().Ascent.Ceil()
	drawer := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(fg),
		Face: face,
		Dot:  fixed.P(x, y+ascent),
	}
	drawer.DrawString(string(text))
}

func hasGlyph(face font.Face, r rune) bool {
	_, _, _, _, ok := face.Glyph(fixed.P(0, 0), r)
	return ok
}

func drawBraille(dst *image.RGBA, r rune, fg color.RGBA, x, y, cellWidth, cellHeight int) {
	bits := int(r - 0x2800)
	dots := [8][2]int{
		{0, 0}, {0, 1}, {0, 2}, {1, 0}, {1, 1}, {1, 2}, {0, 3}, {1, 3},
	}
	radius := maxInt(1, cellWidth/9)
	for index, dot := range dots {
		if bits&(1<<index) == 0 {
			continue
		}
		centerX := x + cellWidth*(2*dot[0]+1)/4
		centerY := y + cellHeight*(2*dot[1]+1)/8
		fillCircle(dst, centerX, centerY, radius, fg)
	}
}

func drawBox(dst *image.RGBA, r rune, fg color.RGBA, x, y, cellWidth, cellHeight int) {
	// Up, down, left, right.
	masks := map[rune][4]bool{
		0x2500: {false, false, true, true},
		0x2501: {false, false, true, true},
		0x2502: {true, true, false, false},
		0x2503: {true, true, false, false},
		0x250c: {false, true, false, true},
		0x250f: {false, true, false, true},
		0x2510: {false, true, true, false},
		0x2513: {false, true, true, false},
		0x2514: {true, false, false, true},
		0x2517: {true, false, false, true},
		0x2518: {true, false, true, false},
		0x251b: {true, false, true, false},
		0x251c: {true, true, false, true},
		0x2523: {true, true, false, true},
		0x2524: {true, true, true, false},
		0x252b: {true, true, true, false},
		0x252c: {false, true, true, true},
		0x2533: {false, true, true, true},
		0x2534: {true, false, true, true},
		0x253b: {true, false, true, true},
		0x253c: {true, true, true, true},
		0x254b: {true, true, true, true},
	}
	mask, ok := masks[r]
	if !ok {
		// Uncommon box glyphs still need to show up as a line rather than a
		// missing glyph. A centered cross is a readable fallback.
		mask = [4]bool{true, false, true, true}
	}
	thickness := maxInt(1, cellHeight/14)
	centerX := x + cellWidth/2
	centerY := y + cellHeight/2
	if mask[2] {
		fillRect(dst, x, centerY, cellWidth/2+1, thickness, fg)
	}
	if mask[3] {
		fillRect(dst, centerX, centerY, cellWidth-cellWidth/2, thickness, fg)
	}
	if mask[0] {
		fillRect(dst, centerX, y, thickness, cellHeight/2+1, fg)
	}
	if mask[1] {
		fillRect(dst, centerX, centerY, thickness, cellHeight-cellHeight/2, fg)
	}
}

func drawMarker(dst *image.RGBA, r rune, fg color.RGBA, x, y, cellWidth, cellHeight int) bool {
	centerX := x + cellWidth/2
	centerY := y + cellHeight/2
	switch r {
	case '●', '•', '·':
		fillCircle(dst, centerX, centerY, maxInt(1, cellWidth/5), fg)
	case '○', '◯':
		drawRing(dst, centerX, centerY, maxInt(1, cellWidth/3), maxInt(1, cellWidth/9), fg)
	case '◉':
		drawRing(dst, centerX, centerY, maxInt(1, cellWidth/3), maxInt(1, cellWidth/9), fg)
		fillCircle(dst, centerX, centerY, maxInt(1, cellWidth/8), fg)
	case '■', '▪':
		size := maxInt(2, cellWidth/3)
		fillRect(dst, centerX-size/2, centerY-size/2, size, size, fg)
	case '□':
		size := maxInt(2, cellWidth/3)
		for i := 0; i < size; i++ {
			dst.Set(centerX-size/2+i, centerY-size/2, fg)
			dst.Set(centerX-size/2+i, centerY+size/2, fg)
			dst.Set(centerX-size/2, centerY-size/2+i, fg)
			dst.Set(centerX+size/2, centerY-size/2+i, fg)
		}
	case '▲':
		drawTriangle(dst, centerX, centerY-cellHeight/4, centerX-cellWidth/3, centerY+cellHeight/4, centerX+cellWidth/3, centerY+cellHeight/4, fg)
	case '▼':
		drawTriangle(dst, centerX, centerY+cellHeight/4, centerX-cellWidth/3, centerY-cellHeight/4, centerX+cellWidth/3, centerY-cellHeight/4, fg)
	case '◆':
		drawDiamond(dst, centerX, centerY, cellWidth/3, cellHeight/3, fg)
	case '◇':
		drawDiamondOutline(dst, centerX, centerY, cellWidth/3, cellHeight/3, fg)
	case '—', '–':
		fillRect(dst, x+1, centerY, cellWidth-2, 1, fg)
	case '×':
		drawCross(dst, fg, x, y, cellWidth, cellHeight)
	case '…':
		for i := 0; i < 3; i++ {
			fillCircle(dst, x+cellWidth/4+i*cellWidth/4, centerY, maxInt(1, cellWidth/12), fg)
		}
	default:
		return false
	}
	return true
}

func fillRect(dst *image.RGBA, x, y, width, height int, fg color.RGBA) {
	if width <= 0 || height <= 0 {
		return
	}
	rect := image.Rect(x, y, x+width, y+height).Intersect(dst.Bounds())
	draw.Draw(dst, rect, image.NewUniform(fg), image.Point{}, draw.Src)
}

func fillCircle(dst *image.RGBA, centerX, centerY, radius int, fg color.RGBA) {
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			if x*x+y*y <= radius*radius {
				dst.Set(centerX+x, centerY+y, fg)
			}
		}
	}
}

func drawRing(dst *image.RGBA, centerX, centerY, radius, thickness int, fg color.RGBA) {
	inner := radius - thickness
	if inner < 0 {
		inner = 0
	}
	for y := -radius; y <= radius; y++ {
		for x := -radius; x <= radius; x++ {
			distance := x*x + y*y
			if distance <= radius*radius && distance >= inner*inner {
				dst.Set(centerX+x, centerY+y, fg)
			}
		}
	}
}

func drawTriangle(dst *image.RGBA, x1, y1, x2, y2, x3, y3 int, fg color.RGBA) {
	minY, maxY := minInt(y1, minInt(y2, y3)), maxInt(y1, maxInt(y2, y3))
	edges := [3][2]int{{x1, y1}, {x2, y2}, {x3, y3}}
	for y := minY; y <= maxY; y++ {
		scan := float64(y) + 0.5
		intersections := make([]float64, 0, 2)
		for index := 0; index < 3; index++ {
			a := edges[index]
			b := edges[(index+1)%3]
			ay, by := float64(a[1]), float64(b[1])
			if (ay <= scan && by > scan) || (by <= scan && ay > scan) {
				x := float64(a[0]) + (scan-ay)*(float64(b[0])-float64(a[0]))/(by-ay)
				intersections = append(intersections, x)
			}
		}
		if len(intersections) < 2 {
			continue
		}
		if intersections[0] > intersections[1] {
			intersections[0], intersections[1] = intersections[1], intersections[0]
		}
		left := int(math.Round(intersections[0]))
		right := int(math.Round(intersections[1]))
		fillRect(dst, left, y, right-left+1, 1, fg)
	}
}

func drawDiamond(dst *image.RGBA, centerX, centerY, halfWidth, halfHeight int, fg color.RGBA) {
	for y := -halfHeight; y <= halfHeight; y++ {
		width := halfWidth * (halfHeight - absInt(y)) / maxInt(1, halfHeight)
		fillRect(dst, centerX-width, centerY+y, width*2+1, 1, fg)
	}
}

func drawDiamondOutline(dst *image.RGBA, centerX, centerY, halfWidth, halfHeight int, fg color.RGBA) {
	for y := -halfHeight; y <= halfHeight; y++ {
		width := halfWidth * (halfHeight - absInt(y)) / maxInt(1, halfHeight)
		dst.Set(centerX-width, centerY+y, fg)
		dst.Set(centerX+width, centerY+y, fg)
	}
}

func drawCross(dst *image.RGBA, fg color.RGBA, x, y, cellWidth, cellHeight int) {
	size := minInt(cellWidth, cellHeight) / 3
	for i := -size; i <= size; i++ {
		dst.Set(x+cellWidth/2+i, y+cellHeight/2+i, fg)
		dst.Set(x+cellWidth/2+i, y+cellHeight/2-i, fg)
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// CellWidth is exposed for tests and diagnostics.
func CellWidth(frame string) int {
	face, err := newFace(16, "")
	if err != nil {
		return 0
	}
	width := int(math.Ceil(float64(font.MeasureString(face, "M"))/64)) + 1
	return maxInt(width, 7)
}

// RuneWidth returns the terminal width used by the export renderer.
func RuneWidth(r rune) int {
	return runewidth.RuneWidth(r)
}
