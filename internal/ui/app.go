package ui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/i18n"
	"github.com/sazar/plot/internal/render"
)

const (
	minTUIWidth     = 40
	minTUIHeight    = 20
	maxContent      = 100
	animationFrames = 12
	animationDur    = 400 * time.Millisecond
)

// Options wires the TUI to the application layer without importing storage or
// the CLI. Load and Build run inside tea.Cmd.
type Options struct {
	Load        func(context.Context) (*domain.Dataset, error)
	Build       func(*domain.Dataset, domain.ChartType, int, int) (*domain.RenderSpec, error)
	InitialType domain.ChartType
	Theme       render.Theme
	Locale      *i18n.Localizer
	SourceName  string
	Title       string
	NoMotion    bool
	Color       bool
}

// Model is the single-screen plot view.
type Model struct {
	opts    Options
	styles  ThemeStyles
	spinner spinner.Model
	help    help.Model
	keys    KeyMap

	width  int
	height int

	loading bool
	err     error
	exit    int

	dataset *domain.Dataset
	source  string
	views   map[domain.ChartType]chartView
	order   []domain.ChartType
	current int

	showHelp bool

	animFrame int
	animating bool
}

// New builds the TUI model.
func New(opts Options) Model {
	opts.Title = domain.SanitizeText(opts.Title)
	opts.SourceName = domain.SanitizeText(opts.SourceName)
	loc := opts.Locale
	if loc == nil {
		loc, _ = i18n.New("en")
		opts.Locale = loc
	}
	styles := NewStyles(opts.Color, opts.Theme.ASCII)
	helpModel := help.New()
	helpModel.Width = 80
	if !opts.Color {
		plain := lipgloss.NewStyle()
		helpModel.Styles = help.Styles{
			Ellipsis:       plain,
			ShortKey:       plain,
			ShortDesc:      plain,
			ShortSeparator: plain,
			FullKey:        plain,
			FullDesc:       plain,
			FullSeparator:  plain,
		}
	}
	if opts.Theme.ASCII {
		helpModel.ShortSeparator = " | "
		helpModel.FullSeparator = " | "
		helpModel.Ellipsis = "..."
	}
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return Model{
		opts:    opts,
		styles:  styles,
		spinner: spin,
		help:    helpModel,
		keys:    DefaultKeyMap(loc),
		loading: true,
	}
}

// Init starts loading and the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadCmd())
}

// Update handles messages without IO.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = contentWidth(m.width)
		return m, nil

	case loadedMsg:
		m.loading = false
		m.err = nil
		m.dataset = msg.dataset
		m.source = msg.source
		m.views = msg.specs
		m.order = msg.order
		m.current = indexOf(m.order, msg.initial)
		m.showHelp = false
		m.help.ShowAll = false
		m.animFrame = 0
		m.animating = !m.opts.NoMotion
		if m.animating {
			return m, m.animationTick()
		}
		return m, nil

	case loadErrMsg:
		m.loading = false
		m.err = msg.err
		m.exit = 1
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case animationTickMsg:
		if !m.animating {
			return m, nil
		}
		m.animFrame++
		if m.animFrame >= animationFrames {
			m.animating = false
			return m, nil
		}
		return m, m.animationTick()

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case keyMatches(msg, m.keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, m.keys.NextType):
		return m.cycle(1)
	case keyMatches(msg, m.keys.PrevType):
		return m.cycle(-1)
	case keyMatches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.help.ShowAll = m.showHelp
		return m, nil
	case keyMatches(msg, m.keys.CloseHelp):
		m.showHelp = false
		m.help.ShowAll = false
		return m, nil
	case keyMatches(msg, m.keys.Up):
		return m, nil
	case keyMatches(msg, m.keys.Down):
		return m, nil
	case keyMatches(msg, m.keys.Left):
		return m, nil
	case keyMatches(msg, m.keys.Right):
		return m, nil
	}
	return m, nil
}

