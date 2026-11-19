package ui_test

import (
	"strings"
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
	assert.True(t, strings.Contains(rendered, "\x1b["))
	primary := theme.Primary("title")
	assert.True(t, strings.Contains(primary, "\x1b["))
}
