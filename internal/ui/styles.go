package ui

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/sazardev/plot/internal/render"
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

// The TUI uses standard ANSI color slots, not fixed RGB values. The terminal
// (and therefore the user's system theme) owns the actual palette. This keeps
// the TUI consistent with the terminal and lets exported screenshots resolve
// the same slots through termtheme.
var (
	primaryColor = lipgloss.Color("5") // magenta
	mutedColor   = lipgloss.Color("8") // bright black
	dangerColor  = lipgloss.Color("1") // red
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
	"3",  // yellow
	"6",  // cyan
	"2",  // green
	"5",  // magenta
	"4",  // blue
	"1",  // red
	"13", // bright magenta
	"8",  // bright black
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
