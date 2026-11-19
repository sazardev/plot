package render_test

import (
	"testing"

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/render"
)

func BenchmarkChartLine10k(b *testing.B) {
	points := make([]domain.Point, 10000)
	for i := range points {
		points[i] = domain.Point{X: float64(i), Y: float64((i * 37) % 100)}
	}
	spec := &domain.RenderSpec{
		ChartType:   domain.ChartLine,
		LabelColumn: -1,
		LabelKind:   domain.LabelNumeric,
		Labels:      make([]string, len(points)),
		Series: []domain.Series{{
			Column: 1,
			Label:  "value",
			Points: points,
		}},
		Width:     80,
		Height:    24,
		Title:     "benchmark",
		GlyphMode: domain.GlyphASCII,
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := render.Chart(spec, render.Theme{ASCII: true}); err != nil {
			b.Fatal(err)
		}
	}
}
