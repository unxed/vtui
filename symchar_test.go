package vtui

import (
	"testing"
	"unicode/utf8"
)

// allSymGlyphs lists every symbol variant defined today, for tests that walk
// the whole table.
var allSymGlyphs = []SymGlyph{SymCheckboxOff, SymCheckboxOn, SymCheckboxMixed, SymRadioOff, SymRadioOn}

// buttonEarSyms lists the button-ear symbol variants. They are kept out of
// allSymGlyphs because, unlike the checkbox/radio glyphs, they are 2 cells
// wide rather than 3, so the generic loops above that walk parts 0..2 do not
// apply to them; they get their own tests below instead.
var buttonEarSyms = []SymGlyph{SymButtonEarLeft, SymButtonEarRight}

func TestSymCharToken_RoundTrip(t *testing.T) {
	for _, sym := range allSymGlyphs {
		for part := 0; part < 3; part++ {
			tok := SymCharToken(sym, part)
			gotSym, gotPart, ok := DecodeSymChar(tok)
			if !ok {
				t.Fatalf("DecodeSymChar(SymCharToken(%v, %d)) reported ok=false", sym, part)
			}
			if gotSym != sym || gotPart != part {
				t.Errorf("DecodeSymChar(SymCharToken(%v, %d)) = (%v, %d), want (%v, %d)", sym, part, gotSym, gotPart, sym, part)
			}
			if !IsSymChar(tok) {
				t.Errorf("IsSymChar(SymCharToken(%v, %d)) = false, want true", sym, part)
			}
		}
	}
}

// TestSymCharFlag_NoCollision locks in the bit layout the design comment on
// f4#285 promises: SymCharFlag (bit 62) can never be mistaken for
// CompCharFlag (bit 63), for WideCharFiller (all bits set), or for any real
// Unicode code point (max U+10FFFF) -- including Private Use Area code
// points a Nerd Font might use.
func TestSymCharFlag_NoCollision(t *testing.T) {
	if SymCharFlag&CompCharFlag != 0 {
		t.Fatalf("SymCharFlag (%#x) overlaps CompCharFlag (%#x)", SymCharFlag, CompCharFlag)
	}
	if SymCharFlag == WideCharFiller {
		t.Fatalf("SymCharFlag (%#x) equals WideCharFiller", SymCharFlag)
	}

	for _, sym := range allSymGlyphs {
		for part := 0; part < 3; part++ {
			tok := SymCharToken(sym, part)

			if tok <= uint64(utf8.MaxRune) {
				t.Errorf("SymCharToken(%v, %d) = %#x collides with a valid Unicode code point range", sym, part, tok)
			}
			if tok&CompCharFlag != 0 {
				t.Errorf("SymCharToken(%v, %d) = %#x sets CompCharFlag", sym, part, tok)
			}
			if IsCompChar(tok) {
				t.Errorf("IsCompChar(SymCharToken(%v, %d)) = true, want false", sym, part)
			}
			if tok == WideCharFiller {
				t.Errorf("SymCharToken(%v, %d) collides with WideCharFiller", sym, part)
			}

			// A registry index (IsCompChar) must never be mistaken for a
			// symbolic token, and vice versa.
			registryIdx := CompCharFlag | uint64(part+1)
			if IsSymChar(registryIdx) {
				t.Errorf("IsSymChar(%#x) = true for a plain CompChar registry index", registryIdx)
			}
		}
	}

	if IsSymChar(WideCharFiller) {
		t.Error("IsSymChar(WideCharFiller) = true, want false")
	}
	if IsSymChar(0) {
		t.Error("IsSymChar(0) = true, want false")
	}
	for _, r := range []rune{'[', ']', 'x', '?', '(', ')', ' ', '•', 0x10FFFF} {
		// #nosec G115 -- every value in the literal above is a small
		// non-negative rune constant.
		if IsSymChar(uint64(r)) {
			t.Errorf("IsSymChar(%q) = true, want false", r)
		}
	}
}

