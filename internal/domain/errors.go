package domain

import (
	"errors"
	"fmt"
	"strings"
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
