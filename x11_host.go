//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"reflect"
	"runtime"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/unxed/keytrans"
	"github.com/unxed/vtinput"
)

type X11Host struct {
	mu     sync.Mutex
	conn   *xgb.Conn
	wid    xproto.Window
	screen *xproto.ScreenInfo
	gc     xproto.Gcontext
	shmSeg uint32
	// shmMajor is MIT-SHM's major opcode, used to recognize an asynchronous
	// X error as coming from a ShmPutImage (see x11IsSHMError).
	shmMajor byte
	// shmVerifiedW/H is the image size the last checked ShmPutImage went
	// through at. The first SHM flush at any other size waits for the
	// server's verdict, so a server that rejects it makes the host fall back
	// to core PutImage instead of leaving a frozen window (f4 #1626).
	shmVerifiedW, shmVerifiedH int
	width                      uint16
	height                     uint16
	depth                      byte
	cellW                      int
	cellH                      int
	scale                      int
	imgBuf                     *image.RGBA
	bgraBuf                    []byte
	reader                     *vtinput.Reader
	cols, rows                 int
	closeChan                  chan struct{}
	atomDelete                 xproto.Atom
	dnd                        *x11Dnd
	dirtyLines                 []bool

	// renderer and scr let SetFont push a reloaded font's cell size to the
	// renderer's rasterizer and the screen's graphics layer, without either
	// of them holding a reference back into X11Host (vtui #136).
	renderer *X11Renderer
	scr      *ScreenBuf
	fontName string
	fontSize float64
	// dpi is the font DPI derived from the desktop DPI (see x11_dpi.go): set
	// at window creation and again whenever the desktop DPI changes. SetFont
	// reuses it so a font hot-swap keeps the current scaling.
	dpi float64
	// dpiWatch tracks the properties that carry the desktop DPI; nil when
	// the window was built without a connection (unit tests).
	dpiWatch *x11DPIWatch
	// gridStale says the cell size changed (DPI change, font hot-swap) since
	// cols x rows were last derived from the window size. The next
	// ConfigureNotify re-derives them even when the pixel size is unchanged:
	// a maximized or tiled window cannot follow the grid, so the grid has to
	// follow the window.
	gridStale bool

	translator     keytrans.Translator
	mouseBtn       uint32
	initialCols    int
	currentMods    vtinput.ControlKeyState
	lCtrl, rCtrl   bool
	lAlt, rAlt     bool
	lShift, rShift bool
}