func (m Model) cycle(delta int) (tea.Model, tea.Cmd) {
	if len(m.order) < 2 || m.err != nil || m.loading {
		return m, nil
	}
	m.current = (m.current + delta + len(m.order)) % len(m.order)
	m.animFrame = 0
	m.animating = !m.opts.NoMotion
	if m.animating {
		return m, m.animationTick()
	}
	return m, nil
}

// View renders the whole screen from the model.
func (m Model) View() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	if width < minTUIWidth || height < minTUIHeight {
		message := m.opts.Locale.T("tui.min_size", nil)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, m.styles.MinSize.Render(message))
	}

	contentW := contentWidth(width)
	headerH, statusH, helpH := 1, 1, 1
	bodyH := height - headerH - statusH - helpH
	chartW := contentW - 4
	chartH := bodyH - 2
	if chartW < 20 || chartH < 8 {
		message := m.opts.Locale.T("tui.min_size", nil)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, m.styles.MinSize.Render(message))
	}

	ellipsis := "…"
	if m.opts.Theme.ASCII {
		ellipsis = "..."
	}
	frameStyle := m.styles.Frame
	header := m.styles.Header.Width(contentW).Render(ansi.Truncate(m.header(), contentW, ellipsis))
	frame := frameStyle.Render(m.body(chartW, chartH))
	status := m.styles.Status.Width(contentW).Render(ansi.Truncate(m.status(), contentW, ellipsis))
	helpLine := lipgloss.NewStyle().Width(contentW).Render(m.help.ShortHelpView(m.keys.ShortHelp()))

	content := lipgloss.JoinVertical(lipgloss.Left, header, frame, status, helpLine)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m Model) header() string {
	loc := m.opts.Locale
	parts := []string{"pl"}
	if m.source != "" {
		parts = append(parts, m.source)
	}
	if m.loading {
		source := m.opts.SourceName
		if source == "" {
			source = "data"
		}
		parts = append(parts, loc.T("tui.loading", map[string]any{"Source": source}))
		return strings.Join(parts, "  ")
	}
	if m.err != nil {
		parts = append(parts, loc.T("tui.could_not_load", nil))
		return strings.Join(parts, "  ")
	}
	view, ok := m.currentView()
	if !ok {
		return strings.Join(parts, "  ")
	}
	if m.opts.Title != "" {
		parts = append(parts, m.opts.Title)
	} else {
		parts = append(parts, string(view.spec.ChartType))
	}
	parts = append(parts, loc.T("tui.rows", map[string]any{"Count": len(view.spec.Labels)}))
	if m.dataset != nil {
		parts = append(parts, loc.T("tui.columns", map[string]any{"Count": len(m.dataset.Columns)}))
	}
	parts = append(parts, loc.T("tui.series", map[string]any{"Count": seriesCount(view.spec)}))
	return strings.Join(parts, "  ")
}

func (m Model) status() string {
	loc := m.opts.Locale
	if m.err != nil {
		return loc.T("tui.press_quit", nil)
	}
	if m.loading {
		return loc.T("tui.loading_status", nil)
	}
	view, ok := m.currentView()
	if !ok {
		return ""
	}
	warnings := len(view.warnings)
	if warnings == 0 {
		return loc.T("tui.status_ready", nil)
	}
	return loc.T("tui.warnings", map[string]any{"Count": warnings})
}

func (m Model) body(width, height int) string {
	loc := m.opts.Locale
	if m.loading {
		text := m.spinner.View() + " " + loc.T("tui.loading", map[string]any{"Source": m.sourceName()})
		return padBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text), width, height)
	}
	if m.err != nil {
		icon := "✗"
		if m.opts.Theme.ASCII {
			icon = "x"
		}
		text := icon + " " + loc.T("tui.could_not_load", nil) + "\n\n" +
			wrapText(domain.SanitizeText(m.err.Error()), width-4) + "\n\n" + loc.T("tui.press_quit", nil)
		return padBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, m.styles.Error.Render(text)), width, height)
	}
	if m.showHelp {
		text := m.help.View(m.keys)
		if warnings := m.warningText(); warnings != "" {
			text += "\n\n" + warnings
		}
		return padBlock(text, width, height)
	}
	return m.chart(width, height)
}

