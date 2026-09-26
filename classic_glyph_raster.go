//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"image"
	"math"
)

func drawClassicGlyph(img *image.RGBA, char rune, px, py, cw, ch, thick int, rgb uint32) bool {
	rects, ok := classicGlyphRects(char, float64(cw), float64(ch), float64(thick))
	if !ok {
		return false
	}
	fillClassicRects(img, rects, px, py, rgb)
	return true
}

// drawSymGlyphRaster draws one checkbox/radio SymGlyph token (symchar.go)
// as a geometric shape spanning the w3 x ch pixel area of the 3 cells the
// token occupies -- the X11/Wayland raster analogue of drawClassicGlyph for
// box-drawing runes, sharing the same fillClassicRects fill loop. false
// means the active GlyphStyle has no shape for sym (always the case for
// GlyphStyleClassic, see symGlyphRectsForStyle), so the caller falls
// through to its ordinary per-cell font path unchanged.
func drawSymGlyphRaster(img *image.RGBA, sym SymGlyph, px, py, w3, ch, thick int, rgb uint32) bool {
	rects, ok := symGlyphRects(sym, float64(w3), float64(ch), float64(thick))
	if !ok {
		return false
	}
	fillClassicRects(img, rects, px, py, rgb)
	return true
}

// fillClassicRects paints a list of cell-relative classicRect primitives
// (classic_glyph.go) onto img, offset by (px, py), one solid colour. Shared
// by drawClassicGlyph (box-drawing) and drawSymGlyphRaster (checkbox/radio
// shapes) so both stay pixel-consistent with the same rounding rules.
func fillClassicRects(img *image.RGBA, rects []classicRect, px, py int, rgb uint32) {
	r8, g8, b8 := uint8((rgb>>16)&0xff), uint8((rgb>>8)&0xff), uint8(rgb&0xff)
	for _, rect := range rects {
		x0 := px + int(math.Floor(rect.x))
		y0 := py + int(math.Floor(rect.y))
		x1 := px + int(math.Ceil(rect.x+rect.w)) - 1
		y1 := py + int(math.Ceil(rect.y+rect.h)) - 1
		for y := y0; y <= y1; y++ {
			drawBoxHLine(img, x0, x1, y, 1, r8, g8, b8)
		}
	}
}