func NewX11Host(cols, rows, cellW, cellH int) (*X11Host, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X11 via XGB: %v", err)
	}

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	dpi := 96.0
	if screen.WidthInMillimeters > 0 {
		dpi = (float64(screen.WidthInPixels) * 25.4) / float64(screen.WidthInMillimeters)
	}
	scale := 1
	if dpi > 120 {
		scale = 2
	}

	host := &X11Host{
		conn:        conn,
		screen:      screen,
		cellW:       cellW,
		cellH:       cellH,
		scale:       scale,
		cols:        cols,
		rows:        rows,
		width:       uint16(cols * cellW),
		height:      uint16(rows * cellH),
		closeChan:   make(chan struct{}),
		dirtyLines:  make([]bool, rows*cellH),
		initialCols: cols,
	}

	var visualID xproto.Visualid
	var depth byte = screen.RootDepth

	for _, d := range screen.AllowedDepths {
		if d.Depth == 24 || d.Depth == 32 {
			for _, v := range d.Visuals {
				if v.Class == xproto.VisualClassTrueColor {
					visualID = v.VisualId
					depth = d.Depth
					break
				}
			}
		}
		if visualID != 0 {
			break
		}
	}

	if visualID == 0 {
		visualID = screen.RootVisual
	}
	host.depth = depth

	host.wid, err = xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}

	cmap, err := xproto.NewColormapId(conn)
	if err != nil {
		return nil, err
	}
	xproto.CreateColormap(conn, xproto.ColormapAllocNone, cmap, screen.Root, visualID)

	// Values follow the order of the mask bits. BitGravity NorthWest keeps the
	// window contents across a resize: with the default Forget gravity the
	// server discards them and fills the window with the background pixel, so
	// every configure of a drag-resize is a black flash until the repaint that
	// follows the resize event lands (f4 #283).
	mask := uint32(xproto.CwBackPixel | xproto.CwBitGravity | xproto.CwEventMask | xproto.CwColormap)
	values := []uint32{
		screen.BlackPixel,
		uint32(xproto.GravityNorthWest),
		uint32(xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease |
			xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease |
			xproto.EventMaskPointerMotion | xproto.EventMaskExposure |
			xproto.EventMaskStructureNotify | xproto.EventMaskFocusChange),
		uint32(cmap),
	}

	xproto.CreateWindow(conn, depth, host.wid, screen.Root,
		0, 0, host.width, host.height, 0,
		xproto.WindowClassInputOutput, visualID,
		mask, values)

	title := AppName + " (X11)"
	xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, xproto.AtomWmName,
		xproto.AtomString, 8, uint32(len(title)), []byte(title))
	if wmClass, err := xproto.InternAtom(conn, false, 8, "WM_CLASS").Reply(); err == nil && wmClass != nil {
		data := x11WindowClassProperty()
		xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, wmClass.Atom,
			xproto.AtomString, 8, uint32(len(data)), data) // #nosec G115 -- WM_CLASS instance/class name, always a short string
	}

	host.gc, err = xproto.NewGcontextId(conn)
	if err == nil {
		xproto.CreateGC(conn, host.gc, xproto.Drawable(host.wid),
			xproto.GcForeground|xproto.GcBackground,
			[]uint32{screen.BlackPixel, screen.WhitePixel})
	}

	host.imgBuf = image.NewRGBA(image.Rect(0, 0, int(host.width), int(host.height)))

	forceNoShm := os.Getenv("VTUI_NO_SHM") != ""
	if !forceNoShm {
		setupX11SHM()
	}

	if shmReady && !forceNoShm {
		host.shmSeg = x11shmInit(conn, shmId)
		host.shmMajor = x11shmMajorOpcode(conn)
	}
	if host.shmSeg != 0 {
		host.bgraBuf = shmData
		DebugLog("X11: drawing through MIT-SHM (segment %d bytes)", len(shmData))
	} else {
		host.bgraBuf = make([]byte, len(host.imgBuf.Pix))
		DebugLog("X11: drawing through core PutImage (VTUI_NO_SHM=%t)", forceNoShm)
	}

	protocolsAtom, _ := xproto.InternAtom(conn, false, 12, "WM_PROTOCOLS").Reply()
	deleteAtom, _ := xproto.InternAtom(conn, false, 16, "WM_DELETE_WINDOW").Reply()
	if protocolsAtom != nil && deleteAtom != nil {
		host.atomDelete = deleteAtom.Atom
		data := make([]byte, 4)
		xgb.Put32(data, uint32(deleteAtom.Atom))
		xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, protocolsAtom.Atom,
			xproto.AtomAtom, 32, 1, data)
	}

	// Map the window at the geometry requested by the caller. In particular,
	// do not seed _NET_WM_STATE with the maximized flags here: that state makes
	// the window manager replace the requested dimensions before the first
	// configure event. Native maximize/restore transitions are handled by the
	// renderer when the window is resized.
	xproto.MapWindow(conn, host.wid)
	_, _ = xproto.GetInputFocus(conn).Reply()
	host.dnd = newX11Dnd(host)
	if host.dnd != nil {
		SetDragBackend(host)
	}

	go func() {
		translator := newX11Translator(conn, uint32(host.wid))
		host.mu.Lock()
		host.translator = translator
		host.mu.Unlock()
		if translator != nil {
			DebugLog("X11: Keytrans translator initialized asynchronously with backend: %s", translator.Name())
		} else {
			DebugLog("X11: WARNING - Keytrans translator failed to initialize asynchronously")
		}
	}()

	return host, nil
}

