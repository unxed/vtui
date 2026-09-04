//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"image"
	"image/color"
	"time"

	"github.com/jezek/xgb/xproto"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type X11Renderer struct {
	host        *X11Host
	face        font.Face
	w, h        int
	glyphCache  map[glyphKey]*image.RGBA
	cursorX     int
	cursorY     int
	cursorVis   bool
	cursorShape CursorShape

	oldCursorX int
	oldCursorY int

	lastCursorReset time.Time
	blinkState      bool
	lastBlinkTime   time.Time

	stats renderStats

	gfxList  []ImagePlacement
	gfxCache nativeGraphicsCache
	gfxGen   uint64
	gfxKnown bool
}

func NewX11Renderer(host *X11Host, face font.Face) *X11Renderer {
	return &X11Renderer{
		host:            host,
		face:            face,
		glyphCache:      make(map[glyphKey]*image.RGBA),
		lastCursorReset: time.Now(),
		blinkState:      true,
		lastBlinkTime:   time.Now(),
	}
}

func (r *X11Renderer) SetPalette(pal *[256]uint32) {
}

func (r *X11Renderer) ResizeWindow(cols, rows int) {
	r.host.mu.Lock()
	conn := r.host.conn
	wid := r.host.wid
	screen := r.host.screen
	initialCols := r.host.initialCols
	r.host.mu.Unlock()

	if conn == nil || screen == nil {
		return
	}

	// Посылаем ClientMessage родителю (Window Manager) для нативного разворачивания/восстановления
	stateAtom, _ := xproto.InternAtom(conn, false, 13, "_NET_WM_STATE").Reply()
	maxVertAtom, _ := xproto.InternAtom(conn, false, 28, "_NET_WM_STATE_MAXIMIZED_VERT").Reply()
	maxHorzAtom, _ := xproto.InternAtom(conn, false, 28, "_NET_WM_STATE_MAXIMIZED_HORZ").Reply()

	if stateAtom != nil && maxVertAtom != nil && maxHorzAtom != nil {
		action := 0 // _NET_WM_STATE_REMOVE (восстановить)
		if cols > initialCols {
			action = 1 // _NET_WM_STATE_ADD (развернуть)
		}

		var data8 [20]byte
		put32 := func(b []byte, v uint32) {
			b[0] = byte(v)
			b[1] = byte(v >> 8)
			b[2] = byte(v >> 16)
			b[3] = byte(v >> 24)
		}
		put32(data8[0:], uint32(action))
		put32(data8[4:], uint32(maxVertAtom.Atom))
		put32(data8[8:], uint32(maxHorzAtom.Atom))
		put32(data8[12:], 1)

		ev := xproto.ClientMessageEvent{
			Format: 32,
			Window: wid,
			Type:   stateAtom.Atom,
			Data:   xproto.ClientMessageDataUnion{Data8: data8[:]},
		}

		xproto.SendEvent(conn, false, screen.Root,
			xproto.EventMaskSubstructureRedirect|xproto.EventMaskSubstructureNotify,
			string(ev.Bytes()))
	}
}

func (r *X11Renderer) SetCursor(x, y int, visible bool, shape CursorShape) {
	if r.cursorX != x || r.cursorY != y || r.cursorVis != visible || r.cursorShape != shape {
		r.oldCursorX = r.cursorX
		r.oldCursorY = r.cursorY
		r.cursorX = x
		r.cursorY = y
		r.cursorVis = visible
		r.cursorShape = shape
		r.lastCursorReset = time.Now()
		r.blinkState = true
	}
}

func (r *X11Renderer) SetWindowTitle(title string) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()

	titleBytes := []byte(title)
	xproto.ChangeProperty(r.host.conn, xproto.PropModeReplace, r.host.wid, xproto.AtomWmName, xproto.AtomString, 8, uint32(len(titleBytes)), titleBytes)

	netWmName, err := xproto.InternAtom(r.host.conn, false, 12, "_NET_WM_NAME").Reply()
	if err == nil && netWmName != nil {
		utf8String, err2 := xproto.InternAtom(r.host.conn, false, 11, "UTF8_STRING").Reply()
		if err2 == nil && utf8String != nil {
			xproto.ChangeProperty(r.host.conn, xproto.PropModeReplace, r.host.wid, netWmName.Atom, utf8String.Atom, 8, uint32(len(titleBytes)), titleBytes)
		}
	}
}

