package domain

import "time"

// SourceKind identifies where a dataset came from.
type SourceKind string

const (
	SourceStdin SourceKind = "stdin"
	SourceFile  SourceKind = "file"
	SourceDB    SourceKind = "db"
)

// ColumnKind is the role a column plays in a chart.
type ColumnKind uint8

const (
	ColumnLabel ColumnKind = iota
	ColumnValue
)

// ValueType describes the values stored in a column.
type ValueType string

const (
	TypeString ValueType = "string"
	TypeInt    ValueType = "int"
	TypeFloat  ValueType = "float"
	TypeTime   ValueType = "time"
	TypeBool   ValueType = "bool"
	TypeEmpty  ValueType = "empty"
	TypeBlob   ValueType = "blob"
)

// CellKind is the normalized type of one cell.
type CellKind uint8

const (
	CellNull CellKind = iota
	CellNumber
	CellTime
	CellText
	CellBool
	CellBlob
)

// Cell is one typed value. Raw numeric input is discarded after parsing so a
// dataset is not duplicated in memory (NFR-2).
type Cell struct {
	Kind   CellKind
	Number float64
	Time   time.Time
	Text   string
}

// Null is an empty cell.
func Null() Cell { return Cell{Kind: CellNull} }

// NumberCell builds a numeric cell.
func NumberCell(v float64) Cell { return Cell{Kind: CellNumber, Number: v} }

// TimeCell builds a time cell.
func TimeCell(t time.Time) Cell { return Cell{Kind: CellTime, Time: t} }

// TextCell builds a text cell.
func TextCell(s string) Cell { return Cell{Kind: CellText, Text: s} }

// IsNull reports whether the cell holds no value.
func (c Cell) IsNull() bool { return c.Kind == CellNull }

// Column is one parsed column and its inferred role.
type Column struct {
	Name             string
	OriginalName     string
	Index            int
	Kind             ColumnKind
	ValueType        ValueType
	NullCount        int
	CoercedNullCount int
	DistinctCount    int
}

// Warning is a user-facing message ID plus its template data. It is localized
// at the CLI/TUI edge, never inside the domain.
type Warning struct {
	ID   string
	Data map[string]string
}

// Dataset is the in-memory table after parsing and inference.
type Dataset struct {
	Columns     []Column
	Rows        [][]Cell
	SourceKind  SourceKind
	SourceID    string
	SourceCount int
	RowCount    int
	Warnings    []Warning
}

// Point is one plotted value. X is an index, a number or a Unix timestamp
// depending on RenderSpec.LabelKind. Null points are gaps.
type Point struct {
	X    float64
	Y    float64
	Null bool
}

// Series is one plotted value column.
type Series struct {
	Column    int
	Label     string
	Points    []Point
	ColorSlot int
	Marker    int
}

// ChartType is one of the four supported chart families.
type ChartType string

const (
	ChartLine  ChartType = "line"
	ChartBar   ChartType = "bar"
	ChartPie   ChartType = "pie"
	ChartDonut ChartType = "donut"
)

// LabelKind describes the x-axis values.
type LabelKind string

const (
	LabelCategorical LabelKind = "categorical"
	LabelNumeric     LabelKind = "numeric"
	LabelTime        LabelKind = "time"
)

// ColorMode is the resolved color preference.
type ColorMode string

const (
	ColorAuto   ColorMode = "auto"
	ColorAlways ColorMode = "always"
	ColorNever  ColorMode = "never"
)

// GlyphMode is the resolved Unicode/ASCII preference.
type GlyphMode string

const (
	GlyphAuto    GlyphMode = "auto"
	GlyphUnicode GlyphMode = "unicode"
	GlyphASCII   GlyphMode = "ascii"
)

// PieSlice is one non-zero slice of a pie or donut.
type PieSlice struct {
	Label     string
	Value     float64
	ColorSlot int
	Marker    int
}

// RenderSpec is the sole input to the renderer.
type RenderSpec struct {
	ChartType   ChartType
	LabelColumn int // source column index, or -1 for a synthetic row index
	LabelName   string
	LabelKind   LabelKind
	Labels      []string
	Series      []Series
	PieSlices   []PieSlice
	Width       int
	Height      int
	Title       string
	ColorMode   ColorMode
	GlyphMode   GlyphMode
	Locale      string
}

// BuildOptions are the user-controlled inputs to chart resolution.
type BuildOptions struct {
	ChartType string
	Label     string
	Columns   []string
	Renames   []string
	Sorts     []string
	Filters   []string
	Width     int
	Height    int
	Title     string
	Locale    string
	ColorMode ColorMode
	GlyphMode GlyphMode
	MaxSeries int
}
