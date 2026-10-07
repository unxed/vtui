//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"encoding/binary"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/unxed/vtinput"
)

// Live DPI tracking for the X11 backend.
//
// X11 has no per-window scale: the desktop publishes one DPI for all
// clients, either through XSETTINGS ("Xft/DPI", what GNOME, Xfce, Cinnamon
// and MATE set, already multiplied by the window scaling factor) or through
// the "Xft.dpi" X resource on the root window (what KDE and `xrdb` set).
// Both live in window properties, so a change in display scale arrives as a
// PropertyNotify. The host listens for it and reloads the font, keeping the
// grid and resizing the window around it, as a font hot-swap does.

const (
	x11DefaultDPI = 96.0
	// xsettingsTypeInteger is the XSETTINGS type tag of a CARD32 value.
	xsettingsTypeInteger = 0
	xsettingsTypeString  = 1
	xsettingsTypeColor   = 2
)

// parseXftDPI returns the Xft.dpi value of an X resource database string
// (the RESOURCE_MANAGER property), or 0 when it has none.
func parseXftDPI(resources string) float64 {
	for _, line := range strings.Split(resources, "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != "Xft.dpi" {
			continue
		}
		if dpi, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil && dpi > 0 {
			return dpi
		}
	}
	return 0
}

// parseXSettingsDPI returns the Xft/DPI value of an _XSETTINGS_SETTINGS
// property, or 0 when it has none or the data is malformed. The format is
// the freedesktop XSETTINGS one: a byte-order byte, three pad bytes, a
// serial and a setting count, then per setting a type byte, a pad byte, a
// 16-bit name length, the name padded to 4, a 32-bit serial and the value.
// Xft/DPI is an integer holding the DPI times 1024.
func parseXSettingsDPI(data []byte) float64 {
	if len(data) < 12 {
		return 0
	}
	var order binary.ByteOrder = binary.LittleEndian
	if data[0] == 1 { // MSBFirst; LSBFirst is 0
		order = binary.BigEndian
	}
	count := order.Uint32(data[8:12])
	pos := 12
	pad4 := func(n int) int { return (n + 3) &^ 3 }
	for i := uint32(0); i < count; i++ {
		if pos+4 > len(data) {
			return 0
		}
		typ := data[pos]
		nameLen := int(order.Uint16(data[pos+2 : pos+4]))
		pos += 4
		if pos+pad4(nameLen)+4 > len(data) {
			return 0
		}
		name := string(data[pos : pos+nameLen])
		pos += pad4(nameLen) + 4 // name, then the last-change serial
		switch typ {
		case xsettingsTypeInteger:
			if pos+4 > len(data) {
				return 0
			}
			if name == "Xft/DPI" {
				// A signed value: -1 (0xFFFFFFFF) and anything above
				// MaxInt32 mean "not set", not a huge DPI.
				v := order.Uint32(data[pos : pos+4])
				if v == 0 || v > math.MaxInt32 {
					return 0
				}
				return float64(v) / 1024
			}
			pos += 4
		case xsettingsTypeString:
			if pos+4 > len(data) {
				return 0
			}
			pos += 4 + pad4(int(order.Uint32(data[pos:pos+4])))
		case xsettingsTypeColor:
			pos += 8
		default:
			return 0
		}
	}
	return 0
}

// x11FontDPI turns a desktop DPI into the font DPI loadBestFont takes and the
// integer scale used for line thickness. The 72 baseline matches the font
// size convention of the other backends (size in pixels at 96 DPI).
func x11FontDPI(dpi float64) (fontDPI float64, scale int) {
	if dpi <= 0 {
		dpi = x11DefaultDPI
	}
	factor := dpi / x11DefaultDPI
	scale = int(factor + 0.5)
	if scale < 1 {
		scale = 1
	}
	return 72.0 * factor, scale
}

// x11DPIConn is the part of the X protocol the DPI watch uses. The real one
// is an xgb connection; tests give a fake, since no display is at hand.
type x11DPIConn interface {
	internAtom(name string) xproto.Atom
	selectionOwner(selection xproto.Atom) xproto.Window
	// property returns a format-8 property's bytes, nil when it is absent.
	property(win xproto.Window, prop xproto.Atom) []byte
	selectEvents(win xproto.Window, mask uint32)
}

// xgbDPIConn is x11DPIConn over a live connection.
type xgbDPIConn struct{ conn *xgb.Conn }

