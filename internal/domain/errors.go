package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors let the CLI map failures to exit codes and localized
// messages in one place.
var (
	ErrNoData            = errors.New("no data")
	ErrNoRows            = errors.New("no data rows")
	ErrNoNumericColumn   = errors.New("no numeric column")
	ErrNoValues          = errors.New("no plottable values")
	ErrUsage             = errors.New("usage")
	ErrSource            = errors.New("source")
	ErrInputTooLarge     = errors.New("input too large")
	ErrInvalidData       = errors.New("invalid data")
	ErrIncompatibleChart = errors.New("incompatible chart")
	ErrUnsupportedChart  = errors.New("unsupported chart")
	ErrTooSmall          = errors.New("terminal too small")
	ErrTooManyBars       = errors.New("too many bars")
	ErrColumnNotFound    = errors.New("column not found")
	ErrInvalidFilter     = errors.New("invalid filter")
	ErrInvalidSort       = errors.New("invalid sort")
	ErrInvalidRename     = errors.New("invalid rename")
	ErrDuplicateLabel    = errors.New("duplicate display label")
	ErrTooManySeries     = errors.New("too many series")
	ErrNoRowsAfterFilter = errors.New("no rows after filter")
	ErrColumnNotNumeric  = errors.New("column is not numeric")
)

// NoNumericColumnError lists the columns the user could have selected.
type NoNumericColumnError struct {
	Available []string
}

func (e *NoNumericColumnError) Error() string {
	if len(e.Available) == 0 {
		return "no numeric column"
	}
	return fmt.Sprintf("no numeric column; available: %s", strings.Join(e.Available, ", "))
}

func (e *NoNumericColumnError) Unwrap() error { return ErrNoNumericColumn }

// UsageError is a bad invocation rather than a data or source failure.
type UsageError struct {
	Detail string
}

func (e *UsageError) Error() string {
	if e.Detail == "" {
		return "usage error"
	}
	return e.Detail
}

func (e *UsageError) Unwrap() error { return ErrUsage }

// SourceError wraps a failure to open or read an input source.
type SourceError struct {
	Path string
	Err  error
}

