//go:build windows

package vtui

import (
	"image"
	"math"
	"syscall"
	"unsafe"
)

// Per-monitor DPI support for the Win32 GUI backend.
//
// The process is per-monitor-v2 DPI aware (the application manifest says so),
// which has two consequences this file deals with. First, GetDeviceCaps on
// the screen DC reports the *system* DPI, which Windows fixes at sign-in: after
// the display scale or resolution changes it keeps reporting the old value
// until the user signs out, so a window measured with it opens with the font
// of the previous display. Second, Windows does not stretch the window when
// it moves to a monitor with another scale, or when that scale changes: it
// sends WM_DPICHANGED and expects the window to redraw itself at the new DPI.
//
// The procs are looked up lazily and every one is optional: GetDpiForWindow
// and AdjustWindowRectExForDpi need Windows 10 1607, GetDpiForMonitor needs
// 8.1, and on anything older the backend keeps the system DPI it always used.

const (
	wmDpiChanged = 0x02E0

	monitorDefaultToPrimary = 0x00000001
	mdtEffectiveDPI         = 0

	defaultWin32DPI = 96.0
)

var (
	procGetDpiForWindow          = user32.NewProc("GetDpiForWindow")
	procAdjustWindowRectExForDpi = user32.NewProc("AdjustWindowRectExForDpi")
	procMonitorFromPoint         = user32.NewProc("MonitorFromPoint")
	procMonitorFromWindow        = user32.NewProc("MonitorFromWindow")

	shcoreDLL            = syscall.NewLazyDLL("shcore.dll")
	procGetDpiForMonitor = shcoreDLL.NewProc("GetDpiForMonitor")
)

// monitorDPI returns the effective DPI of a monitor, or 0 when it cannot be
// read (no monitor, or Windows older than 8.1).
func monitorDPI(hmon uintptr) float64 {
	if hmon == 0 || procGetDpiForMonitor.Find() != nil {
		return 0
	}
	var dpiX, dpiY uint32
	hr, _, _ := procGetDpiForMonitor.Call(hmon, mdtEffectiveDPI,
		uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	if hr != 0 || dpiX == 0 {
		return 0
	}
	return float64(dpiX)
}

// primaryMonitorDPI is the DPI a new window is measured with before it
// exists: that of the primary monitor, where CW_USEDEFAULT puts it.
func primaryMonitorDPI() float64 {
	if procMonitorFromPoint.Find() == nil {
		// MonitorFromPoint takes a POINT by value; {0,0} packs to 0.
		hmon, _, _ := procMonitorFromPoint.Call(0, monitorDefaultToPrimary)
		if dpi := monitorDPI(hmon); dpi > 0 {
			return dpi
		}
	}
	return getWin32DPI()
}

// win32WindowDPI is the DPI of the monitor the window is on now.
func win32WindowDPI(hwnd syscall.Handle) float64 {
	if hwnd != 0 && procGetDpiForWindow.Find() == nil {
		if dpi, _, _ := procGetDpiForWindow.Call(uintptr(hwnd)); dpi != 0 {
			return float64(dpi)
		}
	}
	if hwnd != 0 && procMonitorFromWindow.Find() == nil {
		hmon, _, _ := procMonitorFromWindow.Call(uintptr(hwnd), monitorDefaultToNearest)
		if dpi := monitorDPI(hmon); dpi > 0 {
			return dpi
		}
	}
	return getWin32DPI()
}

// win32DPIScale turns a monitor DPI into the font DPI loadBestFont takes and
// the integer scale used for line thickness. The 72 baseline matches the
// font size convention of the other backends (size in pixels at 100%), and,
// as before, a DPI under 96 is not scaled down.
func win32DPIScale(dpi float64) (fontDPI float64, scale int) {
	if dpi <= 0 {
		dpi = defaultWin32DPI
	}
	factor := dpi / defaultWin32DPI
	if factor < 1 {
		factor = 1
	}
	scale = int(factor + 0.5)
	if scale < 1 {
		scale = 1
	}
	return 72.0 * factor, scale
}

// win32FontDPIChanged reports whether two font DPIs differ enough to reload
// the font for.
func win32FontDPIChanged(old, new float64) bool {
	return math.Abs(old-new) >= 0.5
}

// adjustWindowRectForDPI grows a client rectangle to the outer window
// rectangle at the given DPI. The non-client area (title bar, borders) of a
// per-monitor-v2 window is scaled by Windows, so the plain
// AdjustWindowRectEx, which uses the system DPI, gets it wrong on any
// monitor whose scale differs from the sign-in one.
func adjustWindowRectForDPI(rc *win32Rect, dpi float64) {
	style := uintptr(wsOverlappedWindow)
	exStyle := uintptr(wsExAcceptFiles | wsExAppWindow)
	if dpi > 0 && procAdjustWindowRectExForDpi.Find() == nil {
		ok, _, _ := procAdjustWindowRectExForDpi.Call(uintptr(unsafe.Pointer(rc)), style, 0, exStyle, uintptr(uint32(dpi)))
		if ok != 0 {
			return
		}
	}
	_, _, _ = procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(rc)), style, 0, exStyle)
}

