package vtui

import (
	"image"
	"image/color"
)

// drawUnderline paints the CommonLvbUnderscore decoration used by terminal
// and URL-hover rendering. Pixel backends do not pass ANSI attributes to a
// terminal, so they must render this decoration themselves.
func drawUnderline(img *image.RGBA, px, py, width, height, scale int, rgb uint32) {
	if img == nil || width <= 0 || height <= 0 {
		return
	}
	thickness := 1
	if scale > 1 {
		thickness = 2
	}
	if thickness > height {
		thickness = height
	}
	y := py + height - thickness
	if px < 0 {
		width += px
		px = 0
	}
	if y < 0 {
		thickness += y
		y = 0
	}
	if px+width > img.Rect.Dx() {
		width = img.Rect.Dx() - px
	}
	if y+thickness > img.Rect.Dy() {
		thickness = img.Rect.Dy() - y
	}
	if width <= 0 || thickness <= 0 {
		return
	}

	col := color.RGBA{R: uint8(rgb >> 16), G: uint8(rgb >> 8), B: uint8(rgb), A: 255}
	for row := y; row < y+thickness; row++ {
		off := row*img.Stride + px*4
		for x := 0; x < width; x++ {
			p := off + x*4
			img.Pix[p], img.Pix[p+1], img.Pix[p+2], img.Pix[p+3] = col.R, col.G, col.B, col.A
		}
	}
}
