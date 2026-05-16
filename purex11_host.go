//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd

package vtui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"sync"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/unxed/vtinput"
	"github.com/unxed/xkb-go"
)

type rawRequest []byte

func (r rawRequest) Bytes() []byte { return r }

type XkbState struct {
	DeviceID         byte
	Mods             byte
	BaseMods         byte
	LatchedMods      byte
	LockedMods       byte
	Group            byte
	LockedGroup      byte
	BaseGroup        int16
	LatchedGroup     int16
	CompatState      byte
	GrabMods         byte
	CompatGrabMods   byte
	LookupMods       byte
	CompatLookupMods byte
	PtrBtnState      uint16
}

func initXkbExtension(X *xgb.Conn, xkbOpcode byte) error {
	buf := make([]byte, 8)
	buf[0] = xkbOpcode
	buf[1] = 0 // XkbUseExtension
	xgb.Put16(buf[2:], 2)
	xgb.Put16(buf[4:], 1) // wantedMajor
	xgb.Put16(buf[6:], 0) // wantedMinor

	cookie := X.NewCookie(true, true)
	X.NewRequest(rawRequest(buf), cookie)
	_, err := cookie.Reply()
	return err
}

func queryXkbState(X *xgb.Conn, xkbOpcode byte) (*XkbState, error) {
	buf := make([]byte, 8)
	buf[0] = xkbOpcode
	buf[1] = 4 // XkbGetState
	xgb.Put16(buf[2:], 2)
	xgb.Put16(buf[4:], 0x0100) // deviceSpec = XkbUseCoreKbd

	cookie := X.NewCookie(true, true)
	X.NewRequest(rawRequest(buf), cookie)
	reply, err := cookie.Reply()
	if err != nil {
		return nil, err
	}
	if len(reply) < 25 {
		return nil, fmt.Errorf("XKB reply too short")
	}

	return &XkbState{
		DeviceID:         reply[1],
		Mods:             reply[8],
		BaseMods:         reply[9],
		LatchedMods:      reply[10],
		LockedMods:       reply[11],
		Group:            reply[12],
		LockedGroup:      reply[13],
		BaseGroup:        int16(xgb.Get16(reply[14:])),
		LatchedGroup:     int16(xgb.Get16(reply[16:])),
		CompatState:      reply[18],
		GrabMods:         reply[19],
		CompatGrabMods:   reply[20],
		LookupMods:       reply[21],
		CompatLookupMods: reply[22],
		PtrBtnState:      xgb.Get16(reply[24:]),
	}, nil
}

type PureX11Host struct {
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
	dirtyLines []bool

	xkbOpcode byte
	keymap    *xkb.Keymap
	xkbState  *xkb.State
}

