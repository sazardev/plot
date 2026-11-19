package termtheme

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseResponses(t *testing.T) {
	t.Parallel()

	palette, detected := parseResponses(
		"\x1b]4;1;rgb:ffff/0000/1111\x1b\\"+
			"\x1b]4;2;#00ff00\x1b\\"+
			"\x1b]10;rgb:aaaa/bbbb/cccc\x1b\\"+
			"\x1b]11;rgb:0000/0000/0000\x1b\\",
		DefaultPalette(),
	)
	require.True(t, detected)
	assert.Equal(t, uint8(255), palette.ANSI[1].R)
	assert.Equal(t, uint8(0), palette.ANSI[1].G)
	assert.Equal(t, uint8(0x11), palette.ANSI[1].B)
	assert.Equal(t, uint8(0), palette.ANSI[2].R)
	assert.Equal(t, uint8(255), palette.ANSI[2].G)
	assert.Equal(t, uint8(0xaa), palette.Foreground.R)
	assert.Equal(t, uint8(0), palette.Background.R)
}

func TestParseComponent(t *testing.T) {
	t.Parallel()

	tests := map[string]uint8{"0": 0, "f": 255, "ff": 255, "8000": 128}
	for input, want := range tests {
		got, ok := parseComponent(input)
		require.True(t, ok, input)
		assert.Equal(t, want, got, input)
	}
}

func TestFontSize(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 16.0, FontSize("", 16))
	assert.Equal(t, 18.5, FontSize("18.5", 16))
	assert.Equal(t, 16.0, FontSize("1", 16))
	assert.Equal(t, 16.0, FontSize("huge", 16))
}
