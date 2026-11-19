package render

import (
	"math"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/sazar/plot/internal/domain"
)

const (
	minWidth  = 20
	minHeight = 8
)

// Chart renders one static frame. It is a pure function of spec and theme.
func Chart(spec *domain.RenderSpec, theme Theme) (string, error) {
	if spec == nil {
		return "", domain.ErrNoValues
	}
	if spec.Width < minWidth || spec.Height < minHeight {
		return "", &domain.TooSmallError{Width: spec.Width, Height: spec.Height}
	}
	if spec.ChartType == domain.ChartPie || spec.ChartType == domain.ChartDonut {
		if len(spec.PieSlices) == 0 {
			return "", domain.ErrNoValues
		}
		return renderPie(spec, theme)
	}
	if len(spec.Series) == 0 {
		return "", domain.ErrNoValues
	}

	r, err := newRenderer(spec, theme)
	if err != nil {
		return "", err
	}

	r.drawAxes()
	switch spec.ChartType {
	case domain.ChartLine:
		if err := r.renderLine(); err != nil {
			return "", err
		}
	case domain.ChartBar:
		if err := r.renderBar(); err != nil {
			return "", err
		}
	default:
		return "", &domain.UnsupportedChartError{Type: string(spec.ChartType)}
	}
	r.drawXLabels()
	r.drawTitle()
	r.drawLegend()
	return r.canvas.string(), nil
}

type cell struct {
	text  string
	style StyleFunc
}

type row []cell

func newRow(width int) row {
	r := make(row, width)
	for i := range r {
		r[i] = cell{text: " "}
	}
	return r
}

func (r row) put(x int, text string, style StyleFunc) {
	if text == "" || x < 0 || x >= len(r) {
		return
	}
	width := runewidth.StringWidth(text)
	if width <= 0 {
		return
	}
	if x+width > len(r) {
		text = runewidth.Truncate(text, len(r)-x, "")
		width = runewidth.StringWidth(text)
		if text == "" || width <= 0 {
			return
		}
	}
	r[x] = cell{text: text, style: style}
	for i := 1; i < width && x+i < len(r); i++ {
		r[x+i] = cell{}
	}
}

func (r row) trimRight() row {
	for len(r) > 0 {
		last := r[len(r)-1]
		if last.text == " " || last.text == "" {
			r = r[:len(r)-1]
			continue
		}
		break
	}
	return r
}

func (r row) render() string {
	var b strings.Builder
	for _, c := range r {
		if c.text == "" {
			continue
		}
		if c.style != nil {
			b.WriteString(c.style(c.text))
		} else {
			b.WriteString(c.text)
		}
	}
	return b.String()
}

type canvas struct {
	rows []row
}

func newCanvas(width, height int) *canvas {
	rows := make([]row, height)
	for i := range rows {
		rows[i] = newRow(width)
	}
	return &canvas{rows: rows}
}

func (c *canvas) set(x, y int, text string, style StyleFunc) {
	if y < 0 || y >= len(c.rows) {
		return
	}
	c.rows[y].put(x, text, style)
}

