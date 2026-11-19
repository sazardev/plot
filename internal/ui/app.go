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

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/i18n"
	"github.com/sazardev/plot/internal/render"
)

const (
	minTUIWidth       = 40
	minTUIHeight      = 20
	maxContent        = 100
	animationFrames   = 12
	typeChangeFrames  = 4
	animationInterval = 33 * time.Millisecond
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
	// OnLoaded runs after the first loaded frame is scheduled. The application
	// uses it for best-effort cache writes, which must stay outside Update.
	OnLoaded func() tea.Cmd
	// Export writes the current chart in format (png, jpg or pdf) and returns
	// the path. It runs inside a tea.Cmd.
	Export func(*domain.RenderSpec, string) (string, error)
	// ExportDir is shown in the export panel and help; it is the directory
	// where the application callback writes files.
	ExportDir string
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

	showHelp   bool
	helpOffset int

	exporting  bool
	lastExport string
	exportErr  error
	notice     string

	animFrame  int
	animFrames int
	animating  bool
}

// New builds the TUI model.
func New(opts Options) Model {
	opts.Title = domain.SanitizeText(opts.Title)
	opts.SourceName = domain.SanitizeText(opts.SourceName)
	opts.ExportDir = domain.SanitizeText(opts.ExportDir)
	if opts.ExportDir == "" {
		opts.ExportDir = "."
	}
	loc := opts.Locale
	if loc == nil {
		loc, _ = i18n.New("en")
		opts.Locale = loc
	}
	styles := NewStyles(opts.Color, opts.Theme.ASCII)
	helpModel := help.New()
	helpModel.Width = 80
	plain := lipgloss.NewStyle()
	if !opts.Color {
		helpModel.Styles = help.Styles{
			Ellipsis:       plain,
			ShortKey:       plain,
			ShortDesc:      plain,
			ShortSeparator: plain,
			FullKey:        plain,
			FullDesc:       plain,
			FullSeparator:  plain,
		}
	} else {
		keyStyle := lipgloss.NewStyle().Foreground(primaryColor)
		descStyle := lipgloss.NewStyle().Foreground(mutedColor)
		helpModel.Styles = help.Styles{
			Ellipsis:       descStyle,
			ShortKey:       keyStyle,
			ShortDesc:      descStyle,
			ShortSeparator: descStyle,
			FullKey:        keyStyle,
			FullDesc:       descStyle,
			FullSeparator:  descStyle,
		}
	}
	if opts.Theme.ASCII {
		helpModel.ShortSeparator = " | "
		helpModel.FullSeparator = " | "
		helpModel.Ellipsis = "..."
	}
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	if opts.Theme.ASCII {
		spin.Spinner = spinner.Line
	}
	if opts.Color {
		spin.Style = lipgloss.NewStyle().Foreground(primaryColor)
	}
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
	if m.opts.NoMotion {
		return m.loadCmd()
	}
	return tea.Batch(m.spinner.Tick, m.loadCmd())
}

