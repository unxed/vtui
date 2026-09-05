package vtui

import (
	"image"
	"image/color"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// Win32GuiRenderer renders the character grid into a 32-bit software bitmap
// and blits it to a native Win32 window HDC using GDI (SetDIBitsToDevice/BitBlt).
type Win32GuiRenderer struct {
	mu           sync.Mutex
	host         *Win32GuiHost
	face         font.Face
	cellW, cellH int
	cols, rows   int
	scale        int

	imgBuf  *image.RGBA
	bgraBuf []byte
	dirty   bool

	glyphCache map[glyphKey]*image.RGBA

	gfxList  []ImagePlacement
	gfxCache nativeGraphicsCache
	gfxGen   uint64
	gfxKnown bool

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape

	paintedCursor  bool
	paintedCursorY int

	blinkState    bool
	lastBlinkTime time.Time

	// Windows-only double-buffer handles (GDI memory DC + compatible
	// bitmap). Typed as uintptr, not syscall.Handle, so this struct stays
	// buildable on every platform: this file has no build tag, only
	// win32_gui_windows.go (which does) touches these as GDI handles.
	// See blitTo() in win32_gui_windows.go and f4 issue #514.
	memDC      uintptr
	memBitmap  uintptr
	memBits    uintptr
	memW, memH int
}

func NewWin32GuiRenderer(host *Win32GuiHost, face font.Face, cellW, cellH int) *Win32GuiRenderer {
	scale := 1
	if host != nil && host.scale > 1 {
		scale = host.scale
	}
	return &Win32GuiRenderer{
		host:          host,
		face:          face,
		cellW:         cellW,
		cellH:         cellH,
		scale:         scale,
		glyphCache:    make(map[glyphKey]*image.RGBA),
		blinkState:    true,
		lastBlinkTime: time.Now(),
	}
}

func (r *Win32GuiRenderer) SetPalette(pal *[256]uint32) {}

func (r *Win32GuiRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cursorX != x || r.cursorY != y || r.cursorVis != visible || r.cursorShape != shape {
		r.cursorX, r.cursorY = x, y
		r.cursorVis = visible
		r.cursorShape = shape
		r.blinkState = true
		r.lastBlinkTime = time.Now()
		r.dirty = true
	}
}

func (r *Win32GuiRenderer) SetWindowTitle(title string) {
	if r.host != nil {
		r.host.SetTitle(title)
	}
}

func (r *Win32GuiRenderer) ResizeWindow(cols, rows int) {
	if r.host != nil {
		r.host.ResizeGrid(cols, rows)
	}
}

func (r *Win32GuiRenderer) getCellColors(cell CharInfo) (uint32, uint32) {
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

func (r *Win32GuiRenderer) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {
	if w <= 0 || h <= 0 || len(buf) < w*h || len(shadow) < w*h {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
		r.blinkState = !r.blinkState
		r.lastBlinkTime = r.lastBlinkTime.Add(500 * time.Millisecond)
		if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
			r.lastBlinkTime = now
		}
	}
	cursorVisible := r.cursorVis && r.blinkState

	pixW, pixH := w*r.cellW, h*r.cellH
	if r.imgBuf == nil || r.imgBuf.Rect.Dx() != pixW || r.imgBuf.Rect.Dy() != pixH {
		r.imgBuf = image.NewRGBA(image.Rect(0, 0, pixW, pixH))
		r.bgraBuf = make([]byte, pixW*pixH*4)
		forceRedraw = true
		// A brand-new bitmap always has to reach the window, even if the
		// row loop below finds nothing to draw (an all-blank screen at
		// startup compares equal to a blank shadow buffer).
		r.dirty = true
	}
	r.cols, r.rows = w, h
	img := r.imgBuf

	for y := 0; y < h; y++ {
		rowOff := y * w
		rowDirty := forceRedraw ||
			(cursorVisible && y == r.cursorY) ||
			(r.paintedCursor && y == r.paintedCursorY)
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
		r.dirty = true

		for x := 0; x < w; {
			cell := buf[rowOff+x]
			_, bg := r.getCellColors(cell)

			spanW := 0
			for x+spanW < w {
				next := buf[rowOff+x+spanW]
				if next.Char == WideCharFiller {
					spanW++
					continue
				}
				if _, nextBg := r.getCellColors(next); nextBg != bg {
					break
				}
				spanW++
			}

			r.fillRect(img, x*r.cellW, y*r.cellH, spanW*r.cellW, r.cellH, bg)

			for sx := 0; sx < spanW; {
				currX := x + sx
				curr := buf[rowOff+currX]
				if curr.Char == WideCharFiller {
					sx++
					continue
				}

				_, rw := CellSpanAt(buf, w, currX, y)
				if rw < 1 {
					rw = 1
				}
				fg, cbg := r.getCellColors(curr)
				px, py := currX*r.cellW, y*r.cellH

				if ch := CellBaseRune(curr.Char); ch != 0 && ch != ' ' {
					if !(isBoxDrawRune(ch) &&
						drawBoxGlyph(img, ch, px, py, r.cellW*rw, r.cellH, r.scale, fg)) {
						r.drawCachedGlyph(img, curr.Char, px, py, rw, fg, cbg)
					}
				}
				// GDI never sees the cell attributes, so the underline of a
				// hovered URL (f4 #459) is painted here like in the other
				// pixel backends.
				if curr.Attributes&CommonLvbUnderscore != 0 {
					drawUnderline(img, px, py, r.cellW*rw, r.cellH, r.scale, fg)
				}

				if cursorVisible && y == r.cursorY && r.cursorX >= currX && r.cursorX < currX+rw {
					r.invertCursor(img, currX*r.cellW, y*r.cellH, rw)
				}
				sx += rw
			}
			x += spanW
		}
	}

	r.paintedCursor = cursorVisible
	r.paintedCursorY = r.cursorY
}

func (r *Win32GuiRenderer) fillRect(img *image.RGBA, px, py, w, h int, rgb uint32) {
	if w <= 0 || h <= 0 {
		return
	}
	if px+w > img.Rect.Dx() {
		w = img.Rect.Dx() - px
	}
	if w <= 0 {
		return
	}
	cr, cg, cb := uint8(rgb>>16), uint8(rgb>>8), uint8(rgb)

	base := py*img.Stride + px*4
	rowBytes := w * 4
	if base < 0 || base+rowBytes > len(img.Pix) {
		return
	}
	img.Pix[base], img.Pix[base+1], img.Pix[base+2], img.Pix[base+3] = cr, cg, cb, 255
	for n := 4; n < rowBytes; n *= 2 {
		copy(img.Pix[base+n:base+rowBytes], img.Pix[base:base+n])
	}
	for iy := 1; iy < h; iy++ {
		if py+iy >= img.Rect.Dy() {
			break
		}
		off := (py+iy)*img.Stride + px*4
		if off+rowBytes <= len(img.Pix) {
			copy(img.Pix[off:off+rowBytes], img.Pix[base:base+rowBytes])
		}
	}
}

func (r *Win32GuiRenderer) drawCachedGlyph(img *image.RGBA, cellVal uint64, px, py, rw int, fg, bg uint32) {
	key := glyphKey{ch: cellVal, fg: fg, bg: bg, w: rw}
	cached, ok := r.glyphCache[key]
	drawW := r.cellW * rw

	if !ok {
		cached = image.NewRGBA(image.Rect(0, 0, drawW, r.cellH))
		fgCol := color.RGBA{R: uint8(fg >> 16), G: uint8(fg >> 8), B: uint8(fg), A: 255}
		bgCol := color.RGBA{R: uint8(bg >> 16), G: uint8(bg >> 8), B: uint8(bg), A: 255}

		for i := 0; i+3 < len(cached.Pix); i += 4 {
			cached.Pix[i], cached.Pix[i+1], cached.Pix[i+2], cached.Pix[i+3] = bgCol.R, bgCol.G, bgCol.B, 255
		}

		if r.face != nil {
			d := &font.Drawer{
				Dst:  cached,
				Src:  image.NewUniform(fgCol),
				Face: r.face,
				Dot:  fixed.Point26_6{X: 0, Y: r.face.Metrics().Ascent},
			}
			d.DrawString(CellString(cellVal))
		}
		r.glyphCache[key] = cached
	}

	rowBytes := drawW * 4
	for iy := 0; iy < r.cellH; iy++ {
		if py+iy >= img.Rect.Dy() {
			break
		}
		dst := (py+iy)*img.Stride + px*4
		src := iy * cached.Stride
		if dst+rowBytes <= len(img.Pix) && src+rowBytes <= len(cached.Pix) {
			copy(img.Pix[dst:dst+rowBytes], cached.Pix[src:src+rowBytes])
		}
	}
}

func (r *Win32GuiRenderer) invertCursor(img *image.RGBA, px, py, rw int) {
	startY := 0
	if r.cursorShape != CursorShapeBlock {
		thickness := 2
		if r.scale > 1 {
			thickness = 4
		}
		startY = r.cellH - thickness
		if startY < 0 {
			startY = 0
		}
	}
	for iy := startY; iy < r.cellH; iy++ {
		if py+iy >= img.Rect.Dy() {
			break
		}
		row := (py + iy) * img.Stride
		for ix := 0; ix < r.cellW*rw; ix++ {
			if px+ix >= img.Rect.Dx() {
				break
			}
			off := row + (px+ix)*4
			if off+2 < len(img.Pix) {
				img.Pix[off] = 255 - img.Pix[off]
				img.Pix[off+1] = 255 - img.Pix[off+1]
				img.Pix[off+2] = 255 - img.Pix[off+2]
			}
		}
	}
}

func (r *Win32GuiRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil || layer.Protocol() != GraphicsNative {
		return
	}

	gen := layer.Generation()
	if !force && r.gfxKnown && gen == r.gfxGen && !layer.DirtyRowsUnder(buf, shadow, w, h) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.imgBuf == nil || r.cellW <= 0 || r.cellH <= 0 {
		return
	}
	r.gfxGen = gen
	r.gfxKnown = true

	r.gfxList, _ = layer.Snapshot(r.gfxList)
	drawNativePlacements(r.imgBuf, r.gfxList, r.cellW, r.cellH, &r.gfxCache)
	r.dirty = true
}

func (r *Win32GuiRenderer) Flush() {
	r.mu.Lock()
	dirty := r.dirty
	r.dirty = false
	r.mu.Unlock()

	if r.host == nil {
		return
	}
	// Re-invalidate while a previous invalidation has not yet produced a
	// painted frame. Without this the backend gets exactly one chance per
	// content change: BeginPaint validates the update region whether or not
	// anything was blitted, and an idle UI never changes a row again, so a
	// single lost or empty WM_PAINT leaves the window blank until the next
	// keystroke -- or forever. FrameManager already ticks this renderer at
	// ~250ms via the software-blink heartbeat, so recovery is automatic.
	if dirty || r.host.paintOutstanding() {
		r.host.Invalidate()
	}
}

func (r *Win32GuiRenderer) syncBGRA() (w, h int, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.syncBGRALocked()
}

// syncBGRALocked converts the composed RGBA frame into the BGRA scratch buffer
// GDI expects. The caller must hold r.mu, and must keep holding it for as long
// as it uses bgraBuf: Render() reallocates both buffers when the grid resizes.
func (r *Win32GuiRenderer) syncBGRALocked() (w, h int, ok bool) {
	if r.imgBuf == nil || len(r.imgBuf.Pix) == 0 {
		return 0, 0, false
	}
	w = r.imgBuf.Rect.Dx()
	h = r.imgBuf.Rect.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0, false
	}

	lineStride := w * 4
	if len(r.bgraBuf) != len(r.imgBuf.Pix) {
		r.bgraBuf = make([]byte, len(r.imgBuf.Pix))
	}

	for y := 0; y < h; y++ {
		off := y * lineStride
		rgbaToBGRA(r.bgraBuf[off:off+lineStride], r.imgBuf.Pix[off:off+lineStride], lineStride)
	}
	return w, h, true
}

// needsIdleBlinkHeartbeat marks Win32GuiRenderer as needing the idle
// blink heartbeat in FrameManager.Run(). See softwareBlinkRenderer.
func (r *Win32GuiRenderer) needsIdleBlinkHeartbeat() {}