// TestSymGlyphClassicText_MatchesLiteral pins the classic style's
// cell-by-cell expansion of every token to the literal text checkbox/radio
// widgets wrote directly before f4#285: CellString and CellBaseRune are the
// choke point every text-mode backend (ANSI, Windows console, far2l-
// terminal, screendump, tests) and every graphics backend go through, so
// this table is what keeps their classic-style output byte/pixel-identical.
func TestSymGlyphClassicText_MatchesLiteral(t *testing.T) {
	cases := []struct {
		sym   SymGlyph
		cells [3]string
	}{
		{SymCheckboxOff, [3]string{"[", " ", "]"}},
		{SymCheckboxOn, [3]string{"[", "x", "]"}},
		{SymCheckboxMixed, [3]string{"[", "?", "]"}},
		{SymRadioOff, [3]string{"(", " ", ")"}},
		{SymRadioOn, [3]string{"(", "•", ")"}},
	}
	for _, c := range cases {
		for part := 0; part < 3; part++ {
			tok := SymCharToken(c.sym, part)
			want := c.cells[part]
			if got := CellString(tok); got != want {
				t.Errorf("CellString(SymCharToken(%v, %d)) = %q, want %q", c.sym, part, got, want)
			}
			wantRune, _ := utf8.DecodeRuneInString(want)
			if got := CellBaseRune(tok); got != wantRune {
				t.Errorf("CellBaseRune(SymCharToken(%v, %d)) = %q, want %q", c.sym, part, got, wantRune)
			}
			if got := CellRunes(tok); len(got) != 1 || got[0] != wantRune {
				t.Errorf("CellRunes(SymCharToken(%v, %d)) = %v, want [%q]", c.sym, part, got, wantRune)
			}
		}
	}
}

// TestSymChar_GraphicsPathUnaffected confirms that once a graphics backend
// resolves a token through CellBaseRune, nothing routes it into the
// geometric box-drawing table (classic_glyph.go, gui_boxdraw.go): checkbox
// and radio glyphs were always plain font-rendered text, never a shape, so
// "classic" needs no shape-table entry to stay pixel-identical (f4#285). The
// gogpu-specific half of this guarantee (the batched-string fast path never
// sees a raw token) is covered separately in symchar_gogpu_test.go, which
// carries the same build constraint as gogpuBatchRune's home file.
func TestSymChar_GraphicsPathUnaffected(t *testing.T) {
	for _, sym := range allSymGlyphs {
		for part := 0; part < 3; part++ {
			tok := SymCharToken(sym, part)
			base := CellBaseRune(tok)
			if isBoxDrawRune(base) {
				t.Errorf("isBoxDrawRune(CellBaseRune(SymCharToken(%v, %d))) = true for %q, want false", sym, part, base)
			}
			if _, ok := classicGlyphRects(base, 16, 16, 1); ok {
				t.Errorf("classicGlyphRects claims a shape for %q (from SymCharToken(%v, %d))", base, sym, part)
			}
		}
	}
}

// TestCheckbox_ClassicTextRoundTrip renders a Checkbox in each of its states
// and checks the row of text a terminal backend would see is byte-identical
// to the literal "[ ] "/"[x] "/"[?] " text the widget wrote directly before
// it was rewritten to emit SymCharFlag tokens.
func TestCheckbox_ClassicTextRoundTrip(t *testing.T) {
	SetDefaultPalette()
	cases := []struct {
		state int
		want  string
	}{
		{0, "[ ] Label"},
		{1, "[x] Label"},
		{2, "[?] Label"},
	}
	for _, c := range cases {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(20, 1)
		cb := NewCheckbox(0, 0, "Label", true)
		cb.State = c.state
		cb.Show(scr)

		if got := ScreenRow(scr, 0, 0, 8); got != c.want {
			t.Errorf("state %d: ScreenRow = %q, want %q", c.state, got, c.want)
		}
	}
}

