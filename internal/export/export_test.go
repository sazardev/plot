package export_test

import (
	"bytes"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/export"
	"github.com/sazardev/plot/internal/render"
)

func exportFrame(t *testing.T) string {
	t.Helper()
	spec := &domain.RenderSpec{
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
		Width:     60,
		Height:    16,
		Title:     "Revenue by month",
		GlyphMode: domain.GlyphUnicode,
	}
	frame, err := render.Chart(spec, render.Theme{})
	require.NoError(t, err)
	return frame
}

func TestRenderFrame(t *testing.T) {
	t.Parallel()

	img, err := export.RenderFrame(exportFrame(t))
	require.NoError(t, err)
	require.NotNil(t, img)
	assert.Greater(t, img.Bounds().Dx(), 100)
	assert.Greater(t, img.Bounds().Dy(), 100)

	background := img.RGBAAt(0, 0)
	foundForeground := false
	for y := 0; y < img.Bounds().Dy() && !foundForeground; y += 2 {
		for x := 0; x < img.Bounds().Dx(); x += 2 {
			if img.RGBAAt(x, y) != background {
				foundForeground = true
				break
			}
		}
	}
	assert.True(t, foundForeground, "the raster should contain rendered chart pixels")
}

func TestWriteFrameFormats(t *testing.T) {
	t.Parallel()

	frame := exportFrame(t)
	dir := t.TempDir()
	tests := []struct {
		name   string
		suffix string
		prefix []byte
	}{
		{name: "png", suffix: ".png", prefix: []byte{0x89, 'P', 'N', 'G'}},
		{name: "jpg", suffix: ".jpg", prefix: []byte{0xff, 0xd8, 0xff}},
		{name: "pdf", suffix: ".pdf", prefix: []byte("%PDF-")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(dir, "nested", "chart"+test.suffix)
			require.NoError(t, export.WriteFrame(frame, path))
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.True(t, bytes.HasPrefix(data, test.prefix), "unexpected %s header: %x", test.name, data[:min(8, len(data))])
		})
	}
}

func TestFormatForPath(t *testing.T) {
	t.Parallel()

	format, err := export.FormatForPath("chart.PNG")
	require.NoError(t, err)
	assert.Equal(t, "png", format)
	format, err = export.FormatForPath("chart.jpeg")
	require.NoError(t, err)
	assert.Equal(t, "jpg", format)
	_, err = export.FormatForPath("chart.webp")
	require.Error(t, err)
	_, err = export.FormatForPath("chart")
	require.Error(t, err)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestRenderFrameWithPalette(t *testing.T) {
	t.Parallel()

	options := export.DefaultOptions()
	options.Palette.Background = color.RGBA{R: 10, G: 20, B: 30, A: 255}
	options.Palette.Foreground = color.RGBA{R: 240, G: 230, B: 220, A: 255}
	options.Palette.ANSI[3] = color.RGBA{R: 0, G: 0, B: 255, A: 255}
	img, err := export.RenderFrameWithOptions(exportFrame(t), options)
	require.NoError(t, err)
	assert.Equal(t, options.Palette.Background, img.RGBAAt(0, 0))
}
