package domain_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/domain"
)

func TestSanitizeTextRemovesControlAndBidiCharacters(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "a [31mb", domain.SanitizeText("a\x1b[31mb"))
	assert.Equal(t, "a b", domain.SanitizeText("a\nb"))
	assert.Equal(t, "a b", domain.SanitizeText("a\tb"))
	assert.Equal(t, "a b", domain.SanitizeText("a\u202eb"))
	assert.Equal(t, "a b", domain.SanitizeText("a\u2028b"))
	assert.Equal(t, "a\uFFFD", domain.SanitizeText("a\xff"))
	assert.Equal(t, "café", domain.SanitizeText("café"))
}

func TestInferTreatsBlobColumnAsLabel(t *testing.T) {
	t.Parallel()

	ds := &domain.Dataset{
		Columns: []domain.Column{{Name: "payload", Index: 0}, {Name: "value", Index: 1}},
		Rows: [][]domain.Cell{
			{{Kind: domain.CellBlob, Text: "<blob 2 bytes>"}, domain.TextCell("10")},
			{{Kind: domain.CellBlob, Text: "<blob 3 bytes>"}, domain.TextCell("20")},
		},
		RowCount: 2,
	}
	require.NoError(t, domain.Infer(ds))

	assert.Equal(t, domain.ColumnLabel, ds.Columns[0].Kind)
	assert.Equal(t, domain.TypeBlob, ds.Columns[0].ValueType)
	assert.Equal(t, domain.ColumnValue, ds.Columns[1].Kind)
}
