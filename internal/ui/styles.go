package ui

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sazar/plot/internal/render"
)

// ThemeOptions are the resolved color/glyph decisions from the CLI.
type ThemeOptions struct {
	Color   bool
	ASCII   bool
	Profile termenv.Profile
}

func styleFunc(style lipgloss.Style) render.StyleFunc {
	return func(s string) string { return style.Render(s) }
}

var seriesColors = []string{
	"#E69F00", // orange
	"#56B4E9", // sky blue
	"#009E73", // bluish green
	"#F0E442", // yellow
	"#0072B2", // blue
	"#D55E00", // vermillion
	"#CC79A7", // reddish purple
	"#999999", // grey
}

// NewTheme builds the renderer theme. All colors live here (DESIGN.md); views
// and the renderer never hard-code one.
func NewTheme(opts ThemeOptions) render.Theme {
	if !opts.Color {
		return render.Theme{ASCII: opts.ASCII}
	}

	renderer := lipgloss.NewRenderer(io.Discard)
	profile := opts.Profile
	if profile == 0 {
		profile = termenv.TrueColor
	}
	renderer.SetColorProfile(profile)

	styles := make([]render.StyleFunc, len(seriesColors))
	for i, color := range seriesColors {
		styles[i] = styleFunc(renderer.NewStyle().Foreground(lipgloss.Color(color)))
	}

	primary := renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{
		Light: "#7D56F4",
		Dark:  "#9B7CFF",
	})
	muted := renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{
		Light: "#8A8A8A",
		Dark:  "#6C6C6C",
	})
	bold := renderer.NewStyle().Bold(true)
	dim := renderer.NewStyle().Foreground(lipgloss.AdaptiveColor{
		Light: "#8A8A8A",
		Dark:  "#6C6C6C",
	})

	return render.Theme{
		Series:  styles,
		Primary: styleFunc(primary),
		Muted:   styleFunc(muted),
		Bold:    styleFunc(bold),
		Dim:     styleFunc(dim),
		ASCII:   opts.ASCII,
	}
}
