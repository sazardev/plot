package ui

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/i18n"
	"github.com/sazardev/plot/internal/render"
)

var updateTUI = flag.Bool("update-tui", false, "update TUI golden files")

func internalModel(t *testing.T, width, height int) Model {
	t.Helper()
	loc, err := i18n.New("en")
	require.NoError(t, err)

	ds := &domain.Dataset{
		Columns: []domain.Column{
			{Name: "month", OriginalName: "month", Index: 0},
			{Name: "revenue", OriginalName: "revenue", Index: 1},
		},
		Rows: [][]domain.Cell{
			{domain.TextCell("Jan"), domain.TextCell("10")},
			{domain.TextCell("Feb"), domain.TextCell("20")},
			{domain.TextCell("Mar"), domain.TextCell("30")},
		},
		SourceKind:  domain.SourceFile,
		SourceID:    "sales.csv",
		SourceCount: 1,
		RowCount:    3,
	}
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Width:     80,
		Height:    24,
		GlyphMode: domain.GlyphASCII,
	})
	require.NoError(t, err)

	helpModel := help.New()
	helpModel.Width = contentWidth(width)
	helpModel.ShortSeparator = " | "
	helpModel.FullSeparator = " | "
	helpModel.Ellipsis = "..."
	return Model{
		opts: Options{
			Locale: loc,
			Theme:  render.Theme{ASCII: true},
			Color:  false,
		},
		styles:  NewStyles(false, true),
		help:    helpModel,
		keys:    DefaultKeyMap(loc),
		width:   width,
		height:  height,
		dataset: ds,
		source:  "sales.csv",
		views: map[domain.ChartType]chartView{
			spec.ChartType: {spec: spec},
		},
		order:   []domain.ChartType{spec.ChartType},
		current: 0,
	}
}

func TestTUIGolden80x24(t *testing.T) {
	model := internalModel(t, 80, 24)
	assertTUIGolden(t, filepath.Join("testdata", "tui_80x24.golden"), model.View())
}

func TestTUIGolden40x20(t *testing.T) {
	model := internalModel(t, 40, 20)
	assertTUIGolden(t, filepath.Join("testdata", "tui_40x20.golden"), model.View())
}

func assertTUIGolden(t *testing.T, path, got string) {
	t.Helper()
	if *updateTUI {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}

func TestTUIAnimationRevealsMoreContent(t *testing.T) {
	model := internalModel(t, 80, 24)
	model.animating = true
	model.animFrames = animationFrames
	model.animFrame = 0
	first := model.View()
	model.animFrame = animationFrames
	full := model.View()
	require.NotEqual(t, first, full)
}

func TestTUICustomTitleAppearsInHeader(t *testing.T) {
	model := internalModel(t, 80, 24)
	model.opts.Title = "My chart"
	require.Contains(t, model.View(), "My chart")
}

func TestTUINoColorStylesHaveNoForeground(t *testing.T) {
	styles := NewStyles(false, true)
	require.NotContains(t, styles.Header.Render("x"), "\x1b[")
	require.NotContains(t, styles.Frame.Render("x"), "\x1b[")
	require.Contains(t, styles.Frame.Render("x"), "+")
}

func TestTUIWarningsInStatusAndHelp(t *testing.T) {
	model := internalModel(t, 80, 24)
	for chartType, view := range model.views {
		view.warnings = []domain.Warning{{ID: "cli.warn.zero_slices", Data: map[string]string{"Count": "1"}}}
		model.views[chartType] = view
	}
	require.Contains(t, model.View(), "1 warning")
	model.showHelp = true
	model.help.ShowAll = true
	require.Contains(t, model.View(), "zero-valued")
}

func TestScrollLinesWindowsText(t *testing.T) {
	t.Parallel()

	text := strings.Join([]string{"a", "b", "c", "d", "e"}, "\n")
	require.Equal(t, "a\nb", scrollLines(text, 0, 2))
	require.Equal(t, "c\nd", scrollLines(text, 2, 2))
	require.Equal(t, "d\ne", scrollLines(text, 99, 2))
}

func TestTUIHelpScrollKeysUpdateOffset(t *testing.T) {
	t.Parallel()

	model := internalModel(t, 80, 24)
	for chartType, view := range model.views {
		view.warnings = make([]domain.Warning, 40)
		for i := range view.warnings {
			view.warnings[i] = domain.Warning{ID: "cli.warn.ordinal_axis", Data: map[string]string{"Rows": "10"}}
		}
		model.views[chartType] = view
	}
	model.showHelp = true
	model.help.ShowAll = true
	require.Positive(t, model.maxHelpOffset())

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = updated.(Model)
	require.Equal(t, 1, model.helpOffset)

	for i := 0; i < 100; i++ {
		updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyDown})
		model = updated.(Model)
	}
	require.Equal(t, model.maxHelpOffset(), model.helpOffset, "scrolling stops at the last page")

	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	require.Equal(t, model.maxHelpOffset()-1, model.helpOffset)

	model.helpOffset = 0
	updated, _ = model.Update(tea.KeyMsg{Type: tea.KeyUp})
	model = updated.(Model)
	require.Equal(t, 0, model.helpOffset, "scrolling up at the top is a no-op")
}

func TestTUICommandPanicBecomesErrorPanel(t *testing.T) {
	t.Parallel()

	loc, err := i18n.New("en")
	require.NoError(t, err)
	model := New(Options{
		Load: func(context.Context) (*domain.Dataset, error) {
			panic("boom")
		},
		Build: func(*domain.Dataset, domain.ChartType, int, int) (*domain.RenderSpec, error) {
			return nil, nil
		},
		Locale:   loc,
		NoMotion: true,
	})

	msg := model.Init()()
	loadErr, ok := msg.(loadErrMsg)
	require.True(t, ok)
	require.Error(t, loadErr.err)
	require.Contains(t, loadErr.err.Error(), loc.T("tui.error.internal", nil))
}

func TestTUIExportMenuAndCommand(t *testing.T) {
	t.Parallel()

	loc, err := i18n.New("en")
	require.NoError(t, err)
	model := internalModel(t, 80, 24)
	model.opts.Locale = loc
	chartType := model.order[0]
	model.opts.Build = func(*domain.Dataset, domain.ChartType, int, int) (*domain.RenderSpec, error) {
		return model.views[chartType].spec, nil
	}
	model.opts.Export = func(_ *domain.RenderSpec, format string) (string, error) {
		return "/tmp/chart." + format, nil
	}

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	model = updated.(Model)
	require.True(t, model.exporting)
	require.Contains(t, model.View(), "Export current chart")

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	model = updated.(Model)
	require.False(t, model.exporting)
	require.NotNil(t, cmd)

	msg := cmd()
	exported, ok := msg.(exportedMsg)
	require.True(t, ok)
	require.NoError(t, exported.err)
	require.Equal(t, "/tmp/chart.png", exported.path)

	updated, _ = model.Update(exported)
	model = updated.(Model)
	require.Contains(t, model.status(), "/tmp/chart.png")
}

func TestTUITabExplainsSingleCompatibleChart(t *testing.T) {
	t.Parallel()

	model := internalModel(t, 80, 24)
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyTab})
	model = updated.(Model)
	require.Contains(t, model.status(), "only bar fits")
}