func (e *SourceError) Error() string {
	if e.Path == "" {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

func (e *SourceError) Unwrap() []error { return []error{ErrSource, e.Err} }

// ParseError identifies a malformed input record.
type ParseError struct {
	Line int
	Err  error
}

func (e *ParseError) Error() string {
	if e.Line <= 0 {
		return e.Err.Error()
	}
	return fmt.Sprintf("line %d: %v", e.Line, e.Err)
}

func (e *ParseError) Unwrap() []error { return []error{ErrInvalidData, e.Err} }

// InputTooLargeError reports the configured input limit.
type InputTooLargeError struct {
	Limit int64
}

func (e *InputTooLargeError) Error() string {
	return fmt.Sprintf("input exceeds %d bytes", e.Limit)
}

func (e *InputTooLargeError) Unwrap() error { return ErrInputTooLarge }

// IncompatibleChartError explains why a chart type cannot render the data.
type IncompatibleChartError struct {
	Type   ChartType
	Reason string
}

func (e *IncompatibleChartError) Error() string {
	return fmt.Sprintf("%s: %s", e.Type, e.Reason)
}

func (e *IncompatibleChartError) Unwrap() error { return ErrIncompatibleChart }

// UnsupportedChartError reports a valid chart type that is not implemented yet.
type UnsupportedChartError struct {
	Type string
}

func (e *UnsupportedChartError) Error() string {
	return fmt.Sprintf("chart type %q is not implemented yet", e.Type)
}

func (e *UnsupportedChartError) Unwrap() error { return ErrUnsupportedChart }

// TooSmallError reports a terminal or requested size below the chart minimum.
type TooSmallError struct {
	Width  int
	Height int
}

func (e *TooSmallError) Error() string {
	return fmt.Sprintf("terminal too small: %dx%d", e.Width, e.Height)
}

func (e *TooSmallError) Unwrap() error { return ErrTooSmall }

// TooManyBarsError reports that the rows cannot fit the chart width.
type TooManyBarsError struct {
	Count int
	Width int
}

func (e *TooManyBarsError) Error() string {
	return fmt.Sprintf("too many bars: %d rows for width %d", e.Count, e.Width)
}

func (e *TooManyBarsError) Unwrap() error { return ErrTooManyBars }

// ColumnNotFoundError lists the columns available for a reference.
type ColumnNotFoundError struct {
	Spec      string
	Available []string
}

func (e *ColumnNotFoundError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("column %q not found", e.Spec)
	}
	return fmt.Sprintf("column %q not found; available: %s", e.Spec, strings.Join(e.Available, ", "))
}

func (e *ColumnNotFoundError) Unwrap() error { return ErrColumnNotFound }

// InvalidFilterError reports a malformed filter expression or value.
type InvalidFilterError struct {
	Detail string
}

func (e *InvalidFilterError) Error() string { return e.Detail }
func (e *InvalidFilterError) Unwrap() error { return ErrInvalidFilter }

// InvalidSortError reports a malformed sort expression.
type InvalidSortError struct {
	Detail string
}

func (e *InvalidSortError) Error() string { return e.Detail }
func (e *InvalidSortError) Unwrap() error { return ErrInvalidSort }

// InvalidRenameError reports a malformed rename expression.
type InvalidRenameError struct {
	Detail string
}

func (e *InvalidRenameError) Error() string { return e.Detail }
func (e *InvalidRenameError) Unwrap() error { return ErrInvalidRename }

// DuplicateLabelError reports two plotted columns with the same display name.
type DuplicateLabelError struct {
	Label string
}

func (e *DuplicateLabelError) Error() string {
	return fmt.Sprintf("duplicate display label %q", e.Label)
}

func (e *DuplicateLabelError) Unwrap() error { return ErrDuplicateLabel }

// TooManySeriesError reports an explicit selection beyond the chart limit.
type TooManySeriesError struct {
	Chosen int
	Max    int
}

func (e *TooManySeriesError) Error() string {
	return fmt.Sprintf("selected %d series; maximum is %d", e.Chosen, e.Max)
}

func (e *TooManySeriesError) Unwrap() error { return ErrTooManySeries }

// NoRowsAfterFilterError reports a filter that removed every row.
type NoRowsAfterFilterError struct {
	Filters int
}

func (e *NoRowsAfterFilterError) Error() string {
	return fmt.Sprintf("no rows after %d filter(s)", e.Filters)
}

func (e *NoRowsAfterFilterError) Unwrap() error { return ErrNoRowsAfterFilter }

// ColumnNotNumericError reports a --columns entry that is not a value column.
type ColumnNotNumericError struct {
	Column string
}

func (e *ColumnNotNumericError) Error() string {
	return fmt.Sprintf("column %q is not numeric", e.Column)
}

func (e *ColumnNotNumericError) Unwrap() error { return ErrColumnNotNumeric }

// DatabaseError wraps a SQLite/driver failure. The CLI prints Err verbatim,
// as required by SPEC FR-4 and AGENTS.md.
type DatabaseError struct {
	Err error
}

func (e *DatabaseError) Error() string {
	if e == nil || e.Err == nil {
		return "database error"
	}
	return e.Err.Error()
}

func (e *DatabaseError) Unwrap() []error { return []error{ErrSource, e.Err} }

// DatabaseTimeoutError reports a query that exceeded --timeout.
type DatabaseTimeoutError struct {
	Timeout time.Duration
}

func (e *DatabaseTimeoutError) Error() string {
	return fmt.Sprintf("query timed out after %s", e.Timeout)
}

func (e *DatabaseTimeoutError) Unwrap() error { return ErrSource }