func x11WindowClassProperty() []byte {
	instance := AppName
	if instance == "" {
		instance = "vtui"
	}
	className := AppID
	if className == "" {
		className = instance
	}
	data := append([]byte(instance), 0)
	return append(data, append([]byte(className), 0)...)
}

func (h *X11Host) sendEvent(ev *vtinput.InputEvent) {
	h.mu.Lock()
	closed := h.reader == nil || h.reader.EventChan == nil
	h.mu.Unlock()
	if closed {
		return
	}

	defer func() {
		recover() // Безопасно гасим панику при гонке закрытия канала
	}()

	select {
	case h.reader.EventChan <- ev:
	case <-h.closeChan:
	}
}

func (h *X11Host) Close() {
	if h.dnd != nil {
		SetDragBackend(nil)
		SetDropTarget(nil)
	}
	if h.shmSeg != 0 {
		x11shmDetach(h.conn, h.shmSeg)
	}
	h.mu.Lock()
	tr := h.translator
	h.translator = nil
	h.mu.Unlock()
	if tr != nil {
		tr.Close()
	}
	if h.conn != nil {
		h.conn.Close()
	}
	close(h.closeChan)
}

func (h *X11Host) RunEventLoop() {
	for {
		ev, err := h.conn.WaitForEvent()
		if err != nil {
			h.handleXError(err)
			continue
		}
		if ev == nil {
			break
		}
		if h.handleDPIEvent(ev) {
			continue
		}

		switch e := ev.(type) {
		case xproto.ExposeEvent:
			h.mu.Lock()
			for i := range h.dirtyLines {
				h.dirtyLines[i] = true
			}
			h.mu.Unlock()
			h.flushImage()

		case xproto.ConfigureNotifyEvent:
			h.onConfigureNotify(e)

		case xproto.FocusInEvent:
			h.handleFocusEvent(true)
		case xproto.FocusOutEvent:
			h.handleFocusEvent(false)

		case xproto.MappingNotifyEvent:
			h.mu.Lock()
			if h.translator != nil {
				h.translator.Close()
			}
			h.translator = newX11Translator(h.conn, uint32(h.wid))
			if h.translator != nil {
				DebugLog("X11: Keyboard mapping reloaded after MappingNotify (Active backend: %s)", h.translator.Name())
			}
			h.mu.Unlock()

		case xproto.KeyPressEvent:
			h.handleKeyEvent(e.Detail, e.State, true)
		case xproto.KeyReleaseEvent:
			h.handleKeyEvent(e.Detail, e.State, false)

		case xproto.ButtonPressEvent:
			if h.dnd != nil && h.dnd.draggingOut() {
				continue
			}
			h.handleButtonEvent(e.EventX, e.EventY, e.Detail, e.State, true)
		case xproto.ButtonReleaseEvent:
			if h.dnd != nil && h.dnd.draggingOut() {
				h.dnd.srcRelease(e.Time)
				continue
			}
			h.handleButtonEvent(e.EventX, e.EventY, e.Detail, e.State, false)

		case xproto.MotionNotifyEvent:
			if h.dnd != nil && h.dnd.draggingOut() {
				h.dnd.srcMotion(int(e.RootX), int(e.RootY), e.Time)
				continue
			}
			h.mu.Lock()
			btn := h.mouseBtn
			h.mu.Unlock()
			cellW, cellH := h.cellSize()
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          pixelToCell(int(e.EventX), cellW),
				MouseY:          pixelToCell(int(e.EventY), cellH),
				MouseEventFlags: vtinput.MouseMoved,
				ButtonState:     btn,
				ControlKeyState: h.translateModifiers(e.State),
			})

		case xproto.ClientMessageEvent:
			if h.dnd != nil && h.dnd.handleClientMessage(&e) {
				continue
			}
			if e.Data.Data32[0] == uint32(h.atomDelete) {
				postQuitCommand()
			}

		case xproto.SelectionNotifyEvent:
			if h.dnd != nil {
				h.dnd.handleSelectionNotify(&e)
			}
		case xproto.SelectionRequestEvent:
			if h.dnd != nil {
				h.dnd.handleSelectionRequest(&e)
			}
		}
	}
}

