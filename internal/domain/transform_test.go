package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/domain"
)

func build(t *testing.T, ds *domain.Dataset, opts domain.BuildOptions) *domain.RenderSpec {
	t.Helper()
	require.NoError(t, domain.Infer(ds))
	if opts.Width == 0 {
		opts.Width = 80
	}
	if opts.Height == 0 {
		opts.Height = 24
	}
	spec, err := domain.BuildRenderSpec(ds, opts)
	require.NoError(t, err)
	return spec
}

func TestResolveColumnByNameAndPosition(t *testing.T) {
	t.Parallel()

	ds := dataset([]string{"month", "revenue"})
	index, err := domain.ResolveColumn(ds, "revenue")
	require.NoError(t, err)
	assert.Equal(t, 1, index)

	index, err = domain.ResolveColumn(ds, "1")
	require.NoError(t, err)
	assert.Equal(t, 0, index)

	_, err = domain.ResolveColumn(ds, "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrColumnNotFound)

	var notFound *domain.ColumnNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, []string{"month", "revenue"}, notFound.Available)
}

func TestBuildSelectsColumnsInOrder(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue", "cost", "orders"},
		[]string{"Jan", "10", "4", "2"},
		[]string{"Feb", "12", "5", "3"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Label:   "month",
		Columns: []string{"orders", "revenue"},
	})

	require.Len(t, spec.Series, 2)
	assert.Equal(t, "orders", spec.Series[0].Label)
	assert.Equal(t, "revenue", spec.Series[1].Label)
	assert.Equal(t, domain.ChartLine, spec.ChartType)
}

func TestBuildLabelOverrideMakesNumericColumnTheXAxis(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"id", "month", "revenue"},
		[]string{"1", "Jan", "10"},
		[]string{"2", "Feb", "12"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Label:   "id",
		Columns: []string{"revenue"},
	})

	assert.Equal(t, domain.LabelNumeric, spec.LabelKind)
	assert.Equal(t, 0, spec.LabelColumn)
	assert.Equal(t, []string{"1", "2"}, spec.Labels)
	require.Len(t, spec.Series, 1)
}

func TestBuildRejectsLabelAlsoSelectedAsSeries(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue"},
		[]string{"Jan", "10"},
		[]string{"Feb", "12"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Label:   "month",
		Columns: []string{"month", "revenue"},
		Width:   80,
		Height:  24,
	})
	assert.ErrorIs(t, err, domain.ErrUsage)
}

func TestBuildRejectsNonNumericColumnSelection(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"region", "month", "revenue"},
		[]string{"EU", "Jan", "10"},
		[]string{"EU", "Feb", "12"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Columns: []string{"month"},
		Width:   80,
		Height:  24,
	})
	assert.ErrorIs(t, err, domain.ErrColumnNotNumeric)
}

func TestBuildRejectsExplicitTooManySeries(t *testing.T) {
	t.Parallel()

	names := []string{"month"}
	row := []string{"Jan"}
	for i := 0; i < 9; i++ {
		names = append(names, "v"+string(rune('a'+i)))
		row = append(row, "1")
	}
	ds := dataset(names, row)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Columns:   names[1:],
		Width:     80,
		Height:    24,
		MaxSeries: 8,
	})
	assert.ErrorIs(t, err, domain.ErrTooManySeries)
}

func TestRenameIsDisplayOnly(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue"},
		[]string{"Jan", "10"},
		[]string{"Feb", "12"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Renames: []string{"revenue=Revenue"},
	})

	assert.Equal(t, "Revenue", spec.Series[0].Label)
	assert.Equal(t, "revenue", ds.Columns[1].Name)
	assert.Equal(t, []string{"Jan", "Feb"}, spec.Labels)
}

func TestRenameMissingColumn(t *testing.T) {
	t.Parallel()

	ds := dataset([]string{"month", "revenue"}, []string{"Jan", "10"})
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Renames: []string{"missing=New"},
		Width:   80,
		Height:  24,
	})
	assert.ErrorIs(t, err, domain.ErrColumnNotFound)
}

