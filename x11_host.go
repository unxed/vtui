//go:build linux || openbsd || netbsd || dragonfly || darwin

package vtui

import (
	"image"
	"unsafe"
	"sync"
	"fmt"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/ebitengine/purego"
	"github.com/unxed/vtinput"
)


// X11 Constants
const (
	KeyPress         = 2
	KeyRelease       = 3
	ButtonPress      = 4
	ButtonRelease    = 5
	MotionNotify     = 6
	Expose           = 12
	ConfigureNotify  = 22
	ClientMessage    = 33

	KeyPressMask        = 1 << 0
	KeyReleaseMask      = 1 << 1
	ButtonPressMask     = 1 << 2
	ButtonReleaseMask   = 1 << 3
	PointerMotionMask   = 1 << 6
	ExposureMask        = 1 << 15
	StructureNotifyMask = 1 << 17

	XIMPreeditNothing = 0x0010
	XIMStatusNothing  = 0x0400
)

type xEvent [192]byte

type xKeyEvent struct {
	Type       int32
	_          [4]byte
	Serial     uint64
	SendEvent  int32
	_          [4]byte
	Display    uintptr
	Window     uint64
	Root       uint64
	Subwindow  uint64
	Time       uint64
	X, Y       int32
	XRoot, YRoot int32
	State      uint32
	Keycode    uint32
	SameScreen int32
}

type xButtonEvent struct {
	Type       int32
	_          [4]byte
	Serial     uint64
	SendEvent  int32
	_          [4]byte
	Display    uintptr
	Window     uint64
	Root       uint64
	Subwindow  uint64
	Time       uint64
	X, Y       int32
	XRoot, YRoot int32
	State      uint32
	Button     uint32
	SameScreen int32
}

type xMotionEvent struct {
	Type       int32
	_          [4]byte
	Serial     uint64
	SendEvent  int32
	_          [4]byte
	Display    uintptr
	Window     uint64
	Root       uint64
	Subwindow  uint64
	Time       uint64
	X, Y       int32
	XRoot, YRoot int32
	State      uint32
	IsHint     byte
	_          [3]byte
	SameScreen int32
}

type xConfigureEvent struct {
	Type             int32
	_                [4]byte
	Serial           uint64
	SendEvent        int32
	_                [4]byte
	Display          uintptr
	Event            uint64
	Window           uint64
	X, Y             int32
	Width, Height    int32
	BorderWidth      int32
	Above            uint64
	OverrideRedirect int32
}

type xClientMessageEvent struct {
	Type        int32
	_           [4]byte
	Serial      uint64
	SendEvent   int32
	_           [4]byte
	Display     uintptr
	Window      uint64
	MessageType uint64
	Format      int32
	_           [4]byte
	Data        [40]byte
}

var (
	libX11 uintptr

	xOpenDisplay        func(string) uintptr
	xSelectInput        func(uintptr, uintptr, int64)
	xNextEvent          func(uintptr, *xEvent)
	xFilterEvent        func(*xEvent, uintptr) bool
	xOpenIM             func(uintptr, uintptr, uintptr, uintptr) uintptr
	xSetLocaleModifiers func(string) uintptr

	xCreateICPtr         uintptr
	xutf8LookupStringPtr uintptr
	setlocale            func(int, string) uintptr
)

func initNative() error {
	// Список возможных имен для libX11 на разных ОС
	xlibNames := []string{
		"libX11.so.6",      // Linux
		"libX11.so",        // BSDs
		"libX11.6.dylib",   // macOS (XQuartz)
		"/usr/local/lib/libX11.so",
		"/opt/X11/lib/libX11.6.dylib",
	}

	var lib uintptr
	var err error
	for _, name := range xlibNames {
		lib, err = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("could not find X11 library: %w", err)
	}
	libX11 = lib

	purego.RegisterLibFunc(&xOpenDisplay, lib, "XOpenDisplay")
	purego.RegisterLibFunc(&xSelectInput, lib, "XSelectInput")
	purego.RegisterLibFunc(&xNextEvent, lib, "XNextEvent")
	purego.RegisterLibFunc(&xFilterEvent, lib, "XFilterEvent")
	purego.RegisterLibFunc(&xOpenIM, lib, "XOpenIM")
	purego.RegisterLibFunc(&xSetLocaleModifiers, lib, "XSetLocaleModifiers")

	xCreateICPtr, _ = purego.Dlsym(lib, "XCreateIC")
	xutf8LookupStringPtr, _ = purego.Dlsym(lib, "Xutf8LookupString")

	// Ищем стандартную библиотеку C (для setlocale)
	libcNames := []string{
		"libc.so.6",        // Linux
		"libc.so.7",        // FreeBSD
		"libc.so",          // Other BSDs
		"libSystem.B.dylib", // macOS
	}

	var clib uintptr
	for _, name := range libcNames {
		clib, _ = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if clib != 0 {
			break
		}
	}

	if clib != 0 {
		purego.RegisterLibFunc(&setlocale, clib, "setlocale")
	}

	return nil
}