func (h *X11Host) handleFocusEvent(focused bool) {
	if !focused {
		h.mu.Lock()
		h.resetKeyboardStateLocked()
		h.mu.Unlock()
	}
	h.sendEvent(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: focused})
}

func (h *X11Host) resetKeyboardStateLocked() {
	h.currentMods = 0
	h.lCtrl, h.rCtrl = false, false
	h.lAlt, h.rAlt = false, false
	h.lShift, h.rShift = false, false
}

func isX11ModifierVK(vk uint16) bool {
	switch vk {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU:
		return true
	default:
		return false
	}
}

func (h *X11Host) syncModifierStateLocked(state uint16, vk uint16, isDown bool) vtinput.ControlKeyState {
	assumeActive := !isX11ModifierVK(vk)
	keepShift := isDown && (vk == vtinput.VK_SHIFT || vk == vtinput.VK_LSHIFT || vk == vtinput.VK_RSHIFT)
	keepCtrl := isDown && (vk == vtinput.VK_CONTROL || vk == vtinput.VK_LCONTROL || vk == vtinput.VK_RCONTROL)
	keepAlt := isDown && (vk == vtinput.VK_MENU || vk == vtinput.VK_LMENU || vk == vtinput.VK_RMENU)

	if state&xproto.ModMaskShift == 0 && !keepShift {
		h.lShift, h.rShift = false, false
	} else if state&xproto.ModMaskShift != 0 && assumeActive && !h.lShift && !h.rShift {
		h.lShift = true
	}
	if state&xproto.ModMaskControl == 0 && !keepCtrl {
		h.lCtrl, h.rCtrl = false, false
	} else if state&xproto.ModMaskControl != 0 && assumeActive && !h.lCtrl && !h.rCtrl {
		h.lCtrl = true
	}
	if state&xproto.ModMask1 == 0 && !keepAlt {
		h.lAlt, h.rAlt = false, false
	} else if state&xproto.ModMask1 != 0 && assumeActive && !h.lAlt && !h.rAlt {
		h.lAlt = true
	}

	var mods vtinput.ControlKeyState
	if state&xproto.ModMaskShift != 0 {
		mods |= vtinput.ShiftPressed
	}
	if state&xproto.ModMaskControl != 0 {
		if h.rCtrl {
			mods |= vtinput.RightCtrlPressed
		} else {
			mods |= vtinput.LeftCtrlPressed
		}
	}
	if state&xproto.ModMask1 != 0 {
		if h.rAlt {
			mods |= vtinput.RightAltPressed
		} else {
			mods |= vtinput.LeftAltPressed
		}
	}
	if state&xproto.ModMaskLock != 0 {
		mods |= vtinput.CapsLockOn
	}
	if state&xproto.ModMask2 != 0 {
		mods |= vtinput.NumLockOn
	}
	h.currentMods = mods
	return mods
}

func (h *X11Host) handleKeyEvent(detail xproto.Keycode, state uint16, isDown bool) {
	h.mu.Lock()
	tr := h.translator
	h.mu.Unlock()
	if tr != nil {
		wev := tr.TranslateX11(uint8(detail), state, isDown)
		vk := wev.VirtualKeyCode

		h.mu.Lock()
		if isDown {
			if vk == vtinput.VK_LCONTROL {
				h.lCtrl = true
			}
			if vk == vtinput.VK_RCONTROL {
				h.rCtrl = true
			}
			if vk == vtinput.VK_LMENU {
				h.lAlt = true
			}
			if vk == vtinput.VK_RMENU {
				h.rAlt = true
			}
			if vk == vtinput.VK_LSHIFT {
				h.lShift = true
			}
			if vk == vtinput.VK_RSHIFT {
				h.rShift = true
			}
		} else {
			if vk == vtinput.VK_LCONTROL {
				h.lCtrl = false
			}
			if vk == vtinput.VK_RCONTROL {
				h.rCtrl = false
			}
			if vk == vtinput.VK_LMENU {
				h.lAlt = false
			}
			if vk == vtinput.VK_RMENU {
				h.rAlt = false
			}
			if vk == vtinput.VK_LSHIFT {
				h.lShift = false
			}
			if vk == vtinput.VK_RSHIFT {
				h.rShift = false
			}
		}

		sysMods := h.syncModifierStateLocked(state, vk, isDown)
		h.mu.Unlock()

		event := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         wev.KeyDown,
			VirtualKeyCode:  vk,
			Char:            wev.Char,
			ControlKeyState: sysMods,
			InputSource:     wev.InputSource,
		}
		h.sendEvent(event)
	}
}

