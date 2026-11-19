package ui_test

import (
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"

	"github.com/sazardev/plot/internal/ui"
)

func TestNoColorThemeIsPlain(t *testing.T) {
	t.Parallel()

	theme := ui.NewTheme(ui.ThemeOptions{Color: false, ASCII: true})
	assert.True(t, theme.ASCII)
	assert.Nil(t, theme.Primary)
	assert.Empty(t, theme.Series)
}

func TestColorThemeHasEightSeries(t *testing.T) {
	t.Parallel()

	theme := ui.NewTheme(ui.ThemeOptions{
		Color:   true,
		Profile: termenv.ANSI,
	})
	assert.Len(t, theme.Series, 8)
	rendered := theme.Series[0]("x")
	assert.Contains(t, rendered, "\x1b[33m", "first series should be terminal ANSI yellow")
	primary := theme.Primary("title")
	assert.Contains(t, primary, "\x1b[35m", "chrome should be terminal ANSI magenta")
}
