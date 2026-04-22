//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package vtui

import (
	"fmt"
	"image"
	"sync"
	"syscall"
	"unsafe"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/shm"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/unxed/vtinput"
)

var (
	shmId   int
	shmAddr uintptr
	shmData []byte
)

func init() {
	// Constants for Linux IPC
	const (
		ipcPrivate = 0
		ipcCreat   = 01000
		ipcRmid    = 0
	)

	// Allocate a 32MB segment (enough for 4K display)
	size := 3840 * 2160 * 4
	
	r1, _, err := syscall.Syscall(syscall.SYS_SHMGET, uintptr(ipcPrivate), uintptr(size), uintptr(ipcCreat|0600))
	if err != 0 {
		DebugLog("X11: shmget failed: %v", err)
		return
	}
	id := int(r1)

	r1, _, err = syscall.Syscall(syscall.SYS_SHMAT, uintptr(id), 0, 0)
	if err != 0 {
		syscall.Syscall(syscall.SYS_SHMCTL, uintptr(id), uintptr(ipcRmid), 0)
		DebugLog("X11: shmat failed: %v", err)
		return
	}
	addr := r1

	shmId = id
	shmAddr = addr
	shmData = unsafe.Slice((*byte)(unsafe.Pointer(shmAddr)), size)
	DebugLog("X11: Allocated shared memory segment (ID: %d)", shmId)
}

// X11Host encapsulates the connection to the X server and window management.
type X11Host struct {
	mu           sync.Mutex
	conn         *xgb.Conn
	wid          xproto.Window
	screen       *xproto.ScreenInfo
	gc           xproto.Gcontext
	pixmap       xproto.Pixmap
	shmSeg       shm.Seg // Shared memory segment
	width        uint16
	height       uint16
	cellW        int
	cellH        int
	scale        int // Scaling factor (1 for standard, 2 for HiDPI, etc.)
	imgBuf       *image.RGBA
	bgraBuf      []byte
	reader       *vtinput.Reader
	cols, rows   int
	closeChan    chan struct{}
	keyMap       []xproto.Keysym
	keysPerCode  byte
	minKeyCode   xproto.Keycode
	atomDelete   xproto.Atom
	dirtyLines   []bool
	lCtrl, rCtrl bool
	lAlt, rAlt   bool
	lShift, rShift bool
}

func NewX11Host(cols, rows, cellW, cellH int) (*X11Host, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X11: %v", err)
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

	host.wid, err = xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}

	xproto.CreateWindow(conn, screen.RootDepth, host.wid, screen.Root,
		0, 0, host.width, host.height, 0,
		xproto.WindowClassInputOutput, screen.RootVisual,
		xproto.CwBackPixel|xproto.CwEventMask,
		[]uint32{
			screen.BlackPixel,
			xproto.EventMaskExposure | xproto.EventMaskKeyPress | xproto.EventMaskKeyRelease |
				xproto.EventMaskButtonPress | xproto.EventMaskButtonRelease | xproto.EventMaskPointerMotion |
				xproto.EventMaskStructureNotify,
		})

	title := "f4 (X11)"
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
		if err := shm.Init(conn); err == nil {
			host.shmSeg, _ = shm.NewSegId(conn)
			if host.shmSeg != 0 {
				shm.Attach(conn, host.shmSeg, uint32(shmId), false)
			}
		}
	}

	host.minKeyCode = setup.MinKeycode
	kmReply, err := xproto.GetKeyboardMapping(conn, host.minKeyCode, byte(setup.MaxKeycode-host.minKeyCode+1)).Reply()
	if err == nil {
		host.keyMap = kmReply.Keysyms
		host.keysPerCode = kmReply.KeysymsPerKeycode
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

	xproto.MapWindow(conn, host.wid)
	return host, nil
}

func (h *X11Host) translateModifiers(state uint16, vk uint16, isDown bool) vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	if state&xproto.ModMaskControl != 0 { mods |= vtinput.LeftCtrlPressed }
	if state&xproto.ModMask1 != 0 { mods |= vtinput.LeftAltPressed }
	if state&xproto.ModMaskShift != 0 { mods |= vtinput.ShiftPressed }
	if state&xproto.ModMaskLock != 0 { mods |= vtinput.CapsLockOn }
	if state&xproto.ModMask2 != 0 { mods |= vtinput.NumLockOn }
	return mods
}

func (h *X11Host) getKeysym(detail xproto.Keycode, state uint16) xproto.Keysym {
	if h.keyMap == nil { return 0 }
	baseIdx := int(detail-h.minKeyCode) * int(h.keysPerCode)
	shift := state&xproto.ModMaskShift != 0
	group := (state >> 13) & 0x03
	col := int(group)*2
	if shift { col += 1 }
	if col >= int(h.keysPerCode) { col = col % 2 }
	sym := h.keyMap[baseIdx+col]
	if sym == 0 && col > 0 { sym = h.keyMap[baseIdx] }
	return sym
}

func (h *X11Host) Close() {
	if h.shmSeg != 0 { shm.Detach(h.conn, h.shmSeg) }
	h.conn.Close()
	close(h.closeChan)
}

