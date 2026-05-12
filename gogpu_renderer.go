//go:build !freebsd

package vtui

import (
	"image/color"
	"time"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gg/text"
)

type GogpuRenderer struct {
	host         *GogpuHost
	face         text.Face
	cellW, cellH int

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape

	canvas    *ggcanvas.Canvas
	renderBuf []CharInfo
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
	// Render is called by ScreenBuf.Flush while the mutex is held.
	// We copy data to a local buffer so Flush() can draw it asynchronously
	// without deadlocking on GetCell calls.
	if len(r.renderBuf) != len(buf) {
		r.renderBuf = make([]CharInfo, len(buf))
	}
	copy(r.renderBuf, buf)
}

func (r *GogpuRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.cursorX, r.cursorY = x, y
	r.cursorVis = visible
	r.cursorShape = shape
}

func (r *GogpuRenderer) SetPalette(pal *[256]uint32) {}

func (r *GogpuRenderer) Flush() {
	if r.host.ctx == nil || len(r.renderBuf) == 0 {
		return
	}

	w, h := r.host.ctx.Width(), r.host.ctx.Height()
	// Dynamically calculate cell size to support HiDPI scaling
	r.cellW = w / r.host.cols
	r.cellH = h / r.host.rows

	if r.canvas == nil {
		provider := r.host.app.GPUContextProvider()
		if provider == nil { return }
		r.canvas, _ = ggcanvas.New(provider, w, h)
	} else {
		r.canvas.Resize(w, h)
	}

	r.canvas.Draw(func(dc *gg.Context) {
		if r.face != nil {
			dc.SetFont(r.face)
		}

		for y := 0; y < r.host.rows; y++ {
			rowOff := y * r.host.cols
			for x := 0; x < r.host.cols; x++ {
				cell := r.renderBuf[rowOff+x]
				if cell.Char == WideCharFiller {
					continue
				}

				fg, bg := r.getCellColors(cell)

				dc.SetColor(bg)
				dc.DrawRectangle(float64(x*r.cellW), float64(y*r.cellH), float64(r.cellW), float64(r.cellH))
				dc.Fill()

				if cell.Char != 0 && cell.Char != ' ' && r.face != nil {
					dc.SetColor(fg)
					dc.DrawStringAnchored(string(rune(cell.Char)), float64(x*r.cellW), float64(y*r.cellH), 0, 0.85)
				}
			}
		}

		if r.cursorVis && (time.Now().UnixMilli()/500)%2 == 0 {
			dc.SetColor(color.White)
			cy := float64(r.cursorY * r.cellH)
			if r.cursorShape == CursorShapeUnderline {
				cy += float64(r.cellH) - 2
				dc.DrawRectangle(float64(r.cursorX*r.cellW), cy, float64(r.cellW), 2)
			} else {
				dc.DrawRectangle(float64(r.cursorX*r.cellW), cy, float64(r.cellW), float64(r.cellH))
			}
			dc.Fill()
		}
	})

	r.canvas.Render(r.host.ctx.RenderTarget())
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