func NewPureX11Host(cols, rows, cellW, cellH int) (*PureX11Host, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X11 via XGB: %v", err)
	}

	extCookie := xproto.QueryExtension(conn, uint16(len("XKEYBOARD")), "XKEYBOARD")
	extReply, err := extCookie.Reply()
	if err != nil || !extReply.Present {
		return nil, fmt.Errorf("XKEYBOARD extension is not supported by the server")
	}
	xkbOpcode := extReply.MajorOpcode

	if err := initXkbExtension(conn, xkbOpcode); err != nil {
		return nil, fmt.Errorf("failed to initialize XKEYBOARD: %v", err)
	}

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	rulesAtomCookie := xproto.InternAtom(conn, true, uint16(len("_XKB_RULES_NAMES")), "_XKB_RULES_NAMES")
	rulesAtomReply, err := rulesAtomCookie.Reply()
	if err != nil {
		return nil, fmt.Errorf("failed to get _XKB_RULES_NAMES atom: %v", err)
	}

	propCookie := xproto.GetProperty(conn, false, screen.Root, rulesAtomReply.Atom, xproto.AtomAny, 0, 1024)
	propReply, err := propCookie.Reply()
	if err != nil {
		return nil, fmt.Errorf("failed to read _XKB_RULES_NAMES property: %v", err)
	}

	var rmlvo xkb.RuleNames
	if propReply != nil && len(propReply.Value) > 0 {
		parts := bytes.Split(propReply.Value, []byte{0})
		if len(parts) > 0 { rmlvo.Rules = string(parts[0]) }
		if len(parts) > 1 { rmlvo.Model = string(parts[1]) }
		if len(parts) > 2 { rmlvo.Layout = string(parts[2]) }
		if len(parts) > 3 { rmlvo.Variant = string(parts[3]) }
		if len(parts) > 4 { rmlvo.Options = string(parts[4]) }
	}

	xkbCtx := xkb.NewContext(context.Background(), xkb.ContextNoFlags)
	keymap, err := xkbCtx.NewKeymapFromNames(&rmlvo)
	if err != nil {
		return nil, fmt.Errorf("failed to compile keymap: %v", err)
	}
	state := keymap.NewState()

	dpi := 96.0
	if screen.WidthInMillimeters > 0 {
		dpi = (float64(screen.WidthInPixels) * 25.4) / float64(screen.WidthInMillimeters)
	}
	scale := 1
	if dpi > 120 {
		scale = 2
	}

	host := &PureX11Host{
		conn:       conn,
		screen:     screen,
		cellW:      cellW,
		cellH:      cellH,
		scale:      scale,
		cols:       cols,
		rows:       rows,
		width:      uint16(cols * cellW),
		height:     uint16(rows * cellH),
		closeChan:  make(chan struct{}),
		dirtyLines: make([]bool, rows*cellH),
		xkbOpcode:  xkbOpcode,
		keymap:     keymap,
		xkbState:   state,
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

	// ВАЖНО: Порядок в []uint32 должен соответствовать возрастанию значений констант Cw*
	// CwBackPixel (2), CwEventMask (2048), CwColormap (8192)
	mask := uint32(xproto.CwBackPixel | xproto.CwEventMask | xproto.CwColormap)
	values := []uint32{
		screen.BlackPixel,
		uint32(xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease |
			xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease |
			xproto.EventMaskPointerMotion | xproto.EventMaskExposure |
			xproto.EventMaskStructureNotify),
		uint32(cmap),
	}

	xproto.CreateWindow(conn, depth, host.wid, screen.Root,
		0, 0, host.width, host.height, 0,
		xproto.WindowClassInputOutput, visualID,
		mask, values)

	title := AppName + " (PureX11)"
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

	stateAtom, _ := xproto.InternAtom(conn, false, 13, "_NET_WM_STATE").Reply()
	maxVertAtom, _ := xproto.InternAtom(conn, false, 28, "_NET_WM_STATE_MAXIMIZED_VERT").Reply()
	maxHorzAtom, _ := xproto.InternAtom(conn, false, 28, "_NET_WM_STATE_MAXIMIZED_HORZ").Reply()
	if stateAtom != nil && maxVertAtom != nil && maxHorzAtom != nil {
		data := make([]byte, 8)
		xgb.Put32(data, uint32(maxVertAtom.Atom))
		xgb.Put32(data[4:], uint32(maxHorzAtom.Atom))
		xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, stateAtom.Atom, xproto.AtomAtom, 32, 2, data)
	}

	xproto.MapWindow(conn, host.wid)

	// В XGB для синхронизации (flush + wait) используется любой запрос с ответом.
	// Это гарантирует, что сервер обработал CreateWindow и MapWindow.
	_, _ = xproto.GetInputFocus(conn).Reply()

	return host, nil
}

func (h *PureX11Host) Close() {
	if h.shmSeg != 0 {
		x11shmDetach(h.conn, h.shmSeg)
	}
	h.conn.Close()
	close(h.closeChan)
}

func (h *PureX11Host) RunEventLoop() {
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
				h.imgBuf = image.NewRGBA(image.Rect(0, 0, int(h.width), int(h.height)))
				if h.shmSeg == 0 {
					h.bgraBuf = make([]byte, len(h.imgBuf.Pix))
				}
				h.dirtyLines = make([]bool, int(ht))
				for i := range h.dirtyLines {
					h.dirtyLines[i] = true
				}
				h.mu.Unlock()
				if h.reader != nil {
					h.reader.NativeEventChan <- &vtinput.InputEvent{Type: vtinput.ResizeEventType}
				}
			}

		case xproto.KeyPressEvent:
			h.handleKeyEvent(e.Detail, e.State, true)
		case xproto.KeyReleaseEvent:
			h.handleKeyEvent(e.Detail, e.State, false)

		case xproto.ButtonPressEvent:
			h.handleButtonEvent(e.EventX, e.EventY, e.Detail, e.State, true)
		case xproto.ButtonReleaseEvent:
			h.handleButtonEvent(e.EventX, e.EventY, e.Detail, e.State, false)

		case xproto.MotionNotifyEvent:
			if h.reader != nil {
				h.reader.NativeEventChan <- &vtinput.InputEvent{
					Type:            vtinput.MouseEventType,
					MouseX:          uint16(int(e.EventX) / h.cellW),
					MouseY:          uint16(int(e.EventY) / h.cellH),
					MouseEventFlags: vtinput.MouseMoved,
				}
			}

		case xproto.ClientMessageEvent:
			if e.Data.Data32[0] == uint32(h.atomDelete) {
				FrameManager.EmitCommand(CmQuit, nil)
			}
		}
	}
}