func (h *X11Host) RunEventLoop() {
	for {
		ev, err := h.conn.WaitForEvent()
		if ev == nil && err == nil { return }
		switch e := ev.(type) {
		case xproto.ExposeEvent:
			h.mu.Lock()
			for i := range h.dirtyLines { h.dirtyLines[i] = true }
			h.mu.Unlock()
			if e.Count == 0 { h.flushImage() }
		case xproto.ConfigureNotifyEvent:
			if e.Width != h.width || e.Height != h.height {
				h.mu.Lock()
				h.width, h.height = e.Width, e.Height
				h.cols, h.rows = int(e.Width)/h.cellW, int(e.Height)/h.cellH
				h.imgBuf = image.NewRGBA(image.Rect(0, 0, int(h.width), int(h.height)))
				h.dirtyLines = make([]bool, int(e.Height))
				for i := range h.dirtyLines { h.dirtyLines[i] = true }
				h.mu.Unlock()
				if h.reader != nil {
					h.reader.NativeEventChan <- &vtinput.InputEvent{Type: vtinput.ResizeEventType}
				}
			}
		case xproto.KeyPressEvent, xproto.KeyReleaseEvent:
			var detail xproto.Keycode
			var state uint16
			isDown := false
			if kp, ok := e.(xproto.KeyPressEvent); ok {
				detail, state, isDown = kp.Detail, kp.State, true
			} else if kr, ok := e.(xproto.KeyReleaseEvent); ok {
				detail, state, isDown = kr.Detail, kr.State, false
			}
			keysym := h.getKeysym(detail, state)
			vk := keysymToVK(uint32(keysym))
			char := keysymToRune(uint32(keysym))
			if h.reader != nil {
				h.reader.NativeEventChan <- &vtinput.InputEvent{
					Type: vtinput.KeyEventType, KeyDown: isDown, VirtualKeyCode: vk,
					Char: char, ControlKeyState: h.translateModifiers(state, vk, isDown),
				}
			}
		case xproto.ButtonPressEvent, xproto.ButtonReleaseEvent:
			var bx, by int16
			var detail xproto.Button
			var state uint16
			isDown := false
			if bp, ok := e.(xproto.ButtonPressEvent); ok {
				bx, by, detail, state, isDown = bp.EventX, bp.EventY, bp.Detail, bp.State, true
			} else {
				br := e.(xproto.ButtonReleaseEvent)
				bx, by, detail, state, isDown = br.EventX, br.EventY, br.Detail, br.State, false
			}
			event := &vtinput.InputEvent{
				Type:            vtinput.MouseEventType, MouseX: uint16(int(bx) / h.cellW),
				MouseY:          uint16(int(by) / h.cellH), KeyDown: isDown,
				ControlKeyState: h.translateModifiers(state, 0, false),
			}
			switch detail {
			case 1: event.ButtonState = vtinput.FromLeft1stButtonPressed
			case 2: event.ButtonState = vtinput.FromLeft2ndButtonPressed
			case 3: event.ButtonState = vtinput.RightmostButtonPressed
			case 4: if isDown { event.WheelDirection = 1 } else { continue }
			case 5: if isDown { event.WheelDirection = -1 } else { continue }
			}
			if h.reader != nil { h.reader.NativeEventChan <- event }
		case xproto.MotionNotifyEvent:
			if h.reader != nil {
				h.reader.NativeEventChan <- &vtinput.InputEvent{
					Type:            vtinput.MouseEventType, MouseX: uint16(int(e.EventX) / h.cellW),
					MouseY:          uint16(int(e.EventY) / h.cellH), MouseEventFlags: vtinput.MouseMoved,
				}
			}
		case xproto.ClientMessageEvent:
			if e.Type == h.atomDelete { FrameManager.EmitCommand(CmQuit, nil) }
		}
	}
}

func (h *X11Host) flushImage() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.imgBuf.Bounds()
	w, h2 := b.Dx(), b.Dy()
	if w <= 0 || h2 <= 0 { return 0 }
	minY, maxY := -1, -1
	for y := 0; y < h2; y++ {
		if h.dirtyLines[y] {
			if minY == -1 { minY = y }
			maxY = y
		}
	}
	if minY == -1 { return 0 }
	for y := minY; y <= maxY; y++ { h.dirtyLines[y] = false }

	if h.shmSeg != 0 {
		stride := w * 4
		for y := minY; y <= maxY; y++ {
			srcOff, dstOff := y*stride, y*stride
			if dstOff+stride > len(h.bgraBuf) || srcOff+stride > len(h.imgBuf.Pix) { continue }
			srcRow, dstRow := h.imgBuf.Pix[srcOff:srcOff+stride], h.bgraBuf[dstOff:dstOff+stride]
			for i := 0; i < stride; i += 4 {
				dstRow[i], dstRow[i+1], dstRow[i+2], dstRow[i+3] = srcRow[i+2], srcRow[i+1], srcRow[i], 255
			}
		}
		// shm.PutImage: 16 arguments
		shm.PutImage(h.conn, xproto.Drawable(h.wid), h.gc,
			uint16(w), uint16(h2), // total_width, total_height
			0, 0, // src_x, src_y (0 because we use offset)
			uint16(w), uint16(maxY-minY+1), // src_width, src_height
			0, int16(minY), // dst_x, dst_y
			24, 2, 0, // depth, format (ZPixmap), send_event
			h.shmSeg, uint32(minY*stride))
		return 1
	}

	pix, lineStride := h.imgBuf.Pix, w*4
	maxReq := int(xproto.Setup(h.conn).MaximumRequestLength) * 4
	rowsPerReqLimit := (maxReq - 24) / lineStride
	putCalls := 0
	for y := minY; y <= maxY; {
		chunkEnd := y + rowsPerReqLimit
		if chunkEnd > maxY+1 { chunkEnd = maxY + 1 }
		xproto.PutImage(h.conn, xproto.ImageFormatZPixmap, xproto.Drawable(h.wid), h.gc,
			uint16(w), uint16(chunkEnd-y), 0, int16(y), 0, 24, pix[y*lineStride:chunkEnd*lineStride])
		putCalls++
		y = chunkEnd
	}
	return putCalls
}
