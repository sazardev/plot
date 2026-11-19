package domain_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/domain"
)

func dataset(names []string, rows ...[]string) *domain.Dataset {
	columns := make([]domain.Column, len(names))
	for i, name := range names {
		columns[i] = domain.Column{
			Name:      name,
			Index:     i,
			Kind:      domain.ColumnLabel,
			ValueType: domain.TypeString,
		}
	}
	out := make([][]domain.Cell, 0, len(rows))
	for _, row := range rows {
		cells := make([]domain.Cell, len(names))
		for i := range names {
			if i < len(row) {
				cells[i] = domain.TextCell(row[i])
			} else {
				cells[i] = domain.Null()
			}
		}
		out = append(out, cells)
	}
	return &domain.Dataset{
		Columns:    columns,
		Rows:       out,
		SourceKind: domain.SourceStdin,
		SourceID:   "stdin",
		RowCount:   len(out),
	}
}

func TestInferClassifiesColumnsAndValues(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"day", "signups", "rate"},
		[]string{"Mon", "3", "1.5"},
		[]string{"Tue", "5", "2.5"},
		[]string{"Wed", "4", "3.5"},
	)
	require.NoError(t, domain.Infer(ds))

	assert.Equal(t, domain.ColumnLabel, ds.Columns[0].Kind)
	assert.Equal(t, domain.TypeString, ds.Columns[0].ValueType)
	assert.Equal(t, domain.ColumnValue, ds.Columns[1].Kind)
	assert.Equal(t, domain.TypeInt, ds.Columns[1].ValueType)
	assert.Equal(t, domain.ColumnValue, ds.Columns[2].Kind)
	assert.Equal(t, domain.TypeFloat, ds.Columns[2].ValueType)
	assert.Equal(t, domain.CellNumber, ds.Rows[0][1].Kind)
	assert.InDelta(t, 3, ds.Rows[0][1].Number, 0.0001)
}

func TestInferTreatsNullTokensAsMissing(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"day", "value"},
		[]string{"Mon", "1"},
		[]string{"Tue", "N/A"},
		[]string{"Wed", "3"},
	)
	require.NoError(t, domain.Infer(ds))

	assert.Equal(t, domain.ColumnValue, ds.Columns[1].Kind)
	assert.Equal(t, 1, ds.Columns[1].NullCount)
	assert.True(t, ds.Rows[1][1].IsNull())
}

func TestInferCoercesNonNumericCellsInValueColumns(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"day", "value"},
		[]string{"a", "1"},
		[]string{"b", "2"},
		[]string{"c", "3"},
		[]string{"d", "4"},
		[]string{"e", "oops"},
	)
	require.NoError(t, domain.Infer(ds))

	assert.Equal(t, domain.ColumnValue, ds.Columns[1].Kind)
	assert.Equal(t, 1, ds.Columns[1].CoercedNullCount)
	assert.Equal(t, 1, ds.Columns[1].NullCount)
	assert.True(t, ds.Rows[4][1].IsNull())
}

func TestInferRecognizesTimeAndBooleanLabels(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"day", "enabled", "value"},
		[]string{"2024-01-01", "true", "1"},
		[]string{"2024-01-02", "false", "2"},
	)
	require.NoError(t, domain.Infer(ds))

	assert.Equal(t, domain.TypeTime, ds.Columns[0].ValueType)
	assert.Equal(t, domain.TypeBool, ds.Columns[1].ValueType)
	assert.Equal(t, domain.CellTime, ds.Rows[0][0].Kind)
	assert.Equal(t, domain.CellBool, ds.Rows[0][1].Kind)
}

func TestInferMarksEmptyColumns(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"day", "empty", "value"},
		[]string{"Mon", "", "1"},
		[]string{"Tue", "N/A", "2"},
	)
	require.NoError(t, domain.Infer(ds))

	assert.Equal(t, domain.TypeEmpty, ds.Columns[1].ValueType)
	assert.Equal(t, 2, ds.Columns[1].NullCount)
	assert.Len(t, ds.Warnings, 1)
	assert.Equal(t, "cli.warn.empty_column", ds.Warnings[0].ID)
}

func TestBuildRenderSpecUsesCategoricalLabelAndBarDefault(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue"},
		[]string{"Jan", "10"},
		[]string{"Feb", "12"},
	)
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	require.NoError(t, err)

	assert.Equal(t, domain.ChartBar, spec.ChartType)
	assert.Equal(t, domain.LabelCategorical, spec.LabelKind)
	assert.Equal(t, "month", spec.LabelName)
	assert.Len(t, spec.Series, 1)
	assert.Equal(t, []string{"Jan", "Feb"}, spec.Labels)
	assert.Equal(t, "stdin — bar", spec.Title)
}

func TestBuildRenderSpecDefaultsToLineWithMultipleSeries(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue", "cost"},
		[]string{"Jan", "10", "4"},
		[]string{"Feb", "12", "5"},
	)
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	require.NoError(t, err)

	assert.Equal(t, domain.ChartLine, spec.ChartType)
	assert.Len(t, spec.Series, 2)
}

