package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sazar/plot/internal/domain"
)

// SQLiteOptions selects and runs the read-only database source.
type SQLiteOptions struct {
	Path      string
	Query     string
	QueryFile string
	Timeout   time.Duration
}

const defaultQueryTimeout = 10 * time.Second

// ReadSQLite executes one read-only query and returns a dataset in the same
// shape as CSV input.
func ReadSQLite(ctx context.Context, opts SQLiteOptions) (*domain.Dataset, error) {
	if strings.TrimSpace(opts.Path) == "" {
		return nil, &domain.UsageError{Detail: "database path is required"}
	}
	query := strings.TrimSpace(opts.Query)
	if query != "" && strings.TrimSpace(opts.QueryFile) != "" {
		return nil, &domain.UsageError{Detail: "use only one of --query and --query-file"}
	}
	if file := strings.TrimSpace(opts.QueryFile); file != "" {
		bytes, err := os.ReadFile(file)
		if err != nil {
			return nil, &domain.SourceError{Path: file, Err: err}
		}
		query = strings.TrimSpace(string(bytes))
	}
	if query == "" {
		return nil, &domain.UsageError{Detail: "query is required"}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultQueryTimeout
	}
	queryCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	dsn := "file:" + filepath.ToSlash(opts.Path) + "?mode=ro"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, &domain.DatabaseError{Err: err}
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)

	if err := db.PingContext(queryCtx); err != nil {
		return nil, databaseContextError(queryCtx, timeout, err)
	}
	if _, err := db.ExecContext(queryCtx, "PRAGMA query_only=ON"); err != nil {
		return nil, &domain.DatabaseError{Err: err}
	}
	if _, err := db.ExecContext(queryCtx, "PRAGMA busy_timeout=5000"); err != nil {
		return nil, &domain.DatabaseError{Err: err}
	}

	rows, err := db.QueryContext(queryCtx, query)
	if err != nil {
		return nil, databaseContextError(queryCtx, timeout, err)
	}
	defer func() { _ = rows.Close() }()

	columnNames, err := rows.Columns()
	if err != nil {
		return nil, databaseContextError(queryCtx, timeout, err)
	}
	columns := make([]domain.Column, len(columnNames))
	used := make(map[string]int, len(columnNames))
	var warnings []domain.Warning
	for i, original := range columnNames {
		original = domain.SanitizeText(strings.TrimSpace(original))
		name := uniqueName(original, i+1, used)
		columns[i] = domain.Column{
			Name:         name,
			OriginalName: original,
			Index:        i,
			Kind:         domain.ColumnLabel,
			ValueType:    domain.TypeString,
		}
		if name != original && original != "" {
			warnings = append(warnings, domain.Warning{
				ID:   "cli.warn.duplicate_header",
				Data: map[string]string{"Name": original, "New": name},
			})
		}
	}

	table := make([][]domain.Cell, 0)
	values := make([]any, len(columnNames))
	pointers := make([]any, len(columnNames))
	for i := range values {
		pointers[i] = &values[i]
	}
	for rows.Next() {
		if err := rows.Scan(pointers...); err != nil {
			return nil, databaseContextError(queryCtx, timeout, err)
		}
		row := make([]domain.Cell, len(values))
		for i, value := range values {
			row[i] = databaseCell(value)
		}
		table = append(table, row)
	}
	if err := rows.Err(); err != nil {
		return nil, databaseContextError(queryCtx, timeout, err)
	}

	return &domain.Dataset{
		Columns:     columns,
		Rows:        table,
		SourceKind:  domain.SourceDB,
		SourceID:    opts.Path,
		SourceCount: 1,
		RowCount:    len(table),
		Warnings:    warnings,
	}, nil
}

func databaseCell(value any) domain.Cell {
	switch typed := value.(type) {
	case nil:
		return domain.Null()
	case int64:
		return domain.NumberCell(float64(typed))
	case float64:
		return domain.NumberCell(typed)
	case bool:
		return domain.Cell{Kind: domain.CellBool, Text: fmt.Sprintf("%t", typed)}
	case time.Time:
		return domain.TimeCell(typed)
	case []byte:
		return domain.Cell{Kind: domain.CellBlob, Text: fmt.Sprintf("<blob %d bytes>", len(typed))}
	case string:
		return domain.TextCell(domain.SanitizeText(typed))
	default:
		return domain.TextCell(domain.SanitizeText(fmt.Sprint(typed)))
	}
}

func databaseContextError(ctx context.Context, timeout time.Duration, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &domain.DatabaseTimeoutError{Timeout: timeout}
	}
	return &domain.DatabaseError{Err: err}
}
