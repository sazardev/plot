package domain

import (
	"fmt"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Null tokens accepted by inference. A bare "-" is treated as missing rather
// than as a malformed number.
func IsNullToken(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "n/a", "na", "null", "nil", "nan", "inf", "-":
		return true
	default:
		return false
	}
}

// ParseNumber parses a finite decimal or scientific number. NaN and infinities
// are not plot values.
func ParseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

var timeLayouts = []string{
	time.RFC3339,
	"2006-01-02",
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"15:04",
	"15:04:05",
}

// ParseTime accepts the ISO-like layouts documented in SPEC FR-5. Naive
// layouts use the local timezone; the timezone is part of NFR-5's inputs.
func ParseTime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range timeLayouts {
		var (
			t   time.Time
			err error
		)
		if layout == time.RFC3339 {
			t, err = time.Parse(layout, s)
		} else {
			t, err = time.ParseInLocation(layout, s, time.Local)
		}
		if err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// IsBoolToken recognizes label-like booleans. They are labels in v1.
func IsBoolToken(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "false", "yes", "no", "on", "off":
		return strings.ToLower(strings.TrimSpace(s)), true
	default:
		return "", false
	}
}

type columnStats struct {
	nonNull int
	numeric int
	time    int
	boolean int
	allInt  bool
}

// Infer classifies every column and converts its cells to typed values. It is
// idempotent enough to run once per parsed dataset.
func Infer(ds *Dataset) error {
	if ds == nil || len(ds.Columns) == 0 {
		return ErrNoData
	}

	for i := range ds.Columns {
		stats := analyzeColumn(ds, i)
		col := &ds.Columns[i]
		col.NullCount = 0
		col.CoercedNullCount = 0
		col.DistinctCount = 0
		col.Kind = ColumnLabel

		switch {
		case stats.nonNull == 0:
			col.ValueType = TypeEmpty
			col.Kind = ColumnLabel
			ds.Warnings = append(ds.Warnings, Warning{
				ID:   "cli.warn.empty_column",
				Data: map[string]string{"Name": col.Name},
			})
		case stats.numeric > 0 && float64(stats.numeric)/float64(stats.nonNull) >= 0.8:
			col.Kind = ColumnValue
			if stats.allInt {
				col.ValueType = TypeInt
			} else {
				col.ValueType = TypeFloat
			}
		case stats.time > 0 && float64(stats.time)/float64(stats.nonNull) >= 0.8:
			col.ValueType = TypeTime
		case stats.boolean == stats.nonNull:
			col.ValueType = TypeBool
		default:
			col.ValueType = TypeString
		}

		normalizeColumn(ds, i)
	}

	return nil
}

func analyzeColumn(ds *Dataset, index int) columnStats {
	stats := columnStats{allInt: true}
	for _, row := range ds.Rows {
		if index >= len(row) {
			continue
		}
		cell := row[index]
		switch cell.Kind {
		case CellNull:
			continue
		case CellNumber:
			stats.nonNull++
			stats.numeric++
			if math.Trunc(cell.Number) != cell.Number || math.Abs(cell.Number) > 1e15 {
				stats.allInt = false
			}
			continue
		case CellTime:
			stats.nonNull++
			stats.time++
			continue
		case CellBool:
			stats.nonNull++
			stats.boolean++
			continue
		case CellText:
			raw := cell.Text
			if IsNullToken(raw) {
				continue
			}
			stats.nonNull++
			if v, ok := ParseNumber(raw); ok {
				stats.numeric++
				if math.Trunc(v) != v || math.Abs(v) > 1e15 {
					stats.allInt = false
				}
				continue
			}
			if _, ok := ParseTime(raw); ok {
				stats.time++
				continue
			}
			if _, ok := IsBoolToken(raw); ok {
				stats.boolean++
			}
		}
	}
	return stats
}

