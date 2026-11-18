package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sazar/plot/internal/domain"
	"github.com/sazar/plot/internal/i18n"
)

const (
	exitOK         = 0
	exitData       = 1
	exitUsage      = 2
	exitSource     = 3
	exitBrokenPipe = 141
)

type appError struct {
	code  int
	id    string
	data  map[string]any
	cause error
}

func (e *appError) Error() string {
	if e == nil {
		return ""
	}
	if e.id != "" {
		return e.id
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	return fmt.Sprintf("exit %d", e.code)
}

func (e *appError) Unwrap() error { return e.cause }

func newAppError(code int, id string, data map[string]any, cause error) *appError {
	return &appError{code: code, id: id, data: data, cause: cause}
}

func usageError(id string, data map[string]any) *appError {
	return newAppError(exitUsage, id, data, nil)
}

func printAppError(w io.Writer, loc *i18n.Localizer, err *appError) {
	if err == nil || err.id == "" {
		return
	}
	fmt.Fprintln(w, domain.SanitizeText(loc.T(err.id, err.data)))
	if err.code == exitUsage {
		fmt.Fprintln(w, loc.T("cli.error.usage_hint", nil))
	}
}

func mapDomainError(err error, sourcePath string, fromStdin bool) *appError {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, domain.ErrNoData):
		if fromStdin || sourcePath == "" {
			return newAppError(exitData, "cli.error.no_data_stdin", nil, err)
		}
		return newAppError(exitData, "cli.error.no_data_file", map[string]any{"Path": sourcePath}, err)

	case errors.Is(err, domain.ErrNoRows):
		return newAppError(exitData, "cli.error.no_rows", nil, err)

	case errors.Is(err, domain.ErrNoNumericColumn):
		var noNumeric *domain.NoNumericColumnError
		if errors.As(err, &noNumeric) {
			columns := strings.Join(noNumeric.Available, ", ")
			if columns == "" {
				return newAppError(exitData, "cli.error.no_numeric_column_none", nil, err)
			}
			return newAppError(exitData, "cli.error.no_numeric_column", map[string]any{"Columns": columns}, err)
		}
		return newAppError(exitData, "cli.error.no_numeric_column_none", nil, err)

	case errors.Is(err, domain.ErrInputTooLarge):
		var tooLarge *domain.InputTooLargeError
		if errors.As(err, &tooLarge) {
			return newAppError(exitData, "cli.error.input_too_large", map[string]any{
				"Limit": strconv.FormatInt(tooLarge.Limit, 10),
			}, err)
		}
		return newAppError(exitData, "cli.error.input_too_large", map[string]any{"Limit": "?"}, err)

	case errors.Is(err, domain.ErrSource):
		var sourceErr *domain.SourceError
		if errors.As(err, &sourceErr) {
			path := sourceErr.Path
			if path == "" {
				path = sourcePath
			}
			return newAppError(exitSource, "cli.error.source", map[string]any{
				"Path":   path,
				"Detail": domain.SanitizeText(sourceErr.Err.Error()),
			}, err)
		}
		return newAppError(exitSource, "cli.error.source", map[string]any{
			"Path":   domain.SanitizeText(sourcePath),
			"Detail": domain.SanitizeText(err.Error()),
		}, err)

	case errors.Is(err, domain.ErrInvalidData):
		var parseErr *domain.ParseError
		if errors.As(err, &parseErr) {
			return newAppError(exitData, "cli.error.parse", map[string]any{"Detail": parseErr.Error()}, err)
		}
		return newAppError(exitData, "cli.error.parse", map[string]any{"Detail": err.Error()}, err)

	case errors.Is(err, domain.ErrUnsupportedChart):
		var chartErr *domain.UnsupportedChartError
		if errors.As(err, &chartErr) {
			return newAppError(exitUsage, "cli.error.unsupported_chart", map[string]any{"Type": chartErr.Type}, err)
		}
		return newAppError(exitUsage, "cli.error.unsupported_chart", map[string]any{"Type": "?"}, err)

	case errors.Is(err, domain.ErrIncompatibleChart):
		var chartErr *domain.IncompatibleChartError
		if errors.As(err, &chartErr) {
			return newAppError(exitUsage, "cli.error.incompatible_chart", map[string]any{
				"Type":   string(chartErr.Type),
				"Reason": chartErr.Reason,
			}, err)
		}
		return newAppError(exitUsage, "cli.error.incompatible_chart", map[string]any{
			"Type":   "?",
			"Reason": err.Error(),
		}, err)

	case errors.Is(err, domain.ErrTooSmall):
		var small *domain.TooSmallError
		if errors.As(err, &small) {
			return newAppError(exitData, "cli.error.too_small", map[string]any{
				"Width":  strconv.Itoa(small.Width),
				"Height": strconv.Itoa(small.Height),
			}, err)
		}
		return newAppError(exitData, "cli.error.too_small", map[string]any{"Width": "20", "Height": "8"}, err)

	case errors.Is(err, domain.ErrTooManyBars):
		var bars *domain.TooManyBarsError
		if errors.As(err, &bars) {
			return newAppError(exitData, "cli.error.too_many_bars", map[string]any{
				"Count": strconv.Itoa(bars.Count),
				"Width": strconv.Itoa(bars.Width),
			}, err)
		}
		return newAppError(exitData, "cli.error.too_many_bars", map[string]any{"Count": "?", "Width": "?"}, err)

	case errors.Is(err, domain.ErrNoValues):
		return newAppError(exitData, "cli.error.no_values", nil, err)

	case errors.Is(err, domain.ErrUsage):
		var usage *domain.UsageError
		if errors.As(err, &usage) {
			return newAppError(exitUsage, "cli.error.usage", map[string]any{"Detail": usage.Detail}, err)
		}
		return newAppError(exitUsage, "cli.error.usage", map[string]any{"Detail": err.Error()}, err)
	}

	return newAppError(exitData, "cli.error.internal", map[string]any{"Detail": err.Error()}, err)
}

func warningData(w domain.Warning) map[string]any {
	if len(w.Data) == 0 {
		return nil
	}
	data := make(map[string]any, len(w.Data))
	for key, value := range w.Data {
		data[key] = value
	}
	return data
}
