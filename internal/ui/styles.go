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

var (
	primaryColor = lipgloss.AdaptiveColor{Light: "#7D56F4", Dark: "#9B7CFF"}
	mutedColor   = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#6C6C6C"}
	dangerColor  = lipgloss.AdaptiveColor{Light: "#E0245E", Dark: "#FF5F87"}
)

// ThemeStyles keeps the TUI chrome readable in color, NO_COLOR and ASCII
// modes without mutating global Lip Gloss state.
type ThemeStyles struct {
	Header  lipgloss.Style
	Frame   lipgloss.Style
	Status  lipgloss.Style
	Error   lipgloss.Style
	MinSize lipgloss.Style
}

var asciiBorder = lipgloss.Border{
	Top:         "-",
	Bottom:      "-",
	Left:        "|",
	Right:       "|",
	TopLeft:     "+",
	TopRight:    "+",
	BottomLeft:  "+",
	BottomRight: "+",
}

// NewStyles builds TUI chrome styles for the resolved color and glyph modes.
func NewStyles(color, ascii bool) ThemeStyles {
	header := lipgloss.NewStyle()
	frame := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	status := lipgloss.NewStyle()
	errStyle := lipgloss.NewStyle()
	minSize := lipgloss.NewStyle().Align(lipgloss.Center)
	if ascii {
		frame = frame.Border(asciiBorder)
	}
	if color {
		header = header.Bold(true).Foreground(primaryColor)
		frame = frame.BorderForeground(primaryColor)
		status = status.Foreground(mutedColor)
		errStyle = errStyle.Bold(true).Foreground(dangerColor)
		minSize = minSize.Foreground(mutedColor)
	}
	return ThemeStyles{Header: header, Frame: frame, Status: status, Error: errStyle, MinSize: minSize}
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

	primary := renderer.NewStyle().Foreground(primaryColor)
	muted := renderer.NewStyle().Foreground(mutedColor)
	bold := renderer.NewStyle().Bold(true)
	dim := renderer.NewStyle().Foreground(mutedColor)

	return render.Theme{
		Series:  styles,
		Primary: styleFunc(primary),
		Muted:   styleFunc(muted),
		Bold:    styleFunc(bold),
		Dim:     styleFunc(dim),
		ASCII:   opts.ASCII,
	}
}
