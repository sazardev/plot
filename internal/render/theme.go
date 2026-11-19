package render

// StyleFunc colors one plain-text fragment. Returning the input unchanged is
// the no-color behavior.
type StyleFunc func(string) string

// Theme contains the resolved styles for a render. The UI package builds it;
// the renderer never imports Lip Gloss or Bubble Tea.
type Theme struct {
	Series  []StyleFunc
	Primary StyleFunc
	Muted   StyleFunc
	Bold    StyleFunc
	Dim     StyleFunc
	ASCII   bool
}

func (t Theme) seriesStyle(slot int) StyleFunc {
	if len(t.Series) == 0 {
		return nil
	}
	return t.Series[slot%len(t.Series)]
}

func (t Theme) marker(slot int) string {
	if t.ASCII {
		return asciiMarkers[slot%len(asciiMarkers)]
	}
	return unicodeMarkers[slot%len(unicodeMarkers)]
}

func (t Theme) barGlyph() string {
	if t.ASCII {
		return "#"
	}
	return "█"
}

func (t Theme) lineGlyph() string {
	if t.ASCII {
		return "-"
	}
	return "·"
}

func (t Theme) axisGlyphs() (vertical, horizontal, corner, tick, baseline string) {
	if t.ASCII {
		return "|", "-", "+", "+", "-"
	}
	return "│", "─", "└", "┤", "─"
}

var unicodeMarkers = []string{"●", "▲", "■", "◆", "▼", "○", "◇", "✚"}
var asciiMarkers = []string{"o", "^", "#", "*", "v", "O", "+", "x"}

func (t Theme) pieGlyph(slot int) string {
	if t.ASCII {
		return asciiPieGlyphs[slot%len(asciiPieGlyphs)]
	}
	return unicodePieGlyphs[slot%len(unicodePieGlyphs)]
}

var unicodePieGlyphs = []string{"█", "▓", "▒", "░", "▚", "▞", "▙", "▛"}
var asciiPieGlyphs = []string{"#", "%", "*", "+", "=", "-", "@", ":"}
