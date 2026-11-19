package render

import (
	"strings"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// decimalSeparator returns the locale decimal separator. Axis numbers keep
// Western digits and SI suffixes; only the separator follows the locale.
func decimalSeparator(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return "."
	}
	probe := message.NewPrinter(language.Make(locale)).Sprintf("%.1f", 1.5)
	runes := []rune(probe)
	if len(runes) < 3 || !unicode.IsDigit(runes[0]) || !unicode.IsDigit(runes[len(runes)-1]) {
		return "."
	}
	separator := string(runes[1 : len(runes)-1])
	if separator == "" {
		return "."
	}
	return separator
}

func localizeDecimal(text, separator string) string {
	if separator == "." || separator == "" {
		return text
	}
	return strings.ReplaceAll(text, ".", separator)
}
