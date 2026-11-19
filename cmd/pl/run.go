package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/muesli/termenv"

	"github.com/sazardev/plot/internal/config"
	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/export"
	"github.com/sazardev/plot/internal/i18n"
	"github.com/sazardev/plot/internal/render"
	"github.com/sazardev/plot/internal/storage"
	"github.com/sazardev/plot/internal/ui"
)

func run(args []string, stream appIO) (code int) {
	cleanup := setupLogging(stream)
	defer cleanup()

	loc, err := i18n.New(resolveLocale(stream.Env))
	if err != nil {
		_, _ = fmt.Fprintf(stream.Err, "pl: %v\n", err)
		return exitData
	}
	// A recovered panic never prints a stack: one line to stderr, the stack
	// only to the debug file, and the documented data-error exit code.
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("recovered panic",
				"panic", fmt.Sprint(recovered),
				"stack", string(debug.Stack()),
			)
			if stream.Err != nil {
				_, _ = fmt.Fprintln(stream.Err, loc.T("cli.error.panic", nil))
			}
			code = exitData
		}
	}()

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
					_, _ = fmt.Fprintln(stream.Err, domain.SanitizeText(appErr.cause.Error()))
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
	if err := applyConfig(opts, stream); err != nil {
		return err
	}
	var cache *storage.Cache
	if !opts.NoCache {
		cache = storage.NewCache(opts.CacheDir, opts.CacheTTL, opts.CacheMax)
		if cache != nil {
			defer func() { _ = cache.Close() }()
		}
	}
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
	exportTheme := theme
	forceExportTheme := false
	if opts.Export != "" && !colorEnabled && colorMode != domain.ColorNever {
		forceExportTheme = true
		// A file export is not a terminal stream: keep the terminal chart
		// monochrome when the user asked for it, but make automatic non-TTY
		// exports colorful so the saved image retains the series palette.
		exportTheme = ui.NewTheme(ui.ThemeOptions{
			Color:   true,
			ASCII:   ascii,
			Profile: termenv.TrueColor,
		})
	}

	if opts.TUI && opts.Export != "" {
		return usageError("cli.error.export_tui", nil)
	}
	if opts.TUI && (opts.Width != 0 || opts.Height != 0) {
		return usageError("cli.error.tui_size", nil)
	}
	if opts.Export != "" {
		if _, err := export.FormatForPath(opts.Export); err != nil {
			return usageError("cli.error.export_format", map[string]any{
				"Path":   domain.SanitizeText(opts.Export),
				"Detail": domain.SanitizeText(err.Error()),
			})
		}
	}
	if opts.TUI {
		if !stream.InTTY || !stream.OutTTY {
			if stream.Err != nil {
				_, _ = fmt.Fprintln(stream.Err, loc.T("cli.warn.tui_fallback", nil))
			}
		} else {
			return runTUI(args, opts, stream, loc, delimiter, headerMode, colorMode, colorEnabled, glyphMode, theme, cache)
		}
	}

	ds, sourcePath, fromStdin, pendingWrite, err := loadDataset(args, opts, stream, delimiter, headerMode, cache)
	if err != nil {
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
	if opts.Export != "" {
		exportFrame := frame
		if forceExportTheme {
			var renderErr error
			exportFrame, renderErr = render.Chart(spec, exportTheme)
			if renderErr != nil {
				return asAppError(renderErr, sourcePath, fromStdin)
			}
		}
		if err := export.WriteFrame(exportFrame, opts.Export); err != nil {
			return newAppError(exitData, "cli.error.export", map[string]any{
				"Path":   domain.SanitizeText(opts.Export),
				"Detail": domain.SanitizeText(err.Error()),
			}, err)
		}
	}

	if stream.Err != nil {
		for _, warning := range ds.Warnings {
			_, _ = fmt.Fprintln(stream.Err, domain.SanitizeText(loc.T(warning.ID, warningData(warning))))
		}
	}
	pendingWrite.Save()
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

// pendingCache is a best-effort write scheduled for after the first frame.
type pendingCache struct {
	cache   *storage.Cache
	key     string
	dataset *domain.Dataset
}

// Save writes the dataset after the chart has been displayed. Failures are
// debug-level notes and never change output or the exit code.
func (p *pendingCache) Save() {
	if p == nil || p.cache == nil || p.key == "" || p.dataset == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Error("recovered cache write panic",
				"panic", fmt.Sprint(recovered),
				"stack", string(debug.Stack()),
			)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := p.cache.Put(ctx, p.key, p.dataset); err != nil {
		slog.Debug("cache write failed", "error", err)
	}
}

func loadDataset(args []string, opts *cliOptions, stream appIO, delimiter rune, header storage.HeaderMode, cache *storage.Cache) (*domain.Dataset, string, bool, *pendingCache, error) {
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
			return nil, "", false, nil, usageError("cli.error.source_conflict", nil)
		}
		hasDBSource = false
	}
	if hasDBSource {
		if dbPath == "" {
			return nil, "", false, nil, usageError("cli.error.db_missing", nil)
		}
		if query != "" && queryFile != "" {
			return nil, "", false, nil, usageError("cli.error.query_conflict", nil)
		}
		if query == "" && queryFile == "" {
			return nil, "", false, nil, usageError("cli.error.query_missing", nil)
		}
		if opts.Timeout <= 0 {
			return nil, "", false, nil, usageError("cli.error.invalid_timeout", nil)
		}
		queryText := query
		if queryFile != "" {
			data, err := os.ReadFile(queryFile)
			if err != nil {
				return nil, "", false, nil, &domain.SourceError{Path: queryFile, Err: err}
			}
			queryText = strings.TrimSpace(string(data))
		}
		key := storage.DBCacheKey(dbPath, queryText)
		if cache != nil {
			if ds, ok := cache.Get(context.Background(), key); ok {
				return ds, dbPath, false, nil, nil
			}
		}
		ds, err := storage.ReadSQLite(context.Background(), storage.SQLiteOptions{
			Path:    dbPath,
			Query:   queryText,
			Timeout: opts.Timeout,
		})
		if err != nil {
			return nil, dbPath, false, nil, err
		}
		return inferDataset(ds, cache, key, dbPath, false)
	}

	options := storage.CSVOptions{
		Delimiter:  delimiter,
		HeaderMode: header,
		MaxBytes:   maxInputBytes(stream.Env),
	}

	switch len(args) {
	case 0:
		if stream.InTTY {
			return nil, "", true, nil, usageError("cli.error.no_input", nil)
		}
		return loadStdin(stream.In, options, cache)
	case 1:
		if args[0] == "-" {
			if stream.InTTY {
				return nil, "", true, nil, usageError("cli.error.no_input", nil)
			}
			return loadStdin(stream.In, options, cache)
		}
		return loadOneFile(args[0], options, cache)
	default:
		for _, arg := range args {
			if arg == "-" {
				return nil, "", true, nil, usageError("cli.error.stdin_with_files", nil)
			}
		}
		return loadManyFiles(args, options, cache)
	}
}

