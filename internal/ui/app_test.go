package ui_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/i18n"
	"github.com/sazardev/plot/internal/render"
	"github.com/sazardev/plot/internal/ui"
)

func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.Ascii)
	os.Exit(m.Run())
}

func fixtureDataset(t *testing.T) *domain.Dataset {
	t.Helper()
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
	return ds
}

func buildSpec(ds *domain.Dataset, chartType domain.ChartType, width, height int) (*domain.RenderSpec, error) {
	return domain.BuildRenderSpec(ds, domain.BuildOptions{
		ChartType: string(chartType),
		Width:     width,
		Height:    height,
		GlyphMode: domain.GlyphASCII,
	})
}

func testModel(t *testing.T, noMotion bool, loadErr error) ui.Model {
	t.Helper()
	loc, err := i18n.New("en")
	require.NoError(t, err)
	ds := fixtureDataset(t)
	return ui.New(ui.Options{
		Load: func(context.Context) (*domain.Dataset, error) {
			if loadErr != nil {
				return nil, loadErr
			}
			return ds, nil
		},
		Build:       buildSpec,
		Theme:       render.Theme{ASCII: true},
		Locale:      loc,
		Color:       false,
		SourceName:  "sales.csv",
		NoMotion:    noMotion,
		InitialType: "",
	})
}

func waitForText(t *testing.T, tm *teatest.TestModel, text string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte(text))
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(10*time.Millisecond))
}

func TestTUIShowsChartAndQuits(t *testing.T) {
	model := testModel(t, true, nil)
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))
	t.Cleanup(func() { _ = tm.Quit() })

	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return bytes.Contains(out, []byte("Jan")) && bytes.Contains(out, []byte("sales.csv"))
	}, teatest.WithDuration(3*time.Second), teatest.WithCheckInterval(10*time.Millisecond))

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	final, ok := tm.FinalModel(t).(ui.Model)
	require.True(t, ok)
	assert.Equal(t, 0, final.ExitCode())
}

func TestTUICyclesChartTypes(t *testing.T) {
	model := testModel(t, true, nil)
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))
	t.Cleanup(func() { _ = tm.Quit() })

	waitForText(t, tm, "Jan")
	tm.Send(tea.KeyMsg{Type: tea.KeyTab})
	waitForText(t, tm, "pie")
	tm.Send(tea.KeyMsg{Type: tea.KeyShiftTab})
	waitForText(t, tm, "bar")

	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestTUIHelpToggle(t *testing.T) {
	model := testModel(t, true, nil)
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))
	t.Cleanup(func() { _ = tm.Quit() })

	waitForText(t, tm, "Jan")
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	waitForText(t, tm, "previous chart")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}

func TestTUIResizeAndMinimumSize(t *testing.T) {
	model := testModel(t, true, nil)
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
	require.IsType(t, ui.Model{}, updated)
	view := updated.View()
	assert.Contains(t, view, "terminal too small")

	updated, _ = model.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	require.IsType(t, ui.Model{}, updated)
	assert.NotContains(t, updated.View(), "terminal too small")
}

func TestTUILoadErrorExitCode(t *testing.T) {
	model := testModel(t, true, errors.New("bad input"))
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(80, 24))
	t.Cleanup(func() { _ = tm.Quit() })

	waitForText(t, tm, "could not load data")
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))

	final, ok := tm.FinalModel(t).(ui.Model)
	require.True(t, ok)
	assert.Equal(t, 1, final.ExitCode())
}

func TestTUIAtFortyByTwenty(t *testing.T) {
	model := testModel(t, true, nil)
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(40, 20))
	t.Cleanup(func() { _ = tm.Quit() })
	waitForText(t, tm, "Jan")
	tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	tm.WaitFinished(t, teatest.WithFinalTimeout(3*time.Second))
}
