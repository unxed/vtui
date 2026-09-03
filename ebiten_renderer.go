//go:build (linux || windows || darwin) && !android && (amd64 || arm64)

package vtui

import (
	"image"
	"image/color"
	"sync"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

// EbitenRenderer draws the CharInfo grid into an RGBA framebuffer which the
// Ebitengine game loop then uploads to the GPU as a single texture.
//
// Rasterising on the CPU and blitting once is a deliberate choice for this
// backend rather than a shortcut. Ebitengine's own text API would put one
// draw call behind every glyph, and a full screen of a file manager is a few
// thousand cells; the batch would be rebuilt every frame for a UI that
// usually changes two or three rows. Writing whole cells into a byte slice
// keeps redraw cost proportional to what actually changed, and the single
// WritePixels at the end is the only GPU traffic. It is the same shape as the
// X11 and Wayland backends, which is also why the glyph cache below is keyed
// identically.
//
// Render runs on the FrameManager goroutine and DrawTo runs on Ebitengine's
// loop, so every field they share sits behind mu.
type EbitenRenderer struct {
	mu sync.Mutex

	host         *EbitenHost
	face         font.Face
	cellW, cellH int
	cols, rows   int

	// scale is the display scale factor the font was measured at. It only
	// sets line thickness for the geometric glyphs: the font face is already
	// rasterised at the scaled size, so everything else is in real pixels.
	scale int

	// img is the rasterised frame. dirty says it has changed since the last
	// upload, so the game loop knows whether the texture needs rewriting.
	img   *image.RGBA
	dirty bool

	glyphCache map[glyphKey]*image.RGBA

	gfxList  []ImagePlacement
	gfxCache nativeGraphicsCache
	gfxGen   uint64
	gfxKnown bool

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      CursorShape

	// paintedCursor records where a caret was actually drawn into img on the
	// previous frame, which is not the same as where the caret currently is:
	// a caret that moved, went invisible, or blinked off leaves pixels behind
	// that only a repaint of that row can erase. Tracking the drawn position
	// separately from the logical one is what keeps a hidden caret from
	// dirtying its row forever.
	paintedCursor  bool
	paintedCursorY int

	blinkState    bool
	lastBlinkTime time.Time

	title        string
	titleChanged bool
}

// NewEbitenRenderer builds a renderer for a font face already measured into
// cellW by cellH pixels. scale is the display scale factor the face was
// rasterised at; pass 1 for a non-HiDPI screen.
func NewEbitenRenderer(host *EbitenHost, face font.Face, cellW, cellH, scale int) *EbitenRenderer {
	if scale < 1 {
		scale = 1
	}
	return &EbitenRenderer{
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

func (r *EbitenRenderer) getCellColors(cell CharInfo) (uint32, uint32) {
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

// Render rasterises the changed rows of buf into the framebuffer.
func (r *EbitenRenderer) Render(buf, shadow []CharInfo, w, h int, forceRedraw bool) {
	if w <= 0 || h <= 0 || len(buf) < w*h || len(shadow) < w*h {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// The blink phase advances on wall clock, not on frame count, so that a
	// backend running at 60fps and one running at 15fps blink alike. Only a
	// visible caret makes the phase change worth a repaint.
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
	if r.img == nil || r.img.Rect.Dx() != pixW || r.img.Rect.Dy() != pixH {
		r.img = image.NewRGBA(image.Rect(0, 0, pixW, pixH))
		forceRedraw = true
	}
	r.cols, r.rows = w, h
	img := r.img

	for y := 0; y < h; y++ {
		rowOff := y * w

		// A row is repainted when its cells changed, when the caret is about
		// to be drawn on it, or when the caret was drawn on it last frame and
		// those pixels now have to go.
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

			// Runs of identical background paint as one rectangle. Solid
			// backgrounds are most of a TUI frame, so this is where the time
			// goes if it is skipped.
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

				if ch := CellBaseRune(curr.Char); ch != 0 && ch != ' ' {
					// Frames, arrows and blocks are drawn geometrically so
					// that neighbouring cells join without hairline gaps;
					// anything the geometric path declines goes to the font.
					px, py := currX*r.cellW, y*r.cellH
					if isBoxDrawRune(ch) &&
						drawBoxGlyph(img, ch, px, py, r.cellW*rw, r.cellH, r.scale, fg) {
						sx += rw
						continue
					}
					r.drawCachedGlyph(img, curr.Char, px, py, rw, fg, cbg)
				}
				if curr.Attributes&CommonLvbUnderscore != 0 {
					drawUnderline(img, currX*r.cellW, y*r.cellH, r.cellW*rw, r.cellH, r.scale, fg)
				}

				if cursorVisible && y == r.cursorY && r.cursorX >= currX && r.cursorX < currX+rw {
					r.invertCursor(img, currX*r.cellW, y*r.cellH, rw)
				}
				sx += rw
			}
			x += spanW
		}
	}

	// Record what was actually drawn, so the next frame knows which row still
	// carries caret pixels that need erasing.
	r.paintedCursor = cursorVisible
	r.paintedCursorY = r.cursorY
}

// fillRect paints a solid rectangle. The first scanline is built once and the
// remaining ones are copied, which is what keeps a full-screen background fill
// off the profile.
func (r *EbitenRenderer) fillRect(img *image.RGBA, px, py, w, h int, rgb uint32) {
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

// drawCachedGlyph blits one cell, rasterising it first if this exact
// character, colour pair and width has not been seen before. Caching the
// composited cell rather than the glyph mask means the common case is a
// straight memcpy per scanline.
func (r *EbitenRenderer) drawCachedGlyph(img *image.RGBA, cellVal uint64, px, py, rw int, fg, bg uint32) {
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

// invertCursor inverts the cell under the caret, matching the X11 backend so
// the caret stays visible whatever colours the cell happens to carry.
func (r *EbitenRenderer) invertCursor(img *image.RGBA, px, py, rw int) {
	// Same geometry as the X11 backend: a block fills the cell, anything else
	// is an underline two pixels tall, four when the display is scaled, so it
	// stays visible on a HiDPI screen instead of thinning to a hair.
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

// RenderGraphics implements GraphicsRenderer, drawing the image layer over
// the text that Render has just laid down.
//
// It follows the X11 backend: the placements go into the same framebuffer, so
// images and text share one upload and cannot tear apart from each other. The
// work is skipped unless the layer changed or the cells beneath it did, since
// a picture that nothing has disturbed is already on screen.
func (r *EbitenRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil || layer.Protocol() != GraphicsNative {
		return
	}

	gen := layer.Generation()
	if !force && r.gfxKnown && gen == r.gfxGen && !layer.DirtyRowsUnder(buf, shadow, w, h) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.img == nil || r.cellW <= 0 || r.cellH <= 0 {
		return
	}
	r.gfxGen = gen
	r.gfxKnown = true

	r.gfxList, _ = layer.Snapshot(r.gfxList)
	drawNativePlacements(r.img, r.gfxList, r.cellW, r.cellH, &r.gfxCache)
	r.dirty = true
}

func (r *EbitenRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cursorX != x || r.cursorY != y || r.cursorVis != visible || r.cursorShape != shape {
		r.cursorX, r.cursorY = x, y
		r.cursorVis = visible
		r.cursorShape = shape
		// Restart the blink so the caret is solid right after it moves, which
		// is what makes fast typing feel like it is keeping up.
		r.blinkState = true
		r.lastBlinkTime = time.Now()
		r.dirty = true
	}
}

// SetPalette is a no-op: getCellColors reads ThemePalette directly, so an
// indexed cell picks up a palette change on the next repaint without the
// renderer holding a second copy that could drift out of date.
func (r *EbitenRenderer) SetPalette(pal *[256]uint32) {}

func (r *EbitenRenderer) SetWindowTitle(title string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.title != title {
		r.title = title
		r.titleChanged = true
	}
}

// Flush is where the other backends push bytes at the display. Ebitengine
// owns the frame clock, so there is nothing to push: the game loop reads the
// dirty flag and uploads on its own schedule.
func (r *EbitenRenderer) Flush() {}

// ResizeWindow asks Ebitengine for a window that fits the requested grid.
func (r *EbitenRenderer) ResizeWindow(cols, rows int) {
	r.mu.Lock()
	host, cw, ch := r.host, r.cellW, r.cellH
	r.mu.Unlock()
	if host != nil && cw > 0 && ch > 0 {
		host.requestSize(cols*cw, rows*ch)
	}
}

// WindowPosition returns the current desktop position of the Ebitengine
// window. Ebitengine owns the native window, so the query is delegated to
// its platform-aware API.
func (r *EbitenRenderer) WindowPosition() (x, y int, ok bool) {
	x, y = ebiten.WindowPosition()
	return x, y, true
}

// SetWindowPosition moves the Ebitengine window without changing its size.
func (r *EbitenRenderer) SetWindowPosition(x, y int) {
	ebiten.SetWindowPosition(x, y)
}

// takeFrame hands the current framebuffer to the game loop and reports
// whether it changed since last time. The pixels are returned rather than
// copied because WritePixels reads them synchronously, still under the lock.
func (r *EbitenRenderer) takeFrame() (pix []byte, w, h int, changed bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.img == nil {
		return nil, 0, 0, false
	}
	changed = r.dirty
	r.dirty = false
	return r.img.Pix, r.img.Rect.Dx(), r.img.Rect.Dy(), changed
}

// takeTitle returns a pending window title, if one was set since the last call.
func (r *EbitenRenderer) takeTitle() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.titleChanged {
		return "", false
	}
	r.titleChanged = false
	return r.title, true
}

// needsIdleBlinkHeartbeat marks EbitenRenderer as needing the idle blink
// heartbeat in FrameManager.Run(). See softwareBlinkRenderer.
func (r *EbitenRenderer) needsIdleBlinkHeartbeat() {}
