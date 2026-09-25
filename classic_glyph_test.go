package vtui

import "testing"

func TestClassicGlyphRects_CoversBoxFamily(t *testing.T) {
	for _, r := range []rune{
		'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼',
		'═', '║', '╔', '╗', '╚', '╝', '╠', '╣', '╩', '╦', '╟', '╢', '╬',
		'━', '┃', '┏', '┓', '┗', '┛', '┣', '┫', '┳', '┻', '╋',
	} {
		rects, ok := classicGlyphRects(r, 16, 16, 1)
		if !ok || len(rects) == 0 {
			t.Errorf("classicGlyphRects(%q) = (%v, %v), want non-empty", r, rects, ok)
		}
	}
}

func TestClassicGlyphRects_UnknownRune(t *testing.T) {
	if rects, ok := classicGlyphRects('A', 16, 16, 1); ok || rects != nil {
		t.Fatalf("classicGlyphRects('A') = (%v, %v), want (nil, false)", rects, ok)
	}
}
