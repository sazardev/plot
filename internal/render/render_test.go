package render_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/render"
	"github.com/sazar/plot/internal/testutil"
)

func lineSpec() *domain.RenderSpec {
	return &domain.RenderSpec{
		ChartType:   domain.ChartLine,
		LabelColumn: 0,
		LabelName:   "day",
		LabelKind:   domain.LabelCategorical,
		Labels:      []string{"Mon", "Tue", "Wed"},
		Series: []domain.Series{{
			Column: 1,
			Label:  "signups",
			Points: []domain.Point{{X: 0, Y: 3}, {X: 1, Y: 5}, {X: 2, Y: 4}},
			Marker: 0,
		}},
		Width:     40,
		Height:    12,
		Title:     "stdin — line",
		GlyphMode: domain.GlyphUnicode,
	}
}

func barSpec() *domain.RenderSpec {
	return &domain.RenderSpec{
		ChartType:   domain.ChartBar,
		LabelColumn: 0,
		LabelName:   "month",
		LabelKind:   domain.LabelCategorical,
		Labels:      []string{"Jan", "Feb", "Mar"},
		Series: []domain.Series{{
			Column: 1,
			Label:  "revenue",
			Points: []domain.Point{{X: 0, Y: 10}, {X: 1, Y: 14}, {X: 2, Y: 7}},
			Marker: 0,
		}},
		Width:     40,
		Height:    12,
		Title:     "stdin — bar",
		GlyphMode: domain.GlyphUnicode,
	}
}

func TestChartLineRendersTitleAxesLabelsAndMarker(t *testing.T) {
	t.Parallel()

	out, err := render.Chart(lineSpec(), render.Theme{})
	require.NoError(t, err)
	t.Log("\n" + out)

	assert.Contains(t, out, "stdin — line")
	assert.Contains(t, out, "Mon")
	assert.Contains(t, out, "Tue")
	assert.Contains(t, out, "Wed")
	assert.Contains(t, out, "└")
	assert.Contains(t, out, "●")
	assert.Contains(t, out, "signups")
	assert.NotContains(t, out, "\x1b[")
}

func TestChartBarRendersBlocksAndCategories(t *testing.T) {
	t.Parallel()

	out, err := render.Chart(barSpec(), render.Theme{})
	require.NoError(t, err)
	t.Log("\n" + out)

	assert.Contains(t, out, "stdin — bar")
	assert.Contains(t, out, "Jan")
	assert.Contains(t, out, "Feb")
	assert.Contains(t, out, "Mar")
	assert.Contains(t, out, "█")
	assert.Contains(t, out, "revenue")
}

func TestChartASCIIFallback(t *testing.T) {
	t.Parallel()

	spec := barSpec()
	spec.Title = "stdin - bar"
	spec.GlyphMode = domain.GlyphASCII
	out, err := render.Chart(spec, render.Theme{ASCII: true})
	require.NoError(t, err)
	t.Log("\n" + out)

	assert.NotContains(t, out, "█")
	assert.NotContains(t, out, "└")
	assert.Contains(t, out, "#")
	assert.Contains(t, out, "|")
	assert.Contains(t, out, "+")
}

func TestChartRejectsTooSmallFrame(t *testing.T) {
	t.Parallel()

	spec := lineSpec()
	spec.Width = 10
	_, err := render.Chart(spec, render.Theme{})
	assert.ErrorIs(t, err, domain.ErrTooSmall)
}

func TestChartRejectsTooManyBars(t *testing.T) {
	t.Parallel()

	spec := barSpec()
	spec.Width = 20
	spec.Labels = make([]string, 30)
	spec.Series[0].Points = make([]domain.Point, 30)
	for i := range spec.Labels {
		spec.Labels[i] = "x"
		spec.Series[0].Points[i] = domain.Point{X: float64(i), Y: float64(i + 1)}
	}
	_, err := render.Chart(spec, render.Theme{})
	assert.ErrorIs(t, err, domain.ErrTooManyBars)
}

func TestChartRejectsNoValues(t *testing.T) {
	t.Parallel()

	spec := lineSpec()
	spec.Series[0].Points = []domain.Point{{Null: true}, {Null: true}}
	_, err := render.Chart(spec, render.Theme{})
	assert.ErrorIs(t, err, domain.ErrNoValues)
}

func TestChartDoesNotEndWithTrailingBlankLines(t *testing.T) {
	t.Parallel()

	out, err := render.Chart(lineSpec(), render.Theme{})
	require.NoError(t, err)
	assert.False(t, strings.HasSuffix(out, "\n"))
	assert.NotEmpty(t, strings.TrimSpace(out))
}

func TestChartGoldens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		spec *domain.RenderSpec
		file string
	}{
		{name: "line", spec: lineSpec(), file: "line_40x12.golden"},
		{name: "bar", spec: barSpec(), file: "bar_40x12.golden"},
		{
			name: "bar ascii",
			spec: func() *domain.RenderSpec {
				spec := barSpec()
				spec.Title = "stdin - bar"
				spec.GlyphMode = domain.GlyphASCII
				return spec
			}(),
			file: "bar_ascii_40x12.golden",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			theme := render.Theme{ASCII: tt.spec.GlyphMode == domain.GlyphASCII}
			out, err := render.Chart(tt.spec, theme)
			require.NoError(t, err)
			testutil.AssertGolden(t, filepath.Join("testdata", tt.file), []byte(out+"\n"))
		})
	}
}

func TestChartIsDeterministic(t *testing.T) {
	t.Parallel()

	spec := lineSpec()
	first, err := render.Chart(spec, render.Theme{})
	require.NoError(t, err)
	for i := 0; i < 50; i++ {
		next, err := render.Chart(spec, render.Theme{})
		require.NoError(t, err)
		require.Equal(t, first, next)
	}
}

func TestChartMinimumSize(t *testing.T) {
	t.Parallel()

	spec := lineSpec()
	spec.Width = 20
	spec.Height = 8
	out, err := render.Chart(spec, render.Theme{})
	require.NoError(t, err)
	for _, line := range strings.Split(out, "\n") {
		assert.LessOrEqual(t, len([]rune(line)), 20, "line: %q", line)
	}
}

func TestChartNegativeBar(t *testing.T) {
	t.Parallel()

	spec := barSpec()
	spec.Labels = []string{"a", "b", "c"}
	spec.Series[0].Points = []domain.Point{{X: 0, Y: -10}, {X: 1, Y: 5}, {X: 2, Y: -2}}
	out, err := render.Chart(spec, render.Theme{})
	require.NoError(t, err)
	assert.Contains(t, out, "█")
	assert.Contains(t, out, "a")
}

func TestChartNullGap(t *testing.T) {
	t.Parallel()

	spec := lineSpec()
	spec.Series[0].Points[1] = domain.Point{Null: true}
	out, err := render.Chart(spec, render.Theme{})
	require.NoError(t, err)
	assert.Contains(t, out, "●")
	assert.NotEmpty(t, out)
}

func TestChartLongTitleIsTruncated(t *testing.T) {
	t.Parallel()

	spec := lineSpec()
	spec.Title = strings.Repeat("x", 200)
	out, err := render.Chart(spec, render.Theme{})
	require.NoError(t, err)
	first := strings.SplitN(out, "\n", 2)[0]
	assert.LessOrEqual(t, len([]rune(first)), spec.Width)
	assert.Contains(t, first, "…")
}