func (h *PureX11Host) handleKeyEvent(detail xproto.Keycode, state uint16, isDown bool) {
	xkbState, err := queryXkbState(h.conn, h.xkbOpcode)
	if err == nil {
		h.xkbState.UpdateMask(
			xkb.ModMask(xkbState.BaseMods),
			xkb.ModMask(xkbState.LatchedMods),
			xkb.ModMask(xkbState.LockedMods),
			xkb.Group(xkbState.BaseGroup),
			xkb.Group(xkbState.LatchedGroup),
			xkb.Group(xkbState.LockedGroup),
		)
	}

	kc := xkb.Keycode(detail)
	sym := h.xkbState.KeyGetOneSym(kc)
	charStr := h.xkbState.KeyGetUTF8(kc)

	char := rune(0)
	if charStr != "" {
		for _, r := range charStr {
			char = r
			break
		}
	}

	vk := keysymToVK(uint32(sym))
	mods := h.translateModifiers(state)

	if h.reader != nil {
		h.reader.NativeEventChan <- &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: isDown, VirtualKeyCode: vk,
			Char: char, ControlKeyState: mods,
		}
	}
}

func (h *PureX11Host) handleButtonEvent(x, y int16, detail xproto.Button, state uint16, isDown bool) {
	event := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		MouseX:          uint16(int(x) / h.cellW),
		MouseY:          uint16(int(y) / h.cellH),
		KeyDown:         isDown,
		ControlKeyState: h.translateModifiers(state),
	}

	switch detail {
	case 1:
		event.ButtonState = vtinput.FromLeft1stButtonPressed
	case 2:
		event.ButtonState = vtinput.FromLeft2ndButtonPressed
	case 3:
		event.ButtonState = vtinput.RightmostButtonPressed
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
	if h.reader != nil {
		h.reader.NativeEventChan <- event
	}
}

func (h *PureX11Host) translateModifiers(state uint16) vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	if state&1 != 0 {
		mods |= vtinput.ShiftPressed
	}
	if state&4 != 0 {
		mods |= vtinput.LeftCtrlPressed
	}
	if state&8 != 0 {
		mods |= vtinput.LeftAltPressed
	}
	if state&2 != 0 {
		mods |= vtinput.CapsLockOn
	}
	if state&16 != 0 {
		mods |= vtinput.NumLockOn
	}
	return mods
}

func (h *PureX11Host) flushImage() int {
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

func runInPureX11Window(cols, rows int, setupApp func()) error {
	fontSize := 22.0
	tempConn, _ := xgb.NewConn()
	dpi := 96.0
	if tempConn != nil {
		setup := xproto.Setup(tempConn)
		screen := setup.DefaultScreen(tempConn)
		if screen.WidthInMillimeters > 0 {
			dpi = (float64(screen.WidthInPixels) * 25.4) / float64(screen.WidthInMillimeters)
		}
		tempConn.Close()
	}

	face, cellW, cellH := loadBestFont(fontSize, dpi)

	host, err := NewPureX11Host(cols, rows, cellW, cellH)
	if err != nil {
		return err
	}
	defer host.Close()

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = NewPureX11Renderer(host, face)

	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr)
	if reader.NativeEventChan == nil {
		reader.NativeEventChan = make(chan *vtinput.InputEvent, 1024)
	}
	host.reader = reader

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	go host.RunEventLoop()
	setupApp()
	FrameManager.Run(reader)

	return nil
}
