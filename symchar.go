package vtui

// SymCharFlag marks a CharInfo.Char value as a symbolic glyph token rather
// than a Unicode code point or a CompCharFlag registry index (textseg.go).
// It occupies bit 62, the space directly below CompCharFlag (bit 63), and
// like CompCharFlag it lies entirely outside Unicode (the largest code
// point is U+10FFFF), so it can never collide with user text -- including
// Private Use Area code points used by Nerd Fonts. WideCharFiller (all bits
// set) shares the bit and is not a token, exactly as it is not a CompChar;
// see IsSymChar.
//
// A symbolic token separates "what to draw" from "how to draw it": a widget
// writes the identity of a glyph (which checkbox/radio state, and which of
// its cells) instead of literal text, and each backend decides how that
// glyph actually looks. Every cell of a symbol occupies exactly as many
// columns as the classic text it replaces, so switching style never changes
// dialog layout.
//
// Text-mode backends (ANSI, Windows console, far2l-terminal, screendump,
// tests) expand a token back to the classic style's literal text through
// CellString/CellBaseRune -- the same choke point that already expands
// CompCharFlag registry entries -- so with the default "classic" style their
// output is unchanged by the token's existence. Graphics backends resolve a
// token's classic rune through the same functions and keep drawing it via
// the ordinary font path: checkbox/radio glyphs were never geometric shapes
// the way box-drawing characters are (see classic_glyph.go), so the classic
// style needs no shape-table entry to stay pixel-identical either.
const SymCharFlag uint64 = 1 << 62

// symGlyphPartBits is the number of low bits SymCharToken reserves for the
// index of a cell within a multi-cell symbolic glyph. 2 bits cover the 3
// cells (0..2) a checkbox or radio glyph needs today with room to spare.
const symGlyphPartBits = 2
const symGlyphPartMask = (uint64(1) << symGlyphPartBits) - 1

// SymGlyph identifies a symbolic glyph variant: one visual state of a
// checkbox or radio button. Each variant is 3 cells wide, exactly like the
// classic text it replaces ("[x] ", "( ) ", ...), so a symbolic checkbox or
// radio button lays out identically to the literal text it replaced.
type SymGlyph uint32

const (
	// SymCheckboxOff, SymCheckboxOn and SymCheckboxMixed are the 3 states a
	// Checkbox or a CheckGroup item can render: unchecked, checked, and (for
	// a three-state Checkbox) indeterminate.
	SymCheckboxOff SymGlyph = iota
	SymCheckboxOn
	SymCheckboxMixed
	// SymRadioOff and SymRadioOn are the 2 states a RadioButton or a
	// RadioGroup item can render: unselected and selected.
	SymRadioOff
	SymRadioOn
)

// SymCharToken packs a symbol variant and the index of one of its cells
// (0-based, left to right) into a CharInfo.Char value tagged with
// SymCharFlag.
func SymCharToken(sym SymGlyph, part int) uint64 {
	return SymCharFlag | uint64(sym)<<symGlyphPartBits | (uint64(part) & symGlyphPartMask)
}

// DecodeSymChar extracts the symbol variant and cell index a SymCharFlag
// token carries. ok is false when ch is not such a token.
func DecodeSymChar(ch uint64) (sym SymGlyph, part int, ok bool) {
	if !IsSymChar(ch) {
		return 0, 0, false
	}
	payload := ch &^ SymCharFlag
	part = int(payload & symGlyphPartMask)
	sym = SymGlyph(payload >> symGlyphPartBits)
	return sym, part, true
}

// IsSymChar reports whether a CharInfo.Char value is a symbolic glyph token
// rather than a plain rune or a CompCharFlag registry index. WideCharFiller
// shares the bit and is not one, exactly as with IsCompChar.
func IsSymChar(ch uint64) bool {
	return ch != WideCharFiller && ch&SymCharFlag != 0
}

// symGlyphClassicCells is the classic style's literal, cell-by-cell text for
// each symbolic glyph variant. Every text-mode backend reproduces exactly
// this text for a token whatever the active GlyphStyle: a future "modern"
// style only changes graphics output, terminals always keep "[x]"/"( )"
// (see the design comment on f4#285).
var symGlyphClassicCells = map[SymGlyph][3]string{
	SymCheckboxOff:   {"[", " ", "]"},
	SymCheckboxOn:    {"[", "x", "]"},
	SymCheckboxMixed: {"[", "?", "]"},
	SymRadioOff:      {"(", " ", ")"},
	SymRadioOn:       {"(", "•", ")"},
}

// symCharClassicString returns the classic style's literal text for a
// SymCharFlag token. It is the checkbox/radio analogue of the CompCharFlag
// registry lookup CellString already performs for composite clusters.
func symCharClassicString(ch uint64) string {
	sym, part, ok := DecodeSymChar(ch)
	if !ok {
		return "?"
	}
	cells, ok := symGlyphClassicCells[sym]
	if !ok || part < 0 || part > 2 {
		return "?"
	}
	return cells[part]
}

// SymGlyphCharInfo builds the 3 CharInfo cells a symbolic glyph token
// occupies, ready to hand to ScreenBuf.Write. attr is applied to all 3
// cells; callers that colour the indicator differently from the text next
// to it (Checkbox, RadioButton) write that separately, the way they used to
// draw a literal "[x] "/"( ) " prefix once with the normal attribute and
// then overwrite only its first 3 cells with the indicator attribute.
func SymGlyphCharInfo(sym SymGlyph, attr uint64) []CharInfo {
	return []CharInfo{
		{Char: SymCharToken(sym, 0), Attributes: attr},
		{Char: SymCharToken(sym, 1), Attributes: attr},
		{Char: SymCharToken(sym, 2), Attributes: attr},
	}
}