func TestRenameRejectsDuplicateDisplayLabels(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue", "cost"},
		[]string{"Jan", "10", "4"},
		[]string{"Feb", "12", "5"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Renames: []string{"revenue=Total", "cost=Total"},
		Width:   80,
		Height:  24,
	})
	assert.ErrorIs(t, err, domain.ErrDuplicateLabel)
}

func TestRenameRejectsCollisionWithPlottedColumn(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue", "cost"},
		[]string{"Jan", "10", "4"},
		[]string{"Feb", "12", "5"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Renames: []string{"revenue=cost"},
		Width:   80,
		Height:  24,
	})
	assert.ErrorIs(t, err, domain.ErrDuplicateLabel)
}

func TestSortAscendingDescendingAndNullsLast(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"group", "value"},
		[]string{"b", "2"},
		[]string{"a", "3"},
		[]string{"a", "1"},
		[]string{"c", "N/A"},
	)
	spec := build(t, ds, domain.BuildOptions{Sorts: []string{"value:desc"}})

	got := make([]float64, 0, len(spec.Series[0].Points))
	for _, point := range spec.Series[0].Points {
		if !point.Null {
			got = append(got, point.Y)
		}
	}
	assert.Equal(t, []float64{3, 2, 1}, got)
	assert.True(t, spec.Series[0].Points[len(spec.Series[0].Points)-1].Null)
}

func TestSortByTwoKeysIsStable(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"group", "value", "rank"},
		[]string{"b", "2", "1"},
		[]string{"a", "3", "2"},
		[]string{"a", "1", "1"},
	)
	spec := build(t, ds, domain.BuildOptions{Sorts: []string{"group:asc", "value:desc"}})

	labels := spec.Labels
	assert.Equal(t, []string{"a", "a", "b"}, labels)
	assert.Equal(t, 3.0, spec.Series[0].Points[0].Y)
	assert.Equal(t, 1.0, spec.Series[0].Points[1].Y)
}

func TestFilterNumericAndTextAreANDed(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"region", "value"},
		[]string{"EU", "10"},
		[]string{"EU", "20"},
		[]string{"US", "30"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Label:   "region",
		Filters: []string{"region=EU", "value>=15"},
	})

	require.Len(t, spec.Series[0].Points, 1)
	assert.Equal(t, 20.0, spec.Series[0].Points[0].Y)
}

func TestFilterContains(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"name", "value"},
		[]string{"Ada Lovelace", "1"},
		[]string{"Grace Hopper", "2"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Label:   "name",
		Filters: []string{`name contains Ada`},
	})

	require.Len(t, spec.Series[0].Points, 1)
	assert.Equal(t, 1.0, spec.Series[0].Points[0].Y)
}

func TestFilterNullSemantics(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"name", "value"},
		[]string{"a", "1"},
		[]string{"b", "N/A"},
		[]string{"c", "3"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Label:   "name",
		Filters: []string{"value=null"},
	})
	require.Len(t, spec.Series[0].Points, 1)
	assert.True(t, spec.Series[0].Points[0].Null)

	ds = dataset(
		[]string{"name", "value"},
		[]string{"a", "1"},
		[]string{"b", "N/A"},
		[]string{"c", "3"},
	)
	spec = build(t, ds, domain.BuildOptions{
		Label:   "name",
		Filters: []string{"value!=null"},
	})
	require.Len(t, spec.Series[0].Points, 2)
	assert.Equal(t, 1.0, spec.Series[0].Points[0].Y)
	assert.Equal(t, 3.0, spec.Series[0].Points[1].Y)
}

func TestFilterNoRowsIsAnError(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"name", "value"},
		[]string{"a", "1"},
		[]string{"b", "2"},
	)
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Filters: []string{"value>100"},
		Width:   80,
		Height:  24,
	})
	assert.ErrorIs(t, err, domain.ErrNoRowsAfterFilter)
}

