//go:build linux || openbsd || netbsd || dragonfly

package vtui

import (
	"image"
	"image/color"
	"time"

	"github.com/mattn/go-runewidth"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// WaylandRenderer draws VTUI frames to an image.RGBA, then requests Wayland to flush.
// Its drawing logic heavily mimics X11Renderer for visual consistency.
type WaylandRenderer struct {
	host       *WaylandHost
	face       font.Face
	w, h       int
	glyphCache map[glyphKey]*image.RGBA

	cursorX, cursorY   int
	oldCursorX, oldCursorY int
	cursorVis          bool
	cursorShape        CursorShape

	stats renderStats
}

func NewWaylandRenderer(host *WaylandHost, face font.Face) *WaylandRenderer {
	return &WaylandRenderer{
		host:       host,
		face:       face,
		glyphCache: make(map[glyphKey]*image.RGBA),
	}
}

func (r *WaylandRenderer) SetPalette(pal *[256]uint32) {
	// Native RGB environment, palette mapping occurs logically upstream
}

func (r *WaylandRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	if r.cursorX != x || r.cursorY != y || r.cursorVis != visible || r.cursorShape != shape {
		r.oldCursorX = r.cursorX
		r.oldCursorY = r.cursorY
		r.cursorX = x
		r.cursorY = y
		r.cursorVis = visible
		r.cursorShape = shape
	}
}

func (r *WaylandRenderer) getCellColors(cell CharInfo) (uint32, uint32) {
	bg := GetRGBBack(cell.Attributes)
	if cell.Attributes&IsBgRGB == 0 {
		bg = ThemePalette[GetIndexBack(cell.Attributes)]
	}
	fg := GetRGBFore(cell.Attributes)
	if cell.Attributes&IsFgRGB == 0 {
		fg = ThemePalette[GetIndexFore(cell.Attributes)]
	}
	return fg, bg
}

func (r *WaylandRenderer) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {
	start := time.Now()
	r.host.mu.Lock()
	defer r.host.mu.Unlock()

	blinkState := (time.Now().UnixNano()/int64(500*time.Millisecond))%2 == 0
	r.w, r.h = w, h
	img := r.host.imgBuf
	cw, ch := r.host.cellW, r.host.cellH

	for y := 0; y < h; y++ {
		r.stats.totalRows++
		rowOff := y * w
		rowDirty := forceRedraw || y == r.cursorY || y == r.oldCursorY
		if !rowDirty {
			for x := 0; x < w; x++ {
				if buf[rowOff+x] != shadow[rowOff+x] {
					rowDirty = true
					break
				}
			}
		}
		if !rowDirty {
			continue
		}
		r.stats.dirtyRows++

		if y == r.oldCursorY && (r.oldCursorX != r.cursorX || r.oldCursorY != r.cursorY) {
			r.oldCursorX = -1
			r.oldCursorY = -1
		}

		for x := 0; x < w; {
			idx := rowOff + x
			cell := buf[idx]
			_, bg := r.getCellColors(cell)

			spanW := 0
			for x+spanW < w {
				nextCell := buf[rowOff+x+spanW]
				if nextCell.Char == WideCharFiller {
					spanW++
					continue
				}
				_, nextBg := r.getCellColors(nextCell)
				if nextBg != bg {
					break
				}
				spanW++
			}

			px := x * cw
			py := y * ch
			spanPixW := spanW * cw
			br, bgG, bb := uint8(bg>>16), uint8(bg>>8), uint8(bg)

			baseOff := py*img.Stride + px*4
			maxBytes := spanPixW * 4
			if baseOff+maxBytes <= len(img.Pix) {
				img.Pix[baseOff], img.Pix[baseOff+1], img.Pix[baseOff+2], img.Pix[baseOff+3] = br, bgG, bb, 255
				for n := 4; n < maxBytes; n *= 2 {
					copy(img.Pix[baseOff+n:baseOff+maxBytes], img.Pix[baseOff:baseOff+n])
				}
				for iy := 1; iy < ch; iy++ {
					if py+iy >= img.Rect.Max.Y {
						break
					}
					lineOff := (py+iy)*img.Stride + px*4
					if lineOff+maxBytes <= len(img.Pix) {
						copy(img.Pix[lineOff:lineOff+maxBytes], img.Pix[baseOff:baseOff+maxBytes])
					}
				}
			}

			for sx := 0; sx < spanW; {
				currX := x + sx
				cIdx := rowOff + currX
				currCell := buf[cIdx]
				if currCell.Char == WideCharFiller {
					sx++
					continue
				}

				char := rune(currCell.Char)
				rw := runewidth.RuneWidth(char)
				if rw < 1 { rw = 1 }

				cpx := currX * cw
				cfg, cbg := r.getCellColors(currCell)
				fgColor := color.RGBA{R: uint8(cfg >> 16), G: uint8(cfg >> 8), B: uint8(cfg), A: 255}
				bgColor := color.RGBA{R: uint8(cbg >> 16), G: uint8(cbg >> 8), B: uint8(cbg), A: 255}

				if char != 0 && char != ' ' {
					r.stats.glyphs++
					if !r.drawCustomChar(img, char, cpx, py, cw, ch, fgColor) {
						r.drawCachedGlyph(img, char, cpx, py, rw, cfg, cbg, fgColor, bgColor)
					}
				}

				if currX == r.cursorX && y == r.cursorY && r.cursorVis && blinkState {
					var startY int
					if r.cursorShape == CursorShapeBlock {
						startY = 0
					} else {
						thickness := 2
						if r.host.scale > 1 { thickness = 4 }
						startY = ch - thickness
					}
					for iy := startY; iy < ch; iy++ {
						pixelY := py + iy
						if pixelY < 0 || pixelY >= img.Rect.Max.Y { continue }
						rowStart := pixelY * img.Stride
						for ix := 0; ix < cw; ix++ {
							pixelX := cpx + ix
							if pixelX < 0 || pixelX >= img.Rect.Max.X { continue }
							off := rowStart + pixelX*4
							if off+2 < len(img.Pix) {
								img.Pix[off] = 255 - img.Pix[off]
								img.Pix[off+1] = 255 - img.Pix[off+1]
								img.Pix[off+2] = 255 - img.Pix[off+2]
							}
						}
					}
				}
				sx += rw
			}
			x += spanW
		}
	}

	if r.cursorVis {
		r.oldCursorX = r.cursorX
		r.oldCursorY = r.cursorY
	}
	r.stats.totalDraw += time.Since(start)
}

func (r *WaylandRenderer) drawCachedGlyph(img *image.RGBA, char rune, px, py, rw int, fg, bg uint32, fgCol, bgCol color.RGBA) {
	key := glyphKey{char, fg, bg, rw}
	cached, ok := r.glyphCache[key]

	cw, ch := r.host.cellW, r.host.cellH
	drawW := cw * rw
	if !ok {
		cached = image.NewRGBA(image.Rect(0, 0, drawW, ch))
		for iy := 0; iy < ch; iy++ {
			for ix := 0; ix < drawW; ix++ {
				off := iy*cached.Stride + ix*4
				cached.Pix[off] = bgCol.R
				cached.Pix[off+1] = bgCol.G
				cached.Pix[off+2] = bgCol.B
				cached.Pix[off+3] = 255
			}
		}

		metrics := r.face.Metrics()
		d := &font.Drawer{
			Dst:  cached,
			Src:  image.NewUniform(fgCol),
			Face: r.face,
			Dot:  fixed.Point26_6{X: fixed.I(0), Y: metrics.Ascent},
		}
		d.DrawString(string(char))
		r.glyphCache[key] = cached
	}

	for iy := 0; iy < ch; iy++ {
		if py+iy >= img.Rect.Max.Y { break }
		dstOff := (py+iy)*img.Stride + px*4
		srcOff := iy * cached.Stride
		if dstOff+drawW*4 <= len(img.Pix) {
			copy(img.Pix[dstOff:dstOff+drawW*4], cached.Pix[srcOff:srcOff+drawW*4])
		}
	}
}

func (r *WaylandRenderer) drawCustomChar(img *image.RGBA, char rune, px, py, cw, ch int, col color.Color) bool {
	mx, my := px+cw/2, py+ch/2
	thick := r.host.scale
	cr, cg, cb, _ := col.RGBA()
	r8, g8, b8 := uint8(cr>>8), uint8(cg>>8), uint8(cb>>8)

	drawHLine := func(x1, x2, y int) {
		for x := x1; x <= x2; x++ {
			if x < 0 || x >= img.Rect.Max.X { continue }
			for t := 0; t < thick; t++ {
				if y+t < 0 || y+t >= img.Rect.Max.Y { continue }
				off := (y+t)*img.Stride + x*4
				if off+3 < len(img.Pix) {
					img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
				}
			}
		}
	}
	drawVLine := func(x, y1, y2 int) {
		for y := y1; y <= y2; y++ {
			if y < 0 || y >= img.Rect.Max.Y { continue }
			for t := 0; t < thick; t++ {
				if x+t < 0 || x+t >= img.Rect.Max.X { continue }
				off := y*img.Stride + (x+t)*4
				if off+3 < len(img.Pix) {
					img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
				}
			}
		}
	}

	ofs := cw / 4
	if ofs < 1 { ofs = 1 }

	switch char {
	case '─': drawHLine(px, px+cw-1, my); return true
	case '│': drawVLine(mx, py, py+ch-1); return true
	case '┌': drawHLine(mx, px+cw-1, my); drawVLine(mx, my, py+ch-1); return true
	case '┐': drawHLine(px, mx, my); drawVLine(mx, my, py+ch-1); return true
	case '└': drawHLine(mx, px+cw-1, my); drawVLine(mx, py, my); return true
	case '┘': drawHLine(px, mx, my); drawVLine(mx, py, my); return true
	case '├': drawHLine(mx, px+cw-1, my); drawVLine(mx, py, py+ch-1); return true
	case '┤': drawHLine(px, mx, my); drawVLine(mx, py, py+ch-1); return true
	case '┬': drawHLine(px, px+cw-1, my); drawVLine(mx, my, py+ch-1); return true
	case '┴': drawHLine(px, px+cw-1, my); drawVLine(mx, py, my); return true
	case '┼': drawHLine(px, px+cw-1, my); drawVLine(mx, py, py+ch-1); return true
	case '═': drawHLine(px, px+cw-1, my-ofs); drawHLine(px, px+cw-1, my+ofs); return true
	case '║': drawVLine(mx-ofs, py, py+ch-1); drawVLine(mx+ofs, py, py+ch-1); return true
	case '╔':
		drawHLine(mx-ofs, px+cw-1, my-ofs); drawHLine(mx+ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, my-ofs, py+ch-1); drawVLine(mx+ofs, my+ofs, py+ch-1)
		return true
	case '╗':
		drawHLine(px, mx+ofs, my-ofs); drawHLine(px, mx-ofs, my+ofs)
		drawVLine(mx+ofs, my-ofs, py+ch-1); drawVLine(mx-ofs, my+ofs, py+ch-1)
		return true
	case '╚':
		drawHLine(mx-ofs, px+cw-1, my-ofs); drawHLine(mx+ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, py, my-ofs); drawVLine(mx+ofs, py, my+ofs)
		return true
	case '╝':
		drawHLine(px, mx+ofs, my-ofs); drawHLine(px, mx-ofs, my+ofs)
		drawVLine(mx+ofs, py, my-ofs); drawVLine(mx-ofs, py, my+ofs)
		return true
	case '╠':
		drawHLine(mx-ofs, px+cw-1, my-ofs); drawHLine(mx+ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, py, py+ch-1); drawVLine(mx+ofs, py, py+ch-1)
		return true
	case '╣':
		drawHLine(px, mx+ofs, my-ofs); drawHLine(px, mx-ofs, my+ofs)
		drawVLine(mx-ofs, py, py+ch-1); drawVLine(mx+ofs, py, py+ch-1)
		return true
	case '╩':
		drawHLine(px, px+cw-1, my+ofs)
		drawHLine(px, mx-ofs, my-ofs); drawHLine(mx+ofs, px+cw-1, my-ofs)
		drawVLine(mx-ofs, py, my-ofs); drawVLine(mx+ofs, py, my-ofs)
		return true
	case '╦':
		drawHLine(px, px+cw-1, my-ofs)
		drawHLine(px, mx-ofs, my+ofs); drawHLine(mx+ofs, px+cw-1, my+ofs)
		drawVLine(mx-ofs, my+ofs, py+ch-1); drawVLine(mx+ofs, my+ofs, py+ch-1)
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

func (r *WaylandRenderer) Flush() {
	start := time.Now()
	// Trigger the Wayland host to push the buffer to the compositor
	r.host.widget.ScheduleRedraw()
	r.stats.totalFlush += time.Since(start)
	r.stats.frameCount++
}
