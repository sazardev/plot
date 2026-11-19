package storage

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/sazardev/plot/internal/domain"
)

// HeaderMode controls first-record handling.
type HeaderMode uint8

const (
	HeaderAuto HeaderMode = iota
	HeaderPresent
	HeaderAbsent
)

// CSVOptions are the delimited-input parser options.
type CSVOptions struct {
	Delimiter  rune // 0 means auto-detect
	HeaderMode HeaderMode
	SourceKind domain.SourceKind
	SourceID   string
	MaxBytes   int64
}

const defaultMaxBytes = 1 << 30 // 1 GiB, SPEC FR-1.

// ReadFile opens path and parses it as delimited text.
func ReadFile(path string, opts CSVOptions) (*domain.Dataset, error) {
	data, err := ReadFileBytes(path, opts.MaxBytes)
	if err != nil {
		return nil, err
	}
	opts.SourceKind = domain.SourceFile
	opts.SourceID = path
	ds, err := ReadCSVBytes(data, opts)
	if err != nil {
		if errors.Is(err, domain.ErrNoData) ||
			errors.Is(err, domain.ErrInputTooLarge) ||
			errors.Is(err, domain.ErrInvalidData) {
			return nil, err
		}
		return nil, &domain.SourceError{Path: path, Err: err}
	}
	ds.SourceCount = 1
	return ds, nil
}

// ReadFileBytes reads a source file with the same size limit as ReadFile.
func ReadFileBytes(path string, maxBytes int64) ([]byte, error) {
	limit := maxBytes
	if limit <= 0 {
		limit = defaultMaxBytes
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, &domain.SourceError{Path: path, Err: err}
	}
	defer func() { _ = file.Close() }()
	if info, statErr := file.Stat(); statErr == nil && info.Size() > limit {
		return nil, &domain.InputTooLargeError{Limit: limit}
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, &domain.SourceError{Path: path, Err: err}
	}
	if int64(len(data)) > limit {
		return nil, &domain.InputTooLargeError{Limit: limit}
	}
	return data, nil
}

// ReadFiles parses several files and concatenates rows when their schemas
// match. The first file owns the schema; every later file must match it.
func ReadFiles(paths []string, opts CSVOptions) (*domain.Dataset, error) {
	if len(paths) == 0 {
		return nil, domain.ErrNoData
	}
	datasets := make([]*domain.Dataset, 0, len(paths))
	for _, path := range paths {
		ds, err := ReadFile(path, opts)
		if err != nil {
			return nil, err
		}
		datasets = append(datasets, ds)
	}
	return MergeDatasets(datasets, paths)
}

// MergeDatasets combines already-parsed files after validating their schema.
func MergeDatasets(datasets []*domain.Dataset, paths []string) (*domain.Dataset, error) {
	if len(datasets) == 0 {
		return nil, domain.ErrNoData
	}
	combined := datasets[0]
	for i := 1; i < len(datasets); i++ {
		next := datasets[i]
		if len(next.Columns) != len(combined.Columns) {
			return nil, &domain.SourceError{
				Path: paths[i],
				Err:  fmt.Errorf("expected %d fields, got %d", len(combined.Columns), len(next.Columns)),
			}
		}
		for column := range combined.Columns {
			if next.Columns[column].Name != combined.Columns[column].Name {
				return nil, &domain.SourceError{
					Path: paths[i],
					Err:  fmt.Errorf("expected column %q, got %q", combined.Columns[column].Name, next.Columns[column].Name),
				}
			}
		}
		combined.Rows = append(combined.Rows, next.Rows...)
		combined.RowCount += next.RowCount
		combined.Warnings = append(combined.Warnings, next.Warnings...)
	}
	combined.SourceKind = domain.SourceFile
	combined.SourceID = paths[0]
	combined.SourceCount = len(paths)
	return combined, nil
}

// ReadCSV parses delimited text from r. It returns text cells only; callers
// run domain.Infer afterwards.
func ReadCSV(r io.Reader, opts CSVOptions) (*domain.Dataset, error) {
	limit := opts.MaxBytes
	if limit <= 0 {
		limit = defaultMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, &domain.InputTooLargeError{Limit: limit}
	}
	return ReadCSVBytes(data, opts)
}

