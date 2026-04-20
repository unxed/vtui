//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package vtui

import (
	"image"
	"image/color"
	"time"

	"github.com/mattn/go-runewidth"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// X11Renderer implements SurfaceRenderer by drawing directly to an image.RGBA buffer
// and pushing it to the X11Host.
type glyphKey struct {
	r  rune
	fg uint32
	bg uint32
	w  int
}

type X11Renderer struct {
	host       *X11Host
	face       font.Face
	w, h       int
	glyphCache map[glyphKey]*image.RGBA
	cursorX    int
	cursorY    int
	cursorVis  bool

	// Состояние для управления миганием и очистки "шлейфа"
	oldCursorX int
	oldCursorY int
}

func NewX11Renderer(host *X11Host, face font.Face) *X11Renderer {
	return &X11Renderer{
		host:       host,
		face:       face,
		glyphCache: make(map[glyphKey]*image.RGBA),
	}
}

func (r *X11Renderer) SetPalette(pal *[256]uint32) {
	// X11 renderer uses TrueColor naturally, no palette switching needed for the host window.
}

func (r *X11Renderer) SetCursor(x, y int, visible bool) {
	r.cursorX = x
	r.cursorY = y
	r.cursorVis = visible
}

func (r *X11Renderer) getCellColors(cell CharInfo) (uint32, uint32) {
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

func (r *X11Renderer) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()

	blinkState := (time.Now().UnixNano()/int64(500*time.Millisecond))%2 == 0
	r.w, r.h = w, h
	img := r.host.imgBuf
	cw, ch := r.host.cellW, r.host.cellH

	for y := 0; y < h; y++ {
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

		for iy := 0; iy < ch; iy++ {
			lineIdx := y*ch + iy
			if lineIdx >= 0 && lineIdx < len(r.host.dirtyLines) {
				r.host.dirtyLines[lineIdx] = true
			}
		}

		for x := 0; x < w; {
			idx := rowOff + x
			cell := buf[idx]
			_, bg := r.getCellColors(cell)

			// 1. Находим "спан" (группу ячеек) с одинаковым фоном
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

			// 2. Быстрая заливка фона для всего спана
			px := x * cw
			py := y * ch
			spanPixW := spanW * cw
			br, bgG, bb := uint8(bg>>16), uint8(bg>>8), uint8(bg)

			baseOff := py*img.Stride + px*4
			maxBytes := spanPixW * 4
			if baseOff+maxBytes <= len(img.Pix) {
				img.Pix[baseOff], img.Pix[baseOff+1], img.Pix[baseOff+2], img.Pix[baseOff+3] = bb, bgG, br, 255
				for n := 4; n < maxBytes; n *= 2 {
					copy(img.Pix[baseOff+n:baseOff+maxBytes], img.Pix[baseOff:baseOff+n])
				}
				for iy := 1; iy < ch; iy++ {
					if py+iy >= int(r.host.height) { break }
					lineOff := (py+iy)*img.Stride + px*4
					copy(img.Pix[lineOff:lineOff+maxBytes], img.Pix[baseOff:baseOff+maxBytes])
				}
			}

			// 3. Отрисовка контента внутри спана
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
					if !r.drawCustomChar(img, char, cpx, py, cw, ch, fgColor) {
						r.drawCachedGlyph(img, char, cpx, py, rw, cfg, cbg, fgColor, bgColor)
					}
				}

				// 4. Курсор
				if currX == r.cursorX && y == r.cursorY && r.cursorVis && blinkState {
					thickness := 2
					if r.host.scale > 1 { thickness = 4 }
					for iy := ch - thickness; iy < ch; iy++ {
						rowStart := (py+iy)*img.Stride + cpx*4
						for ix := 0; ix < cw; ix++ {
							off := rowStart + ix*4
							img.Pix[off] = 255 - img.Pix[off]
							img.Pix[off+1] = 255 - img.Pix[off+1]
							img.Pix[off+2] = 255 - img.Pix[off+2]
						}
					}
				}
				sx += rw
			}
			x += spanW
		}
	}
	r.oldCursorX = r.cursorX
	r.oldCursorY = r.cursorY
}

func (r *X11Renderer) drawCachedGlyph(img *image.RGBA, char rune, px, py, rw int, fg, bg uint32, fgCol, bgCol color.RGBA) {
	key := glyphKey{char, fg, bg, rw}
	cached, ok := r.glyphCache[key]

	cw, ch := r.host.cellW, r.host.cellH
	drawW := cw * rw
	if !ok {
		cached = image.NewRGBA(image.Rect(0, 0, drawW, ch))

		for iy := 0; iy < ch; iy++ {
			for ix := 0; ix < drawW; ix++ {
				// Оптимизированное заполнение фона при создании нового глифа в кэше
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

		// Разовая оптимизация: меняем R и B местами во всем кэше глифа,
		// чтобы потом копировать его в основной буфер одним махом без обработки.
		for p := 0; p < len(cached.Pix); p += 4 {
			cached.Pix[p], cached.Pix[p+2] = cached.Pix[p+2], cached.Pix[p]
		}

		r.glyphCache[key] = cached
	}
	for iy := 0; iy < ch; iy++ {
		dstOff := (py+iy)*img.Stride + px*4
		srcOff := iy * cached.Stride
		// Прямое копирование строки пикселей из кэша глифа в основной буфер кадра
		copy(img.Pix[dstOff:dstOff+drawW*4], cached.Pix[srcOff:srcOff+drawW*4])
	}
}

func (r *X11Renderer) Flush() {
	r.host.flushImage()
}

// drawCustomChar performs pixel-perfect drawing of lines and blocks.
// Returns true if the character was handled.
func (r *X11Renderer) drawCustomChar(img *image.RGBA, char rune, px, py, cw, ch int, col color.Color) bool {
	mx := px + cw/2
	my := py + ch/2
	thick := r.host.scale

	cr, cg, cb, _ := col.RGBA()
	r8, g8, b8 := uint8(cr>>8), uint8(cg>>8), uint8(cb>>8)

	drawHLine := func(x1, x2, y int) {
		for x := x1; x <= x2; x++ {
			for t := 0; t < thick; t++ {
				off := (y+t)*img.Stride + x*4
				img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
			}
		}
	}
	drawVLine := func(x, y1, y2 int) {
		for y := y1; y <= y2; y++ {
			for t := 0; t < thick; t++ {
				off := y*img.Stride + (x+t)*4
				img.Pix[off], img.Pix[off+1], img.Pix[off+2], img.Pix[off+3] = r8, g8, b8, 255
			}
		}
	}

	// Double line specifics
	ofs := cw / 4
	if ofs < 1 { ofs = 1 }

	switch char {
	// Single Lines
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

	// Double Lines
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
			// Пишем сразу в BGRA
			img.Pix[baseOff], img.Pix[baseOff+1], img.Pix[baseOff+2], img.Pix[baseOff+3] = b8, g8, r8, 255
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