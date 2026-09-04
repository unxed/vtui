//go:build linux && !android && (amd64 || arm64)

package vtui

import (
	"image"
	"image/color"
	"math"
	"time"

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

	cursorX, cursorY       int
	oldCursorX, oldCursorY int
	cursorVis              bool
	cursorShape            CursorShape

	lastCursorReset time.Time
	blinkState      bool
	lastBlinkTime   time.Time

	stats renderStats

	gfxList  []ImagePlacement
	gfxCache nativeGraphicsCache
	gfxGen   uint64
	gfxKnown bool
}

func NewWaylandRenderer(host *WaylandHost, face font.Face) *WaylandRenderer {
	return &WaylandRenderer{
		host:            host,
		face:            face,
		glyphCache:      make(map[glyphKey]*image.RGBA),
		lastCursorReset: time.Now(),
		blinkState:      true,
		lastBlinkTime:   time.Now(),
	}
}

// setFace replaces the rasterizer after the Wayland output scale changes.
// The caller holds host.mu, which also protects rendering.
func (r *WaylandRenderer) setFace(face font.Face) {
	r.face = face
	r.glyphCache = make(map[glyphKey]*image.RGBA)
	r.gfxKnown = false
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
		r.lastCursorReset = time.Now()
		r.blinkState = true
	}
}

func (r *WaylandRenderer) SetWindowTitle(title string) {
	r.host.mu.Lock()
	defer r.host.mu.Unlock()
	if r.host.win != nil {
		r.host.win.SetTitle(title)
	}
}
func (r *WaylandRenderer) ResizeWindow(cols, rows int) {
	r.host.mu.Lock()
	widget := r.host.widget
	cw := r.host.cellW
	ch := r.host.cellH
	scale := r.host.scale
	r.host.mu.Unlock()

	if widget != nil {
		widget.ScheduleResize(logicalWaylandPixels(cols*cw, scale), logicalWaylandPixels(rows*ch, scale))
	}
}

// RenderGraphics implements GraphicsRenderer. The Wayland host pushes the
// whole buffer to the compositor on every flush, so unlike X11 there are no
// dirty lines to mark.
func (r *WaylandRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
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
	drawNativePlacements(img, r.gfxList, cw, ch, &r.gfxCache)
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

	now := time.Now()
	if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
		r.blinkState = !r.blinkState
		r.lastBlinkTime = r.lastBlinkTime.Add(500 * time.Millisecond)
		if now.Sub(r.lastBlinkTime) >= 500*time.Millisecond {
			r.lastBlinkTime = now
		}
	}

	cursorVisible := r.cursorVis && r.blinkState

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
					drawUnderline(img, cpx, py, cw*rw, ch, int(math.Ceil(r.host.scale)), cfg)
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

func (r *WaylandRenderer) drawCachedGlyph(img *image.RGBA, cellVal uint64, px, py, rw int, fg, bg uint32, fgCol, bgCol color.RGBA) {
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
		if py+iy >= img.Rect.Max.Y {
			break
		}
		dstOff := (py+iy)*img.Stride + px*4
		srcOff := iy * cached.Stride
		if dstOff+drawW*4 <= len(img.Pix) {
			copy(img.Pix[dstOff:dstOff+drawW*4], cached.Pix[srcOff:srcOff+drawW*4])
		}
	}
}

func (r *WaylandRenderer) drawCustomChar(img *image.RGBA, char rune, px, py, cw, ch int, fg uint32) bool {
	return drawBoxGlyph(img, char, px, py, cw, ch, int(math.Ceil(r.host.scale)), fg)
}

func (r *WaylandRenderer) Flush() {
	start := time.Now()
	// Wake DisplayRun and let its callback schedule the deferred redraw on
	// the Wayland goroutine. Direct ScheduleRedraw from FrameManager's
	// goroutine can leave the queued redraw invisible until another event.
	r.host.requestPresent()
	r.stats.totalFlush += time.Since(start)
	r.stats.frameCount++
}

// needsIdleBlinkHeartbeat marks WaylandRenderer as needing the idle blink
// heartbeat in FrameManager.Run(). See softwareBlinkRenderer.
func (r *WaylandRenderer) needsIdleBlinkHeartbeat() {}
