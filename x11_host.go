//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"fmt"
	"image"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/unxed/keytrans"
	"github.com/unxed/vtinput"
)

type X11Host struct {
	mu         sync.Mutex
	conn       *xgb.Conn
	wid        xproto.Window
	screen     *xproto.ScreenInfo
	gc         xproto.Gcontext
	shmSeg     uint32
	width      uint16
	height     uint16
	depth      byte
	cellW      int
	cellH      int
	scale      int
	imgBuf     *image.RGBA
	bgraBuf    []byte
	reader     *vtinput.Reader
	cols, rows int
	closeChan  chan struct{}
	atomDelete xproto.Atom
	dnd        *x11Dnd
	dirtyLines []bool

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

	mask := uint32(xproto.CwBackPixel | xproto.CwEventMask | xproto.CwColormap)
	values := []uint32{
		screen.BlackPixel,
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

	if shmId > 0 && !forceNoShm {
		host.bgraBuf = shmData
		host.shmSeg = x11shmInit(conn, shmId)
	} else {
		host.bgraBuf = make([]byte, len(host.imgBuf.Pix))
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
		info := keytrans.OSInfo{
			DisplayString: os.Getenv("DISPLAY"),
			XgbConn:       conn,
			WindowID:      uint32(host.wid),
		}
		translator := keytrans.NewX11Translator(info)
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
			continue
		}
		if ev == nil {
			break
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
			w, ht := e.Width, e.Height
			if w != h.width || ht != h.height {
				h.mu.Lock()
				h.width, h.height = w, ht
				h.cols, h.rows = int(w)/h.cellW, int(ht)/h.cellH
				h.mu.Unlock()
				h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})
			}

		case xproto.FocusInEvent:
			h.handleFocusEvent(true)
		case xproto.FocusOutEvent:
			h.handleFocusEvent(false)

		case xproto.MappingNotifyEvent:
			h.mu.Lock()
			if h.translator != nil {
				h.translator.Close()
			}
			info := keytrans.OSInfo{
				DisplayString: os.Getenv("DISPLAY"),
				XgbConn:       h.conn,
				WindowID:      uint32(h.wid),
			}
			h.translator = keytrans.NewX11Translator(info)
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
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          int16(int(e.EventX) / h.cellW),
				MouseY:          int16(int(e.EventY) / h.cellH),
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

	event := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          int16(int(x) / h.cellW),
		MouseY:          int16(int(y) / h.cellH),
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
	b := h.imgBuf.Bounds()
	w, h2 := b.Dx(), b.Dy()
	if w <= 0 || h2 <= 0 {
		return 0
	}

	pix := h.imgBuf.Pix
	lineStride := w * 4
	putCalls := 0

	maxReq := int(xproto.Setup(h.conn).MaximumRequestLength) * 4
	rowsPerReqLimit := (maxReq - 24) / lineStride

	for y := 0; y < h2; {
		if !h.dirtyLines[y] {
			y++
			continue
		}

		start := y
		for y < h2 && h.dirtyLines[y] && (y-start) < rowsPerReqLimit {
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
			x11shmPutImage(h.conn, h.wid, h.gc, uint16(w), uint16(h2), start, end-1, h.shmSeg)
		} else {
			xproto.PutImage(h.conn, xproto.ImageFormatZPixmap, xproto.Drawable(h.wid), h.gc,
				uint16(w), uint16(end-start), 0, int16(start), 0, h.depth, h.bgraBuf[start*lineStride:end*lineStride])
		}
		putCalls++
	}

	return putCalls
}

func runInX11Window(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	if runtime.GOOS == "windows" && os.Getenv("DISPLAY") == "" {
		os.Setenv("DISPLAY", "127.0.0.1:0.0")
	}

	if fontSize <= 0 {
		fontSize = 18.0
	}
	tempConn, _ := xgb.NewConn()
	xftDpi := 96.0
	if tempConn != nil {
		setup := xproto.Setup(tempConn)
		screen := setup.DefaultScreen(tempConn)

		// Attempt to read explicit DPI scaling from the X11 Resource Manager
		atomReply, err := xproto.InternAtom(tempConn, false, 16, "RESOURCE_MANAGER").Reply()
		if err == nil && atomReply != nil {
			propReply, err := xproto.GetProperty(tempConn, false, screen.Root, atomReply.Atom, xproto.AtomAny, 0, 1024*1024).Reply()
			if err == nil && propReply != nil && propReply.Format == 8 {
				val := string(propReply.Value)
				for _, line := range strings.Split(val, "\n") {
					if strings.HasPrefix(line, "Xft.dpi:") {
						parts := strings.Split(line, ":")
						if len(parts) == 2 {
							parsed, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
							if err == nil && parsed > 0 {
								xftDpi = parsed
								break
							}
						}
					}
				}
			}
		}
		tempConn.Close()
	}

	// gogpu treats fontSize as pixels (DPI=72). We want to match this visually.
	// Scale the 72 DPI baseline by the OS scale factor (Xft.dpi / 96.0).
	scaleFactor := xftDpi / 96.0
	dpi := 72.0 * scaleFactor

	face, cellW, cellH := loadBestFont(fontName, fontSize, dpi)

	host, err := NewX11Host(cols, rows, cellW, cellH)
	if err != nil {
		return err
	}
	defer host.Close()

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = NewX11Renderer(host, face)
	scr.Graphics().SetProtocol(GraphicsNative)
	scr.Graphics().SetCellSize(cellW, cellH)

	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	go host.RunEventLoop()
	setupApp()
	// After setupApp: the application installs the debug log sink during
	// setup, so a backend announced before it is logged nowhere.
	SetActiveBackend("x11")
	FrameManager.Run(reader)

	return nil
}
