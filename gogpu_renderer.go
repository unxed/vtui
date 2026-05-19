//go:build !freebsd

package vtui

import (
	"image/color"
	"math"
	"strings"
	"time"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
	_ "github.com/gogpu/gg/gpu" // Включаем аппаратное ускорение рендеринга
)

type GogpuRenderer struct {
	mu           sync.Mutex
	host         *GogpuHost
	face         text.Face
	cellW, cellH int // logical cell sizes from font measurement
	cols, rows   int // dimensions of the current renderBuf

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape

	canvas    *ggcanvas.Canvas
	renderBuf []CharInfo
	dirty     bool
}

func NewGogpuRenderer(host *GogpuHost, face text.Face, cw, ch int) *GogpuRenderer {
	return &GogpuRenderer{
		host:  host,
		face:  face,
		cellW: cw,
		cellH: ch,
	}
}

func (r *GogpuRenderer) Render(buf, shadow []CharInfo, w, h int, force bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.cols = w
	r.rows = h

	needsRedraw := force
	if !needsRedraw {
		for i := 0; i < len(buf); i++ {
			if buf[i] != shadow[i] {
				needsRedraw = true
				break
			}
		}
	}
	if !needsRedraw {
		return
	}

	if len(r.renderBuf) != len(buf) {
		r.renderBuf = make([]CharInfo, len(buf))
	}
	copy(r.renderBuf, buf)
	r.dirty = true
}

func (r *GogpuRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.cursorX, r.cursorY = x, y
	r.cursorVis = visible
	r.cursorShape = shape
}

func (r *GogpuRenderer) SetPalette(pal *[256]uint32) {}