func (c xgbDPIConn) internAtom(name string) xproto.Atom {
	n := len(name)
	if n > math.MaxUint16 {
		return 0
	}
	reply, err := xproto.InternAtom(c.conn, false, uint16(n), name).Reply()
	if err != nil || reply == nil {
		return 0
	}
	return reply.Atom
}

func (c xgbDPIConn) selectionOwner(selection xproto.Atom) xproto.Window {
	reply, err := xproto.GetSelectionOwner(c.conn, selection).Reply()
	if err != nil || reply == nil {
		return 0
	}
	return reply.Owner
}

func (c xgbDPIConn) property(win xproto.Window, prop xproto.Atom) []byte {
	reply, err := xproto.GetProperty(c.conn, false, win, prop, xproto.AtomAny, 0, 1<<20).Reply()
	if err != nil || reply == nil || reply.Format != 8 {
		return nil
	}
	return reply.Value
}

func (c xgbDPIConn) selectEvents(win xproto.Window, mask uint32) {
	xproto.ChangeWindowAttributes(c.conn, win, xproto.CwEventMask, []uint32{mask})
}

// x11DPIWatch is what the host needs to notice a DPI change: the atoms and
// windows whose properties carry the DPI.
type x11DPIWatch struct {
	conn            x11DPIConn
	root            xproto.Window
	resourceManager xproto.Atom
	manager         xproto.Atom
	xsettingsSel    xproto.Atom
	xsettingsProp   xproto.Atom
	// xsettingsOwner is the window of the running XSETTINGS manager, 0 when
	// there is none.
	xsettingsOwner xproto.Window
}

// x11DPIEventMask is what the watch selects on the root and on the XSETTINGS
// manager window: property changes, and the structure events that carry the
// MANAGER announcement of a new XSETTINGS manager.
const x11DPIEventMask = uint32(xproto.EventMaskPropertyChange | xproto.EventMaskStructureNotify)

// newX11DPIWatch interns the atoms and finds the XSETTINGS manager of the
// given screen.
func newX11DPIWatch(conn x11DPIConn, root xproto.Window, screenNum int) *x11DPIWatch {
	w := &x11DPIWatch{
		conn:            conn,
		root:            root,
		resourceManager: conn.internAtom("RESOURCE_MANAGER"),
		manager:         conn.internAtom("MANAGER"),
		xsettingsSel:    conn.internAtom(fmt.Sprintf("_XSETTINGS_S%d", screenNum)),
		xsettingsProp:   conn.internAtom("_XSETTINGS_SETTINGS"),
	}
	w.findXSettingsOwner()
	return w
}

// findXSettingsOwner looks up the XSETTINGS manager window and subscribes to
// changes of its properties.
func (w *x11DPIWatch) findXSettingsOwner() {
	w.xsettingsOwner = 0
	if w.xsettingsSel == 0 {
		return
	}
	owner := w.conn.selectionOwner(w.xsettingsSel)
	if owner == 0 {
		return
	}
	w.xsettingsOwner = owner
	w.conn.selectEvents(owner, x11DPIEventMask)
}

// subscribe asks for the root window property changes that carry Xft.dpi
// and for the MANAGER announcement of a new XSETTINGS manager.
func (w *x11DPIWatch) subscribe() {
	w.conn.selectEvents(w.root, x11DPIEventMask)
}

// readDPI returns the desktop DPI: XSETTINGS first, the Xft.dpi resource
// next, 96 when neither says.
func (w *x11DPIWatch) readDPI() float64 {
	if w.xsettingsOwner != 0 && w.xsettingsProp != 0 {
		if dpi := parseXSettingsDPI(w.conn.property(w.xsettingsOwner, w.xsettingsProp)); dpi > 0 {
			return dpi
		}
	}
	if w.resourceManager != 0 {
		if dpi := parseXftDPI(string(w.conn.property(w.root, w.resourceManager))); dpi > 0 {
			return dpi
		}
	}
	return x11DefaultDPI
}

// isDPIProperty reports whether a property change may have changed the DPI.
func (w *x11DPIWatch) isDPIProperty(win xproto.Window, atom xproto.Atom) bool {
	if win == w.root && atom == w.resourceManager && atom != 0 {
		return true
	}
	return win != 0 && win == w.xsettingsOwner && atom == w.xsettingsProp && atom != 0
}

// isNewXSettingsManager reports whether a client message on the root window
// announces a new XSETTINGS manager for this screen.
func (w *x11DPIWatch) isNewXSettingsManager(e *xproto.ClientMessageEvent) bool {
	return e.Window == w.root && e.Type == w.manager && w.manager != 0 &&
		e.Format == 32 && xproto.Atom(e.Data.Data32[1]) == w.xsettingsSel
}