func loadStdin(reader io.Reader, options storage.CSVOptions, cache *storage.Cache) (*domain.Dataset, string, bool, *pendingCache, error) {
	options.SourceKind = domain.SourceStdin
	options.SourceID = "stdin"
	if cache == nil {
		ds, err := storage.ReadCSV(reader, options)
		if err != nil {
			return nil, "", true, nil, err
		}
		return inferDataset(ds, nil, "", "", true)
	}
	data, err := storage.ReadInput(reader, options.MaxBytes)
	if err != nil {
		return nil, "", true, nil, err
	}
	key := storage.StdinCacheKey(data, options)
	if ds, ok := cache.Get(context.Background(), key); ok {
		return ds, "", true, nil, nil
	}
	ds, err := storage.ReadCSVBytes(data, options)
	if err != nil {
		return nil, "", true, nil, err
	}
	return inferDataset(ds, cache, key, "", true)
}

func loadOneFile(path string, options storage.CSVOptions, cache *storage.Cache) (*domain.Dataset, string, bool, *pendingCache, error) {
	options.SourceKind = domain.SourceFile
	options.SourceID = path
	if cache == nil {
		ds, err := storage.ReadFile(path, options)
		if err != nil {
			return nil, path, false, nil, err
		}
		return inferDataset(ds, nil, "", path, false)
	}
	data, err := storage.ReadFileBytes(path, options.MaxBytes)
	if err != nil {
		return nil, path, false, nil, err
	}
	key := storage.FilesCacheKey([]storage.FileInput{{Path: path, Data: data}}, options)
	if ds, ok := cache.Get(context.Background(), key); ok {
		return ds, path, false, nil, nil
	}
	ds, err := storage.ReadCSVBytes(data, options)
	if err != nil {
		return nil, path, false, nil, err
	}
	return inferDataset(ds, cache, key, path, false)
}

func loadManyFiles(paths []string, options storage.CSVOptions, cache *storage.Cache) (*domain.Dataset, string, bool, *pendingCache, error) {
	if cache == nil {
		ds, err := storage.ReadFiles(paths, options)
		if err != nil {
			return nil, paths[0], false, nil, err
		}
		return inferDataset(ds, nil, "", paths[0], false)
	}
	inputs := make([]storage.FileInput, 0, len(paths))
	for _, path := range paths {
		data, err := storage.ReadFileBytes(path, options.MaxBytes)
		if err != nil {
			return nil, paths[0], false, nil, err
		}
		inputs = append(inputs, storage.FileInput{Path: path, Data: data})
	}
	key := storage.FilesCacheKey(inputs, options)
	if ds, ok := cache.Get(context.Background(), key); ok {
		return ds, paths[0], false, nil, nil
	}
	datasets := make([]*domain.Dataset, 0, len(inputs))
	for _, input := range inputs {
		fileOptions := options
		fileOptions.SourceKind = domain.SourceFile
		fileOptions.SourceID = input.Path
		ds, err := storage.ReadCSVBytes(input.Data, fileOptions)
		if err != nil {
			return nil, paths[0], false, nil, err
		}
		datasets = append(datasets, ds)
	}
	combined, err := storage.MergeDatasets(datasets, paths)
	if err != nil {
		return nil, paths[0], false, nil, err
	}
	return inferDataset(combined, cache, key, paths[0], false)
}