func (h *X11Host) handleButtonEvent(x, y int16, detail xproto.Button, state uint16, isDown bool) {
	h.mu.Lock()
	var btn uint32
	if isDown {
		switch detail {
		case 1:
			btn = uint32(vtinput.FromLeft1stButtonPressed)
		case 2:
			btn = uint32(vtinput.FromLeft2ndButtonPressed)
		case 3:
			btn = uint32(vtinput.RightmostButtonPressed)
		}
		h.mouseBtn = btn
	} else {
		h.mouseBtn = 0
	}
	currMouseBtn := h.mouseBtn
	h.mu.Unlock()
	cellW, cellH := h.cellSize()

	event := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          pixelToCell(int(x), cellW),
		MouseY:          pixelToCell(int(y), cellH),
		KeyDown:         isDown,
		ButtonState:     currMouseBtn,
		ControlKeyState: h.translateModifiers(state),
	}

	switch detail {
	case 4:
		if isDown {
			event.WheelDirection = 1
		} else {
			return
		}
	case 5:
		if isDown {
			event.WheelDirection = -1
		} else {
			return
		}
	}
	h.sendEvent(event)
}

func (h *X11Host) translateModifiers(state uint16) vtinput.ControlKeyState {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.syncModifierStateLocked(state, 0, false)
}

func (h *X11Host) flushImage() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.flushImageLocked()
}

func (h *X11Host) flushImageLocked() int {
	b := h.imgBuf.Bounds()
	w, h2 := b.Dx(), b.Dy()
	// X11 image and window dimensions are CARD16: nothing larger can be
	// put on the window, so such a frame is not drawn at all.
	if w <= 0 || h2 <= 0 || w > math.MaxUint16 || h2 > math.MaxUint16 {
		return 0
	}
	w16, h16 := uint16(w), uint16(h2)
	h.ensureBGRABufLocked()

	pix := h.imgBuf.Pix
	lineStride := w * 4
	putCalls := 0

	maxReq := int(xproto.Setup(h.conn).MaximumRequestLength) * 4
	rowsPerReqLimit := (maxReq - 24) / lineStride
	if rowsPerReqLimit < 1 {
		rowsPerReqLimit = 1
	}

	for y := 0; y < h2 && y < len(h.dirtyLines); {
		if !h.dirtyLines[y] {
			y++
			continue
		}

		start := y
		for y < h2 && y < len(h.dirtyLines) && h.dirtyLines[y] && (y-start) < rowsPerReqLimit {
			h.dirtyLines[y] = false
			y++
		}
		end := y

		for sy := start; sy < end; sy++ {
			off := sy * lineStride
			if off+lineStride > len(h.bgraBuf) || off+lineStride > len(pix) {
				continue
			}
			srcRow, dstRow := pix[off:off+lineStride], h.bgraBuf[off:off+lineStride]
			for i := 0; i < lineStride; i += 4 {
				dstRow[i], dstRow[i+1], dstRow[i+2], dstRow[i+3] = srcRow[i+2], srcRow[i+1], srcRow[i], 255
			}
		}

		if h.shmSeg != 0 {
			checked := x11SHMNeedsCheck(w, h2, h.shmVerifiedW, h.shmVerifiedH)
			if err := x11shmPutImage(h.conn, h.wid, h.gc, w16, h16, start, end-1, h.depth, h.shmSeg, checked); err != nil {
				// The server refused the image at this size: stop using
				// shared memory and send the whole frame the core way.
				h.disableSHMLocked(fmt.Sprintf("ShmPutImage %dx%d failed: %v", w, h2, err))
				return putCalls + h.flushImageLocked()
			}
			if checked {
				h.shmVerifiedW, h.shmVerifiedH = w, h2
				DebugLog("X11: MIT-SHM PutImage verified at %dx%d", w, h2)
			}
		} else {
			xproto.PutImage(h.conn, xproto.ImageFormatZPixmap, xproto.Drawable(h.wid), h.gc,
				uint16(w), uint16(end-start), 0, int16(start), 0, h.depth, h.bgraBuf[start*lineStride:end*lineStride])
		}
		putCalls++
	}

	return putCalls
}

