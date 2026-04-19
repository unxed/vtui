//go:build linux || freebsd || openbsd || netbsd || dragonfly

package vtui

import (
	"image"
	"image/color"

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

func (r *X11Renderer) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()

	r.w, r.h = w, h
	img := r.host.imgBuf
	cw, ch := r.host.cellW, r.host.cellH


	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := y*w + x
			isCursorCell := (x == r.cursorX && y == r.cursorY && r.cursorVis)

			if !forceRedraw && buf[idx] == shadow[idx] && !isCursorCell {
				continue
			}

			cell := buf[idx]
			px := x * cw
			py := y * ch

			if cell.Char == WideCharFiller {
				continue // Already handled by the previous wide character cell
			}

			// 1. Extract Colors
			bgRGB := GetRGBBack(cell.Attributes)
			if cell.Attributes&IsBgRGB == 0 {
				bgRGB = ThemePalette[GetIndexBack(cell.Attributes)]
			}
			fgRGB := GetRGBFore(cell.Attributes)
			if cell.Attributes&IsFgRGB == 0 {
				fgRGB = ThemePalette[GetIndexFore(cell.Attributes)]
			}

			// 2. Calculate widths
			char := rune(cell.Char)
			rw := runewidth.RuneWidth(char)
			if rw < 1 { rw = 1 }
			drawW := cw * rw

			// 3. Draw Background
			bgColor := color.RGBA{R: uint8(bgRGB >> 16), G: uint8(bgRGB >> 8), B: uint8(bgRGB), A: 255}
			for iy := 0; iy < ch; iy++ {
				for ix := 0; ix < drawW; ix++ {
					img.Set(px+ix, py+iy, bgColor)
				}
			}

			// 4. Draw Character
			if char != 0 && char != ' ' {
				fgColor := color.RGBA{R: uint8(fgRGB >> 16), G: uint8(fgRGB >> 8), B: uint8(fgRGB), A: 255}
				if !r.drawCustomChar(img, char, px, py, cw, ch, fgColor) {
					r.drawCachedGlyph(img, char, px, py, rw, fgRGB, bgRGB, fgColor, bgColor)
				}
			}

			// 5. Draw Cursor (Inverted Block)
			if isCursorCell {
				for iy := 0; iy < ch; iy++ {
					for ix := 0; ix < cw; ix++ {
						old := img.RGBAAt(px+ix, py+iy)
						inv := color.RGBA{255 - old.R, 255 - old.G, 255 - old.B, 255}
						img.SetRGBA(px+ix, py+iy, inv)
					}
				}
			}
		}
	}
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
				cached.Set(ix, iy, bgCol)
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
		for ix := 0; ix < drawW; ix++ {
			img.Set(px+ix, py+iy, cached.At(ix, iy))
		}
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

	drawHLine := func(x1, x2, y int) {
		for x := x1; x <= x2; x++ {
			for t := 0; t < thick; t++ {
				img.Set(x, y+t, col)
			}
		}
	}
	drawVLine := func(x, y1, y2 int) {
		for y := y1; y <= y2; y++ {
			for t := 0; t < thick; t++ {
				img.Set(x+t, y, col)
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
		for y := py; y < py+ch; y++ {
			for x := px; x < px+cw; x++ {
				img.Set(x, y, col)
			}
		}
		return true
	}
	return false
}