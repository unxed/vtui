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
	r.host.mu.Unlock()

	if ctx == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.renderBuf) == 0 {
		return
	}
	
	w, h := ctx.Width(), ctx.Height()
	logW := r.host.cols * r.host.cellW
	logH := r.host.rows * r.host.cellH

	// ГИПОТЕЗА: ctx.Width() врет из-за кривого HiDPI в gogpu (отдает 400 вместо 800).
	// Форсируем размер текстуры канваса до требуемого логического размера (800x630).
	canvasW, canvasH := logW, logH

	if r.canvas == nil {
		provider := r.host.app.GPUContextProvider()
		if provider == nil { return }
		r.canvas, _ = ggcanvas.New(provider, canvasW, canvasH)
	} else {
		r.canvas.Resize(canvasW, canvasH)
	}

	if r.dirty {
		r.canvas.Draw(func(dc *gg.Context) {
			dc.SetRGB(0, 0, 0)
			dc.Clear()

			// Убираем сжатие. Рисуем пиксель в пиксель.
			dc.Identity()

			// Логируем размеры для отладки
			if debugDrawCount % 10 == 0 {
				DebugLog("GOGPU_PROBE: Ctx=%dx%d CanvasForced=%dx%d", w, h, canvasW, canvasH)
			}

			if r.face != nil {
				dc.SetFont(r.face)
			}
			metrics := r.face.Metrics()
			ascent := float64(metrics.Ascent)

			for y := 0; y < r.host.rows; y++ {
				rowOff := y * r.host.cols
				for x := 0; x < r.host.cols; x++ {
					cell := r.renderBuf[rowOff+x]
					if cell.Char == WideCharFiller {
						continue
					}

					fg, bg := r.getCellColors(cell)
					lx := float64(x * r.cellW)
					ly := float64(y * r.cellH)

					dc.SetColor(bg)
					dc.DrawRectangle(lx, ly, float64(r.cellW), float64(r.cellH))
					dc.Fill()

					if cell.Char != 0 && cell.Char != ' ' && r.face != nil {
						dc.SetColor(fg)
						// Monospace font character alignment inside the cell using ascent baseline
						dc.DrawString(string(rune(cell.Char)), lx, ly+ascent)
					}
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