// X11Host encapsulates the connection to the X server and window management.
type X11Host struct {
	mu         sync.Mutex
	conn       *xgb.Conn
	dpy        uintptr
	ic         uintptr
	wid        xproto.Window
	screen     *xproto.ScreenInfo
	gc         xproto.Gcontext
	pixmap     xproto.Pixmap
	shmSeg     uint32 // Shared memory segment (shm.Seg)
	width      uint16
	height     uint16
	cellW      int
	cellH      int
	scale      int // Scaling factor (1 for standard, 2 for HiDPI, etc.)
	imgBuf     *image.RGBA
	bgraBuf    []byte
	reader     *vtinput.Reader
	cols, rows int
	closeChan  chan struct{}
	atomDelete xproto.Atom
	dirtyLines []bool
}

func NewX11Host(cols, rows, cellW, cellH int) (*X11Host, error) {
	if err := initNative(); err != nil {
		return nil, err
	}

	if setlocale != nil {
		setlocale(6, "") // LC_ALL
	}
	xSetLocaleModifiers("")

	dpy := xOpenDisplay("")
	if dpy == 0 {
		return nil, fmt.Errorf("XOpenDisplay failed")
	}

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
		dpy:        dpy,
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

	host.wid, err = xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}

	// CwEventMask MUST NOT be set here, because Xlib will select events!
	xproto.CreateWindow(conn, screen.RootDepth, host.wid, screen.Root,
		0, 0, host.width, host.height, 0,
		xproto.WindowClassInputOutput, screen.RootVisual,
		xproto.CwBackPixel,
		[]uint32{
			screen.BlackPixel,
		})

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
	if shmId > 0 {
		host.bgraBuf = shmData
	} else {
		host.bgraBuf = host.imgBuf.Pix
	}

	if shmId > 0 {
		host.shmSeg = x11shmInit(conn, shmId)
	}

	// Set up Xlib input
	xSelectInput(dpy, uintptr(host.wid), KeyPressMask|KeyReleaseMask|ButtonPressMask|ButtonReleaseMask|PointerMotionMask|ExposureMask|StructureNotifyMask)

	im := xOpenIM(dpy, 0, 0, 0)
	nInputStyle, nClientWindow := []byte("inputStyle\x00"), []byte("clientWindow\x00")

	ic, _, _ := purego.SyscallN(xCreateICPtr, im,
		uintptr(unsafe.Pointer(&nInputStyle[0])), uintptr(XIMPreeditNothing|XIMStatusNothing),
		uintptr(unsafe.Pointer(&nClientWindow[0])), uintptr(host.wid), 0)

	host.ic = ic

	protocolsAtom, _ := xproto.InternAtom(conn, false, 12, "WM_PROTOCOLS").Reply()
	deleteAtom, _ := xproto.InternAtom(conn, false, 16, "WM_DELETE_WINDOW").Reply()
	if protocolsAtom != nil && deleteAtom != nil {
		host.atomDelete = deleteAtom.Atom
		data := make([]byte, 4)
		xgb.Put32(data, uint32(deleteAtom.Atom))
		xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, protocolsAtom.Atom,
			xproto.AtomAtom, 32, 1, data)
	}

	// Request maximization via EWMH
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
	return host, nil
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

func (h *X11Host) Close() {
	if h.shmSeg != 0 {
		x11shmDetach(h.conn, h.shmSeg)
	}
	h.conn.Close()
	close(h.closeChan)
}

