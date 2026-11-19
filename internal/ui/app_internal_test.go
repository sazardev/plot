package ui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/bubbles/help"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/i18n"
	"github.com/sazar/plot/internal/render"
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
