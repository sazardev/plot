package i18n_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sazar/plot/internal/i18n"
)

func TestEnglishMessagesLoad(t *testing.T) {
	t.Parallel()

	loc, err := i18n.New("en")
	require.NoError(t, err)

	assert.Equal(t, "no data on stdin", loc.T("cli.error.no_data_stdin", nil))
	assert.Equal(t, "no numeric columns; available: key, value", loc.T("cli.error.no_numeric_column", map[string]any{
		"Columns": "key, value",
	}))
	assert.Equal(t, "record 2: expected 3 fields, got 2; padded or truncated", loc.T("cli.warn.ragged_row", map[string]any{
		"Line": "2", "Expected": "3", "Got": "2",
	}))
}

func TestMissingMessageFallsBackToID(t *testing.T) {
	t.Parallel()

	loc, err := i18n.New("en")
	require.NoError(t, err)
	assert.Equal(t, "cli.does.not.exist", loc.T("cli.does.not.exist", nil))
}

func TestUnknownLocaleFallsBackToEnglish(t *testing.T) {
	t.Parallel()

	loc, err := i18n.New("zz-ZZ")
	require.NoError(t, err)
	assert.Equal(t, "no data", loc.T("cli.error.no_data", nil))
}

func TestFormatIntUsesLocaleGrouping(t *testing.T) {
	t.Parallel()

	en, err := i18n.New("en")
	require.NoError(t, err)
	de, err := i18n.New("de")
	require.NoError(t, err)

	assert.Equal(t, "1,234", en.FormatInt(1234))
	assert.Equal(t, "1.234", de.FormatInt(1234))
}
