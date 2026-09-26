//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"bytes"
	"image"
	"testing"
)

func newTestSurface(w, h int) *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, w, h))
}

func litPixels(img *image.RGBA) int {
	n := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i] != 0 || img.Pix[i+1] != 0 || img.Pix[i+2] != 0 {
			n++
		}
	}
	return n
}

func TestDrawBoxGlyph_DrawsKnownRunes(t *testing.T) {
	const white = uint32(0xffffff)
	for _, r := range []rune{'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼',
		'═', '║', '╔', '╗', '╚', '╝', '╠', '╣', '↑', '↓', '▲', '▼', '█'} {
		img := newTestSurface(16, 16)
		if !drawBoxGlyph(img, r, 0, 0, 16, 16, 1, white) {
			t.Errorf("drawBoxGlyph(%q) returned false, want it handled", r)
			continue
		}
		if litPixels(img) == 0 {
			t.Errorf("drawBoxGlyph(%q) handled the rune but drew nothing", r)
		}
	}
}

// Unhandled runes must report false without touching the surface, so the
// caller can fall back to the font over untouched background.
func TestDrawBoxGlyph_DeclinesUnknownRunes(t *testing.T) {
	for _, r := range []rune{'A', 'ж', '0', ' ', '字', '€'} {
		img := newTestSurface(16, 16)
		if drawBoxGlyph(img, r, 0, 0, 16, 16, 1, 0xffffff) {
			t.Errorf("drawBoxGlyph(%q) claimed to handle a text rune", r)
		}
		if litPixels(img) != 0 {
			t.Errorf("drawBoxGlyph(%q) declined but still drew", r)
		}
	}
}

// Drawing near the edge must clip rather than panic or wrap onto the next row.
func TestDrawBoxGlyph_ClipsAtSurfaceEdge(t *testing.T) {
	img := newTestSurface(16, 16)
	for _, r := range []rune{'┼', '╬', '█', '▲'} {
		drawBoxGlyph(img, r, 12, 12, 8, 8, 2, 0xffffff) // runs past both edges
	}
	drawBoxGlyph(img, '┼', -4, -4, 8, 8, 1, 0xffffff)
}

// Adjacent horizontal frame cells must join: the seam column between two
// cells has to be lit, which is the whole reason these are not drawn as text.
func TestDrawBoxGlyph_AdjacentCellsJoin(t *testing.T) {
	const cw, ch = 8, 16
	img := newTestSurface(cw*2, ch)
	drawBoxGlyph(img, '─', 0, 0, cw, ch, 1, 0xffffff)
	drawBoxGlyph(img, '─', cw, 0, cw, ch, 1, 0xffffff)

	mid := ch / 2
	for x := 0; x < cw*2; x++ {
		off := mid*img.Stride + x*4
		if img.Pix[off] == 0 && img.Pix[off+1] == 0 && img.Pix[off+2] == 0 {
			t.Fatalf("gap in a two-cell horizontal rule at x=%d", x)
		}
	}
}

// A thicker line for HiDPI must actually be thicker.
func TestDrawBoxGlyph_ScaleThickensLines(t *testing.T) {
	thin := newTestSurface(16, 16)
	thick := newTestSurface(16, 16)
	drawBoxGlyph(thin, '─', 0, 0, 16, 16, 1, 0xffffff)
	drawBoxGlyph(thick, '─', 0, 0, 16, 16, 3, 0xffffff)

	if litPixels(thick) <= litPixels(thin) {
		t.Errorf("scale 3 lit %d pixels, scale 1 lit %d; expected more",
			litPixels(thick), litPixels(thin))
	}
}

// Every image-backed GUI backend goes through drawBoxGlyph. Keep the shared
// classic table pixel-compatible with the raster geometry it replaced; this
// also protects X11, Wayland, Win32 GUI and Ebiten from backend-specific drift.
func TestDrawBoxGlyph_ClassicMatchesLegacyRasterGeometry(t *testing.T) {
	// ╬ is intentionally omitted: the old raster switch did not handle it;
	// classicGlyphRects coverage verifies the newly shared implementation.
	runes := []rune{
		'─', '│', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼',
		'═', '║', '╔', '╗', '╚', '╝', '╠', '╣', '╩', '╦', '╟', '╢',
	}
	for _, size := range []struct{ w, h int }{{8, 16}, {10, 20}, {16, 16}} {
		for _, thick := range []int{1, 2, 3} {
			for _, char := range runes {
				got := newTestSurface(size.w, size.h)
				want := newTestSurface(size.w, size.h)
				if !drawBoxGlyph(got, char, 0, 0, size.w, size.h, thick, 0x204060) {
					t.Fatalf("drawBoxGlyph(%q) declined a classic rune", char)
				}
				if !drawBoxGlyphLegacy(want, char, 0, 0, size.w, size.h, thick, 0x204060) {
					t.Fatalf("drawBoxGlyphLegacy(%q) declined a classic rune", char)
				}
				if !bytes.Equal(got.Pix, want.Pix) {
					t.Errorf("drawBoxGlyph(%q, %dx%d, thick=%d) changed classic raster geometry", char, size.w, size.h, thick)
				}
			}
		}
	}
}