func TestBuildRenderSpecUsesFirstNumericColumnAsX(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"x", "y"},
		[]string{"1", "10"},
		[]string{"2", "20"},
	)
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	require.NoError(t, err)

	assert.Equal(t, domain.LabelNumeric, spec.LabelKind)
	assert.Equal(t, 0, spec.LabelColumn)
	assert.Len(t, spec.Series, 1)
	assert.Equal(t, 1, spec.Series[0].Column)
	assert.Equal(t, []string{"1", "2"}, spec.Labels)
}

func TestBuildRenderSpecCreatesSyntheticRowForSingleColumn(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"value"},
		[]string{"10"},
		[]string{"20"},
		[]string{"30"},
	)
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	require.NoError(t, err)

	assert.Equal(t, domain.ChartLine, spec.ChartType)
	assert.Equal(t, -1, spec.LabelColumn)
	assert.Equal(t, "row", spec.LabelName)
	assert.Equal(t, domain.LabelNumeric, spec.LabelKind)
	assert.Equal(t, []string{"1", "2", "3"}, spec.Labels)
	assert.Len(t, spec.Series, 1)
}

func TestBuildRenderSpecRejectsNoNumericColumns(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"city", "country"},
		[]string{"Paris", "FR"},
		[]string{"Lisbon", "PT"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrNoNumericColumn)

	var noNumeric *domain.NoNumericColumnError
	require.ErrorAs(t, err, &noNumeric)
	assert.Equal(t, []string{"city", "country"}, noNumeric.Available)
}

func TestBuildRenderSpecRejectsIncompatibleBar(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue", "cost"},
		[]string{"Jan", "10", "4"},
		[]string{"Feb", "12", "5"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{ChartType: "bar", Width: 80, Height: 24})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrIncompatibleChart)
}

func TestBuildRenderSpecBuildsPieSlices(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue"},
		[]string{"Jan", "10"},
		[]string{"Feb", "12"},
	)
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{ChartType: "pie", Width: 80, Height: 24})
	require.NoError(t, err)
	assert.Equal(t, domain.ChartPie, spec.ChartType)
	require.Len(t, spec.PieSlices, 2)
	assert.Equal(t, "Jan", spec.PieSlices[0].Label)
	assert.Equal(t, 10.0, spec.PieSlices[0].Value)
}

func TestBuildRenderSpecRejectsHeaderOnlyInput(t *testing.T) {
	t.Parallel()

	ds := dataset([]string{"month", "revenue"})
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	assert.ErrorIs(t, err, domain.ErrNoRows)
}

func TestBuildRenderSpecAddsMultipleLabelWarning(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "region", "revenue"},
		[]string{"Jan", "EU", "10"},
		[]string{"Feb", "EU", "12"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24})
	require.NoError(t, err)
	require.NotEmpty(t, ds.Warnings)
	assert.True(t, hasWarning(ds.Warnings, "cli.warn.multiple_labels"))
}

func TestBuildRenderSpecTruncatesSeries(t *testing.T) {
	t.Parallel()

	names := []string{"label"}
	values := []string{"Mon"}
	for i := 0; i < 10; i++ {
		names = append(names, "v"+string(rune('a'+i)))
		values = append(values, "1")
	}
	ds := dataset(names, values)
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 80, Height: 24, MaxSeries: 8})
	require.NoError(t, err)
	assert.Len(t, spec.Series, 8)
	assert.True(t, hasWarning(ds.Warnings, "cli.warn.series_truncated"))
}

func TestBuildRenderSpecRejectsUnknownChartType(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue"},
		[]string{"Jan", "10"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{ChartType: "scatter", Width: 80, Height: 24})
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrUsage)
}

func TestInferRejectsEmptyDataset(t *testing.T) {
	t.Parallel()

	err := domain.Infer(&domain.Dataset{})
	assert.True(t, errors.Is(err, domain.ErrNoData))
}

func hasWarning(warnings []domain.Warning, id string) bool {
	for _, w := range warnings {
		if w.ID == id {
			return true
		}
	}
	return false
}

func TestBuildRenderSpecWarnsWhenCategoricalDataUsesRowOrder(t *testing.T) {
	t.Parallel()

	names := []string{"label", "value"}
	rows := make([][]string, 0, 31)
	for i := 0; i < 31; i++ {
		rows = append(rows, []string{string(rune('a' + i%26)), "1"})
	}
	ds := dataset(names, rows...)
	require.NoError(t, domain.Infer(ds))

	spec, err := domain.BuildRenderSpec(ds, domain.BuildOptions{Width: 40, Height: 12})
	require.NoError(t, err)
	assert.Equal(t, domain.ChartLine, spec.ChartType)
	assert.True(t, hasWarning(ds.Warnings, "cli.warn.ordinal_axis"))
}

func TestInferIsIdempotent(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"day", "enabled", "value"},
		[]string{"2024-01-01", "true", "1.5"},
		[]string{"2024-01-02", "false", "2.5"},
	)
	require.NoError(t, domain.Infer(ds))
	first := make([][]domain.Cell, len(ds.Rows))
	for i := range ds.Rows {
		first[i] = append([]domain.Cell(nil), ds.Rows[i]...)
	}
	require.NoError(t, domain.Infer(ds))
	assert.Equal(t, first, ds.Rows)
}