func (m Model) chart(width, height int) string {
	view, ok := m.currentView()
	if !ok {
		return padBlock("", width, height)
	}
	spec := *view.spec
	spec.Title = ""
	spec.Width = width
	spec.Height = height
	out, err := render.Chart(&spec, m.opts.Theme)
	if err != nil {
		icon := "✗"
		if m.opts.Theme.ASCII {
			icon = "x"
		}
		text := icon + " " + m.opts.Locale.T("tui.chart_error", map[string]any{
			"Detail": domain.SanitizeText(err.Error()),
		})
		return padBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, m.styles.Error.Render(wrapText(text, width-4))), width, height)
	}
	lines := strings.Split(out, "\n")
	if m.animating {
		revealed := (len(lines)*m.animFrame + animationFrames - 1) / animationFrames
		if revealed < 1 {
			revealed = 1
		}
		if revealed < len(lines) {
			lines = lines[:revealed]
		}
	}
	return padBlock(strings.Join(lines, "\n"), width, height)
}

func (m Model) currentView() (chartView, bool) {
	if m.current < 0 || m.current >= len(m.order) {
		return chartView{}, false
	}
	view, ok := m.views[m.order[m.current]]
	return view, ok
}

func (m Model) warningText() string {
	view, ok := m.currentView()
	if !ok {
		return ""
	}
	lines := make([]string, 0, len(view.warnings))
	for _, warning := range view.warnings {
		lines = append(lines, m.opts.Locale.T(warning.ID, warningData(warning)))
	}
	return domain.SanitizeText(strings.Join(lines, "\n"))
}

func (m Model) sourceName() string {
	if m.opts.SourceName != "" {
		return m.opts.SourceName
	}
	return "data"
}

// ExitCode is the code the program returns after a clean quit.
func (m Model) ExitCode() int { return m.exit }

func (m Model) animationTick() tea.Cmd {
	interval := animationDur / animationFrames
	return tea.Tick(interval, func(time.Time) tea.Msg { return animationTickMsg{} })
}

func contentWidth(width int) int {
	if width > maxContent {
		return maxContent
	}
	return width
}

func indexOf(order []domain.ChartType, chartType domain.ChartType) int {
	for i, candidate := range order {
		if candidate == chartType {
			return i
		}
	}
	return 0
}

func seriesCount(spec *domain.RenderSpec) int {
	if spec.ChartType == domain.ChartPie || spec.ChartType == domain.ChartDonut {
		return len(spec.PieSlices)
	}
	return len(spec.Series)
}

func padBlock(text string, width, height int) string {
	lines := strings.Split(text, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		lineWidth := ansi.StringWidth(line)
		if lineWidth > width {
			line = ansi.Truncate(line, width, "")
			lineWidth = ansi.StringWidth(line)
		}
		if lineWidth < width {
			line += strings.Repeat(" ", width-lineWidth)
		}
		lines[i] = line
	}
	blank := strings.Repeat(" ", width)
	for len(lines) < height {
		lines = append(lines, blank)
	}
	return strings.Join(lines, "\n")
}

func wrapText(text string, width int) string {
	if width <= 0 {
		return text
	}
	words := strings.Fields(text)
	var lines []string
	current := ""
	for _, word := range words {
		if current == "" {
			current = word
			continue
		}
		if runewidth.StringWidth(current)+1+runewidth.StringWidth(word) <= width {
			current += " " + word
		} else {
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

func warningData(warning domain.Warning) map[string]any {
	if len(warning.Data) == 0 {
		return nil
	}
	data := make(map[string]any, len(warning.Data))
	for key, value := range warning.Data {
		data[key] = value
	}
	return data
}

func keyMatches(msg tea.KeyMsg, binding interface{ Keys() []string }) bool {
	for _, key := range binding.Keys() {
		if msg.String() == key {
			return true
		}
	}
	return false
}