// RenderGraphics implements GraphicsRenderer by compositing the image layer
// straight into the window framebuffer. That is both faster and sharper than
// any escape sequence protocol, and it needs nothing from the terminal.
func (r *X11Renderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil || layer.Protocol() != GraphicsNative {
		return
	}

	gen := layer.Generation()
	if !force && r.gfxKnown && gen == r.gfxGen && !layer.DirtyRowsUnder(buf, shadow, w, h) {
		return
	}
	r.gfxGen = gen
	r.gfxKnown = true

	r.host.mu.Lock()
	defer r.host.mu.Unlock()

	img := r.host.imgBuf
	cw, ch := r.host.cellW, r.host.cellH
	if img == nil || cw <= 0 || ch <= 0 {
		return
	}

	r.gfxList, _ = layer.Snapshot(r.gfxList)
	rect := drawNativePlacements(img, r.gfxList, cw, ch, &r.gfxCache)
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		if y >= 0 && y < len(r.host.dirtyLines) {
			r.host.dirtyLines[y] = true
		}
	}
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
	start := time.Now()
	r.host.mu.Lock()
	defer r.host.mu.Unlock()

	now := time.Now()
	if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
		r.blinkState = !r.blinkState
		r.lastBlinkTime = r.lastBlinkTime.Add(500 * time.Millisecond)
		if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
			r.lastBlinkTime = now
		}
	}

	cursorVisible := r.cursorVis && r.blinkState

	// Keep the backing image at the native window size, not at the nearest
	// cell-aligned size. X11 configure events may describe a window with a
	// partial cell at the right or bottom edge. If the image were truncated to
	// w*cellW by h*cellH, PutImage would leave the old pixels in those partial
	// margins visible after a resize.
	windowWidth := int(r.host.width)
	windowHeight := int(r.host.height)
	if windowWidth <= 0 {
		windowWidth = w * r.host.cellW
	}
	if windowHeight <= 0 {
		windowHeight = h * r.host.cellH
	}
	if r.host.imgBuf == nil || r.host.imgBuf.Bounds().Dx() != windowWidth || r.host.imgBuf.Bounds().Dy() != windowHeight {
		r.host.imgBuf = image.NewRGBA(image.Rect(0, 0, windowWidth, windowHeight))
		if r.host.shmSeg == 0 {
			r.host.bgraBuf = make([]byte, len(r.host.imgBuf.Pix))
		}
		r.host.dirtyLines = make([]bool, windowHeight)
		for i := range r.host.dirtyLines {
			r.host.dirtyLines[i] = true
		}
		forceRedraw = true
	}

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

		for iy := 0; iy < ch; iy++ {
			lineIdx := y*ch + iy
			if lineIdx >= 0 && lineIdx < len(r.host.dirtyLines) {
				r.host.dirtyLines[lineIdx] = true
			}
		}

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
					if py+iy >= int(r.host.height) {
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

				char := CellBaseRune(currCell.Char)
				_, rw := CellSpanAt(buf, w, currX, y)

				cpx := currX * cw
				cfg, cbg := r.getCellColors(currCell)
				fgColor := color.RGBA{R: uint8(cfg >> 16), G: uint8(cfg >> 8), B: uint8(cfg), A: 255}
				bgColor := color.RGBA{R: uint8(cbg >> 16), G: uint8(cbg >> 8), B: uint8(cbg), A: 255}

				if char != 0 && char != ' ' {
					r.stats.glyphs++
					if !r.drawCustomChar(img, char, cpx, py, cw, ch, cfg) {
						r.drawCachedGlyph(img, currCell.Char, cpx, py, rw, cfg, cbg, fgColor, bgColor)
					}
				}
				if currCell.Attributes&CommonLvbUnderscore != 0 {
					drawUnderline(img, cpx, py, cw*rw, ch, r.host.scale, cfg)
				}

				if cursorVisible && y == r.cursorY && r.cursorX >= currX && r.cursorX < currX+rw {
					var startY int
					if r.cursorShape == CursorShapeBlock {
						startY = 0
					} else {
						thickness := 2
						if r.host.scale > 1 {
							thickness = 4
						}
						startY = ch - thickness
					}
					for iy := startY; iy < ch; iy++ {
						pixelY := py + iy
						if pixelY < 0 || pixelY >= img.Rect.Max.Y {
							continue
						}
						rowStart := pixelY * img.Stride
						for ix := 0; ix < cw*rw; ix++ {
							pixelX := cpx + ix
							if pixelX < 0 || pixelX >= img.Rect.Max.X {
								continue
							}
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

func (r *X11Renderer) drawCachedGlyph(img *image.RGBA, cellVal uint64, px, py, rw int, fg, bg uint32, fgCol, bgCol color.RGBA) {
	key := glyphKey{cellVal, fg, bg, rw}
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
		d.DrawString(CellString(cellVal))
		r.glyphCache[key] = cached
	}
	for iy := 0; iy < ch; iy++ {
		if py+iy >= int(r.host.height) {
			break
		}
		dstOff := (py+iy)*img.Stride + px*4
		srcOff := iy * cached.Stride
		if dstOff+drawW*4 <= len(img.Pix) {
			copy(img.Pix[dstOff:dstOff+drawW*4], cached.Pix[srcOff:srcOff+drawW*4])
		}
	}
}

func (r *X11Renderer) Flush() {
	start := time.Now()
	calls := r.host.flushImage()

	r.stats.totalFlush += time.Since(start)
	r.stats.putImages += calls
	r.stats.frameCount++

	if time.Since(r.stats.lastReport) >= 2*time.Second {
		r.reportStats()
	}
}

func (r *X11Renderer) reportStats() {
	if r.stats.frameCount == 0 {
		r.stats.lastReport = time.Now()
		return
	}

	avgDraw := r.stats.totalDraw / time.Duration(r.stats.frameCount)
	avgFlush := r.stats.totalFlush / time.Duration(r.stats.frameCount)

	DebugLog("[GUI PERF] FPS: %d, AvgDraw: %v, AvgFlush: %v, Dirty: %d/%d rows, PutImages: %d, Glyphs: %d",
		r.stats.frameCount/2,
		avgDraw,
		avgFlush,
		r.stats.dirtyRows/r.stats.frameCount,
		r.stats.totalRows/r.stats.frameCount,
		r.stats.putImages/r.stats.frameCount,
		r.stats.glyphs/r.stats.frameCount)

	r.stats = renderStats{lastReport: time.Now()}
}

func (r *X11Renderer) drawCustomChar(img *image.RGBA, char rune, px, py, cw, ch int, fg uint32) bool {
	return drawBoxGlyph(img, char, px, py, cw, ch, r.host.scale, fg)
}

// needsIdleBlinkHeartbeat marks X11Renderer as needing the idle blink
// heartbeat in FrameManager.Run(). See softwareBlinkRenderer.
func (r *X11Renderer) needsIdleBlinkHeartbeat() {}