// Update handles messages without IO.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = contentWidth(m.width)
		if m.showHelp && m.helpOffset > m.maxHelpOffset() {
			m.helpOffset = m.maxHelpOffset()
		}
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
		m.helpOffset = 0
		m.exporting = false
		m.lastExport = ""
		m.exportErr = nil
		m.notice = ""
		m.animFrame = 0
		m.animFrames = animationFrames
		m.animating = !m.opts.NoMotion
		var cmds []tea.Cmd
		if m.animating {
			cmds = append(cmds, m.animationTick())
		}
		if m.opts.OnLoaded != nil {
			if cmd := m.opts.OnLoaded(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
		return m, tea.Batch(cmds...)

	case loadErrMsg:
		m.loading = false
		m.err = msg.err
		m.exit = 1
		return m, nil

	case exportedMsg:
		m.exporting = false
		m.exportErr = msg.err
		if msg.err == nil {
			m.lastExport = msg.path
		} else {
			m.lastExport = ""
		}
		return m, nil

	case spinner.TickMsg:
		if !m.loading || m.opts.NoMotion {
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
		if m.animFrame >= m.animFrames {
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
	m.notice = ""
	if m.exporting {
		switch msg.String() {
		case "esc", "e":
			m.exporting = false
			m.exportErr = nil
			return m, nil
		case "p":
			m.exporting = false
			m.exportErr = nil
			return m, m.exportCmd("png")
		case "j":
			m.exporting = false
			m.exportErr = nil
			return m, m.exportCmd("jpg")
		case "d":
			m.exporting = false
			m.exportErr = nil
			return m, m.exportCmd("pdf")
		}
		if keyMatches(msg, m.keys.Quit) {
			return m, tea.Quit
		}
		return m, nil
	}
	switch {
	case keyMatches(msg, m.keys.Quit):
		return m, tea.Quit
	case keyMatches(msg, m.keys.Export):
		if m.loading || m.err != nil || m.dataset == nil || len(m.order) == 0 {
			return m, nil
		}
		m.exporting = true
		m.showHelp = false
		m.help.ShowAll = false
		m.exportErr = nil
		m.lastExport = ""
		return m, nil
	case keyMatches(msg, m.keys.NextType):
		return m.cycle(1)
	case keyMatches(msg, m.keys.PrevType):
		return m.cycle(-1)
	case keyMatches(msg, m.keys.Help):
		m.showHelp = !m.showHelp
		m.help.ShowAll = m.showHelp
		m.helpOffset = 0
		return m, nil
	case keyMatches(msg, m.keys.CloseHelp):
		m.showHelp = false
		m.help.ShowAll = false
		m.helpOffset = 0
		return m, nil
	case keyMatches(msg, m.keys.Up):
		if m.showHelp && m.helpOffset > 0 {
			m.helpOffset--
		}
		return m, nil
	case keyMatches(msg, m.keys.Down):
		if m.showHelp && m.helpOffset < m.maxHelpOffset() {
			m.helpOffset++
		}
		return m, nil
	case keyMatches(msg, m.keys.Left), keyMatches(msg, m.keys.Right):
		return m, nil
	}
	return m, nil
}

func (m Model) cycle(delta int) (tea.Model, tea.Cmd) {
	if m.err != nil || m.loading {
		return m, nil
	}
	if len(m.order) < 2 {
		if len(m.order) == 1 {
			m.notice = m.opts.Locale.T("tui.only_compatible", map[string]any{"Type": string(m.order[0])})
		}
		return m, nil
	}
	m.lastExport = ""
	m.exportErr = nil
	m.current = (m.current + delta + len(m.order)) % len(m.order)
	m.animFrame = 0
	m.animFrames = typeChangeFrames
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
	}
	parts = append(parts, string(view.spec.ChartType))
	parts = append(parts, countLabel(loc, "tui.row_one", "tui.rows", len(view.spec.Labels)))
	if m.dataset != nil {
		parts = append(parts, countLabel(loc, "tui.column_one", "tui.columns", len(m.dataset.Columns)))
	}
	series := seriesCount(view.spec)
	parts = append(parts, loc.T("tui.series", map[string]any{
		"Count":     series,
		"CountText": loc.FormatInt(series),
	}))
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
	if m.exportErr != nil {
		return loc.T("tui.export.failed", map[string]any{"Detail": domain.SanitizeText(m.exportErr.Error())})
	}
	if m.lastExport != "" {
		return loc.T("tui.export.done", map[string]any{"Path": domain.SanitizeText(m.lastExport)})
	}
	if m.notice != "" {
		return m.notice
	}
	view, ok := m.currentView()
	if !ok {
		return ""
	}
	warnings := len(view.warnings)
	if warnings == 0 {
		return loc.T("tui.status_ready_export", nil)
	}
	if warnings == 1 {
		return loc.T("tui.warning_one_export", nil)
	}
	return loc.T("tui.warnings_export", map[string]any{
		"Count":     warnings,
		"CountText": loc.FormatInt(warnings),
	})
}

func (m Model) body(width, height int) string {
	loc := m.opts.Locale
	if m.loading {
		text := m.spinner.View() + " " + loc.T("tui.loading", map[string]any{"Source": m.sourceName()})
		return padBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, text), width, height)
	}
	if m.exporting {
		chartType := ""
		if m.current >= 0 && m.current < len(m.order) {
			chartType = string(m.order[m.current])
		}
		lines := []string{
			loc.T("tui.export.title", nil),
			"",
			loc.T("tui.export.current", map[string]any{"Type": chartType}),
			loc.T("tui.export.destination", map[string]any{"Dir": m.opts.ExportDir}),
			loc.T("tui.export.naming", nil),
			"",
			loc.T("tui.export.hint", nil),
			"",
			loc.T("tui.export.note", nil),
		}
		if m.exportErr != nil {
			lines = append(lines, "", loc.T("tui.export.failed", map[string]any{"Detail": domain.SanitizeText(m.exportErr.Error())}))
		}
		text := strings.Join(lines, "\n")
		return padBlock(lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, wrapLines(text, width-4)), width, height)
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
		return padBlock(scrollLines(m.helpText(), m.helpOffset, height), width, height)
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
	if m.animating && m.animFrames > 0 {
		revealed := (len(lines)*m.animFrame + m.animFrames - 1) / m.animFrames
		if revealed < 1 {
			revealed = 1
		}
		if revealed < len(lines) {
			lines = lines[:revealed]
		}
	}
	return padBlock(strings.Join(lines, "\n"), width, height)
}