func inferDataset(ds *domain.Dataset, cache *storage.Cache, key, sourcePath string, fromStdin bool) (*domain.Dataset, string, bool, *pendingCache, error) {
	if err := domain.Infer(ds); err != nil {
		return nil, sourcePath, fromStdin, nil, err
	}
	var pending *pendingCache
	if cache != nil && key != "" {
		// BuildRenderSpec applies filters/sorts in place; cache the parsed and
		// inferred dataset, not the transformed view.
		pending = &pendingCache{cache: cache, key: key, dataset: domain.CloneDataset(ds)}
	}
	return ds, sourcePath, fromStdin, pending, nil
}

func asAppError(err error, sourcePath string, fromStdin bool) *appError {
	var appErr *appError
	if errors.As(err, &appErr) {
		return appErr
	}
	return mapDomainError(err, sourcePath, fromStdin)
}

func applyConfig(opts *cliOptions, stream appIO) *appError {
	path := strings.TrimSpace(opts.Config)
	explicit := path != ""
	if path == "" {
		path = strings.TrimSpace(stream.Env["PL_CONFIG"])
		explicit = path != ""
	}
	cfg, err := config.Load(path, explicit)
	if err != nil {
		return newAppError(exitUsage, "cli.error.config", map[string]any{
			"Path":   domain.SanitizeText(path),
			"Detail": domain.SanitizeText(err.Error()),
		}, err)
	}
	changed := opts.changed
	if changed == nil {
		changed = map[string]bool{}
	}

	if !changed["db"] && strings.TrimSpace(stream.Env["PL_DB"]) == "" && cfg.Database.Path != "" {
		opts.DB = cfg.Database.Path
	}
	queryOverride := changed["query"] || changed["query-file"] ||
		strings.TrimSpace(stream.Env["PL_QUERY"]) != ""
	if !queryOverride && cfg.Database.Query != "" {
		opts.Query = cfg.Database.Query
	}
	if !queryOverride && cfg.Database.QueryFile != "" {
		opts.QueryFile = cfg.Database.QueryFile
	}
	if !changed["timeout"] {
		timeout, err := cfg.Timeout(opts.Timeout)
		if err != nil {
			return newAppError(exitUsage, "cli.error.config", map[string]any{
				"Path":   domain.SanitizeText(cfg.Path),
				"Detail": domain.SanitizeText(err.Error()),
			}, err)
		}
		opts.Timeout = timeout
	}

	term := strings.ToLower(strings.TrimSpace(stream.Env["TERM"]))
	if !changed["color"] && stream.Env["NO_COLOR"] == "" && term != "dumb" && cfg.Display.Color != "" {
		opts.Color = cfg.Display.Color
	}
	if !changed["ascii"] && cfg.Display.ASCII {
		opts.ASCII = true
	}
	if !opts.TUI {
		if !changed["width"] && cfg.Display.Width != 0 {
			opts.Width = cfg.Display.Width
		}
		if !changed["height"] && cfg.Display.Height != 0 {
			opts.Height = cfg.Display.Height
		}
	}

	if !changed["cache-dir"] {
		if dir := strings.TrimSpace(stream.Env["PL_CACHE_DIR"]); dir != "" {
			opts.CacheDir = dir
		} else if cfg.Cache.Dir != "" {
			opts.CacheDir = cfg.Cache.Dir
		}
	}
	if !changed["no-cache"] {
		if strings.TrimSpace(stream.Env["PL_CACHE_DISABLE"]) != "" {
			opts.NoCache = true
		} else if cfg.Cache.Enabled != nil && !*cfg.Cache.Enabled {
			opts.NoCache = true
		}
	}
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 168 * time.Hour
	}
	if opts.CacheMax == 0 {
		opts.CacheMax = 64 << 20
	}
	if ttl, err := cfg.CacheTTL(opts.CacheTTL); err == nil {
		opts.CacheTTL = ttl
	} else {
		return newAppError(exitUsage, "cli.error.config", map[string]any{
			"Path": domain.SanitizeText(cfg.Path), "Detail": domain.SanitizeText(err.Error()),
		}, err)
	}
	if size, err := cfg.CacheMaxSize(opts.CacheMax); err == nil {
		opts.CacheMax = size
	} else {
		return newAppError(exitUsage, "cli.error.config", map[string]any{
			"Path": domain.SanitizeText(cfg.Path), "Detail": domain.SanitizeText(err.Error()),
		}, err)
	}
	return nil
}
