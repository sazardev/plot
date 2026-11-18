package main

import (
	"errors"
	"fmt"
	"io"

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

	ds, sourcePath, fromStdin, err := loadDataset(args, stream, delimiter, headerMode)
	if err != nil {
		return asAppError(err, sourcePath, fromStdin)
	}
	if err := domain.Infer(ds); err != nil {
		return asAppError(err, sourcePath, fromStdin)
	}

	ascii := resolveASCII(*opts, stream.Env)
	glyphMode := domain.GlyphUnicode
	if ascii {
		glyphMode = domain.GlyphASCII
	}

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		ChartType: opts.Type,
		Width:     width,
		Height:    height,
		Title:     opts.Title,
		Locale:    resolveLocale(stream.Env),
		ColorMode: colorMode,
		GlyphMode: glyphMode,
	})
	if err != nil {
		return asAppError(err, sourcePath, fromStdin)
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

func loadDataset(args []string, stream appIO, delimiter rune, header storage.HeaderMode) (*domain.Dataset, string, bool, error) {
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
		return nil, "", false, usageError("cli.error.multiple_files", nil)
	}
}

func asAppError(err error, sourcePath string, fromStdin bool) *appError {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr
	}
	return mapDomainError(err, sourcePath, fromStdin)
}
