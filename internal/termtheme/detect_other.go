//go:build windows || plan9 || js || wasip1

package termtheme

import (
	"os"
	"time"
)

// Detect is unavailable on platforms without POSIX terminal queries.
func Detect(_, _ *os.File, _ time.Duration) (Palette, bool) {
	return DefaultPalette(), false
}