// setScale changes the line-thickness scale of the renderer (underlines, box
// drawing, the cursor) after a DPI change. Cached glyphs were drawn with the
// old scale, so they go. Lives next to its only, Windows-only caller for the
// same reason setFace does.
func (r *Win32GuiRenderer) setScale(scale int) {
	if scale < 1 {
		scale = 1
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.scale == scale {
		return
	}
	r.scale = scale
	r.glyphCache = make(map[glyphKey]*image.RGBA)
}

// applyDPILocked switches the host to a new monitor DPI: it reloads the font
// at the matching font DPI and hands the new cell size and line scale to the
// renderer and the graphics layer. It returns false, and changes nothing,
// when the DPI gives the font DPI the window already uses. The grid
// (cols x rows) is left alone; resizing the window to keep it is the
// caller's job. The caller holds h.mu.
func (h *Win32GuiHost) applyDPILocked(dpi float64) bool {
	fontDPI, scale := win32DPIScale(dpi)
	if !win32FontDPIChanged(h.fontDPI, fontDPI) {
		return false
	}
	DebugLog("WIN32GUI: DPI %.0f -> font DPI %.1f (was %.1f), scale %d", dpi, fontDPI, h.fontDPI, scale)
	h.fontDPI = fontDPI
	h.scale = scale
	if h.renderer != nil {
		h.renderer.setScale(scale)
	}
	h.applyFontLocked(h.fontName, h.fontSize)
	return true
}

// gridWindowRect is the outer window rectangle that holds cols x rows cells
// at the given DPI, placed at (x, y).
func gridWindowRect(x, y int32, cols, rows, cellW, cellH int, dpi float64) win32Rect {
	rc := win32Rect{right: win32Extent(cols, cellW), bottom: win32Extent(rows, cellH)}
	adjustWindowRectForDPI(&rc, dpi)
	return win32Rect{left: x, top: y, right: x + rc.right - rc.left, bottom: y + rc.bottom - rc.top}
}

// handleDPIChange carries out WM_DPICHANGED on the window's own thread. The
// suggested rectangle Windows passes is the old window scaled linearly; its
// position is kept, but the size is recomputed from the new cell size so the
// grid stays cols x rows -- a font does not scale linearly, and a grid that
// lost or gained a column would reflow the panels.
func (h *Win32GuiHost) handleDPIChange(hwnd syscall.Handle, dpi float64, suggested *win32Rect) {
	h.mu.Lock()
	changed := h.applyDPILocked(dpi)
	cols, rows, cellW, cellH := h.cols, h.rows, h.cellW, h.cellH
	h.mu.Unlock()

	if suggested != nil && cols > 0 && rows > 0 && cellW > 0 && cellH > 0 {
		rc := gridWindowRect(suggested.left, suggested.top, cols, rows, cellW, cellH, dpi)
		flags := uintptr(swpNoZOrder | swpNoActivate)
		if zoomed, _, _ := procIsZoomed.Call(uintptr(hwnd)); zoomed != 0 {
			// A maximized window keeps the monitor's size; Windows already
			// handed us that rectangle, and the grid follows it in WM_SIZE.
			rc = *suggested
		}
		_, _, _ = procSetWindowPos.Call(uintptr(hwnd), 0,
			win32IntArg(rc.left), win32IntArg(rc.top),
			win32IntArg(rc.right-rc.left), win32IntArg(rc.bottom-rc.top), flags)
	}
	if changed && FrameManager != nil {
		// When the grid is unchanged, WM_SIZE sends no resize event, so
		// nothing else would repaint the cells at the new size.
		FrameManager.HardRefresh()
	}
	_, _, _ = procInvalidateRect.Call(uintptr(hwnd), 0, 0)
}

// syncWindowDPI measures the window against the monitor it actually opened
// on. The font is loaded before the window exists, at the primary monitor's
// DPI; a window that Windows placed on another monitor gets no WM_DPICHANGED
// for that, so it is checked once here, while the window is still hidden.
func (h *Win32GuiHost) syncWindowDPI(hwnd syscall.Handle) {
	dpi := win32WindowDPI(hwnd)
	h.mu.Lock()
	changed := h.applyDPILocked(dpi)
	cols, rows, cellW, cellH := h.cols, h.rows, h.cellW, h.cellH
	h.mu.Unlock()
	if !changed {
		return
	}
	rc := gridWindowRect(0, 0, cols, rows, cellW, cellH, dpi)
	_, _, _ = procSetWindowPos.Call(uintptr(hwnd), 0, 0, 0,
		win32IntArg(rc.right-rc.left), win32IntArg(rc.bottom-rc.top),
		swpNoMove|swpNoZOrder|swpNoActivate)
}

// cellSize returns the current cell size under the lock: a DPI change or a
// font hot-swap can replace it while input is being translated.
func (h *Win32GuiHost) cellSize() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cellW, h.cellH
}

// win32Extent is the pixel extent of n cells of the given size, as the
// int32 a RECT holds, clamped rather than wrapped.
func win32Extent(n, cell int) int32 {
	v := n * cell
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v)
}

// win32IntArg passes a signed int32 (a window coordinate, which is negative
// on a monitor left of or above the primary one) as a Win32 int argument.
func win32IntArg(v int32) uintptr {
	// #nosec G115 -- deliberate: the callee reads the low 32 bits as a
	// signed int, so the two's-complement bit pattern is what must arrive.
	return uintptr(uint32(v))
}