// TestRadioButton_ClassicTextRoundTrip is TestCheckbox_ClassicTextRoundTrip
// for RadioButton: "( ) "/"(•) ".
func TestRadioButton_ClassicTextRoundTrip(t *testing.T) {
	SetDefaultPalette()
	cases := []struct {
		selected bool
		want     string
	}{
		{false, "( ) Label"},
		{true, "(•) Label"},
	}
	for _, c := range cases {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(20, 1)
		rb := NewRadioButton(0, 0, "Label", c.selected)
		rb.Show(scr)

		if got := ScreenRow(scr, 0, 0, 8); got != c.want {
			t.Errorf("selected=%v: ScreenRow = %q, want %q", c.selected, got, c.want)
		}
	}
}

// TestRadioGroup_ClassicTextRoundTrip and TestCheckGroup_ClassicTextRoundTrip
// cover the grid widgets, whose indicator and item text used to be written
// as a single literal "( ) "+item / "[ ] "+item string.
func TestRadioGroup_ClassicTextRoundTrip(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 1)
	rg := NewRadioGroup(0, 0, 1, []string{"One", "Two"})
	rg.Selected = 1
	rg.Show(scr)

	if got := ScreenRow(scr, 0, 0, 6); got != "( ) One" {
		t.Errorf("row 0 = %q, want %q", got, "( ) One")
	}
}

func TestCheckGroup_ClassicTextRoundTrip(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 1)
	cg := NewCheckGroup(0, 0, 1, []string{"One"})
	cg.States[0] = true
	cg.Show(scr)

	if got := ScreenRow(scr, 0, 0, 6); got != "[x] One" {
		t.Errorf("row 0 = %q, want %q", got, "[x] One")
	}
}

// TestSymButtonEar_RoundTrip is TestSymCharToken_RoundTrip for the 2-cell
// button-ear glyphs.
func TestSymButtonEar_RoundTrip(t *testing.T) {
	for _, sym := range buttonEarSyms {
		for part := 0; part < 2; part++ {
			tok := SymCharToken(sym, part)
			gotSym, gotPart, ok := DecodeSymChar(tok)
			if !ok {
				t.Fatalf("DecodeSymChar(SymCharToken(%v, %d)) reported ok=false", sym, part)
			}
			if gotSym != sym || gotPart != part {
				t.Errorf("DecodeSymChar(SymCharToken(%v, %d)) = (%v, %d), want (%v, %d)", sym, part, gotSym, gotPart, sym, part)
			}
			if !IsSymChar(tok) {
				t.Errorf("IsSymChar(SymCharToken(%v, %d)) = false, want true", sym, part)
			}
		}
	}
}

// TestSymButtonEar_NoCollision is TestSymCharFlag_NoCollision for the 2-cell
// button-ear glyphs: their tokens must stay outside Unicode, off
// CompCharFlag/WideCharFiller, and distinct from every checkbox/radio token
// (f4#285 extends the same SymGlyph enum PR #134 introduced, so a bug that
// aliased a new variant onto an old one would be easy to make by accident).
func TestSymButtonEar_NoCollision(t *testing.T) {
	seen := map[uint64]string{}
	for _, sym := range allSymGlyphs {
		for part := 0; part < 3; part++ {
			seen[SymCharToken(sym, part)] = "checkbox/radio"
		}
	}

	for _, sym := range buttonEarSyms {
		for part := 0; part < 2; part++ {
			tok := SymCharToken(sym, part)

			if tok <= uint64(utf8.MaxRune) {
				t.Errorf("SymCharToken(%v, %d) = %#x collides with a valid Unicode code point range", sym, part, tok)
			}
			if tok&CompCharFlag != 0 {
				t.Errorf("SymCharToken(%v, %d) = %#x sets CompCharFlag", sym, part, tok)
			}
			if IsCompChar(tok) {
				t.Errorf("IsCompChar(SymCharToken(%v, %d)) = true, want false", sym, part)
			}
			if tok == WideCharFiller {
				t.Errorf("SymCharToken(%v, %d) collides with WideCharFiller", sym, part)
			}
			if owner, ok := seen[tok]; ok {
				t.Errorf("SymCharToken(%v, %d) = %#x collides with an existing %s token", sym, part, tok, owner)
			}
			seen[tok] = "button ear"
		}
	}

	for _, r := range []rune{'[', ']', ' ', 0x10FFFF} {
		// #nosec G115 -- every value in the literal above is a small
		// non-negative rune constant.
		if IsSymChar(uint64(r)) {
			t.Errorf("IsSymChar(%q) = true, want false", r)
		}
	}
}