func (m Model) helpText() string {
	text := m.help.View(m.keys)
	if warnings := m.warningText(); warnings != "" {
		text += "\n\n" + warnings
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	width -= 4
	if width < 20 {
		width = 20
	}
	text += "\n\n" + wrapText(m.opts.Locale.T("tui.help.tab", nil), width)
	text += "\n" + wrapText(m.opts.Locale.T("tui.help.export", map[string]any{"Dir": m.opts.ExportDir}), width)
	return text
}

// helpViewHeight mirrors the chart body height in View: total height minus
// header, status, help bar and the rounded frame.
func (m Model) helpViewHeight() int {
	height := m.height
	if height <= 0 {
		height = 24
	}
	bodyHeight := height - 3
	if bodyHeight < 0 {
		return 0
	}
	return bodyHeight - 2
}

func (m Model) maxHelpOffset() int {
	height := m.helpViewHeight()
	if height <= 0 {
		return 0
	}
	lines := len(strings.Split(m.helpText(), "\n"))
	if lines <= height {
		return 0
	}
	return lines - height
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
		lines = append(lines, domain.SanitizeText(m.opts.Locale.T(warning.ID, warningData(warning))))
	}
	return strings.Join(lines, "\n")
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
	return tea.Tick(animationInterval, func(time.Time) tea.Msg { return animationTickMsg{} })
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

// scrollLines returns the visible window of text at offset. Help content can
// be taller than the chart body; arrows and j/k scroll it.
func scrollLines(text string, offset, height int) string {
	lines := strings.Split(text, "\n")
	maxOffset := len(lines) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	if offset < 0 {
		offset = 0
	}
	if offset >= len(lines) {
		return ""
	}
	lines = lines[offset:]
	if len(lines) > height {
		lines = lines[:height]
	}
	return strings.Join(lines, "\n")
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

func wrapLines(text string, width int) string {
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		lines[index] = wrapText(line, width)
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

func countLabel(loc *i18n.Localizer, oneID, manyID string, count int) string {
	if count == 1 {
		return loc.T(oneID, nil)
	}
	return loc.T(manyID, map[string]any{
		"Count":     count,
		"CountText": loc.FormatInt(count),
	})
}