func TestGlyphStyleRoundedOnlyChangesSingleCorners(t *testing.T) {
	previous := CurrentGlyphStyle()
	t.Cleanup(func() { SetGlyphStyle(previous) })

	classicCorner := newTestSurface(16, 16)
	classicLine := newTestSurface(16, 16)
	classicDouble := newTestSurface(16, 16)
	SetGlyphStyle(GlyphStyleClassic)
	drawBoxGlyph(classicCorner, '┌', 0, 0, 16, 16, 1, 0xffffff)
	drawBoxGlyph(classicLine, '─', 0, 0, 16, 16, 1, 0xffffff)
	drawBoxGlyph(classicDouble, '╔', 0, 0, 16, 16, 1, 0xffffff)

	roundedCorner := newTestSurface(16, 16)
	roundedLine := newTestSurface(16, 16)
	roundedDouble := newTestSurface(16, 16)
	SetGlyphStyle(GlyphStyleRounded)
	drawBoxGlyph(roundedCorner, '┌', 0, 0, 16, 16, 1, 0xffffff)
	drawBoxGlyph(roundedLine, '─', 0, 0, 16, 16, 1, 0xffffff)
	drawBoxGlyph(roundedDouble, '╔', 0, 0, 16, 16, 1, 0xffffff)

	if bytes.Equal(classicCorner.Pix, roundedCorner.Pix) {
		t.Fatal("rounded style did not change a single-line corner")
	}
	if !bytes.Equal(classicLine.Pix, roundedLine.Pix) {
		t.Fatal("rounded style changed a straight line")
	}
	if !bytes.Equal(classicDouble.Pix, roundedDouble.Pix) {
		t.Fatal("rounded style changed a double-line corner")
	}
}

// drawSymGlyphRaster is the X11/Wayland/Ebiten/Win32-GUI raster path for a
// checkbox/radio SymGlyph token (symchar.go): the counterpart of
// drawClassicGlyph for box-drawing runes, sharing the same fillClassicRects
// fill loop (classic_glyph_raster.go).

// Under GlyphStyleClassic every symbol must decline and draw nothing, so
// text is byte/pixel-identical to before this change, exactly like
// TestSymChar_GraphicsPathUnaffected already pins down for the ordinary
// font path.
func TestDrawSymGlyphRaster_ClassicDeclines(t *testing.T) {
	previous := CurrentGlyphStyle()
	t.Cleanup(func() { SetGlyphStyle(previous) })
	SetGlyphStyle(GlyphStyleClassic)

	for _, sym := range allSymGlyphs {
		img := newTestSurface(48, 16)
		if drawSymGlyphRaster(img, sym, 0, 0, 48, 16, 1, 0xffffff) {
			t.Errorf("drawSymGlyphRaster(%v) claimed a shape under GlyphStyleClassic", sym)
		}
		if litPixels(img) != 0 {
			t.Errorf("drawSymGlyphRaster(%v) declined under GlyphStyleClassic but still drew", sym)
		}
	}
}

// Under GlyphStyleRounded every checkbox/radio symbol must draw something,
// and a checked/mixed/selected state must look different from its
// unchecked/unselected counterpart -- the actual geometric rendering this
// slice of f4#285 adds.
func TestDrawSymGlyphRaster_RoundedDrawsAndStatesDiffer(t *testing.T) {
	previous := CurrentGlyphStyle()
	t.Cleanup(func() { SetGlyphStyle(previous) })
	SetGlyphStyle(GlyphStyleRounded)

	imgFor := func(sym SymGlyph) *image.RGBA {
		img := newTestSurface(48, 16)
		if !drawSymGlyphRaster(img, sym, 0, 0, 48, 16, 1, 0xffffff) {
			t.Fatalf("drawSymGlyphRaster(%v) declined under GlyphStyleRounded", sym)
		}
		if litPixels(img) == 0 {
			t.Fatalf("drawSymGlyphRaster(%v) claimed a shape but drew nothing", sym)
		}
		return img
	}

	off, on, mixed := imgFor(SymCheckboxOff), imgFor(SymCheckboxOn), imgFor(SymCheckboxMixed)
	if bytes.Equal(off.Pix, on.Pix) {
		t.Error("SymCheckboxOff and SymCheckboxOn rendered identically under GlyphStyleRounded")
	}
	if bytes.Equal(off.Pix, mixed.Pix) {
		t.Error("SymCheckboxOff and SymCheckboxMixed rendered identically under GlyphStyleRounded")
	}
	if bytes.Equal(on.Pix, mixed.Pix) {
		t.Error("SymCheckboxOn and SymCheckboxMixed rendered identically under GlyphStyleRounded")
	}

	radioOff, radioOn := imgFor(SymRadioOff), imgFor(SymRadioOn)
	if bytes.Equal(radioOff.Pix, radioOn.Pix) {
		t.Error("SymRadioOff and SymRadioOn rendered identically under GlyphStyleRounded")
	}
}

func TestIsBoxDrawRune(t *testing.T) {
	for _, r := range []rune{'─', '│', '┼', '═', '║', '╬', '█', '▄', '↑', '↕', '▲', '▼'} {
		if !isBoxDrawRune(r) {
			t.Errorf("isBoxDrawRune(%q) = false, want true", r)
		}
	}
	for _, r := range []rune{'A', 'z', '0', ' ', 'Ж', '字', '\n'} {
		if isBoxDrawRune(r) {
			t.Errorf("isBoxDrawRune(%q) = true, want false", r)
		}
	}
}

// Every rune the geometric path claims must also be one the range test lets
// through, or the renderer would send it to the font and never reach it.
func TestIsBoxDrawRune_CoversEverythingDrawBoxGlyphHandles(t *testing.T) {
	for r := rune(0x2000); r < 0x2800; r++ {
		img := newTestSurface(8, 8)
		if drawBoxGlyph(img, r, 0, 0, 8, 8, 1, 0xffffff) && !isBoxDrawRune(r) {
			t.Errorf("drawBoxGlyph handles %q (U+%04X) but isBoxDrawRune rejects it", r, r)
		}
	}
}