func (r *GogpuRenderer) drawCustomChar(dc *gg.Context, char rune, x, y, w, h float64) bool {
	thick := 1.0

	mx := math.Floor(x + w/2 - thick/2)
	my := math.Floor(y + h/2 - thick/2)

	// Double line offsets
	ofs := math.Floor(math.Min(w, h) / 4)
	if ofs < 1 {
		ofs = 1
	}

	switch char {
	case '─', '━':
		t := thick
		if char == '━' {
			t *= 2
		}
		dc.DrawRectangle(x, math.Floor(y+h/2-t/2), w, t)
	case '│', '┃':
		t := thick
		if char == '┃' {
			t *= 2
		}
		dc.DrawRectangle(math.Floor(x+w/2-t/2), y, t, h)
	case '┌', '┏':
		t := thick
		if char == '┏' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(mxx, myy, w-(mxx-x), t)
		dc.DrawRectangle(mxx, myy, t, h-(myy-y))
	case '┐', '┓':
		t := thick
		if char == '┓' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, mxx-x+t, t)
		dc.DrawRectangle(mxx, myy, t, h-(myy-y))
	case '└', '┗':
		t := thick
		if char == '┗' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(mxx, myy, w-(mxx-x), t)
		dc.DrawRectangle(mxx, y, t, myy-y+t)
	case '┘', '┛':
		t := thick
		if char == '┛' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, mxx-x+t, t)
		dc.DrawRectangle(mxx, y, t, myy-y+t)
	case '├', '┣':
		t := thick
		if char == '┣' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(mxx, myy, w-(mxx-x), t)
		dc.DrawRectangle(mxx, y, t, h)
	case '┤', '┫':
		t := thick
		if char == '┫' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, mxx-x+t, t)
		dc.DrawRectangle(mxx, y, t, h)
	case '┬', '┳':
		t := thick
		if char == '┳' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, w, t)
		dc.DrawRectangle(mxx, myy, t, h-(myy-y))
	case '┴', '┻':
		t := thick
		if char == '┻' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, w, t)
		dc.DrawRectangle(mxx, y, t, myy-y+t)
	case '┼', '╋':
		t := thick
		if char == '╋' {
			t *= 2
		}
		mxx, myy := math.Floor(x+w/2-t/2), math.Floor(y+h/2-t/2)
		dc.DrawRectangle(x, myy, w, t)
		dc.DrawRectangle(mxx, y, t, h)

	// Double lines
	case '═':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
	case '║':
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)
	case '╔':
		dc.DrawRectangle(mx-ofs, my-ofs, w-(mx-x-ofs), thick)
		dc.DrawRectangle(mx+ofs, my+ofs, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, my-ofs, thick, h-(my-y-ofs))
		dc.DrawRectangle(mx+ofs, my+ofs, thick, h-(my-y+ofs))
	case '╗':
		dc.DrawRectangle(x, my-ofs, mx-x+ofs+thick, thick)
		dc.DrawRectangle(x, my+ofs, mx-x-ofs+thick, thick)
		dc.DrawRectangle(mx+ofs, my-ofs, thick, h-(my-y-ofs))
		dc.DrawRectangle(mx-ofs, my+ofs, thick, h-(my-y+ofs))
	case '╚':
		dc.DrawRectangle(mx-ofs, my+ofs, w-(mx-x-ofs), thick)
		dc.DrawRectangle(mx+ofs, my-ofs, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, y, thick, my-y+ofs+thick)
		dc.DrawRectangle(mx+ofs, y, thick, my-y-ofs+thick)
	case '╝':
		dc.DrawRectangle(x, my+ofs, mx-x+ofs+thick, thick)
		dc.DrawRectangle(x, my-ofs, mx-x-ofs+thick, thick)
		dc.DrawRectangle(mx+ofs, y, thick, my-y+ofs+thick)
		dc.DrawRectangle(mx-ofs, y, thick, my-y-ofs+thick)
	case '╠':
		dc.DrawRectangle(mx-ofs, my-ofs, w-(mx-x-ofs), thick)
		dc.DrawRectangle(mx+ofs, my+ofs, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)
	case '╣':
		dc.DrawRectangle(x, my-ofs, mx-x+ofs+thick, thick)
		dc.DrawRectangle(x, my+ofs, mx-x-ofs+thick, thick)
		dc.DrawRectangle(mx+ofs, y, thick, h)
		dc.DrawRectangle(mx-ofs, y, thick, h)
	case '╦':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
		dc.DrawRectangle(mx-ofs, my+ofs, thick, h-(my-y+ofs))
		dc.DrawRectangle(mx+ofs, my+ofs, thick, h-(my-y+ofs))
	case '╩':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
		dc.DrawRectangle(mx-ofs, y, thick, my-y-ofs+thick)
		dc.DrawRectangle(mx+ofs, y, thick, my-y-ofs+thick)
	case '╬':
		dc.DrawRectangle(x, my-ofs, w, thick)
		dc.DrawRectangle(x, my+ofs, w, thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)

	// Mixed (used in VMenu)
	case '╟':
		dc.DrawRectangle(mx+ofs, my, w-(mx-x+ofs), thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)
	case '╢':
		dc.DrawRectangle(x, my, mx-x-ofs+thick, thick)
		dc.DrawRectangle(mx-ofs, y, thick, h)
		dc.DrawRectangle(mx+ofs, y, thick, h)

	// Arrows
	case '↑':
		dc.DrawLine(mx, y+h*0.1, mx, y+h*0.9)
		dc.DrawLine(mx, y+h*0.1, mx-w*0.3, y+h*0.4)
		dc.DrawLine(mx, y+h*0.1, mx+w*0.3, y+h*0.4)
		dc.SetLineWidth(thick)
		dc.Stroke()
		dc.SetLineWidth(0)
		return true
	case '↓':
		dc.DrawLine(mx, y+h*0.1, mx, y+h*0.9)
		dc.DrawLine(mx, y+h*0.9, mx-w*0.3, y+h*0.6)
		dc.DrawLine(mx, y+h*0.9, mx+w*0.3, y+h*0.6)
		dc.SetLineWidth(thick)
		dc.Stroke()
		dc.SetLineWidth(0)
		return true

	// Solid Blocks
	case '█':
		dc.DrawRectangle(x, y, w, h)
	case '▀':
		dc.DrawRectangle(x, y, w, h/2)
	case '▄':
		dc.DrawRectangle(x, y+h/2, w, h/2)
	case '▌':
		dc.DrawRectangle(x, y, w/2, h)
	case '▐':
		dc.DrawRectangle(x+w/2, y, w/2, h)

	default:
		return false
	}

	dc.Fill()
	return true
}
func (r *GogpuRenderer) Flush() {
	r.host.mu.Lock()
	ctx := r.host.ctx
	app := r.host.app
	forceDirty := r.host.resizePending
	r.host.resizePending = false
	r.host.mu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// CRITICAL: If Flush is called from the background FrameManager thread
	// (ctx == nil), we MUST NOT proceed with heavy drawing or library calls.
	if ctx == nil {
		if (r.dirty || forceDirty) && app != nil {
			app.RequestRedraw()
		}
		return
	}

	if len(r.renderBuf) == 0 {
		return
	}

	if forceDirty {
		r.dirty = true
	}

	w, h := ctx.Width(), ctx.Height()
	if w <= 0 || h <= 0 {
		return
	}

	if debugLastCtxW != w || debugLastCtxH != h {
		debugLastCtxW, debugLastCtxH = w, h
		if r.canvas != nil {
			r.canvas.Resize(w, h)
		}
		r.dirty = true
	}

	if r.canvas == nil {
		provider := app.GPUContextProvider()
		if provider == nil { return }
		r.canvas, _ = ggcanvas.New(provider, w, h)
	}

	if r.dirty {
		r.canvas.Draw(func(dc *gg.Context) {
			dc.SetRGB(0, 0, 0)
			dc.Clear()

			drawCols := r.cols
			drawRows := r.rows

			if r.face != nil {
				dc.SetFont(r.face)
			}
			metrics := r.face.Metrics()
			ascent := float64(metrics.Ascent)

			for y := 0; y < drawRows; y++ {
				rowOff := y * drawCols
				for x := 0; x < drawCols; {
					cell := r.renderBuf[rowOff+x]
					fg, bg := r.getCellColors(cell)

					spanW := 0
					for x+spanW < drawCols {
						nextCell := r.renderBuf[rowOff+x+spanW]
						if nextCell.Char == WideCharFiller {
							spanW++
							continue
						}
						nextFg, nextBg := r.getCellColors(nextCell)
						if nextBg != bg || nextFg != fg {
							break
						}
						spanW++
					}

					lx := float64(x * r.cellW)
					ly := float64(y * r.cellH)
					spanPixW := float64(spanW * r.cellW)

					dc.SetColor(bg)
					dc.DrawRectangle(lx, ly, spanPixW, float64(r.cellH))
					dc.Fill()
					var sb strings.Builder
					batchX := lx
					dc.SetColor(fg)

					for sx := 0; sx < spanW; {
						idx := rowOff + x + sx
						currCell := r.renderBuf[idx]

						if currCell.Char == WideCharFiller {
							sx++
							continue
						}

						rw := 1
						if x+sx+1 < drawCols && r.renderBuf[idx+1].Char == WideCharFiller {
							rw = 2
						}

						char := rune(currCell.Char)
						isBox := (char >= 0x2500 && char <= 0x259F) || (char >= 0x2190 && char <= 0x2193)

						if isBox {
							if sb.Len() > 0 {
								dc.DrawString(sb.String(), batchX, ly+ascent)
								sb.Reset()
							}
							if r.drawCustomChar(dc, char, lx+float64(sx*r.cellW), ly, float64(rw*r.cellW), float64(r.cellH)) {
								sx += rw
								continue
							}
						}

						if char != 0 && char != ' ' && r.face != nil {
							if sb.Len() == 0 {
								batchX = lx + float64(sx*r.cellW)
							}
							sb.WriteRune(char)
						} else {
							if sb.Len() > 0 {
								dc.DrawString(sb.String(), batchX, ly+ascent)
								sb.Reset()
							}
						}
						sx += rw
					}

					if sb.Len() > 0 && r.face != nil {
						dc.DrawString(sb.String(), batchX, ly+ascent)
					}

					x += spanW
				}
			}

			if r.cursorVis && (time.Now().UnixMilli()/500)%2 == 0 {
				dc.SetColor(color.White)
				cx := float64(r.cursorX * r.cellW)
				cy := float64(r.cursorY * r.cellH)
				if r.cursorShape == CursorShapeUnderline {
					cy += float64(r.cellH) - 2
					dc.DrawRectangle(cx, cy, float64(r.cellW), 2)
				} else {
					dc.DrawRectangle(cx, cy, float64(r.cellW), float64(r.cellH))
				}
				dc.Fill()
			}
		})
		r.dirty = false
	}

	r.canvas.Render(ctx.RenderTarget())
}

func (r *GogpuRenderer) getCellColors(cell CharInfo) (color.Color, color.Color) {
	bg := GetRGBBack(cell.Attributes)
	if cell.Attributes&IsBgRGB == 0 {
		bg = ThemePalette[GetIndexBack(cell.Attributes)]
	}
	fg := GetRGBFore(cell.Attributes)
	if cell.Attributes&IsFgRGB == 0 {
		fg = ThemePalette[GetIndexFore(cell.Attributes)]
	}

	f := color.RGBA{uint8(fg >> 16), uint8(fg >> 8), uint8(fg), 255}
	b := color.RGBA{uint8(bg >> 16), uint8(bg >> 8), uint8(bg), 255}
	return f, b
}