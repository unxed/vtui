//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
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

// pixelSnapshot copies every pixel of a WxH image into an independent Go
// slice, read immediately after drawing. Comparisons in the tests below use
// this instead of holding several *gg.Context values alive and reading them
// back later, and instead of asserting what an undrawn background's exact
// colour value is: both would tie the test to gg.Context implementation
// details (buffer reuse across contexts, Clear()'s exact colour semantics)
// that have nothing to do with what this slice of f4#285 actually changed.
func pixelSnapshot(dc *gg.Context, w, h int) []uint64 {
	img := dc.Image()
	out := make([]uint64, w*h)
	i := 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r32, g32, b32, a32 := img.At(x, y).RGBA()
			out[i] = uint64(r32)<<48 | uint64(g32)<<32 | uint64(b32)<<16 | uint64(a32)
			i++
		}
	}
	return out
}

func pixelSnapshotsEqual(a, b []uint64) bool {
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

// newBlankGogpuCanvas builds a WxH canvas cleared to white and immediately
// snapshots it, giving every test below the same "what an untouched canvas
// looks like" baseline without asserting a specific colour value for it.
func newBlankGogpuCanvas(w, h int) []uint64 {
	dc := gg.NewContext(w, h)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	return pixelSnapshot(dc, w, h)
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

	blank := newBlankGogpuCanvas(24, 16)
	r := NewGogpuRenderer(nil, nil, 8, 16)
	for _, sym := range allSymGlyphs {
		dc := gg.NewContext(24, 16)
		dc.SetRGB(1, 1, 1)
		dc.Clear()
		if r.drawSymGlyphShape(dc, sym, 0, 0, 24, 16) {
			t.Errorf("drawSymGlyphShape(%v) = true under GlyphStyleClassic, want false", sym)
		}
		if got := pixelSnapshot(dc, 24, 16); !pixelSnapshotsEqual(got, blank) {
			t.Errorf("drawSymGlyphShape(%v) declined under GlyphStyleClassic but changed the canvas", sym)
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

	blank := newBlankGogpuCanvas(24, 16)
	r := NewGogpuRenderer(nil, nil, 8, 16)
	render := func(sym SymGlyph) []uint64 {
		dc := gg.NewContext(24, 16)
		dc.SetRGB(1, 1, 1)
		dc.Clear()
		dc.SetRGB(0, 0, 0)
		rects, ok := symGlyphRects(sym, 24, 16, 1.0)
		t.Logf("DIAG sym=%v ok=%v rects=%+v", sym, ok, rects)
		if !r.drawSymGlyphShape(dc, sym, 0, 0, 24, 16) {
			t.Fatalf("drawSymGlyphShape(%v) = false under GlyphStyleRounded, want true", sym)
		}
		snap := pixelSnapshot(dc, 24, 16)
		if pixelSnapshotsEqual(snap, blank) {
			t.Fatalf("drawSymGlyphShape(%v) claimed success but drew nothing", sym)
		}
		return snap
	}

	off, on, mixed := render(SymCheckboxOff), render(SymCheckboxOn), render(SymCheckboxMixed)
	if pixelSnapshotsEqual(off, on) {
		t.Error("SymCheckboxOff and SymCheckboxOn rendered identically")
	}
	if pixelSnapshotsEqual(off, mixed) {
		t.Error("SymCheckboxOff and SymCheckboxMixed rendered identically")
	}
	if pixelSnapshotsEqual(on, mixed) {
		t.Error("SymCheckboxOn and SymCheckboxMixed rendered identically")
	}

	radioOff, radioOn := render(SymRadioOff), render(SymRadioOn)
	if pixelSnapshotsEqual(radioOff, radioOn) {
		t.Error("SymRadioOff and SymRadioOn rendered identically")
	}
}