func normalizeColumn(ds *Dataset, index int) {
	col := &ds.Columns[index]
	distinct := make(map[string]struct{})

	for r := range ds.Rows {
		if index >= len(ds.Rows[r]) {
			continue
		}
		cell := ds.Rows[r][index]
		switch cell.Kind {
		case CellNull:
			col.NullCount++
			continue
		case CellNumber:
			if col.Kind == ColumnValue {
				distinct[formatNumber(cell.Number)] = struct{}{}
				continue
			}
		case CellTime:
			if col.Kind == ColumnLabel && col.ValueType == TypeTime {
				distinct[formatTime(cell.Time)] = struct{}{}
				continue
			}
		case CellBool:
			if col.Kind == ColumnLabel && col.ValueType == TypeBool {
				distinct[cell.Text] = struct{}{}
				continue
			}
		}
		raw := cell.Text
		if IsNullToken(raw) {
			ds.Rows[r][index] = Null()
			col.NullCount++
			continue
		}

		switch col.Kind {
		case ColumnValue:
			if v, ok := ParseNumber(raw); ok {
				ds.Rows[r][index] = NumberCell(v)
				distinct[formatNumber(v)] = struct{}{}
			} else {
				ds.Rows[r][index] = Null()
				col.NullCount++
				col.CoercedNullCount++
			}
		case ColumnLabel:
			switch col.ValueType {
			case TypeTime:
				if t, ok := ParseTime(raw); ok {
					ds.Rows[r][index] = TimeCell(t)
					distinct[formatTime(t)] = struct{}{}
				} else {
					ds.Rows[r][index] = Null()
					col.NullCount++
					col.CoercedNullCount++
				}
			case TypeBool:
				if b, ok := IsBoolToken(raw); ok {
					ds.Rows[r][index] = Cell{Kind: CellBool, Text: b}
					distinct[b] = struct{}{}
				} else {
					ds.Rows[r][index] = Null()
					col.NullCount++
					col.CoercedNullCount++
				}
			default:
				distinct[raw] = struct{}{}
			}
		}
	}
	col.DistinctCount = len(distinct)
}

func formatNumber(v float64) string {
	if v == 0 {
		return "0"
	}
	if math.Trunc(v) == v && math.Abs(v) < 1e15 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

func formatTime(t time.Time) string {
	if t.Year() == 0 {
		return t.Format("15:04")
	}
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format("2006-01-02 15:04")
}

// BuildRenderSpec resolves label, series and chart type. It appends warnings
// to the dataset; the CLI/TUI localizes them.
func BuildRenderSpec(ds *Dataset, opts BuildOptions) (*RenderSpec, error) {
	if ds == nil || len(ds.Columns) == 0 {
		return nil, ErrNoData
	}
	if len(ds.Rows) == 0 {
		return nil, ErrNoRows
	}
	if opts.MaxSeries <= 0 {
		opts.MaxSeries = 8
	}
	if opts.Width <= 0 {
		opts.Width = 80
	}
	if opts.Height <= 0 {
		opts.Height = 24
	}
	if opts.Locale == "" {
		opts.Locale = "en"
	}
	if opts.ColorMode == "" {
		opts.ColorMode = ColorAuto
	}
	if opts.GlyphMode == "" {
		opts.GlyphMode = GlyphAuto
	}

	labelIdx := -1
	var labelCols []string
	for i := range ds.Columns {
		if ds.Columns[i].Kind == ColumnLabel && ds.Columns[i].ValueType != TypeEmpty {
			labelCols = append(labelCols, ds.Columns[i].Name)
			if labelIdx == -1 {
				labelIdx = i
			}
		}
	}

	valueIdxs := make([]int, 0, len(ds.Columns))
	for i := range ds.Columns {
		if ds.Columns[i].Kind == ColumnValue && ds.Columns[i].ValueType != TypeEmpty {
			valueIdxs = append(valueIdxs, i)
		}
	}
	if len(valueIdxs) == 0 {
		return nil, &NoNumericColumnError{Available: columnNames(ds)}
	}

	syntheticRow := false
	if labelIdx == -1 {
		switch len(valueIdxs) {
		case 1:
			syntheticRow = true
		default:
			labelIdx = valueIdxs[0]
			valueIdxs = valueIdxs[1:]
		}
	}

	seriesIdxs := make([]int, 0, len(valueIdxs))
	for _, idx := range valueIdxs {
		if idx != labelIdx {
			seriesIdxs = append(seriesIdxs, idx)
		}
	}
	if len(seriesIdxs) == 0 {
		return nil, &NoNumericColumnError{Available: columnNames(ds)}
	}

	if len(labelCols) > 1 {
		ds.Warnings = append(ds.Warnings, Warning{
			ID: "cli.warn.multiple_labels",
			Data: map[string]string{
				"Columns": strings.Join(labelCols, ", "),
				"Used":    ds.Columns[labelIdx].Name,
			},
		})
	}

	if len(seriesIdxs) > opts.MaxSeries {
		total := len(seriesIdxs)
		seriesIdxs = seriesIdxs[:opts.MaxSeries]
		ds.Warnings = append(ds.Warnings, Warning{
			ID: "cli.warn.series_truncated",
			Data: map[string]string{
				"Shown": strconv.Itoa(opts.MaxSeries),
				"Total": strconv.Itoa(total),
			},
		})
	}

	labelKind := LabelCategorical
	labelName := "row"
	if !syntheticRow {
		labelName = SanitizeText(ds.Columns[labelIdx].Name)
		switch ds.Columns[labelIdx].ValueType {
		case TypeInt, TypeFloat:
			labelKind = LabelNumeric
		case TypeTime:
			labelKind = LabelTime
		default:
			labelKind = LabelCategorical
		}
	} else {
		labelKind = LabelNumeric
	}

	labels := make([]string, len(ds.Rows))
	xValues := make([]float64, len(ds.Rows))
	for r := range ds.Rows {
		if syntheticRow {
			labels[r] = strconv.Itoa(r + 1)
			xValues[r] = float64(r + 1)
			continue
		}
		cell := ds.Rows[r][labelIdx]
		switch {
		case cell.Kind == CellNumber:
			labels[r] = SanitizeText(formatNumber(cell.Number))
			xValues[r] = cell.Number
		case cell.Kind == CellTime:
			labels[r] = SanitizeText(formatTime(cell.Time))
			xValues[r] = float64(cell.Time.UnixNano()) / 1e9
		case cell.Kind == CellNull:
			labels[r] = ""
			xValues[r] = float64(r)
		default:
			labels[r] = SanitizeText(cell.Text)
			xValues[r] = float64(r)
		}
	}

	series := make([]Series, 0, len(seriesIdxs))
	for n, idx := range seriesIdxs {
		points := make([]Point, len(ds.Rows))
		for r := range ds.Rows {
			cell := ds.Rows[r][idx]
			if cell.Kind != CellNumber {
				points[r] = Point{X: xValues[r], Null: true}
				continue
			}
			points[r] = Point{X: xValues[r], Y: cell.Number}
		}
		series = append(series, Series{
			Column:    idx,
			Label:     SanitizeText(ds.Columns[idx].Name),
			Points:    points,
			ColorSlot: n % 8,
			Marker:    n % 8,
		})
	}

	chartType, err := resolveChartType(opts.ChartType, series, labelKind, ds.RowCount, opts.Width)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.ChartType) == "" && chartType == ChartLine && len(series) == 1 && labelKind == LabelCategorical {
		ds.Warnings = append(ds.Warnings, Warning{
			ID:   "cli.warn.ordinal_axis",
			Data: map[string]string{"Rows": strconv.Itoa(ds.RowCount)},
		})
	}

	title := opts.Title
	if strings.TrimSpace(title) == "" {
		sep := " — "
		if opts.GlyphMode == GlyphASCII {
			sep = " - "
		}
		title = SanitizeText(sourceTitle(ds) + sep + string(chartType))
	}

	return &RenderSpec{
		ChartType:   chartType,
		LabelColumn: labelIdx,
		LabelName:   labelName,
		LabelKind:   labelKind,
		Labels:      labels,
		Series:      series,
		Width:       opts.Width,
		Height:      opts.Height,
		Title:       title,
		ColorMode:   opts.ColorMode,
		GlyphMode:   opts.GlyphMode,
		Locale:      opts.Locale,
	}, nil
}

