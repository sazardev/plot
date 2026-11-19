package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/muesli/termenv"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/i18n"
	"github.com/sazar/plot/internal/render"
	"github.com/sazar/plot/internal/storage"
	"github.com/sazar/plot/internal/ui"
)

func run(args []string, stream appIO) int {
	loc, err := i18n.New(resolveLocale(stream.Env))
	if err != nil {
		fmt.Fprintf(stream.Err, "pl: %v\n", err)
		return exitData
	}

	opts := &cliOptions{}
	cmd := newCommand(opts, stream, loc)
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		var appErr *appError
		if errors.As(err, &appErr) {
			if appErr.code == exitBrokenPipe {
				return appErr.code
			}
			if appErr.id == "" {
				if appErr.cause != nil {
					fmt.Fprintln(stream.Err, domain.SanitizeText(appErr.cause.Error()))
				}
				return appErr.code
			}
			printAppError(stream.Err, loc, appErr)
			return appErr.code
		}
		usage := newAppError(exitUsage, "cli.error.usage", map[string]any{"Detail": err.Error()}, err)
		printAppError(stream.Err, loc, usage)
		return usage.code
	}
	return exitOK
}

func runApp(args []string, opts *cliOptions, stream appIO, loc *i18n.Localizer) error {
	if opts.LabelSet && strings.TrimSpace(opts.Label) == "" {
		return usageError("cli.error.label_empty", nil)
	}
	if opts.ColumnsSet && len(opts.Columns) == 0 {
		return usageError("cli.error.columns_empty", nil)
	}
	colorMode, colorEnabled, colorErr := resolveColor(opts.Color, stream.OutTTY, stream.Env)
	if colorErr != nil {
		return colorErr
	}
	width, height, sizeErr := resolveSize(stream, *opts)
	if sizeErr != nil {
		return sizeErr
	}
	delimiter, delimiterErr := resolveDelimiter(opts.Delimiter)
	if delimiterErr != nil {
		return delimiterErr
	}
	headerMode, headerErr := resolveHeaderMode(opts.Header, opts.NoHeader)
	if headerErr != nil {
		return headerErr
	}

	ascii := resolveASCII(*opts, stream.Env)
	glyphMode := domain.GlyphUnicode
	if ascii {
		glyphMode = domain.GlyphASCII
	}
	profile := stream.Profile
	if colorEnabled && (profile == termenv.Ascii || profile == 0 || !stream.OutTTY) {
		profile = termenv.ANSI
	}
	theme := ui.NewTheme(ui.ThemeOptions{
		Color:   colorEnabled,
		ASCII:   ascii,
		Profile: profile,
	})

	if opts.TUI && (opts.Width != 0 || opts.Height != 0) {
		return usageError("cli.error.tui_size", nil)
	}
	if opts.TUI {
		if !stream.InTTY || !stream.OutTTY {
			if stream.Err != nil {
				fmt.Fprintln(stream.Err, loc.T("cli.warn.tui_fallback", nil))
			}
		} else {
			return runTUI(args, opts, stream, loc, delimiter, headerMode, colorMode, colorEnabled, glyphMode, theme)
		}
	}

	ds, sourcePath, fromStdin, err := loadDataset(args, opts, stream, delimiter, headerMode)
	if err != nil {
		return asAppError(err, sourcePath, fromStdin)
	}
	if err := domain.Infer(ds); err != nil {
		return asAppError(err, sourcePath, fromStdin)
	}

	spec, err := domain.BuildRenderSpec(ds, makeBuildOptions(opts, stream, colorMode, glyphMode, width, height))
	if err != nil {
		return asAppError(err, sourcePath, fromStdin)
	}

	frame, err := render.Chart(spec, theme)
	if err != nil {
		return asAppError(err, sourcePath, fromStdin)
	}
	if _, err := io.WriteString(stream.Out, frame+"\n"); err != nil {
		if isBrokenPipe(err) {
			return newAppError(exitBrokenPipe, "", nil, err)
		}
		return newAppError(exitData, "cli.error.internal", map[string]any{"Detail": err.Error()}, err)
	}

	if stream.Err != nil {
		for _, warning := range ds.Warnings {
			fmt.Fprintln(stream.Err, domain.SanitizeText(loc.T(warning.ID, warningData(warning))))
		}
	}
	return nil
}