func (h *X11Host) RunEventLoop() {
	for {
		var ev xEvent
		xNextEvent(h.dpy, &ev)

		if xFilterEvent(&ev, 0) {
			continue
		}

		evType := *(*int32)(unsafe.Pointer(&ev[0]))

		switch evType {
		case Expose:
			h.mu.Lock()
			for i := range h.dirtyLines {
				h.dirtyLines[i] = true
			}
			h.mu.Unlock()
			h.flushImage()

		case ConfigureNotify:
			cev := (*xConfigureEvent)(unsafe.Pointer(&ev[0]))
			w, ht := uint16(cev.Width), uint16(cev.Height)
			if w != h.width || ht != h.height {
				h.mu.Lock()
				h.width, h.height = w, ht
				h.cols, h.rows = int(w)/h.cellW, int(ht)/h.cellH
				h.imgBuf = image.NewRGBA(image.Rect(0, 0, int(h.width), int(h.height)))
				h.dirtyLines = make([]bool, int(ht))
				for i := range h.dirtyLines {
					h.dirtyLines[i] = true
				}
				h.mu.Unlock()
				if h.reader != nil {
					h.reader.NativeEventChan <- &vtinput.InputEvent{Type: vtinput.ResizeEventType}
				}
			}

		case KeyPress, KeyRelease:
			kev := (*xKeyEvent)(unsafe.Pointer(&ev[0]))
			isDown := evType == KeyPress

			buf := make([]byte, 64)
			var keysym uint32
			var status int
			n, _, _ := purego.SyscallN(xutf8LookupStringPtr, h.ic, uintptr(unsafe.Pointer(&ev)),
				uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)),
				uintptr(unsafe.Pointer(&keysym)), uintptr(unsafe.Pointer(&status)))

			vk := keysymToVK(keysym)
			char := rune(0)
			if n > 0 {
				text := string(buf[:n])
				for _, r := range text {
					char = r
					break
				}
			}

			if h.reader != nil {
				mods := h.translateModifiers(uint16(kev.State))
				h.reader.NativeEventChan <- &vtinput.InputEvent{
					Type: vtinput.KeyEventType, KeyDown: isDown, VirtualKeyCode: vk,
					Char: char, ControlKeyState: mods,
				}
			}

		case ButtonPress, ButtonRelease:
			bev := (*xButtonEvent)(unsafe.Pointer(&ev[0]))
			isDown := evType == ButtonPress

			event := &vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          uint16(int(bev.X) / h.cellW),
				MouseY:          uint16(int(bev.Y) / h.cellH),
				KeyDown:         isDown,
				ControlKeyState: h.translateModifiers(uint16(bev.State)),
			}

			switch bev.Button {
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
					continue
				}
			case 5:
				if isDown {
					event.WheelDirection = -1
				} else {
					continue
				}
			}
			if h.reader != nil {
				h.reader.NativeEventChan <- event
			}

		case MotionNotify:
			mev := (*xMotionEvent)(unsafe.Pointer(&ev[0]))
			if h.reader != nil {
				h.reader.NativeEventChan <- &vtinput.InputEvent{
					Type:            vtinput.MouseEventType,
					MouseX:          uint16(int(mev.X) / h.cellW),
					MouseY:          uint16(int(mev.Y) / h.cellH),
					MouseEventFlags: vtinput.MouseMoved,
				}
			}

		case ClientMessage:
			cev := (*xClientMessageEvent)(unsafe.Pointer(&ev[0]))
			var atomVal xproto.Atom
			if unsafe.Sizeof(uintptr(0)) == 8 {
				atomVal = xproto.Atom(*(*uint64)(unsafe.Pointer(&cev.Data[0])))
			} else {
				atomVal = xproto.Atom(*(*uint32)(unsafe.Pointer(&cev.Data[0])))
			}
			if atomVal == h.atomDelete {
				FrameManager.EmitCommand(CmQuit, nil)
			}
		}
	}
}

func (h *X11Host) flushImage() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.imgBuf.Bounds()
	w, h2 := b.Dx(), b.Dy()
	if w <= 0 || h2 <= 0 {
		return 0
	}
	minY, maxY := -1, -1
	for y := 0; y < h2; y++ {
		if h.dirtyLines[y] {
			if minY == -1 {
				minY = y
			}
			maxY = y
		}
	}
	if minY == -1 {
		return 0
	}
	for y := minY; y <= maxY; y++ {
		h.dirtyLines[y] = false
	}

	if h.shmSeg != 0 {
		stride := w * 4
		for y := minY; y <= maxY; y++ {
			srcOff, dstOff := y*stride, y*stride
			if dstOff+stride > len(h.bgraBuf) || srcOff+stride > len(h.imgBuf.Pix) {
				continue
			}
			srcRow, dstRow := h.imgBuf.Pix[srcOff:srcOff+stride], h.bgraBuf[dstOff:dstOff+stride]
			for i := 0; i < stride; i += 4 {
				dstRow[i], dstRow[i+1], dstRow[i+2], dstRow[i+3] = srcRow[i+2], srcRow[i+1], srcRow[i], 255
			}
		}
		x11shmPutImage(h.conn, h.wid, h.gc, uint16(w), uint16(h2), minY, maxY, h.shmSeg)
		return 1
	}

	pix, lineStride := h.imgBuf.Pix, w*4
	maxReq := int(xproto.Setup(h.conn).MaximumRequestLength) * 4
	rowsPerReqLimit := (maxReq - 24) / lineStride
	putCalls := 0
	for y := minY; y <= maxY; {
		chunkEnd := y + rowsPerReqLimit
		if chunkEnd > maxY+1 {
			chunkEnd = maxY + 1
		}
		xproto.PutImage(h.conn, xproto.ImageFormatZPixmap, xproto.Drawable(h.wid), h.gc,
			uint16(w), uint16(chunkEnd-y), 0, int16(y), 0, 24, pix[y*lineStride:chunkEnd*lineStride])
		putCalls++
		y = chunkEnd
	}
	return putCalls
}
