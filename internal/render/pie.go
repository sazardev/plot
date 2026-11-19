package render

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/sazardev/plot/internal/domain"
)

func renderPie(spec *domain.RenderSpec, theme Theme) (string, error) {
	if len(spec.PieSlices) < 2 {
		return "", &domain.IncompatibleChartError{
			Type:   spec.ChartType,
			Reason: "pie and donut charts need at least two non-zero slices",
		}
	}
	total := 0.0
	values := make([]float64, len(spec.PieSlices))
	for i, slice := range spec.PieSlices {
		if math.IsNaN(slice.Value) || math.IsInf(slice.Value, 0) || slice.Value < 0 {
			return "", &domain.IncompatibleChartError{
				Type:   spec.ChartType,
				Reason: "slice values must be finite and non-negative",
			}
		}
		values[i] = slice.Value
		total += slice.Value
	}
	if total <= 0 {
		return "", domain.ErrNoValues
	}

	percents := piePercentages(values)
	titleRows := 0
	if strings.TrimSpace(spec.Title) != "" {
		titleRows = 1
	}

	labelWidth := 0
	perLine := 0
	lines := 0
	maxLegendRows := spec.Height - titleRows - 5
	if maxLegendRows < 1 {
		return "", pieSpaceError(spec)
	}
	maxLabelWidth := 14
	if spec.Width-11 < maxLabelWidth {
		maxLabelWidth = spec.Width - 11
	}
	for width := maxLabelWidth; width >= 1; width-- {
		entryWidth := width + 11
		candidatePerLine := spec.Width / entryWidth
		if candidatePerLine < 1 {
			candidatePerLine = 1
		}
		candidateLines := (len(spec.PieSlices) + candidatePerLine - 1) / candidatePerLine
		if candidateLines <= maxLegendRows && spec.Height-titleRows-candidateLines >= 5 {
			labelWidth = width
			perLine = candidatePerLine
			lines = candidateLines
			break
		}
	}
	if labelWidth == 0 {
		return "", pieSpaceError(spec)
	}

	pieAreaHeight := spec.Height - titleRows - lines
	if pieAreaHeight < 5 {
		return "", pieSpaceError(spec)
	}

	radiusY := (pieAreaHeight - 1) / 2
	radiusX := radiusY * 2
	if radiusX*2 > spec.Width-2 {
		radiusX = (spec.Width - 2) / 2
		radiusY = radiusX / 2
	}
	if radiusY < 2 {
		return "", pieSpaceError(spec)
	}

	centerX := spec.Width / 2
	centerY := titleRows + (pieAreaHeight-1)/2
	canvas := newCanvas(spec.Width, spec.Height)

	cumulative := make([]float64, len(values))
	sum := 0.0
	for i, value := range values {
		sum += value
		cumulative[i] = sum / total
	}

	for y := centerY - radiusY; y <= centerY+radiusY; y++ {
		for x := centerX - radiusX; x <= centerX+radiusX; x++ {
			nx := float64(x-centerX) / float64(radiusX)
			ny := float64(y-centerY) / float64(radiusY)
			radiusSquared := nx*nx + ny*ny
			if radiusSquared > 1 {
				continue
			}
			if spec.ChartType == domain.ChartDonut && radiusSquared < 0.25 {
				continue
			}
			angle := math.Atan2(nx, -ny)
			if angle < 0 {
				angle += 2 * math.Pi
			}
			fraction := angle / (2 * math.Pi)
			index := sort.SearchFloat64s(cumulative, fraction)
			if index >= len(spec.PieSlices) {
				index = len(spec.PieSlices) - 1
			}
			slice := spec.PieSlices[index]
			canvas.set(x, y, theme.pieGlyph(slice.Marker), theme.seriesStyle(slice.ColorSlot))
		}
	}

	if titleRows > 0 {
		title := clip(spec.Title, spec.Width, theme.ASCII)
		if title != "" {
			canvas.rows[0].put(0, title, theme.Primary)
		}
	}

	legend := pieLegend(spec, theme, labelWidth, perLine, percents)
	for i, row := range legend {
		canvas.rows[spec.Height-len(legend)+i] = row
	}

	return canvas.string(), nil
}

func pieLegend(spec *domain.RenderSpec, theme Theme, labelWidth, perLine int, percents []float64) []row {
	lines := (len(spec.PieSlices) + perLine - 1) / perLine
	rows := make([]row, lines)
	for i := range rows {
		rows[i] = newRow(spec.Width)
	}
	entryWidth := labelWidth + 11

	for i, slice := range spec.PieSlices {
		line := i / perLine
		column := i % perLine
		x := column * entryWidth
		if x >= spec.Width {
			continue
		}
		rows[line].put(x, theme.pieGlyph(slice.Marker), theme.seriesStyle(slice.ColorSlot))
		label := clip(slice.Label, labelWidth, theme.ASCII)
		rows[line].put(x+2, label, nil)
		percentage := localizeDecimal(fmt.Sprintf("%5.1f%%", percents[i]), decimalSeparator(spec.Locale))
		rows[line].put(x+2+labelWidth+1, percentage, theme.Muted)
	}
	return rows
}

// piePercentages returns one-decimal percentages that sum exactly to 100.0
// using the largest-remainder method.
func piePercentages(values []float64) []float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	tenths := make([]int, len(values))
	remainders := make([]float64, len(values))
	sumTenths := 0
	for i, value := range values {
		raw := value / total * 1000
		floor := int(math.Floor(raw))
		tenths[i] = floor
		remainders[i] = raw - float64(floor)
		sumTenths += floor
	}
	remaining := 1000 - sumTenths
	order := make([]int, len(values))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return remainders[order[i]] > remainders[order[j]]
	})
	for i := 0; i < remaining; i++ {
		tenths[order[i%len(order)]]++
	}
	percents := make([]float64, len(values))
	for i, value := range tenths {
		percents[i] = float64(value) / 10
	}
	return percents
}

func pieSpaceError(spec *domain.RenderSpec) error {
	return &domain.IncompatibleChartError{
		Type:   spec.ChartType,
		Reason: "not enough room for the pie and legend; widen the chart or reduce slices",
	}
}
