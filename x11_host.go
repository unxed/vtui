//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris

package vtui

import (
	"fmt"
	"image"
	"io"
	"os"
	"runtime"
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
	dirtyLines []bool

	translator keytrans.Translator
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
			xproto.EventMaskStructureNotify),
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
	_, _ = xproto.GetInputFocus(conn).Reply()

	info := keytrans.OSInfo{
		DisplayString: os.Getenv("DISPLAY"),
		XgbConn:       conn,
		WindowID:      uint32(host.wid),
	}
	host.translator = keytrans.NewX11Translator(info)
	if host.translator != nil {
		DebugLog("X11: Keytrans translator initialized with backend: %s", host.translator.Name())
	} else {
		DebugLog("X11: WARNING - Keytrans translator failed to initialize")
	}

	return host, nil
}

func (h *X11Host) Close() {
	if h.shmSeg != 0 {
		x11shmDetach(h.conn, h.shmSeg)
	}
	if h.translator != nil {
		h.translator.Close()
	}
	h.conn.Close()
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

func (h *X11Host) handleKeyEvent(detail xproto.Keycode, state uint16, isDown bool) {
	if h.translator != nil {
		wev := h.translator.TranslateX11(uint8(detail), state, isDown)
		event := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         wev.KeyDown,
			VirtualKeyCode:  wev.VirtualKeyCode,
			Char:            wev.Char,
			ControlKeyState: vtinput.ControlKeyState(wev.ControlKeyState),
			InputSource:     wev.InputSource,
		}
		if h.reader != nil {
			h.reader.NativeEventChan <- event
		}
	}
}

func (h *X11Host) handleButtonEvent(x, y int16, detail xproto.Button, state uint16, isDown bool) {
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

func (h *X11Host) translateModifiers(state uint16) vtinput.ControlKeyState {
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

func runInX11Window(cols, rows int, setupApp func()) error {
	if runtime.GOOS == "windows" && os.Getenv("DISPLAY") == "" {
		os.Setenv("DISPLAY", "127.0.0.1:0.0")
	}

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

	host, err := NewX11Host(cols, rows, cellW, cellH)
	if err != nil {
		return err
	}
	defer host.Close()

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = NewX11Renderer(host, face)

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
