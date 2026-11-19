package domain

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ResolveColumn accepts a parsed column name or a 1-based position. Names win
// over positions, as documented in SPEC FR-6.
func ResolveColumn(ds *Dataset, spec string) (int, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return -1, &ColumnNotFoundError{Spec: spec, Available: columnNames(ds)}
	}
	for i := range ds.Columns {
		if ds.Columns[i].Name == spec {
			return i, nil
		}
	}
	if position, err := strconv.Atoi(spec); err == nil && position >= 1 && position <= len(ds.Columns) {
		return position - 1, nil
	}
	return -1, &ColumnNotFoundError{Spec: spec, Available: columnNames(ds)}
}

func resolveColumns(ds *Dataset, specs []string) ([]int, error) {
	indexes := make([]int, 0, len(specs))
	seen := make(map[int]struct{}, len(specs))
	for _, spec := range specs {
		index, err := ResolveColumn(ds, spec)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[index]; exists {
			continue
		}
		seen[index] = struct{}{}
		indexes = append(indexes, index)
	}
	return indexes, nil
}

func displayName(ds *Dataset, renames map[int]string, index int) string {
	if name, ok := renames[index]; ok {
		return name
	}
	return SanitizeText(ds.Columns[index].Name)
}

func parseRenames(ds *Dataset, specs []string) (map[int]string, error) {
	specs = expandRenameSpecs(specs)
	renames := make(map[int]string, len(specs))
	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			return nil, &InvalidRenameError{Detail: "expected old=new"}
		}
		oldSpec := strings.TrimSpace(parts[0])
		newName := SanitizeText(strings.TrimSpace(parts[1]))
		if oldSpec == "" || strings.TrimSpace(newName) == "" {
			return nil, &InvalidRenameError{Detail: "rename needs a column and a non-empty label"}
		}
		index, err := ResolveColumn(ds, oldSpec)
		if err != nil {
			return nil, err
		}
		if _, exists := renames[index]; exists {
			return nil, &InvalidRenameError{Detail: fmt.Sprintf("column %q is renamed more than once", ds.Columns[index].Name)}
		}
		renames[index] = newName
	}

	seen := make(map[string]struct{}, len(renames))
	for _, name := range renames {
		if _, exists := seen[name]; exists {
			return nil, &DuplicateLabelError{Label: name}
		}
		seen[name] = struct{}{}
	}
	return renames, nil
}

func expandRenameSpecs(specs []string) []string {
	out := make([]string, 0, len(specs))
	for _, spec := range specs {
		parts := strings.Split(spec, ",")
		if len(parts) < 2 {
			out = append(out, spec)
			continue
		}
		allAssignments := true
		for _, part := range parts {
			if !strings.Contains(part, "=") {
				allAssignments = false
				break
			}
		}
		if allAssignments {
			out = append(out, parts...)
		} else {
			out = append(out, spec)
		}
	}
	return out
}

type sortKey struct {
	column int
	desc   bool
}

func parseSorts(ds *Dataset, specs []string) ([]sortKey, error) {
	keys := make([]sortKey, 0, len(specs))
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			return nil, &InvalidSortError{Detail: "sort needs a column"}
		}
		columnSpec := spec
		desc := false
		if i := strings.LastIndex(spec, ":"); i > 0 {
			switch strings.ToLower(spec[i+1:]) {
			case "asc":
				columnSpec = spec[:i]
			case "desc":
				columnSpec = spec[:i]
				desc = true
			}
		}
		index, err := ResolveColumn(ds, columnSpec)
		if err != nil {
			if i := strings.LastIndex(spec, ":"); i > 0 {
				if _, prefixErr := ResolveColumn(ds, spec[:i]); prefixErr == nil {
					return nil, &InvalidSortError{Detail: fmt.Sprintf("direction %q must be asc or desc", spec[i+1:])}
				}
			}
			return nil, err
		}
		keys = append(keys, sortKey{column: index, desc: desc})
	}
	return keys, nil
}

func applySorts(ds *Dataset, keys []sortKey) {
	if len(keys) == 0 {
		return
	}
	sort.SliceStable(ds.Rows, func(i, j int) bool {
		for _, key := range keys {
			a, b := ds.Rows[i][key.column], ds.Rows[j][key.column]
			aNull, bNull := a.IsNull(), b.IsNull()
			if aNull || bNull {
				switch {
				case aNull && bNull:
					continue
				case aNull:
					return false
				default:
					return true
				}
			}
			comparison := compareCells(a, b)
			if comparison == 0 {
				continue
			}
			if key.desc {
				return comparison > 0
			}
			return comparison < 0
		}
		return false
	})
}