// applyDPILocked switches the host to a desktop DPI: it reloads the font at
// the matching font DPI and hands the new cell size and line scale on. It
// returns false, and changes nothing, when the font DPI is the one the
// window already uses. The grid is left alone. The caller holds h.mu.
func (h *X11Host) applyDPILocked(dpi float64) bool {
	fontDPI, scale := x11FontDPI(dpi)
	if math.Abs(fontDPI-h.dpi) < 0.5 {
		return false
	}
	DebugLog("X11: DPI %.1f -> font DPI %.1f (was %.1f), scale %d", dpi, fontDPI, h.dpi, scale)
	h.dpi = fontDPI
	h.scale = scale
	h.applyFontLocked(h.fontName, h.fontSize)
	return true
}

// refreshDPI re-reads the desktop DPI and, if it changed, rescales the
// window. Runs on the event loop goroutine.
func (h *X11Host) refreshDPI() {
	if h.dpiWatch == nil {
		return
	}
	dpi := h.dpiWatch.readDPI()
	h.mu.Lock()
	changed := h.applyDPILocked(dpi)
	h.mu.Unlock()
	if changed {
		h.resizeToGrid()
	}
}

// handleDPIEvent handles the events the DPI watch subscribed to: a change of
// a property that carries the DPI, and the MANAGER announcement of a new
// XSETTINGS manager. It reports whether the event was one of them, so the
// event loop can skip its own handling. Runs on the event loop goroutine.
func (h *X11Host) handleDPIEvent(ev xgb.Event) bool {
	w := h.dpiWatch
	if w == nil {
		return false
	}
	switch e := ev.(type) {
	case xproto.PropertyNotifyEvent:
		if !w.isDPIProperty(e.Window, e.Atom) {
			return false
		}
		h.refreshDPI()
		return true
	case xproto.ClientMessageEvent:
		if !w.isNewXSettingsManager(&e) {
			return false
		}
		w.findXSettingsOwner()
		h.refreshDPI()
		return true
	}
	return false
}

// resizeToGrid asks the X server for a window that holds the current grid
// at the current cell size, and repaints: the ConfigureNotify that follows
// a size change keeps cols x rows, and a same-size change sends none.
func (h *X11Host) resizeToGrid() {
	h.mu.Lock()
	conn, wid := h.conn, h.wid
	cols, rows, cellW, cellH := h.cols, h.rows, h.cellW, h.cellH
	h.mu.Unlock()

	if conn != nil && cols > 0 && rows > 0 && cellW > 0 && cellH > 0 {
		// #nosec G115 -- cols/rows are the terminal's grid size and
		// cellW/cellH are font-metric pixel sizes: small non-negative values.
		width, height := uint32(cols*cellW), uint32(rows*cellH)
		xproto.ConfigureWindow(conn, wid, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{width, height})
	}
	if FrameManager != nil {
		FrameManager.HardRefresh()
	}
}

// cellSize returns the current cell size under the lock: a DPI change or a
// font hot-swap can replace it while input is being translated.
func (h *X11Host) cellSize() (int, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cellW, h.cellH
}

// handleConfigure applies a ConfigureNotify of the host's window: the grid
// follows the new pixel size. It reports whether a resize event is due.
//
// After the cell size changed (gridStale), the grid is re-derived even when
// the pixel size is the same. resizeToGrid asks for a window that keeps the
// grid, but a maximized, tiled or fullscreen window is refused that size; the
// window manager then answers with a ConfigureNotify of the size it keeps
// (ICCCM 4.1.5), and the grid must shrink or grow to fit it.
func (h *X11Host) handleConfigure(w, ht uint16) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if w == h.width && ht == h.height && !h.gridStale {
		return false
	}
	h.gridStale = false
	h.width, h.height = w, ht
	if h.cellW > 0 && h.cellH > 0 {
		h.cols, h.rows = int(w)/h.cellW, int(ht)/h.cellH
	}
	return true
}

// onConfigureNotify handles a ConfigureNotify from the event loop. The root
// window, watched for DPI changes, reports its own configures (a RandR
// screen resize, say); only the host's window resizes the grid.
func (h *X11Host) onConfigureNotify(e xproto.ConfigureNotifyEvent) {
	if e.Window != h.wid {
		return
	}
	if h.handleConfigure(e.Width, e.Height) {
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})
	}
}
