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
	return true
}