// x11SHMNeedsCheck reports whether a ShmPutImage of a w x h image must be
// checked: the first one at every new size is, so a size the server will
// not accept is noticed at once rather than as a window that stops updating.
func x11SHMNeedsCheck(w, h, verifiedW, verifiedH int) bool {
	return w != verifiedW || h != verifiedH
}

// x11SHMFits reports whether a w x h 32bpp image fits in a segment of
// segLen bytes; flushImage writes the image at offset 0.
func x11SHMFits(w, h, segLen int) bool {
	return w > 0 && h > 0 && w*h*4 <= segLen
}

// ensureBGRABufLocked keeps the BGRA staging buffer in step with imgBuf: the
// shared segment while MIT-SHM is in use and the image fits it, a private
// slice of the image's size otherwise. The caller holds h.mu.
func (h *X11Host) ensureBGRABufLocked() {
	if h.imgBuf == nil {
		return
	}
	b := h.imgBuf.Bounds()
	if h.shmSeg != 0 {
		if x11SHMFits(b.Dx(), b.Dy(), len(shmData)) {
			h.bgraBuf = shmData
			return
		}
		h.disableSHMLocked(fmt.Sprintf("%dx%d image does not fit the %d-byte segment", b.Dx(), b.Dy(), len(shmData)))
	}
	if len(h.bgraBuf) != len(h.imgBuf.Pix) {
		h.bgraBuf = make([]byte, len(h.imgBuf.Pix))
	}
}

// disableSHMLocked switches the host to core PutImage for the rest of its
// life -- what VTUI_NO_SHM=1 does from the start -- and marks the whole image
// dirty so the next flush repaints everything the failed requests lost. The
// caller holds h.mu.
func (h *X11Host) disableSHMLocked(reason string) {
	if h.shmSeg == 0 {
		return
	}
	DebugLog("X11: MIT-SHM disabled, falling back to core PutImage: %s", reason)
	if h.conn != nil {
		x11shmDetach(h.conn, h.shmSeg)
	}
	h.shmSeg = 0
	h.shmVerifiedW, h.shmVerifiedH = 0, 0
	if h.imgBuf != nil {
		h.bgraBuf = make([]byte, len(h.imgBuf.Pix))
	} else {
		h.bgraBuf = nil
	}
	for i := range h.dirtyLines {
		h.dirtyLines[i] = true
	}
}

// handleXError is where RunEventLoop's asynchronous X errors go instead of
// being dropped: each is logged, and one raised by a ShmPutImage turns the
// shared-memory path off and repaints the window through core PutImage.
func (h *X11Host) handleXError(err error) {
	DebugLog("X11: error from the X server: %v", err)
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.shmSeg == 0 || !x11IsSHMError(err, h.shmMajor) {
		return
	}
	h.disableSHMLocked(err.Error())
	if h.imgBuf != nil && h.conn != nil {
		h.flushImageLocked()
	}
}

// x11IsSHMError reports whether err is an X error raised by a MIT-SHM
// request. xgb gives every error its own struct type (core ones such as
// Value, Match and Access, and MIT-SHM's BadSeg) but they all carry the
// failing request's MajorOpcode field, which is read here by reflection.
func x11IsSHMError(err error, shmMajor byte) bool {
	if err == nil || shmMajor == 0 {
		return false
	}
	v := reflect.ValueOf(err)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return false
	}
	f := v.FieldByName("MajorOpcode")
	if !f.IsValid() || f.Kind() != reflect.Uint8 {
		return false
	}
	return f.Uint() == uint64(shmMajor)
}

