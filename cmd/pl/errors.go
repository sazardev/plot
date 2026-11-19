package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/sazardev/plot/internal/domain"
	"github.com/sazardev/plot/internal/i18n"
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
	_, _ = fmt.Fprintln(w, domain.SanitizeText(loc.T(err.id, err.data)))
	if err.code == exitUsage {
		_, _ = fmt.Fprintln(w, loc.T("cli.error.usage_hint", nil))
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
		var dbErr *domain.DatabaseError
		if errors.As(err, &dbErr) {
			return newAppError(exitSource, "", nil, dbErr.Err)
		}
		var timeout *domain.DatabaseTimeoutError
		if errors.As(err, &timeout) {
			return newAppError(exitSource, "cli.error.db_timeout", map[string]any{
				"Timeout": timeout.Timeout.String(),
			}, err)
		}
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

	case errors.Is(err, domain.ErrColumnNotFound):
		var notFound *domain.ColumnNotFoundError
		if errors.As(err, &notFound) {
			return newAppError(exitUsage, "cli.error.column_not_found", map[string]any{
				"Column":    domain.SanitizeText(notFound.Spec),
				"Available": domain.SanitizeText(strings.Join(notFound.Available, ", ")),
			}, err)
		}
		return newAppError(exitUsage, "cli.error.column_not_found", map[string]any{
			"Column": "?", "Available": "?",
		}, err)

	case errors.Is(err, domain.ErrColumnNotNumeric):
		var notNumeric *domain.ColumnNotNumericError
		if errors.As(err, &notNumeric) {
			return newAppError(exitUsage, "cli.error.column_not_numeric", map[string]any{
				"Column": domain.SanitizeText(notNumeric.Column),
			}, err)
		}
		return newAppError(exitUsage, "cli.error.column_not_numeric", map[string]any{"Column": "?"}, err)

	case errors.Is(err, domain.ErrInvalidFilter):
		var invalid *domain.InvalidFilterError
		if errors.As(err, &invalid) {
			return newAppError(exitUsage, "cli.error.filter", map[string]any{
				"Detail": domain.SanitizeText(invalid.Detail),
			}, err)
		}
		return newAppError(exitUsage, "cli.error.filter", map[string]any{"Detail": err.Error()}, err)

	case errors.Is(err, domain.ErrInvalidSort):
		var invalid *domain.InvalidSortError
		if errors.As(err, &invalid) {
			return newAppError(exitUsage, "cli.error.sort", map[string]any{
				"Detail": domain.SanitizeText(invalid.Detail),
			}, err)
		}
		return newAppError(exitUsage, "cli.error.sort", map[string]any{"Detail": err.Error()}, err)

	case errors.Is(err, domain.ErrInvalidRename):
		var invalid *domain.InvalidRenameError
		if errors.As(err, &invalid) {
			return newAppError(exitUsage, "cli.error.rename", map[string]any{
				"Detail": domain.SanitizeText(invalid.Detail),
			}, err)
		}
		return newAppError(exitUsage, "cli.error.rename", map[string]any{"Detail": err.Error()}, err)

	case errors.Is(err, domain.ErrDuplicateLabel):
		var duplicate *domain.DuplicateLabelError
		if errors.As(err, &duplicate) {
			return newAppError(exitUsage, "cli.error.duplicate_label", map[string]any{
				"Label": domain.SanitizeText(duplicate.Label),
			}, err)
		}
		return newAppError(exitUsage, "cli.error.duplicate_label", map[string]any{"Label": "?"}, err)

	case errors.Is(err, domain.ErrTooManySeries):
		var tooMany *domain.TooManySeriesError
		if errors.As(err, &tooMany) {
			return newAppError(exitUsage, "cli.error.too_many_series", map[string]any{
				"Chosen": strconv.Itoa(tooMany.Chosen),
				"Max":    strconv.Itoa(tooMany.Max),
			}, err)
		}
		return newAppError(exitUsage, "cli.error.too_many_series", map[string]any{"Chosen": "?", "Max": "?"}, err)

	case errors.Is(err, domain.ErrNoRowsAfterFilter):
		return newAppError(exitData, "cli.error.no_rows_after_filter", nil, err)

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