func resolveChartType(requested string, series []Series, labelKind LabelKind, rows, width int) (ChartType, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	switch requested {
	case "":
		if len(series) == 1 && labelKind == LabelCategorical && rows <= 30 && rows <= width-8 {
			return ChartBar, nil
		}
		return ChartLine, nil
	case "line", "plot":
		return ChartLine, nil
	case "bar", "graph":
		if len(series) != 1 {
			return "", &IncompatibleChartError{Type: ChartBar, Reason: "bar charts need exactly one value series"}
		}
		if rows > width-8 {
			return "", &TooManyBarsError{Count: rows, Width: width}
		}
		return ChartBar, nil
	case "pie", "donut":
		return "", &UnsupportedChartError{Type: requested}
	default:
		return "", &UsageError{Detail: fmt.Sprintf("unknown chart type %q; use line or bar", requested)}
	}
}

func sourceTitle(ds *Dataset) string {
	switch ds.SourceKind {
	case SourceFile:
		if base := filepath.Base(ds.SourceID); base != "" && base != "." {
			return base
		}
		return "file"
	case SourceDB:
		if base := filepath.Base(ds.SourceID); base != "" && base != "." {
			return base
		}
		return "database"
	case SourceStdin, "":
		return "stdin"
	default:
		return string(ds.SourceKind)
	}
}

func columnNames(ds *Dataset) []string {
	names := make([]string, 0, len(ds.Columns))
	for _, c := range ds.Columns {
		names = append(names, c.Name)
	}
	return names
}
