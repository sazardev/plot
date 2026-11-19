package render

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecimalSeparatorFollowsLocale(t *testing.T) {
	t.Parallel()

	assert.Equal(t, ".", decimalSeparator("en"))
	assert.Equal(t, ",", decimalSeparator("de"))
	assert.Equal(t, ",", decimalSeparator("fr"))
	assert.Equal(t, ".", decimalSeparator(""))
}

func TestFormatAxisLocalizesDecimalSeparator(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "1.5", formatAxis(1.5, false, "."))
	assert.Equal(t, "1,5", formatAxis(1.5, false, ","))
	assert.Equal(t, "1,5k", formatAxis(1500, false, ","))
	assert.Equal(t, "0", formatAxis(0, false, ","))
}
