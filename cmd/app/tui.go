package main

import (
	"context"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/i18n"
	"github.com/sazar/plot/internal/render"
	"github.com/sazar/plot/internal/storage"
	"github.com/sazar/plot/internal/ui"
)

func runTUI(
	args []string,
	opts *cliOptions,
	stream appIO,
	loc *i18n.Localizer,
	delimiter rune,
	headerMode storage.HeaderMode,
	colorMode domain.ColorMode,
	colorEnabled bool,
	glyphMode domain.GlyphMode,
	theme render.Theme,
) error {
	if len(args) == 0 && !databaseSourceConfigured(opts, stream.Env) {
		return usageError("cli.error.no_input", nil)
	}
	for _, arg := range args {
		if arg == "-" {
			return usageError("cli.error.no_input", nil)
		}
	}

	base := makeBuildOptions(opts, stream, colorMode, glyphMode, 80, 24)
	load := func(_ context.Context) (*domain.Dataset, error) {
		ds, _, _, err := loadDataset(args, opts, stream, delimiter, headerMode)
		if err != nil {
			return nil, err
		}
		if err := domain.Infer(ds); err != nil {
			return nil, err
		}
		return ds, nil
	}
	build := func(ds *domain.Dataset, chartType domain.ChartType, width, height int) (*domain.RenderSpec, error) {
		options := base
		options.ChartType = string(chartType)
		options.Width = width
		options.Height = height
		return domain.BuildRenderSpec(ds, options)
	}

	model := ui.New(ui.Options{
		Load:        load,
		Build:       build,
		InitialType: domain.ChartType(strings.ToLower(strings.TrimSpace(opts.Type))),
		Theme:       theme,
		Locale:      loc,
		SourceName:  tuiSourceName(args, opts, stream.Env),
		Title:       opts.Title,
		NoMotion:    tuiNoMotion(opts, stream.Env),
		Color:       colorEnabled,
	})

	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithInput(stream.In),
		tea.WithOutput(stream.Out),
	)
	final, err := program.Run()
	if err != nil {
		return newAppError(exitData, "cli.error.internal", map[string]any{"Detail": err.Error()}, err)
	}
	switch finished := final.(type) {
	case ui.Model:
		if finished.ExitCode() != 0 {
			return newAppError(finished.ExitCode(), "", nil, nil)
		}
	case *ui.Model:
		if finished.ExitCode() != 0 {
			return newAppError(finished.ExitCode(), "", nil, nil)
		}
	}
	return nil
}

func databaseSourceConfigured(opts *cliOptions, env map[string]string) bool {
	return strings.TrimSpace(opts.DB) != "" ||
		strings.TrimSpace(opts.Query) != "" ||
		strings.TrimSpace(opts.QueryFile) != "" ||
		strings.TrimSpace(env["PL_DB"]) != "" ||
		strings.TrimSpace(env["PL_QUERY"]) != ""
}

func tuiNoMotion(opts *cliOptions, env map[string]string) bool {
	return opts.NoMotion ||
		env["NO_COLOR"] != "" ||
		strings.EqualFold(strings.TrimSpace(env["TERM"]), "dumb") ||
		env["CI"] != ""
}

func tuiSourceName(args []string, opts *cliOptions, env map[string]string) string {
	if len(args) > 0 && args[0] != "-" {
		return filepath.Base(args[0])
	}
	db := strings.TrimSpace(opts.DB)
	if db == "" {
		db = strings.TrimSpace(env["PL_DB"])
	}
	if db != "" {
		return filepath.Base(db)
	}
	return "data"
}
