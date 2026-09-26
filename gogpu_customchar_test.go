//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"image"
	"testing"

	"github.com/gogpu/gg"
)

// Proves which glyphs the GPU renderer draws as vector shapes vs which fall
// back to dc.DrawString. Fallback renders as tofu when the font lacks the
// codepoint - a real symptom on Windows (boxes instead of ┌┐└┘ ↑ ▓ ▒).
func TestGogpuRenderer_CustomCharVectorCoverage(t *testing.T) {
	cases := []struct {
		name string
		ch   rune
		want bool // true = must be vector-drawn, false = font fallback is OK
	}{
		// Everything the vtui/f4 widget set actually writes:
		{"h", '─', true}, {"v", '│', true},
		{"tl", '┌', true}, {"tr", '┐', true}, {"bl", '└', true}, {"br", '┘', true},
		{"tee-l", '├', true}, {"tee-r", '┤', true}, {"tee-t", '┬', true}, {"tee-b", '┴', true}, {"cross", '┼', true},
		{"dh", '═', true}, {"dv", '║', true}, {"dtl", '╔', true}, {"dtr", '╗', true},
		{"dbl", '╚', true}, {"dbr", '╝', true}, {"dtee-l", '╠', true}, {"dtee-r", '╣', true},
		{"dtee-t", '╦', true}, {"dtee-b", '╩', true}, {"dcross", '╬', true},
		{"vmenu-l", '╟', true}, {"vmenu-r", '╢', true},
		{"up", '↑', true}, {"down", '↓', true}, {"updown", '↕', true},
		{"tri-up", '▲', true}, {"tri-down", '▼', true},
		{"full", '█', true}, {"upper", '▀', true}, {"lower", '▄', true},
		{"left", '▌', true}, {"right", '▐', true},
		{"shade-light", '░', true}, {"shade-med", '▒', true}, {"shade-dark", '▓', true},
		{"arrow-r", '→', true}, {"arrow-l", '←', true}, {"arrow-both", '↔', true},
		{"q-tl", '▘', true}, {"q-tr", '▗', true}, {"q-bl", '▖', true}, {"q-br", '▝', true},
		{"bar-top", '▔', true}, {"bar-right", '▕', true},
		{"space", ' ', false}, {"letter", 'A', false},
	}

	// Assert the whole used set is vector-drawn; anything outside the switch
	// falls back to the font (possible tofu).
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewGogpuRenderer(nil, nil, 8, 16)
			dc := gg.NewContext(8, 16)
			dc.SetRGB(1, 1, 1)
			dc.Clear()
			ok := r.drawCustomChar(dc, tc.ch, 0, 0, 8, 16, 0)
			if ok != tc.want {
				t.Errorf("drawCustomChar(%q) = %v, want %v", tc.ch, ok, tc.want)
			}
			if !ok {
				return
			}
			img := dc.Image()
			nonWhite := 0
			for y := 0; y < 16; y++ {
				for x := 0; x < 8; x++ {
					r32, g32, b32, _ := img.At(x, y).RGBA()
					if r32 != 0xFFFF || g32 != 0xFFFF || b32 != 0xFFFF {
						nonWhite++
					}
				}
			}
			if nonWhite == 0 {
				t.Errorf("drawCustomChar(%q) claimed success but drew nothing", tc.ch)
			}
		})
	}
}

// countNonWhite reports how many pixels of a WxH gg.Context drawn over a
// white background are no longer white -- the same "did it actually draw
// something" probe TestGogpuRenderer_CustomCharVectorCoverage uses above.
func countNonWhite(dc *gg.Context, w, h int) int {
	img := dc.Image()
	n := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r32, g32, b32, _ := img.At(x, y).RGBA()
			if r32 != 0xFFFF || g32 != 0xFFFF || b32 != 0xFFFF {
				n++
			}
		}
	}
	return n
}

