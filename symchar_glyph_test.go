package vtui

import "testing"

func TestSymGlyphAt_RecognisesWellFormedRun(t *testing.T) {
	for _, sym := range allSymGlyphs {
		c0 := SymCharToken(sym, 0)
		c1 := SymCharToken(sym, 1)
		c2 := SymCharToken(sym, 2)
		got, ok := symGlyphAt(c0, c1, c2)
		if !ok || got != sym {
			t.Errorf("symGlyphAt(%v parts 0,1,2) = (%v, %v), want (%v, true)", sym, got, ok, sym)
		}
	}
}

func TestSymGlyphAt_RejectsMalformedRuns(t *testing.T) {
	off0 := SymCharToken(SymCheckboxOff, 0)
	off1 := SymCharToken(SymCheckboxOff, 1)
	off2 := SymCharToken(SymCheckboxOff, 2)
	on0 := SymCharToken(SymCheckboxOn, 0)
	on1 := SymCharToken(SymCheckboxOn, 1)

	cases := []struct {
		name       string
		c0, c1, c2 uint64
	}{
		{"not part 0", off1, off2, SymCharToken(SymCheckboxOff, 0)},
		{"mixed symbols", off0, on1, off2},
		{"wrong part order", off0, off2, off1},
		{"plain text first cell", uint64('['), off1, off2},
		{"plain text middle cell", off0, uint64('x'), off2},
		{"button-ear cell mistaken for part 2", off0, off1, SymCharToken(SymButtonEarLeft, 0)},
	}
	for _, c := range cases {
		if got, ok := symGlyphAt(c.c0, c.c1, c.c2); ok {
			t.Errorf("%s: symGlyphAt = (%v, true), want ok=false", c.name, got)
		}
	}
}

// TestSymGlyphRectsForStyle_ClassicNeverClaimsAShape locks in item 3 of
// f4#285's shape slice: GlyphStyleClassic must keep every checkbox/radio
// token's graphics output pixel-identical to before this change, so
// symGlyphRectsForStyle must decline every symbol under that style,
// regardless of the geometry parameters passed in.
func TestSymGlyphRectsForStyle_ClassicNeverClaimsAShape(t *testing.T) {
	for _, sym := range allSymGlyphs {
		if rects, ok := symGlyphRectsForStyle(GlyphStyleClassic, sym, 48, 16, 1); ok || rects != nil {
			t.Errorf("symGlyphRectsForStyle(Classic, %v) = (%v, %v), want (nil, false)", sym, rects, ok)
		}
	}
	for _, sym := range buttonEarSyms {
		if rects, ok := symGlyphRectsForStyle(GlyphStyleClassic, sym, 32, 16, 1); ok || rects != nil {
			t.Errorf("symGlyphRectsForStyle(Classic, %v) = (%v, %v), want (nil, false)", sym, rects, ok)
		}
	}
}

// TestSymGlyphRectsForStyle_RoundedButtonEarsOutOfScope confirms button-ear
// tokens get no shape even under GlyphStyleRounded: geometric button ears
// are a separate, later slice of f4#285, not part of this one.
func TestSymGlyphRectsForStyle_RoundedButtonEarsOutOfScope(t *testing.T) {
	for _, sym := range buttonEarSyms {
		if rects, ok := symGlyphRectsForStyle(GlyphStyleRounded, sym, 32, 16, 1); ok || rects != nil {
			t.Errorf("symGlyphRectsForStyle(Rounded, %v) = (%v, %v), want (nil, false)", sym, rects, ok)
		}
	}
}

// TestSymGlyphRectsForStyle_RoundedClaimsCheckboxAndRadio confirms every
// checkbox/radio variant gets a non-empty shape under GlyphStyleRounded --
// the whole point of this slice.
func TestSymGlyphRectsForStyle_RoundedClaimsCheckboxAndRadio(t *testing.T) {
	for _, sym := range allSymGlyphs {
		rects, ok := symGlyphRectsForStyle(GlyphStyleRounded, sym, 48, 16, 1)
		if !ok || len(rects) == 0 {
			t.Errorf("symGlyphRectsForStyle(Rounded, %v) = (%v, %v), want a non-empty shape", sym, rects, ok)
		}
	}
}

func sameClassicRects(a, b []classicRect) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSymGlyphRectsForStyle_CheckboxStatesDiffer is the geometry-level
// regression item 5 of f4#285's shape slice asks for: a checked checkbox
// must not render as the same shape as an unchecked one, and a mixed
// (indeterminate) checkbox must differ from both.
func TestSymGlyphRectsForStyle_CheckboxStatesDiffer(t *testing.T) {
	off, ok := symGlyphRectsForStyle(GlyphStyleRounded, SymCheckboxOff, 48, 16, 1)
	if !ok {
		t.Fatal("SymCheckboxOff: no shape")
	}
	on, ok := symGlyphRectsForStyle(GlyphStyleRounded, SymCheckboxOn, 48, 16, 1)
	if !ok {
		t.Fatal("SymCheckboxOn: no shape")
	}
	mixed, ok := symGlyphRectsForStyle(GlyphStyleRounded, SymCheckboxMixed, 48, 16, 1)
	if !ok {
		t.Fatal("SymCheckboxMixed: no shape")
	}

	if sameClassicRects(off, on) {
		t.Error("SymCheckboxOff and SymCheckboxOn produced the same rects")
	}
	if sameClassicRects(off, mixed) {
		t.Error("SymCheckboxOff and SymCheckboxMixed produced the same rects")
	}
	if sameClassicRects(on, mixed) {
		t.Error("SymCheckboxOn and SymCheckboxMixed produced the same rects")
	}
}

// TestSymGlyphRectsForStyle_RadioStatesDiffer is
// TestSymGlyphRectsForStyle_CheckboxStatesDiffer for the radio glyphs.
func TestSymGlyphRectsForStyle_RadioStatesDiffer(t *testing.T) {
	off, ok := symGlyphRectsForStyle(GlyphStyleRounded, SymRadioOff, 48, 16, 1)
	if !ok {
		t.Fatal("SymRadioOff: no shape")
	}
	on, ok := symGlyphRectsForStyle(GlyphStyleRounded, SymRadioOn, 48, 16, 1)
	if !ok {
		t.Fatal("SymRadioOn: no shape")
	}
	if sameClassicRects(off, on) {
		t.Error("SymRadioOff and SymRadioOn produced the same rects")
	}
	if len(on) <= len(off) {
		t.Errorf("SymRadioOn (%d rects) should add an inner dot on top of SymRadioOff's outline (%d rects)", len(on), len(off))
	}
}

// TestSymGlyphRectsForStyle_DeclinesNonPositiveDimensions mirrors
// glyphRectsForStyle's own guard (classic_glyph.go) for degenerate cell
// sizes.
func TestSymGlyphRectsForStyle_DeclinesNonPositiveDimensions(t *testing.T) {
	for _, dims := range [][2]float64{{0, 16}, {48, 0}, {-1, 16}, {48, -1}} {
		if rects, ok := symGlyphRectsForStyle(GlyphStyleRounded, SymCheckboxOn, dims[0], dims[1], 1); ok || rects != nil {
			t.Errorf("symGlyphRectsForStyle(Rounded, SymCheckboxOn, %v, %v) = (%v, %v), want (nil, false)", dims[0], dims[1], rects, ok)
		}
	}
}