// ReadCSVBytes parses already-read delimited text.
func ReadCSVBytes(data []byte, opts CSVOptions) (*domain.Dataset, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, domain.ErrNoData
	}

	delim := opts.Delimiter
	if delim == 0 {
		delim = detectDelimiter(data)
	}

	records, err := parseRecords(data, delim)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, domain.ErrNoData
	}

	header := false
	switch opts.HeaderMode {
	case HeaderPresent:
		header = true
	case HeaderAbsent:
		header = false
	default:
		header = detectHeader(records)
	}

	width := len(records[0])
	if width == 0 {
		return nil, domain.ErrNoData
	}

	columns := make([]domain.Column, width)
	used := make(map[string]int, width)
	var warnings []domain.Warning
	if header {
		for i := 0; i < width; i++ {
			raw := domain.SanitizeText(strings.TrimSpace(records[0][i]))
			name := uniqueName(raw, i+1, used)
			columns[i] = domain.Column{
				Name:         name,
				OriginalName: raw,
				Index:        i,
				Kind:         domain.ColumnLabel,
				ValueType:    domain.TypeString,
			}
			if name != raw && raw != "" {
				warnings = append(warnings, domain.Warning{
					ID:   "cli.warn.duplicate_header",
					Data: map[string]string{"Name": raw, "New": name},
				})
			}
		}
	} else {
		for i := 0; i < width; i++ {
			columns[i] = domain.Column{
				Name:      fmt.Sprintf("column_%d", i+1),
				Index:     i,
				Kind:      domain.ColumnLabel,
				ValueType: domain.TypeString,
			}
		}
	}

	dataRecords := records
	if header {
		dataRecords = records[1:]
	}
	rows := make([][]domain.Cell, 0, len(dataRecords))
	raggedReported := false
	for i, rec := range dataRecords {
		row := make([]domain.Cell, width)
		for c := 0; c < width; c++ {
			if c < len(rec) {
				row[c] = domain.TextCell(domain.SanitizeText(rec[c]))
			} else {
				row[c] = domain.Null()
			}
		}
		if !raggedReported && len(rec) != width {
			raggedReported = true
			warnings = append(warnings, domain.Warning{
				ID: "cli.warn.ragged_row",
				Data: map[string]string{
					"Line":     strconv.Itoa(i + 1),
					"Expected": strconv.Itoa(width),
					"Got":      strconv.Itoa(len(rec)),
				},
			})
		}
		rows = append(rows, row)
	}

	sourceID := opts.SourceID
	if sourceID == "" {
		sourceID = "stdin"
	}
	sourceKind := opts.SourceKind
	if sourceKind == "" {
		sourceKind = domain.SourceStdin
	}

	return &domain.Dataset{
		Columns:     columns,
		Rows:        rows,
		SourceKind:  sourceKind,
		SourceID:    sourceID,
		SourceCount: 1,
		RowCount:    len(rows),
		Warnings:    warnings,
	}, nil
}

func parseRecords(data []byte, delim rune) ([][]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delim
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false
	records, err := reader.ReadAll()
	if err != nil {
		var parseErr *csv.ParseError
		if errors.As(err, &parseErr) {
			return nil, &domain.ParseError{Line: parseErr.Line, Err: err}
		}
		return nil, &domain.ParseError{Err: err}
	}
	return records, nil
}

func detectDelimiter(data []byte) rune {
	candidates := []rune{',', '\t', ';', '|'}
	best := ','
	bestScore := -1
	for _, candidate := range candidates {
		score := delimiterScore(data, candidate)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}
	return best
}

func delimiterScore(data []byte, delim rune) int {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.Comma = delim
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = false
	reader.ReuseRecord = false

	counts := make(map[int]int)
	for i := 0; i < 20; i++ {
		record, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0
		}
		counts[len(record)]++
	}
	best := 0
	for fields, count := range counts {
		if fields > 1 && count > best {
			best = count
		}
	}
	return best
}

func detectHeader(records [][]string) bool {
	if len(records) == 0 {
		return false
	}
	if len(records) == 1 {
		for _, raw := range records[0] {
			if domain.IsNullToken(raw) {
				continue
			}
			if _, ok := domain.ParseNumber(raw); ok {
				return false
			}
			if _, ok := domain.ParseTime(raw); ok {
				return false
			}
		}
		return true
	}

	width := len(records[0])
	for c := 0; c < width; c++ {
		var numeric, times, total int
		for _, record := range records[1:] {
			if c >= len(record) || domain.IsNullToken(record[c]) {
				continue
			}
			total++
			if _, ok := domain.ParseNumber(record[c]); ok {
				numeric++
				continue
			}
			if _, ok := domain.ParseTime(record[c]); ok {
				times++
			}
		}
		if total == 0 || domain.IsNullToken(records[0][c]) {
			continue
		}
		if numeric*5 >= total*4 {
			if _, ok := domain.ParseNumber(records[0][c]); !ok {
				return true
			}
		} else if times*5 >= total*4 {
			if _, ok := domain.ParseTime(records[0][c]); !ok {
				return true
			}
		}
	}

	seen := make(map[string]struct{}, width)
	for _, raw := range records[0] {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			return true
		}
		seen[name] = struct{}{}
	}
	return false
}

func uniqueName(raw string, position int, used map[string]int) string {
	base := strings.TrimSpace(raw)
	if base == "" {
		base = fmt.Sprintf("column_%d", position)
	}
	if used[base] == 0 {
		used[base] = 1
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s_%d", base, suffix)
		if used[candidate] == 0 {
			used[candidate] = 1
			return candidate
		}
	}
}

// ReadInput reads a bounded input stream and reports the configured limit.
func ReadInput(r io.Reader, maxBytes int64) ([]byte, error) {
	limit := maxBytes
	if limit <= 0 {
		limit = defaultMaxBytes
	}
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if int64(len(data)) > limit {
		return nil, &domain.InputTooLargeError{Limit: limit}
	}
	return data, nil
}
