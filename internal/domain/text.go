package domain

import (
	"strings"
	"unicode/utf8"
)

// SanitizeText makes user-supplied text terminal-safe. Invalid UTF-8 becomes
// U+FFFD; control characters, escape, line separators and bidi controls become
// spaces so a CSV cell cannot inject terminal sequences or reorder a label.
func SanitizeText(s string) string {
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "\uFFFD")
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\t' || r == '\n' || r == '\r':
			return ' '
		case r < 0x20:
			return ' '
		case r == 0x7f:
			return ' '
		case r >= 0x80 && r <= 0x9f:
			return ' '
		case r == 0x200e || r == 0x200f:
			return ' '
		case r >= 0x202a && r <= 0x202e:
			return ' '
		case r == 0x2028 || r == 0x2029:
			return ' '
		case r >= 0x2066 && r <= 0x2069:
			return ' '
		default:
			return r
		}
	}, s)
}
