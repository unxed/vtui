//go:build !freebsd

package vtui

import (
	"image/color"
	"time"
	"sync"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
)

var (
	debugLastPhysW, debugLastPhysH int = -1, -1
	debugDrawCount                 int = 0
)
type GogpuRenderer struct {
	mu           sync.Mutex
	host         *GogpuHost
	face         text.Face
	cellW, cellH int // logical cell sizes from font measurement

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

func (r *GogpuRenderer) Flush() {
	r.host.mu.Lock()
	ctx := r.host.ctx
	app := r.host.app
	r.host.mu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if ctx == nil {
		if r.dirty && app != nil {
			app.RequestRedraw()
		}
		return
	}

	if len(r.renderBuf) == 0 {
		return
	}

	w, h := ctx.Width(), ctx.Height()
	fw, fh := ctx.FramebufferWidth(), ctx.FramebufferHeight()

	if debugLastCtxW != w || debugLastCtxH != h || debugLastPhysW != fw || debugLastPhysH != fh {
		DebugLog("GOGPU_RENDERER_RESIZE: CtxLog:%dx%d CtxPhys:%dx%d HostCell:%dx%d HostGrid:%dx%d ExpectedPhys:%dx%d Scale:%f",
			w, h, fw, fh, r.cellW, r.cellH, r.host.cols, r.host.rows, r.host.cols*r.cellW, r.host.rows*r.cellH, r.host.gogpuScale)
		debugLastCtxW, debugLastCtxH = w, h
		debugLastPhysW, debugLastPhysH = fw, fh
	}

	if r.canvas == nil {
		provider := app.GPUContextProvider()
		if provider == nil { return }
		r.canvas, _ = ggcanvas.New(provider, fw, fh)
	} else {
		r.canvas.Resize(fw, fh)
	}

	if r.dirty {
		drawStart := time.Now()
		var totalFills, totalGlyphs int
		var timeFills, timeGlyphs time.Duration

		r.canvas.Draw(func(dc *gg.Context) {
			dc.SetRGB(0, 0, 0)
			dc.Clear()

			// Убираем сжатие. Рисуем пиксель в пиксель.
			dc.Identity()
			// GetMatrix не поддерживается, логируем доступные параметры контекста через PROBE ниже

			// Логируем размеры для отладки
			if debugDrawCount % 100 == 0 {
				DebugLog("GOGPU_PROBE: Ctx=%dx%d FB=%dx%d r.cellW=%d r.cellH=%d", w, h, fw, fh, r.cellW, r.cellH)
			}
			if debugDrawCount == 0 {
				DebugLog("GOGPU_PROBE_FIRST_FRAME: Expected grid is %dx%d. Drawing...", r.host.cols, r.host.rows)
			}
			debugDrawCount++

			if r.face != nil {
				dc.SetFont(r.face)
			}
			metrics := r.face.Metrics()
			ascent := float64(metrics.Ascent)

			for y := 0; y < r.host.rows; y++ {
				rowOff := y * r.host.cols
				for x := 0; x < r.host.cols; {
					cell := r.renderBuf[rowOff+x]
					_, bg := r.getCellColors(cell)

					spanW := 0
					for x+spanW < r.host.cols {
						nextCell := r.renderBuf[rowOff+x+spanW]
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

					lx := float64(x * r.cellW)
					ly := float64(y * r.cellH)
					spanPixW := float64(spanW * r.cellW)

					t_fill_0 := time.Now()
					dc.SetColor(bg)
					dc.DrawRectangle(lx, ly, spanPixW, float64(r.cellH))
					dc.Fill()
					timeFills += time.Since(t_fill_0)
					totalFills++

					for sx := 0; sx < spanW; sx++ {
						currX := x + sx
						currCell := r.renderBuf[rowOff+currX]
						if currCell.Char == WideCharFiller {
							continue
						}
						if currCell.Char != 0 && currCell.Char != ' ' && r.face != nil {
							t_glyph_0 := time.Now()
							cx := float64(currX * r.cellW)
							cfg, _ := r.getCellColors(currCell)
							dc.SetColor(cfg)
							dc.DrawString(string(rune(currCell.Char)), cx, ly+ascent)
							timeGlyphs += time.Since(t_glyph_0)
							totalGlyphs++
							
							if debugDrawCount == 1 && y == 0 && currX == 0 {
								DebugLog("GOGPU_PROBE_COORD: First char '%c' drawn at cx=%f, ly+ascent=%f", currCell.Char, cx, ly+ascent)
							}
						}
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
		drawDur := time.Since(drawStart)
		if drawDur > 5*time.Millisecond {
			DebugLog("GOGPU_RENDERER_PERF: DrawTotal: %v, Fills(%d): %v, Glyphs(%d): %v",
				drawDur, totalFills, timeFills, totalGlyphs, timeGlyphs)
		}
	}

	renderStart := time.Now()
	r.canvas.Render(ctx.RenderTarget())
	renderDur := time.Since(renderStart)
	if renderDur > 5*time.Millisecond {
		DebugLog("GOGPU_RENDERER: ggcanvas.Render took %v", renderDur)
	}
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