func TestFilterInvalidExpressions(t *testing.T) {
	t.Parallel()

	tests := []string{
		"value~1",
		"value contains x",
		"missing=1",
		"value=not-a-number",
	}
	for _, expression := range tests {
		expression := expression
		t.Run(expression, func(t *testing.T) {
			t.Parallel()
			ds := dataset(
				[]string{"name", "value"},
				[]string{"a", "1"},
				[]string{"b", "2"},
			)
			require.NoError(t, domain.Infer(ds))
			_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
				Filters: []string{expression},
				Width:   80,
				Height:  24,
			})
			require.Error(t, err)
		})
	}
}

func TestFilterThenSort(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue"},
		[]string{"Jan", "10"},
		[]string{"Feb", "30"},
		[]string{"Mar", "20"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Filters: []string{"revenue>=20"},
		Sorts:   []string{"revenue:desc"},
	})

	assert.Equal(t, []string{"Feb", "Mar"}, spec.Labels)
	assert.Equal(t, 30.0, spec.Series[0].Points[0].Y)
	assert.Equal(t, 20.0, spec.Series[0].Points[1].Y)
}

func TestRenameDoesNotAffectFilterOrSort(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue"},
		[]string{"Jan", "10"},
		[]string{"Feb", "30"},
		[]string{"Mar", "20"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Renames: []string{"revenue=Revenue"},
		Filters: []string{"revenue>=20"},
		Sorts:   []string{"revenue:desc"},
	})

	assert.Equal(t, "Revenue", spec.Series[0].Label)
	assert.Equal(t, []string{"Feb", "Mar"}, spec.Labels)
}

func TestBuildAllNumericColumnsExplicitlySelectedUsesSyntheticRow(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"x", "y"},
		[]string{"1", "10"},
		[]string{"2", "20"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Columns: []string{"x", "y"},
	})

	require.Len(t, spec.Series, 2)
	assert.Equal(t, -1, spec.LabelColumn)
	assert.Equal(t, []string{"1", "2"}, spec.Labels)
}

func TestSortInvalidDirection(t *testing.T) {
	t.Parallel()

	ds := dataset([]string{"month", "value"}, []string{"Jan", "1"})
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Sorts:  []string{"value:sideways"},
		Width:  80,
		Height: 24,
	})
	assert.ErrorIs(t, err, domain.ErrInvalidSort)
}

func TestSortMissingColumn(t *testing.T) {
	t.Parallel()

	ds := dataset([]string{"month", "value"}, []string{"Jan", "1"})
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Sorts:  []string{"missing"},
		Width:  80,
		Height: 24,
	})
	assert.ErrorIs(t, err, domain.ErrColumnNotFound)
}

func TestFilterCanUseAnUnplottedColumn(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"month", "revenue", "cost"},
		[]string{"Jan", "10", "4"},
		[]string{"Feb", "12", "8"},
		[]string{"Mar", "14", "2"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Columns: []string{"revenue"},
		Filters: []string{"cost>5"},
	})

	assert.Equal(t, []string{"Feb"}, spec.Labels)
	require.Len(t, spec.Series, 1)
	assert.Equal(t, 12.0, spec.Series[0].Points[0].Y)
}

func TestLabelMissingColumn(t *testing.T) {
	t.Parallel()

	ds := dataset([]string{"month", "value"}, []string{"Jan", "1"})
	require.NoError(t, domain.Infer(ds))

	_, err := domain.BuildRenderSpec(ds, domain.BuildOptions{
		Label:  "missing",
		Width:  80,
		Height: 24,
	})
	assert.ErrorIs(t, err, domain.ErrColumnNotFound)
}

func TestFilterTimeAndBooleanColumns(t *testing.T) {
	t.Parallel()

	ds := dataset(
		[]string{"date", "active", "value"},
		[]string{"2024-01-01", "true", "10"},
		[]string{"2024-01-02", "false", "20"},
		[]string{"2024-01-03", "true", "30"},
	)
	spec := build(t, ds, domain.BuildOptions{
		Label:   "date",
		Columns: []string{"value"},
		Filters: []string{"active=true", `date>="2024-01-02"`},
	})

	assert.Equal(t, []string{"2024-01-03"}, spec.Labels)
	require.Len(t, spec.Series, 1)
	assert.Equal(t, 30.0, spec.Series[0].Points[0].Y)
}