// applyFontLocked reloads the font face at the host's dpi (the value
// computed once from Xft.dpi when the window was created; see
// runInX11Window) and pushes the new cell size to the renderer and the
// screen's graphics layer. The caller holds h.mu.
func (h *X11Host) applyFontLocked(fontName string, fontSize float64) {
	dpi := h.dpi
	if dpi <= 0 {
		dpi = 72.0
	}
	face, cellW, cellH := loadBestFont(fontName, fontSize, dpi)
	h.fontName = fontName
	h.fontSize = fontSize
	h.cellW = cellW
	h.cellH = cellH
	h.gridStale = true
	if h.renderer != nil {
		h.renderer.setFace(face)
	}
	if h.scr != nil {
		h.scr.Graphics().SetCellSize(cellW, cellH)
	}
}

// SetFont reloads the font used to draw the grid and asks the X server to
// resize the window to the new cell size, keeping the grid geometry (cols x
// rows) unchanged -- the same policy WaylandHost.SetFont uses for Wayland
// (vtui #136). It never fails: loadBestFont falls back to a built-in bitmap
// font when fontName cannot be found.
//
// The ConfigureWindow request alone does not repaint: the X server only
// sends a fresh ConfigureNotify (which drives the repaint through
// RunEventLoop's resize handling) when the pixel size actually changes, so a
// same-size font swap would otherwise leave the old glyphs on screen.
// HardRefresh is called unconditionally to cover that case.
func (h *X11Host) SetFont(fontName string, fontSize float64) {
	if fontSize <= 0 {
		fontSize = 18.0
	}
	h.mu.Lock()
	h.applyFontLocked(fontName, fontSize)
	h.mu.Unlock()

	h.resizeToGrid()
}

func runInX11Window(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	if runtime.GOOS == "windows" && os.Getenv("DISPLAY") == "" {
		os.Setenv("DISPLAY", "127.0.0.1:0.0")
	}

	if fontSize <= 0 {
		fontSize = 18.0
	}
	// The desktop DPI, read over a throwaway connection because the font has
	// to be measured before the window can be sized. The host's own
	// connection then keeps watching it (see x11_dpi.go).
	desktopDPI := x11DefaultDPI
	if tempConn, _ := xgb.NewConn(); tempConn != nil {
		root := xproto.Setup(tempConn).DefaultScreen(tempConn).Root
		desktopDPI = newX11DPIWatch(xgbDPIConn{tempConn}, root, tempConn.DefaultScreen).readDPI()
		tempConn.Close()
	}
	dpi, lineScale := x11FontDPI(desktopDPI)

	face, cellW, cellH := loadBestFont(fontName, fontSize, dpi)

	host, err := NewX11Host(cols, rows, cellW, cellH)
	if err != nil {
		return err
	}
	defer host.Close()
	host.fontName = fontName
	host.fontSize = fontSize
	host.dpi = dpi
	host.scale = lineScale
	host.dpiWatch = newX11DPIWatch(xgbDPIConn{host.conn}, host.screen.Root, host.conn.DefaultScreen)
	host.dpiWatch.subscribe()

	renderer := NewX11Renderer(host, face)
	host.renderer = renderer

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = renderer
	scr.Graphics().SetProtocol(GraphicsNative)
	scr.Graphics().SetCellSize(cellW, cellH)
	host.scr = scr

	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	// A window works from the OS clipboard helpers and the internal buffer;
	// UseWindowClipboard decides what becomes of the OSC 52 fallback, which is
	// worth keeping only where there is no helper to be had.
	UseWindowClipboard()

	go host.RunEventLoop()
	setupApp()
	// After setupApp: the application installs the debug log sink during
	// setup, so a backend announced before it is logged nowhere.
	SetActiveBackend("x11")
	FrameManager.Run(reader)

	return nil
}
