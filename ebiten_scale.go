//go:build (linux || windows || darwin) && !android && (amd64 || arm64) && !vtui_noebiten

package vtui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// ebitenMonitorScale reads the device scale factor of the monitor the window
// is on. A variable so tests can stand in for the display.
//
// Ebitengine resolves the monitor from the window, so a move to another
// monitor is seen. It reads each monitor's scale once, though (upstream
// TODO #2343), so changing the scale of the monitor the window is already
// on is not seen until the window moves.
var ebitenMonitorScale = func() float64 {
	if m := ebiten.Monitor(); m != nil {
		return m.DeviceScaleFactor()
	}
	return 1
}

// ebitenScale rounds a device scale factor to the integer scale the
// framebuffer is laid out at.
func ebitenScale(factor float64) int {
	scale := int(factor + 0.5)
	if scale < 1 {
		scale = 1
	}
	return scale
}

// ebitenDIPs converts a window size in device pixels to the
// device-independent pixels SetWindowSize takes, rounding up: rounding down
// an odd size at scale 2 hands Layout back one pixel less than the grid
// needs, and the window comes up a row or column short.
func ebitenDIPs(px, scale int) int {
	if scale < 1 {
		scale = 1
	}
	return (px + scale - 1) / scale
}

// checkScale compares the monitor's scale with the one the font was measured
// at and, when it changed, reloads the font at the new scale through the
// hot-swap path. That keeps the grid and queues the window size in
// device-independent pixels for the new scale, which Update applies right
// after this call. It reports whether the scale changed. Runs on the game
// loop, from Update.
func (h *EbitenHost) checkScale(factor float64) bool {
	scale := ebitenScale(factor)
	h.mu.Lock()
	old := h.scale
	if scale == old {
		h.mu.Unlock()
		return false
	}
	h.scale = scale
	fontName, fontSize := h.fontName, h.fontSize
	h.mu.Unlock()

	DebugLog("EBITEN_HOST: display scale %d -> %d", old, scale)
	if h.renderer != nil {
		h.renderer.setScale(scale)
	}
	h.SetFont(fontName, fontSize)
	return true
}

// setScale changes the line-thickness scale (underlines, box drawing, the
// cursor) after a display scale change. Glyphs cached at the old scale go.
func (r *EbitenRenderer) setScale(scale int) {
	if scale < 1 {
		scale = 1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scale = scale
	r.glyphCache = make(map[glyphKey]*image.RGBA)
}
