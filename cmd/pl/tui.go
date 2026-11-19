package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/i18n"
	"github.com/sazardev/plot/internal/render"
	"github.com/sazardev/plot/internal/storage"
	"github.com/sazardev/plot/internal/ui"
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
	cache *storage.Cache,
) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if stream.OutTTY {
				_, _ = fmt.Fprint(stream.Out, "\x1b[?25h\x1b[?1049l")
			}
			panic(recovered)
		}
	}()
	if len(args) == 0 && !databaseSourceConfigured(opts, stream.Env) {
		return usageError("cli.error.no_input", nil)
	}
	for _, arg := range args {
		if arg == "-" {
			return usageError("cli.error.no_input", nil)
		}
	}

	base := makeBuildOptions(opts, stream, colorMode, glyphMode, 80, 24)
	var pending *pendingCache
	load := func(_ context.Context) (*domain.Dataset, error) {
		ds, _, _, write, err := loadDataset(args, opts, stream, delimiter, headerMode, cache)
		if err != nil {
			return nil, err
		}
		pending = write
		return ds, nil
	}
	// Cache writes run in a tea.Cmd after the loaded frame is scheduled, so a
	// cold cache never delays the first frame (FR-13).
	afterLoad := func() tea.Cmd {
		write := pending
		pending = nil
		if write == nil {
			return nil
		}
		return func() tea.Msg {
			write.Save()
			return nil
		}
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
		OnLoaded:    afterLoad,
	})

	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithoutCatchPanics(),
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
