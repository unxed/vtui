//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"image"
)

// drawBoxGlyph rasterizes box-drawing, arrow, and block characters
// geometrically to ensure seamless joins against the cell rectangle.
func drawBoxGlyph(img *image.RGBA, char rune, px, py, cw, ch, thick int, rgb uint32) bool {
	if drawClassicGlyph(img, char, px, py, cw, ch, thick, rgb) {
		return true
	}
	return drawBoxGlyphLegacy(img, char, px, py, cw, ch, thick, rgb)
}

func drawBoxGlyphLegacy(img *image.RGBA, char rune, px, py, cw, ch, thick int, rgb uint32) bool {
	mx, my := px+cw/2, py+ch/2
	if thick < 1 {
		thick = 1
	}
	r8, g8, b8 := uint8(rgb>>16), uint8(rgb>>8), uint8(rgb)

	ofs := cw / 4
	if ofs < 1 {
		ofs = 1
	}

	switch char {
	case '─':
		drawBoxHLine(img, px, px+cw-1, my, thick, r8, g8, b8)
		return true
	case '│':
		drawBoxVLine(img, mx, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '┌':
		drawBoxHLine(img, mx, px+cw-1, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, my, py+ch-1, thick, r8, g8, b8)
		return true
	case '┐':
		drawBoxHLine(img, px, mx, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, my, py+ch-1, thick, r8, g8, b8)
		return true
	case '└':
		drawBoxHLine(img, mx, px+cw-1, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, py, my, thick, r8, g8, b8)
		return true
	case '┘':
		drawBoxHLine(img, px, mx, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, py, my, thick, r8, g8, b8)
		return true
	case '├':
		drawBoxHLine(img, mx, px+cw-1, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '┤':
		drawBoxHLine(img, px, mx, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '┬':
		drawBoxHLine(img, px, px+cw-1, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, my, py+ch-1, thick, r8, g8, b8)
		return true
	case '┴':
		drawBoxHLine(img, px, px+cw-1, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, py, my, thick, r8, g8, b8)
		return true
	case '┼':
		drawBoxHLine(img, px, px+cw-1, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '═':
		drawBoxHLine(img, px, px+cw-1, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, px, px+cw-1, my+ofs, thick, r8, g8, b8)
		return true
	case '║':
		drawBoxVLine(img, mx-ofs, py, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '╔':
		drawBoxHLine(img, mx+ofs, px+cw-1, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, mx-ofs, px+cw-1, my+ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, my+ofs, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, my-ofs, py+ch-1, thick, r8, g8, b8)
		return true
	case '╗':
		drawBoxHLine(img, px, mx-ofs, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, px, mx+ofs, my+ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, my+ofs, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, my-ofs, py+ch-1, thick, r8, g8, b8)
		return true
	case '╚':
		drawBoxHLine(img, mx-ofs, px+cw-1, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, mx+ofs, px+cw-1, my+ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, py, my-ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, my+ofs, thick, r8, g8, b8)
		return true
	case '╝':
		drawBoxHLine(img, px, mx+ofs, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, px, mx-ofs, my+ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, my-ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, py, my+ofs, thick, r8, g8, b8)
		return true
	case '╠':
		drawBoxHLine(img, mx-ofs, px+cw-1, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, mx+ofs, px+cw-1, my+ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, py, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '╣':
		drawBoxHLine(img, px, mx+ofs, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, px, mx-ofs, my+ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, py, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '╟':
		drawBoxHLine(img, mx+ofs, px+cw-1, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, py, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '╢':
		drawBoxHLine(img, px, mx-ofs, my, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, py, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, py+ch-1, thick, r8, g8, b8)
		return true
	case '╩':
		drawBoxHLine(img, px, px+cw-1, my+ofs, thick, r8, g8, b8)
		drawBoxHLine(img, px, mx-ofs, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, mx+ofs, px+cw-1, my-ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, py, my-ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, py, my-ofs, thick, r8, g8, b8)
		return true
	case '╦':
		drawBoxHLine(img, px, px+cw-1, my-ofs, thick, r8, g8, b8)
		drawBoxHLine(img, px, mx-ofs, my+ofs, thick, r8, g8, b8)
		drawBoxHLine(img, mx+ofs, px+cw-1, my+ofs, thick, r8, g8, b8)
		drawBoxVLine(img, mx-ofs, my+ofs, py+ch-1, thick, r8, g8, b8)
		drawBoxVLine(img, mx+ofs, my+ofs, py+ch-1, thick, r8, g8, b8)
		return true

	// Arrows and Triangles
	case '↑':
		yTop := py + ch/6
		yBot := py + ch - ch/6
		drawBoxVLine(img, mx, yTop, yBot, thick, r8, g8, b8)
		arm := cw / 4
		if arm < 2 {
			arm = 2
		}
		for i := 0; i <= arm; i++ {
			drawBoxHLine(img, mx-i, mx+i, yTop+i, thick, r8, g8, b8)
		}
		return true
	case '↓':
		yTop := py + ch/6
		yBot := py + ch - ch/6
		drawBoxVLine(img, mx, yTop, yBot, thick, r8, g8, b8)
		arm := cw / 4
		if arm < 2 {
			arm = 2
		}
		for i := 0; i <= arm; i++ {
			drawBoxHLine(img, mx-(arm-i), mx+(arm-i), yBot-arm+i, thick, r8, g8, b8)
		}
		return true
	case '↕':
		yTop := py + ch/6
		yBot := py + ch - ch/6
		drawBoxVLine(img, mx, yTop, yBot, thick, r8, g8, b8)
		arm := cw / 4
		if arm < 2 {
			arm = 2
		}
		for i := 0; i <= arm; i++ {
			drawBoxHLine(img, mx-i, mx+i, yTop+i, thick, r8, g8, b8)
		}
		for i := 0; i <= arm; i++ {
			drawBoxHLine(img, mx-(arm-i), mx+(arm-i), yBot-arm+i, thick, r8, g8, b8)
		}
		return true
	case '▲':
		yTop := py + ch/5
		yBot := py + ch - ch/5
		hSpan := yBot - yTop
		maxW := cw / 3
		if maxW < 2 {
			maxW = 2
		}
		for y := yTop; y <= yBot; y++ {
			dx := (y - yTop) * maxW / max(hSpan, 1)
			drawBoxHLine(img, mx-dx, mx+dx, y, thick, r8, g8, b8)
		}
		return true
	case '▼':
		yTop := py + ch/5
		yBot := py + ch - ch/5
		hSpan := yBot - yTop
		maxW := cw / 3
		if maxW < 2 {
			maxW = 2
		}
		for y := yTop; y <= yBot; y++ {
			dx := (yBot - y) * maxW / max(hSpan, 1)
			drawBoxHLine(img, mx-dx, mx+dx, y, thick, r8, g8, b8)
		}
		return true
	case '█':
		baseOff := py*img.Stride + px*4
		maxBytes := cw * 4
		if baseOff+maxBytes <= len(img.Pix) {
			img.Pix[baseOff], img.Pix[baseOff+1], img.Pix[baseOff+2], img.Pix[baseOff+3] = r8, g8, b8, 255
			for n := 4; n < maxBytes; n *= 2 {
				copy(img.Pix[baseOff+n:baseOff+maxBytes], img.Pix[baseOff:baseOff+n])
			}
			for y := 1; y < ch; y++ {
				lineOff := (py+y)*img.Stride + px*4
				if lineOff+maxBytes <= len(img.Pix) {
					copy(img.Pix[lineOff:lineOff+maxBytes], img.Pix[baseOff:baseOff+maxBytes])
				}
			}
		}
		return true
	}
	return false
}

// drawBoxHLine/drawBoxVLine paint one clipped segment. Package functions so
// the packed colour stays a uint32 (color.Color conversion allocates);
// bounds are clamped once up front, not per pixel.
func drawBoxHLine(img *image.RGBA, x1, x2, y, thick int, r8, g8, b8 uint8) {
	if x1 < 0 {
		x1 = 0
	}
	if x2 >= img.Rect.Max.X {
		x2 = img.Rect.Max.X - 1
	}
	y0, y1 := y, y+thick-1
	if y0 < 0 {
		y0 = 0
	}
	if y1 >= img.Rect.Max.Y {
		y1 = img.Rect.Max.Y - 1
	}
	if x1 > x2 || y0 > y1 || y1*img.Stride+x2*4+3 >= len(img.Pix) {
		return
	}
	for yy := y0; yy <= y1; yy++ {
		off := yy*img.Stride + x1*4
		for x := x1; x <= x2; x++ {
			img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
			off += 4
		}
	}
}

func drawBoxVLine(img *image.RGBA, x, y1, y2, thick int, r8, g8, b8 uint8) {
	if y1 < 0 {
		y1 = 0
	}
	if y2 >= img.Rect.Max.Y {
		y2 = img.Rect.Max.Y - 1
	}
	x0, x1 := x, x+thick-1
	if x0 < 0 {
		x0 = 0
	}
	if x1 >= img.Rect.Max.X {
		x1 = img.Rect.Max.X - 1
	}
	if x0 > x1 || y1 > y2 || y2*img.Stride+x1*4+3 >= len(img.Pix) {
		return
	}
	for yy := y1; yy <= y2; yy++ {
		off := yy*img.Stride + x0*4
		for xx := x0; xx <= x1; xx++ {
			img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
			off += 4
		}
	}
}