func compareCells(a, b Cell) int {
	aNull, bNull := a.IsNull(), b.IsNull()
	switch {
	case aNull && bNull:
		return 0
	case aNull:
		return 1
	case bNull:
		return -1
	}
	switch {
	case a.Kind == CellNumber && b.Kind == CellNumber:
		switch {
		case a.Number < b.Number:
			return -1
		case a.Number > b.Number:
			return 1
		default:
			return 0
		}
	case a.Kind == CellTime && b.Kind == CellTime:
		switch {
		case a.Time.Before(b.Time):
			return -1
		case a.Time.After(b.Time):
			return 1
		default:
			return 0
		}
	default:
		return strings.Compare(a.Text, b.Text)
	}
}

type filterOp uint8

const (
	filterEq filterOp = iota
	filterNe
	filterLt
	filterLe
	filterGt
	filterGe
	filterContains
)

type filterValue struct {
	isNull bool
	kind   ValueType
	number float64
	time   time.Time
	text   string
}

type filterClause struct {
	column int
	op     filterOp
	value  filterValue
}

func parseFilters(ds *Dataset, specs []string) ([]filterClause, error) {
	filters := make([]filterClause, 0, len(specs))
	for _, spec := range specs {
		filter, err := parseFilter(ds, spec)
		if err != nil {
			return nil, err
		}
		filters = append(filters, filter)
	}
	return filters, nil
}

func parseFilter(ds *Dataset, spec string) (filterClause, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return filterClause{}, &InvalidFilterError{Detail: "filter expression is empty"}
	}

	columnSpec, valueSpec, op, err := splitFilter(spec)
	if err != nil {
		return filterClause{}, err
	}
	index, err := ResolveColumn(ds, columnSpec)
	if err != nil {
		return filterClause{}, err
	}
	value, err := parseFilterValue(&ds.Columns[index], valueSpec, op)
	if err != nil {
		return filterClause{}, err
	}
	return filterClause{column: index, op: op, value: value}, nil
}

func splitFilter(spec string) (column, value string, op filterOp, err error) {
	fields := strings.Fields(spec)
	for i, field := range fields {
		if !strings.EqualFold(field, "contains") {
			continue
		}
		if i == 0 || i == len(fields)-1 {
			return "", "", 0, &InvalidFilterError{Detail: "contains needs a column and a value"}
		}
		column = strings.Join(fields[:i], " ")
		value = strings.Join(fields[i+1:], " ")
		return column, value, filterContains, nil
	}

	symbolic := []struct {
		text string
		op   filterOp
	}{
		{"!=", filterNe},
		{"<=", filterLe},
		{">=", filterGe},
		{"=", filterEq},
		{"<", filterLt},
		{">", filterGt},
	}
	for i := 0; i < len(spec); i++ {
		for _, candidate := range symbolic {
			if !strings.HasPrefix(spec[i:], candidate.text) {
				continue
			}
			column = strings.TrimSpace(spec[:i])
			value = strings.TrimSpace(spec[i+len(candidate.text):])
			if column == "" || value == "" {
				return "", "", 0, &InvalidFilterError{Detail: "expected column<op>value"}
			}
			return column, value, candidate.op, nil
		}
	}
	return "", "", 0, &InvalidFilterError{Detail: "expected column<op>value"}
}

func parseFilterValue(column *Column, raw string, op filterOp) (filterValue, error) {
	text, quoted, err := unquoteFilterValue(raw)
	if err != nil {
		return filterValue{}, err
	}
	text = SanitizeText(text)

	if op == filterContains {
		if column.Kind == ColumnValue {
			return filterValue{}, &InvalidFilterError{Detail: "contains requires a text or time column"}
		}
		return filterValue{kind: TypeString, text: text}, nil
	}

	if !quoted && strings.EqualFold(strings.TrimSpace(text), "null") {
		if op != filterEq && op != filterNe {
			return filterValue{}, &InvalidFilterError{Detail: "null can only be used with = or !="}
		}
		return filterValue{isNull: true, kind: column.ValueType}, nil
	}

	switch {
	case column.Kind == ColumnValue || column.ValueType == TypeInt || column.ValueType == TypeFloat:
		number, ok := ParseNumber(text)
		if !ok {
			return filterValue{}, &InvalidFilterError{Detail: fmt.Sprintf("%q is not a number", text)}
		}
		return filterValue{kind: TypeFloat, number: number}, nil
	case column.ValueType == TypeTime:
		timestamp, ok := ParseTime(text)
		if !ok {
			return filterValue{}, &InvalidFilterError{Detail: fmt.Sprintf("%q is not a time", text)}
		}
		return filterValue{kind: TypeTime, time: timestamp}, nil
	case column.ValueType == TypeBool:
		normalized, ok := IsBoolToken(text)
		if !ok {
			return filterValue{}, &InvalidFilterError{Detail: fmt.Sprintf("%q is not a boolean", text)}
		}
		return filterValue{kind: TypeBool, text: normalized}, nil
	default:
		return filterValue{kind: TypeString, text: text}, nil
	}
}