// imagesPixelEqual reports whether two images agree on every pixel in a WxH
// area, compared through the image.Image interface so it works whatever
// concrete image type gg.Context.Image() returns.
func imagesPixelEqual(a, b image.Image, w, h int) bool {
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ar, ag, ab, aa := a.At(x, y).RGBA()
			br, bg, bb, ba := b.At(x, y).RGBA()
			if ar != br || ag != bg || ab != bb || aa != ba {
				return false
			}
		}
	}
	return true
}

// TestGogpuRenderer_SymGlyphShape_ClassicDeclines confirms drawSymGlyphShape
// (gogpu_renderer.go), the vector-drawing counterpart of drawCustomChar for
// checkbox/radio SymGlyph tokens (symchar.go), draws nothing under
// GlyphStyleClassic: the graphics path must stay pixel-identical to before
// f4#285's shape slice, the same invariant TestSymChar_GraphicsPathUnaffected
// pins down for the ordinary font path.
func TestGogpuRenderer_SymGlyphShape_ClassicDeclines(t *testing.T) {
	previous := CurrentGlyphStyle()
	t.Cleanup(func() { SetGlyphStyle(previous) })
	SetGlyphStyle(GlyphStyleClassic)

	r := NewGogpuRenderer(nil, nil, 8, 16)
	for _, sym := range allSymGlyphs {
		dc := gg.NewContext(24, 16)
		dc.SetRGB(1, 1, 1)
		dc.Clear()
		if r.drawSymGlyphShape(dc, sym, 0, 0, 24, 16) {
			t.Errorf("drawSymGlyphShape(%v) = true under GlyphStyleClassic, want false", sym)
		}
		if n := countNonWhite(dc, 24, 16); n != 0 {
			t.Errorf("drawSymGlyphShape(%v) declined under GlyphStyleClassic but drew %d pixels", sym, n)
		}
	}
}

// TestGogpuRenderer_SymGlyphShape_RoundedDrawsAndStatesDiffer confirms
// drawSymGlyphShape draws a real shape under GlyphStyleRounded, and that a
// checked/mixed/selected state renders visibly differently from its
// unchecked/unselected counterpart.
func TestGogpuRenderer_SymGlyphShape_RoundedDrawsAndStatesDiffer(t *testing.T) {
	previous := CurrentGlyphStyle()
	t.Cleanup(func() { SetGlyphStyle(previous) })
	SetGlyphStyle(GlyphStyleRounded)

	r := NewGogpuRenderer(nil, nil, 8, 16)
	render := func(sym SymGlyph) *gg.Context {
		dc := gg.NewContext(24, 16)
		dc.SetRGB(1, 1, 1)
		dc.Clear()
		dc.SetRGB(0, 0, 0)
		if !r.drawSymGlyphShape(dc, sym, 0, 0, 24, 16) {
			t.Fatalf("drawSymGlyphShape(%v) = false under GlyphStyleRounded, want true", sym)
		}
		if n := countNonWhite(dc, 24, 16); n == 0 {
			t.Fatalf("drawSymGlyphShape(%v) claimed success but drew nothing", sym)
		}
		return dc
	}

	off, on, mixed := render(SymCheckboxOff), render(SymCheckboxOn), render(SymCheckboxMixed)
	if imagesPixelEqual(off.Image(), on.Image(), 24, 16) {
		t.Error("SymCheckboxOff and SymCheckboxOn rendered identically")
	}
	if imagesPixelEqual(off.Image(), mixed.Image(), 24, 16) {
		t.Error("SymCheckboxOff and SymCheckboxMixed rendered identically")
	}
	if imagesPixelEqual(on.Image(), mixed.Image(), 24, 16) {
		t.Error("SymCheckboxOn and SymCheckboxMixed rendered identically")
	}

	radioOff, radioOn := render(SymRadioOff), render(SymRadioOn)
	if imagesPixelEqual(radioOff.Image(), radioOn.Image(), 24, 16) {
		t.Error("SymRadioOff and SymRadioOn rendered identically")
	}
}