// TestSymButtonEarClassicText_MatchesLiteral pins the classic style's
// cell-by-cell expansion of a button-ear token to the literal "[ "/" ]"
// text Button wrote directly before f4#285, and confirms the expansion
// tracks UIStrings.ButtonBrackets (strings.go) rather than a baked-in "["/
// "]", since an application can relocalize those brackets.
func TestSymButtonEarClassicText_MatchesLiteral(t *testing.T) {
	origBrackets := UIStrings.ButtonBrackets
	t.Cleanup(func() { UIStrings.ButtonBrackets = origBrackets })

	cases := []struct {
		brackets [2]rune
		left     [2]string
		right    [2]string
	}{
		{[2]rune{'[', ']'}, [2]string{"[", " "}, [2]string{" ", "]"}},
		{[2]rune{'<', '>'}, [2]string{"<", " "}, [2]string{" ", ">"}},
	}
	for _, c := range cases {
		UIStrings.ButtonBrackets = c.brackets
		for part := 0; part < 2; part++ {
			tok := SymCharToken(SymButtonEarLeft, part)
			want := c.left[part]
			if got := CellString(tok); got != want {
				t.Errorf("brackets %v: CellString(SymButtonEarLeft, %d) = %q, want %q", c.brackets, part, got, want)
			}
			wantRune, _ := utf8.DecodeRuneInString(want)
			if got := CellBaseRune(tok); got != wantRune {
				t.Errorf("brackets %v: CellBaseRune(SymButtonEarLeft, %d) = %q, want %q", c.brackets, part, got, wantRune)
			}
			if got := CellRunes(tok); len(got) != 1 || got[0] != wantRune {
				t.Errorf("brackets %v: CellRunes(SymButtonEarLeft, %d) = %v, want [%q]", c.brackets, part, got, wantRune)
			}

			tok = SymCharToken(SymButtonEarRight, part)
			want = c.right[part]
			if got := CellString(tok); got != want {
				t.Errorf("brackets %v: CellString(SymButtonEarRight, %d) = %q, want %q", c.brackets, part, got, want)
			}
		}
	}
}

// TestSymButtonEar_GraphicsPathUnaffected is TestSymChar_GraphicsPathUnaffected
// for the button-ear glyphs: a button's ears were always plain font-rendered
// text ("[ "/" ]"), never a geometric shape, so "classic" needs no
// shape-table entry (classic_glyph.go) to stay pixel-identical either.
func TestSymButtonEar_GraphicsPathUnaffected(t *testing.T) {
	for _, sym := range buttonEarSyms {
		for part := 0; part < 2; part++ {
			tok := SymCharToken(sym, part)
			base := CellBaseRune(tok)
			if isBoxDrawRune(base) {
				t.Errorf("isBoxDrawRune(CellBaseRune(SymCharToken(%v, %d))) = true for %q, want false", sym, part, base)
			}
			if _, ok := classicGlyphRects(base, 16, 16, 1); ok {
				t.Errorf("classicGlyphRects claims a shape for %q (from SymCharToken(%v, %d))", base, sym, part)
			}
		}
	}
}

// TestButton_ClassicTextRoundTrip renders a Button, with and without a
// hotkey, and checks the row of text a terminal backend would see is
// byte-identical to the literal "[ Label ]" text the widget wrote directly
// before it was rewritten to emit SymCharFlag tokens for its ears.
func TestButton_ClassicTextRoundTrip(t *testing.T) {
	SetDefaultPalette()
	cases := []struct {
		text string
		want string
	}{
		{"OK", "[ OK ]"},
		{"&OK", "[ OK ]"},
		{"", "[  ]"},
	}
	for _, c := range cases {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(20, 1)
		b := NewButton(0, 0, c.text)
		b.Show(scr)

		if got := ScreenRow(scr, 0, 0, StringWidth(b.cleanText)-1); got != c.want {
			t.Errorf("text %q: ScreenRow = %q, want %q", c.text, got, c.want)
		}
	}
}