func (c *canvas) string() string {
	lines := make([]string, len(c.rows))
	for i, r := range c.rows {
		lines[i] = r.trimRight().render()
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

type yTick struct {
	row   int
	value float64
	label string
}

type renderer struct {
	spec  *domain.RenderSpec
	theme Theme

	width  int
	height int

	plotX int
	plotY int
	plotW int
	plotH int

	axisY      int
	xLabelY    int
	legendY    int
	yAxisW     int
	yTicks     []yTick
	yMin       float64
	yMax       float64
	xMin       float64
	xMax       float64
	canvas     *canvas
	decimalSep string
}

func newRenderer(spec *domain.RenderSpec, theme Theme) (*renderer, error) {
	r := &renderer{
		spec:       spec,
		theme:      theme,
		width:      spec.Width,
		height:     spec.Height,
		decimalSep: decimalSeparator(spec.Locale),
	}
	titleRows := 0
	if strings.TrimSpace(spec.Title) != "" {
		titleRows = 1
	}
	r.plotY = titleRows
	r.plotH = r.height - titleRows - 3 // x-axis, x labels, legend
	if r.plotH < 1 {
		return nil, &domain.TooSmallError{Width: spec.Width, Height: spec.Height}
	}
	r.axisY = r.plotY + r.plotH
	r.xLabelY = r.axisY + 1
	r.legendY = r.height - 1

	yMin, yMax, err := valueRange(spec)
	if err != nil {
		return nil, err
	}
	r.yMin, r.yMax = yMin, yMax
	r.buildYTicks()

	labelWidth := 0
	for _, tick := range r.yTicks {
		if w := runewidth.StringWidth(tick.label); w > labelWidth {
			labelWidth = w
		}
	}
	r.yAxisW = labelWidth + 2
	r.plotX = r.yAxisW + 1
	r.plotW = r.width - r.plotX
	if r.plotW < 1 {
		return nil, &domain.TooSmallError{Width: spec.Width, Height: spec.Height}
	}

	r.xMin, r.xMax = xRange(spec)
	r.canvas = newCanvas(r.width, r.height)
	return r, nil
}

func valueRange(spec *domain.RenderSpec) (float64, float64, error) {
	min, max := math.Inf(1), math.Inf(-1)
	found := false
	for _, series := range spec.Series {
		for _, point := range series.Points {
			if point.Null {
				continue
			}
			found = true
			min = math.Min(min, point.Y)
			max = math.Max(max, point.Y)
		}
	}
	if !found {
		return 0, 0, domain.ErrNoValues
	}

	if spec.ChartType == domain.ChartBar {
		if min > 0 {
			min = 0
		}
		if max < 0 {
			max = 0
		}
	}

	if min == max {
		if spec.ChartType == domain.ChartBar {
			switch {
			case min > 0:
				min = 0
			case min < 0:
				max = 0
			default:
				min, max = -1, 1
			}
		} else {
			pad := math.Abs(min) * 0.1
			if pad == 0 {
				pad = 1
			}
			min -= pad / 2
			max += pad / 2
		}
	}
	if min == max {
		max = min + 1
	}
	return min, max, nil
}

func xRange(spec *domain.RenderSpec) (float64, float64) {
	if spec.ChartType == domain.ChartBar || spec.LabelKind == domain.LabelCategorical {
		last := float64(len(spec.Labels) - 1)
		if last < 0 {
			last = 0
		}
		if last == 0 {
			return -1, 1
		}
		return 0, last
	}

	min, max := math.Inf(1), math.Inf(-1)
	found := false
	for _, series := range spec.Series {
		for _, point := range series.Points {
			if point.Null {
				continue
			}
			found = true
			min = math.Min(min, point.X)
			max = math.Max(max, point.X)
		}
	}
	if !found {
		return -1, 1
	}
	if min == max {
		min--
		max++
	}
	return min, max
}

func (r *renderer) buildYTicks() {
	count := 5
	if r.plotH < count {
		count = r.plotH
	}
	if count < 1 {
		count = 1
	}
	r.yTicks = make([]yTick, 0, count)
	for i := 0; i < count; i++ {
		fraction := 0.0
		if count > 1 {
			fraction = float64(i) / float64(count-1)
		}
		value := r.yMax - fraction*(r.yMax-r.yMin)
		if count == 1 {
			value = (r.yMin + r.yMax) / 2
		}
		row := r.plotY
		if count > 1 {
			row += int(math.Round(fraction * float64(r.plotH-1)))
		}
		r.yTicks = append(r.yTicks, yTick{
			row:   row,
			value: value,
			label: formatAxis(value, r.theme.ASCII, r.decimalSep),
		})
	}
}

func (r *renderer) drawAxes() {
	vertical, horizontal, corner, tickGlyph, _ := r.theme.axisGlyphs()
	for y := r.plotY; y < r.plotY+r.plotH; y++ {
		r.canvas.set(r.yAxisW, y, vertical, r.theme.Muted)
	}
	for _, tick := range r.yTicks {
		r.canvas.set(r.yAxisW, tick.row, tickGlyph, r.theme.Muted)
		label := clip(tick.label, r.yAxisW-2, r.theme.ASCII)
		r.canvas.rows[tick.row].put(r.yAxisW-1-runewidth.StringWidth(label), label, nil)
	}
	r.canvas.set(r.yAxisW, r.axisY, corner, r.theme.Muted)
	for x := r.plotX; x < r.width; x++ {
		r.canvas.set(x, r.axisY, horizontal, r.theme.Muted)
	}
}

func (r *renderer) renderLine() error {
	if r.theme.ASCII {
		return r.renderLineCells()
	}
	return r.renderLineBraille()
}

func (r *renderer) renderLineCells() error {
	for _, series := range r.spec.Series {
		style := r.theme.seriesStyle(series.ColorSlot)
		lastX, lastY := 0, 0
		hasLast := false
		for _, point := range series.Points {
			if point.Null {
				hasLast = false
				continue
			}
			x, y := r.xToCol(point.X), r.yToRow(point.Y)
			if hasLast {
				r.segment(lastX, lastY, x, y, style, r.theme.lineGlyph())
			}
			lastX, lastY = x, y
			hasLast = true
		}
	}
	r.drawLineMarkers()
	return nil
}

func (r *renderer) renderLineBraille() error {
	bits := make([][]uint8, r.plotH)
	owner := make([][]int, r.plotH)
	for y := range bits {
		bits[y] = make([]uint8, r.plotW)
		owner[y] = make([]int, r.plotW)
		for x := range owner[y] {
			owner[y][x] = -1
		}
	}

	for seriesIndex, series := range r.spec.Series {
		lastX, lastY := 0, 0
		hasLast := false
		for _, point := range series.Points {
			if point.Null {
				hasLast = false
				continue
			}
			x, y := r.xToSub(point.X), r.yToSub(point.Y)
			if hasLast {
				r.brailleSegment(lastX, lastY, x, y, seriesIndex, bits, owner)
			}
			r.setBraille(x, y, seriesIndex, bits, owner)
			lastX, lastY = x, y
			hasLast = true
		}
	}

	for y := range bits {
		for x, value := range bits[y] {
			if value == 0 {
				continue
			}
			style := r.theme.seriesStyle(owner[y][x])
			r.canvas.set(r.plotX+x, r.plotY+y, string(rune(0x2800+int(value))), style)
		}
	}

	r.drawLineMarkers()
	return nil
}

func (r *renderer) drawLineMarkers() {
	for _, series := range r.spec.Series {
		style := r.theme.seriesStyle(series.ColorSlot)
		step := 1
		if len(series.Points) > 16 {
			step = len(series.Points) / 8
		}
		for i, point := range series.Points {
			if point.Null {
				continue
			}
			if i%step != 0 && i != 0 && i != len(series.Points)-1 {
				continue
			}
			r.canvas.set(r.xToCol(point.X), r.yToRow(point.Y), r.theme.marker(series.Marker), style)
		}
	}
}

func (r *renderer) brailleSegment(x0, y0, x1, y1, series int, bits [][]uint8, owner [][]int) {
	dx := abs(x1 - x0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	dy := -abs(y1 - y0)
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		r.setBraille(x0, y0, series, bits, owner)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func (r *renderer) setBraille(x, y, series int, bits [][]uint8, owner [][]int) {
	if r.plotW <= 0 || r.plotH <= 0 {
		return
	}
	if x < 0 || y < 0 || x >= r.plotW*2 || y >= r.plotH*4 {
		return
	}
	column := x / 2
	row := y / 4
	bits[row][column] |= brailleBit(x%2, y%4)
	if owner[row][column] < 0 {
		owner[row][column] = series
	}
}

func (r *renderer) xToSub(x float64) int {
	if r.plotW <= 1 || r.xMax == r.xMin {
		return 0
	}
	value := int(math.Round((x - r.xMin) / (r.xMax - r.xMin) * float64(r.plotW*2-1)))
	return clamp(value, 0, r.plotW*2-1)
}

func (r *renderer) yToSub(y float64) int {
	if r.plotH <= 1 || r.yMax == r.yMin {
		return 0
	}
	value := int(math.Round((r.yMax - y) / (r.yMax - r.yMin) * float64(r.plotH*4-1)))
	return clamp(value, 0, r.plotH*4-1)
}

func brailleBit(x, y int) uint8 {
	switch {
	case x == 0 && y == 0:
		return 0x01
	case x == 0 && y == 1:
		return 0x02
	case x == 0 && y == 2:
		return 0x04
	case x == 0 && y == 3:
		return 0x40
	case x == 1 && y == 0:
		return 0x08
	case x == 1 && y == 1:
		return 0x10
	case x == 1 && y == 2:
		return 0x20
	case x == 1 && y == 3:
		return 0x80
	default:
		return 0
	}
}

func (r *renderer) renderBar() error {
	if len(r.spec.Series) != 1 {
		return &domain.IncompatibleChartError{Type: domain.ChartBar, Reason: "bar charts need exactly one value series"}
	}
	points := r.spec.Series[0].Points
	if len(points) > r.plotW {
		return &domain.TooManyBarsError{Count: len(points), Width: r.width}
	}
	if len(points) == 0 {
		return domain.ErrNoValues
	}

	slot, barW := r.barGeometry()
	zeroRow := r.yToRow(0)
	if r.yMin < 0 && r.yMax > 0 {
		_, _, _, _, baseline := r.theme.axisGlyphs()
		for x := r.plotX; x < r.width; x++ {
			r.canvas.set(x, zeroRow, baseline, r.theme.Muted)
		}
	}

	style := r.theme.seriesStyle(r.spec.Series[0].ColorSlot)
	for i, point := range points {
		if point.Null {
			continue
		}
		x0 := r.plotX + i*slot + (slot-barW)/2
		target := r.yToRow(point.Y)
		if point.Y > 0 && target >= zeroRow {
			target = zeroRow - 1
		}
		if point.Y < 0 && target <= zeroRow {
			target = zeroRow + 1
		}
		target = clamp(target, r.plotY, r.plotY+r.plotH-1)

		if point.Y > 0 {
			for y := target; y <= zeroRow; y++ {
				for x := x0; x < x0+barW; x++ {
					r.canvas.set(x, y, r.theme.barGlyph(), style)
				}
			}
		} else if point.Y < 0 {
			for y := zeroRow; y <= target; y++ {
				for x := x0; x < x0+barW; x++ {
					r.canvas.set(x, y, r.theme.barGlyph(), style)
				}
			}
		}
	}
	return nil
}

func (r *renderer) barGeometry() (slot, barWidth int) {
	n := len(r.spec.Series[0].Points)
	slot = r.plotW / n
	if slot < 1 {
		slot = 1
	}
	barWidth = slot
	if slot > 1 {
		barWidth = slot - 1
	}
	if barWidth > 12 {
		barWidth = 12
	}
	return slot, barWidth
}

func (r *renderer) barCenter(i int) int {
	slot, barWidth := r.barGeometry()
	x0 := r.plotX + i*slot + (slot-barWidth)/2
	return x0 + barWidth/2
}

func (r *renderer) drawXLabels() {
	if r.xLabelY < 0 || r.xLabelY >= r.height {
		return
	}
	labels := r.spec.Labels
	if len(labels) == 0 {
		return
	}
	lastEnd := r.plotX - 2

	if r.spec.ChartType == domain.ChartBar || r.spec.LabelKind == domain.LabelCategorical {
		maxLabelWidth := 3
		for _, label := range labels {
			if width := runewidth.StringWidth(label); width > maxLabelWidth {
				maxLabelWidth = width
			}
		}
		if maxLabelWidth > 12 {
			maxLabelWidth = 12
		}
		maxLabels := r.plotW / (maxLabelWidth + 2)
		if maxLabels < 1 {
			maxLabels = 1
		}
		step := (len(labels) + maxLabels - 1) / maxLabels
		if step < 1 {
			step = 1
		}
		for i := 0; i < len(labels); i += step {
			label := clip(labels[i], maxLabelWidth, r.theme.ASCII)
			if label == "" {
				continue
			}
			width := runewidth.StringWidth(label)
			var center int
			if r.spec.ChartType == domain.ChartBar {
				center = r.barCenter(i)
			} else if len(labels) == 1 {
				center = r.plotX + r.plotW/2
			} else {
				center = r.xToCol(float64(i))
			}
			start := center - width/2
			start = clamp(start, r.plotX, r.width-width)
			if start <= lastEnd+1 {
				continue
			}
			r.canvas.rows[r.xLabelY].put(start, label, nil)
			lastEnd = start + width
		}
		return
	}

	tickCount := 5
	if r.plotW < 24 {
		tickCount = 3
	}
	for i := 0; i < tickCount; i++ {
		fraction := 0.0
		if tickCount > 1 {
			fraction = float64(i) / float64(tickCount-1)
		}
		value := r.xMin + fraction*(r.xMax-r.xMin)
		var label string
		if r.spec.LabelKind == domain.LabelTime {
			label = r.nearestTimeLabel(value)
		} else {
			label = formatAxis(value, r.theme.ASCII, r.decimalSep)
		}
		label = clip(label, 12, r.theme.ASCII)
		if label == "" {
			continue
		}
		width := runewidth.StringWidth(label)
		start := clamp(r.xToCol(value)-width/2, r.plotX, r.width-width)
		if start <= lastEnd+1 {
			continue
		}
		r.canvas.rows[r.xLabelY].put(start, label, nil)
		lastEnd = start + width
	}
}

func (r *renderer) nearestTimeLabel(value float64) string {
	if len(r.spec.Series) == 0 {
		return ""
	}
	best := 0
	bestDistance := math.Inf(1)
	for i, point := range r.spec.Series[0].Points {
		if point.Null || i >= len(r.spec.Labels) {
			continue
		}
		distance := math.Abs(point.X - value)
		if distance < bestDistance {
			bestDistance = distance
			best = i
		}
	}
	return r.spec.Labels[best]
}

func (r *renderer) drawTitle() {
	title := clip(r.spec.Title, r.width, r.theme.ASCII)
	if strings.TrimSpace(title) == "" {
		return
	}
	r.canvas.rows[0].put(0, title, r.theme.Primary)
}

func (r *renderer) drawLegend() {
	x := 0
	for i, series := range r.spec.Series {
		marker := r.theme.marker(series.Marker)
		width := runewidth.StringWidth(marker)
		if x+width > r.width {
			break
		}
		r.canvas.rows[r.legendY].put(x, marker, r.theme.seriesStyle(series.ColorSlot))
		x += width
		if x < r.width {
			r.canvas.rows[r.legendY].put(x, " ", nil)
			x++
		}
		label := clip(series.Label, r.width-x, r.theme.ASCII)
		if label == "" {
			break
		}
		r.canvas.rows[r.legendY].put(x, label, nil)
		x += runewidth.StringWidth(label)
		if i == len(r.spec.Series)-1 {
			break
		}
		if x+2 > r.width {
			break
		}
		r.canvas.rows[r.legendY].put(x, "  ", nil)
		x += 2
	}
}

func (r *renderer) xToCol(x float64) int {
	if r.plotW <= 1 {
		return r.plotX
	}
	if r.xMax == r.xMin {
		return r.plotX + r.plotW/2
	}
	col := r.plotX + int(math.Round((x-r.xMin)/(r.xMax-r.xMin)*float64(r.plotW-1)))
	return clamp(col, r.plotX, r.plotX+r.plotW-1)
}

func (r *renderer) yToRow(y float64) int {
	if r.plotH <= 1 {
		return r.plotY
	}
	if r.yMax == r.yMin {
		return r.plotY + r.plotH/2
	}
	row := r.plotY + int(math.Round((r.yMax-y)/(r.yMax-r.yMin)*float64(r.plotH-1)))
	return clamp(row, r.plotY, r.plotY+r.plotH-1)
}

func (r *renderer) segment(x0, y0, x1, y1 int, style StyleFunc, glyph string) {
	dx := abs(x1 - x0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	dy := -abs(y1 - y0)
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		r.canvas.set(x0, y0, glyph, style)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func formatAxis(v float64, ascii bool, decimalSep string) string {
	if math.Abs(v) < 1e-12 {
		return "0"
	}
	absValue := math.Abs(v)
	var factor float64
	suffix := ""
	switch {
	case absValue >= 1e12:
		factor, suffix = 1e12, "T"
	case absValue >= 1e9:
		factor, suffix = 1e9, "G"
	case absValue >= 1e6:
		factor, suffix = 1e6, "M"
	case absValue >= 1e3:
		factor, suffix = 1e3, "k"
	case absValue >= 1:
		factor, suffix = 1, ""
	case absValue >= 1e-3:
		factor, suffix = 1e-3, "m"
	case absValue >= 1e-6:
		factor, suffix = 1e-6, "µ"
	case absValue >= 1e-9:
		factor, suffix = 1e-9, "n"
	default:
		factor, suffix = 1e-12, "p"
	}
	scaled := v / factor
	out := localizeDecimal(strconv.FormatFloat(scaled, 'g', 3, 64), decimalSep) + suffix
	if ascii {
		out = strings.ReplaceAll(out, "µ", "u")
	}
	return out
}

func clip(text string, max int, ascii bool) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) <= max {
		return text
	}
	ellipsis := "…"
	if ascii {
		ellipsis = "..."
	}
	ellipsisWidth := runewidth.StringWidth(ellipsis)
	if max <= ellipsisWidth {
		return runewidth.Truncate(text, max, "")
	}
	return runewidth.Truncate(text, max-ellipsisWidth, "") + ellipsis
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