func makeBuildOptions(opts *cliOptions, stream appIO, colorMode domain.ColorMode, glyphMode domain.GlyphMode, width, height int) domain.BuildOptions {
	return domain.BuildOptions{
		ChartType: opts.Type,
		Label:     opts.Label,
		Columns:   opts.Columns,
		Renames:   opts.Renames,
		Sorts:     opts.Sorts,
		Filters:   opts.Filters,
		Width:     width,
		Height:    height,
		Title:     opts.Title,
		Locale:    resolveLocale(stream.Env),
		ColorMode: colorMode,
		GlyphMode: glyphMode,
	}
}

func loadDataset(args []string, opts *cliOptions, stream appIO, delimiter rune, header storage.HeaderMode) (*domain.Dataset, string, bool, error) {
	dbPath := strings.TrimSpace(opts.DB)
	if dbPath == "" && len(args) == 0 {
		dbPath = strings.TrimSpace(stream.Env["PL_DB"])
	}
	query := strings.TrimSpace(opts.Query)
	queryFile := strings.TrimSpace(opts.QueryFile)
	if query == "" && queryFile == "" && len(args) == 0 {
		query = strings.TrimSpace(stream.Env["PL_QUERY"])
	}
	explicitDB := strings.TrimSpace(opts.DB) != "" ||
		strings.TrimSpace(opts.Query) != "" ||
		strings.TrimSpace(opts.QueryFile) != ""
	hasDBSource := dbPath != "" || query != "" || queryFile != ""
	if hasDBSource && len(args) > 0 {
		if explicitDB {
			return nil, "", false, usageError("cli.error.source_conflict", nil)
		}
		hasDBSource = false
	}
	if hasDBSource {
		if dbPath == "" {
			return nil, "", false, usageError("cli.error.db_missing", nil)
		}
		if query != "" && queryFile != "" {
			return nil, "", false, usageError("cli.error.query_conflict", nil)
		}
		if query == "" && queryFile == "" {
			return nil, "", false, usageError("cli.error.query_missing", nil)
		}
		if opts.Timeout <= 0 {
			return nil, "", false, usageError("cli.error.invalid_timeout", nil)
		}
		ds, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
			Path:      dbPath,
			Query:     query,
			QueryFile: queryFile,
			Timeout:   opts.Timeout,
		})
		return ds, dbPath, false, err
	}

	options := storage.CSVOptions{
		Delimiter:  delimiter,
		HeaderMode: header,
		MaxBytes:   maxInputBytes(stream.Env),
	}

	switch len(args) {
	case 0:
		if stream.InTTY {
			return nil, "", true, usageError("cli.error.no_input", nil)
		}
		options.SourceKind = domain.SourceStdin
		options.SourceID = "stdin"
		ds, err := storage.ReadCSV(stream.In, options)
		return ds, "", true, err
	case 1:
		if args[0] == "-" {
			if stream.InTTY {
				return nil, "", true, usageError("cli.error.no_input", nil)
			}
			options.SourceKind = domain.SourceStdin
			options.SourceID = "stdin"
			ds, err := storage.ReadCSV(stream.In, options)
			return ds, "", true, err
		}
		ds, err := storage.ReadFile(args[0], options)
		return ds, args[0], false, err
	default:
		for _, arg := range args {
			if arg == "-" {
				return nil, "", true, usageError("cli.error.stdin_with_files", nil)
			}
		}
		ds, err := storage.ReadFiles(args, options)
		return ds, args[0], false, err
	}
}

func asAppError(err error, sourcePath string, fromStdin bool) *appError {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr
	}
	return mapDomainError(err, sourcePath, fromStdin)
}