func unquoteFilterValue(raw string) (string, bool, error) {
	if raw == "" {
		return "", false, &InvalidFilterError{Detail: "filter value is empty"}
	}
	switch raw[0] {
	case '"':
		if len(raw) < 2 || raw[len(raw)-1] != '"' {
			return "", false, &InvalidFilterError{Detail: "unterminated quoted value"}
		}
		value, err := strconv.Unquote(raw)
		if err != nil {
			return "", false, &InvalidFilterError{Detail: "invalid quoted value"}
		}
		return value, true, nil
	case '\'':
		if len(raw) < 2 || raw[len(raw)-1] != '\'' {
			return "", false, &InvalidFilterError{Detail: "unterminated quoted value"}
		}
		value := raw[1 : len(raw)-1]
		value = strings.ReplaceAll(value, `\'`, `'`)
		value = strings.ReplaceAll(value, `\\`, `\`)
		return value, true, nil
	default:
		return raw, false, nil
	}
}

func applyFilters(ds *Dataset, filters []filterClause) error {
	if len(filters) == 0 {
		return nil
	}
	kept := make([][]Cell, 0, len(ds.Rows))
	for _, row := range ds.Rows {
		matches := true
		for _, filter := range filters {
			if !filter.matches(row) {
				matches = false
				break
			}
		}
		if matches {
			kept = append(kept, row)
		}
	}
	if len(kept) == 0 {
		return &NoRowsAfterFilterError{Filters: len(filters)}
	}
	ds.Rows = kept
	ds.RowCount = len(kept)
	return nil
}

func (f filterClause) matches(row []Cell) bool {
	if f.column < 0 || f.column >= len(row) {
		return false
	}
	cell := row[f.column]

	if f.op == filterContains {
		if cell.IsNull() {
			return false
		}
		switch cell.Kind {
		case CellTime:
			return strings.Contains(formatTime(cell.Time), f.value.text)
		case CellText, CellBool:
			return strings.Contains(cell.Text, f.value.text)
		default:
			return false
		}
	}

	if f.value.isNull {
		switch f.op {
		case filterEq:
			return cell.IsNull()
		case filterNe:
			return !cell.IsNull()
		default:
			return false
		}
	}
	if cell.IsNull() {
		return false
	}

	switch f.value.kind {
	case TypeInt, TypeFloat:
		if cell.Kind != CellNumber {
			return false
		}
		return compareOrdered(cell.Number, f.value.number, f.op)
	case TypeTime:
		if cell.Kind != CellTime {
			return false
		}
		return compareTime(cell.Time, f.value.time, f.op)
	case TypeBool, TypeString:
		return compareText(cell.Text, f.value.text, f.op)
	default:
		return false
	}
}

func compareOrdered(a, b float64, op filterOp) bool {
	switch op {
	case filterEq:
		return a == b
	case filterNe:
		return a != b
	case filterLt:
		return a < b
	case filterLe:
		return a <= b
	case filterGt:
		return a > b
	case filterGe:
		return a >= b
	default:
		return false
	}
}

func compareTime(a, b time.Time, op filterOp) bool {
	switch op {
	case filterEq:
		return a.Equal(b)
	case filterNe:
		return !a.Equal(b)
	case filterLt:
		return a.Before(b)
	case filterLe:
		return a.Before(b) || a.Equal(b)
	case filterGt:
		return a.After(b)
	case filterGe:
		return a.After(b) || a.Equal(b)
	default:
		return false
	}
}

func compareText(a, b string, op filterOp) bool {
	switch op {
	case filterEq:
		return a == b
	case filterNe:
		return a != b
	case filterLt:
		return a < b
	case filterLe:
		return a <= b
	case filterGt:
		return a > b
	case filterGe:
		return a >= b
	default:
		return false
	}
}

type selection struct {
	labelIdx    int
	synthetic   bool
	labelKind   LabelKind
	labelName   string
	seriesIdxs  []int
	seriesNames []string
}

func resolveSelection(ds *Dataset, opts BuildOptions, renames map[int]string) (*selection, error) {
	valueIdxs := make([]int, 0, len(ds.Columns))
	labelIdxs := make([]int, 0, len(ds.Columns))
	for i := range ds.Columns {
		switch {
		case ds.Columns[i].Kind == ColumnValue && ds.Columns[i].ValueType != TypeEmpty:
			valueIdxs = append(valueIdxs, i)
		case ds.Columns[i].Kind == ColumnLabel && ds.Columns[i].ValueType != TypeEmpty:
			labelIdxs = append(labelIdxs, i)
		}
	}

	explicitLabel := -1
	if strings.TrimSpace(opts.Label) != "" {
		index, err := ResolveColumn(ds, opts.Label)
		if err != nil {
			return nil, err
		}
		if ds.Columns[index].ValueType == TypeEmpty {
			return nil, &UsageError{Detail: fmt.Sprintf("column %q is empty and cannot be the label", ds.Columns[index].Name)}
		}
		explicitLabel = index
	}
	explicitColumns, err := resolveColumns(ds, opts.Columns)
	if err != nil {
		return nil, err
	}

	if len(valueIdxs) == 0 {
		return nil, &NoNumericColumnError{Available: columnNames(ds)}
	}

	inferredLabel := -1
	if len(labelIdxs) > 0 {
		inferredLabel = labelIdxs[0]
	}

	labelIdx := -1
	synthetic := false
	switch {
	case explicitLabel >= 0:
		labelIdx = explicitLabel
	case inferredLabel >= 0:
		labelIdx = inferredLabel
	case len(explicitColumns) > 0 && containsInt(explicitColumns, valueIdxs[0]):
		synthetic = true
	case len(explicitColumns) > 0:
		labelIdx = valueIdxs[0]
	case len(valueIdxs) == 1:
		synthetic = true
	default:
		labelIdx = valueIdxs[0]
	}

	if labelIdx >= 0 && containsInt(explicitColumns, labelIdx) {
		return nil, &UsageError{Detail: fmt.Sprintf("column %q is the label; remove it from --columns or use --label", ds.Columns[labelIdx].Name)}
	}

	var seriesIdxs []int
	if len(explicitColumns) > 0 {
		if len(explicitColumns) > opts.MaxSeries {
			return nil, &TooManySeriesError{Chosen: len(explicitColumns), Max: opts.MaxSeries}
		}
		for _, index := range explicitColumns {
			column := &ds.Columns[index]
			if column.Kind != ColumnValue || column.ValueType == TypeEmpty {
				return nil, &ColumnNotNumericError{Column: column.Name}
			}
			if index == labelIdx {
				return nil, &UsageError{Detail: fmt.Sprintf("column %q is the label and cannot be a series", column.Name)}
			}
			seriesIdxs = append(seriesIdxs, index)
		}
	} else {
		for _, index := range valueIdxs {
			if index == labelIdx {
				continue
			}
			seriesIdxs = append(seriesIdxs, index)
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
	}

	if len(seriesIdxs) == 0 {
		return nil, &NoNumericColumnError{Available: columnNames(ds)}
	}

	labelKind := LabelNumeric
	labelName := "row"
	if !synthetic {
		labelName = displayName(ds, renames, labelIdx)
		switch ds.Columns[labelIdx].ValueType {
		case TypeInt, TypeFloat:
			labelKind = LabelNumeric
		case TypeTime:
			labelKind = LabelTime
		default:
			labelKind = LabelCategorical
		}
	}

	if explicitLabel < 0 && len(labelIdxs) > 1 {
		names := make([]string, 0, len(labelIdxs))
		for _, index := range labelIdxs {
			names = append(names, displayName(ds, renames, index))
		}
		used := labelName
		ds.Warnings = append(ds.Warnings, Warning{
			ID: "cli.warn.multiple_labels",
			Data: map[string]string{
				"Columns": strings.Join(names, ", "),
				"Used":    used,
			},
		})
	}

	seriesNames := make([]string, 0, len(seriesIdxs))
	for _, index := range seriesIdxs {
		seriesNames = append(seriesNames, displayName(ds, renames, index))
	}

	seen := map[string]struct{}{labelName: {}}
	for _, name := range seriesNames {
		if _, exists := seen[name]; exists {
			return nil, &DuplicateLabelError{Label: name}
		}
		seen[name] = struct{}{}
	}

	return &selection{
		labelIdx:    labelIdx,
		synthetic:   synthetic,
		labelKind:   labelKind,
		labelName:   labelName,
		seriesIdxs:  seriesIdxs,
		seriesNames: seriesNames,
	}, nil
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
