//go:build linux || freebsd || openbsd || netbsd || dragonfly

package vtui

import (
	"fmt"
	"image"
	"sync"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/unxed/vtinput"
)

// X11Host encapsulates the connection to the X server and window management.
type X11Host struct {
	mu           sync.Mutex
	conn         *xgb.Conn
	wid          xproto.Window
	screen       *xproto.ScreenInfo
	gc           xproto.Gcontext
	pixmap       xproto.Pixmap
	width        uint16
	height       uint16
	cellW        int
	cellH        int
	scale        int // Scaling factor (1 for standard, 2 for HiDPI, etc.)
	imgBuf       *image.RGBA
	bgraBuf      []byte
	reader       *vtinput.Reader
	closeChan    chan struct{}
	keyMap       []xproto.Keysym
	keysPerCode  byte
	minKeyCode   xproto.Keycode
	atomDelete   xproto.Atom
	dirtyLines   []bool
}

func NewX11Host(cols, rows, cellW, cellH int) (*X11Host, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to X11: %v", err)
	}

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	// Calculate DPI and Scaling Factor
	// Standard DPI is 96.
	dpi := 96.0
	if screen.WidthInMillimeters > 0 {
		dpi = (float64(screen.WidthInPixels) * 25.4) / float64(screen.WidthInMillimeters)
	}

	scale := 1
	if dpi > 120 {
		scale = 2
	}
	if dpi > 210 {
		scale = 3
	}

	host := &X11Host{
		conn:      conn,
		screen:    screen,
		cellW:     cellW,
		cellH:      cellH,
		scale:      scale,
		width:      uint16(cols * cellW),
		height:     uint16(rows * cellH),
		closeChan:  make(chan struct{}),
		dirtyLines: make([]bool, rows*cellH),
	}

	// Create Window
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

	// Set Window Title
	title := "vtui X11 Application"
	xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, xproto.AtomWmName,
		xproto.AtomString, 8, uint32(len(title)), []byte(title))

	// Create GC
	host.gc, err = xproto.NewGcontextId(conn)
	if err == nil {
		xproto.CreateGC(conn, host.gc, xproto.Drawable(host.wid),
			xproto.GcForeground|xproto.GcBackground,
			[]uint32{screen.BlackPixel, screen.WhitePixel})
	}

	// Create backing image buffer
	host.imgBuf = image.NewRGBA(image.Rect(0, 0, int(host.width), int(host.height)))

	xproto.MapWindow(conn, host.wid)

	// Fetch keyboard mapping
	host.minKeyCode = setup.MinKeycode
	maxKeyCode := setup.MaxKeycode
	mapLen := byte(maxKeyCode - host.minKeyCode + 1)
	kmReply, err := xproto.GetKeyboardMapping(conn, host.minKeyCode, mapLen).Reply()
	if err == nil {
		host.keyMap = kmReply.Keysyms
		host.keysPerCode = kmReply.KeysymsPerKeycode
	}

	xproto.MapWindow(conn, host.wid)

	// Intern WM_DELETE_WINDOW atom
	protocolsAtom, _ := xproto.InternAtom(conn, false, 12, "WM_PROTOCOLS").Reply()
	deleteAtom, _ := xproto.InternAtom(conn, false, 16, "WM_DELETE_WINDOW").Reply()
	if protocolsAtom != nil && deleteAtom != nil {
		host.atomDelete = deleteAtom.Atom
		atomData := make([]byte, 4)
		xgb.Put32(atomData, uint32(deleteAtom.Atom))
		xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, protocolsAtom.Atom,
			xproto.AtomAtom, 32, 1, atomData)
	}

	xproto.MapWindow(conn, host.wid)

	return host, nil
}

func (h *X11Host) translateModifiers(state uint16) vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	if state&xproto.ModMaskShift != 0 {
		mods |= vtinput.ShiftPressed
	}
	if state&xproto.ModMaskControl != 0 {
		mods |= vtinput.LeftCtrlPressed
	}
	if state&xproto.ModMask1 != 0 { // Usually Alt
		mods |= vtinput.LeftAltPressed
	}
	if state&xproto.ModMask2 != 0 { // Usually NumLock
		mods |= vtinput.NumLockOn
	}
	return mods
}

func (h *X11Host) getKeysym(detail xproto.Keycode) xproto.Keysym {
	if h.keyMap == nil {
		return 0
	}
	idx := int(detail-h.minKeyCode) * int(h.keysPerCode)
	if idx < 0 || idx >= len(h.keyMap) {
		return 0
	}
	return h.keyMap[idx]
}

func (h *X11Host) Close() {
	h.conn.Close()
	close(h.closeChan)
}

// RunEventLoop starts the X11 event loop. It translates X11 events to vtinput events.
func (h *X11Host) RunEventLoop() {
	for {
		ev, err := h.conn.WaitForEvent()
		if ev == nil && err == nil {
			return
		}

		switch e := ev.(type) {
		case xproto.ExposeEvent:
			// X11 says our window needs repainting (e.g. just appeared).
			// We must mark all lines as dirty to force a full refresh.
			h.mu.Lock()
			for i := range h.dirtyLines {
				h.dirtyLines[i] = true
			}
			h.mu.Unlock()
			if e.Count == 0 {
				h.flushImage()
			}
		case xproto.ConfigureNotifyEvent:
			if e.Width != h.width || e.Height != h.height {
				h.mu.Lock()
				h.width = e.Width
				h.height = e.Height
				h.imgBuf = image.NewRGBA(image.Rect(0, 0, int(h.width), int(h.height)))
				h.dirtyLines = make([]bool, int(e.Height))
				for i := range h.dirtyLines {
					h.dirtyLines[i] = true
				}
				h.mu.Unlock()

				cols := int(e.Width) / h.cellW
				rows := int(e.Height) / h.cellH

				if h.reader != nil && h.reader.NativeEventChan != nil {
					h.reader.NativeEventChan <- &vtinput.InputEvent{
						Type:        vtinput.ResizeEventType,
						InputSource: "x11",
					}
				}
				if FrameManager != nil && FrameManager.scr != nil {
					FrameManager.scr.AllocBuf(cols, rows)
					for _, s := range FrameManager.Screens {
						for _, f := range s.Frames {
							f.ResizeConsole(cols, rows)
						}
					}
					FrameManager.Redraw()
				}
			}

		case xproto.KeyPressEvent, xproto.KeyReleaseEvent:
			if h.reader == nil || h.reader.NativeEventChan == nil {
				continue
			}
			var detail xproto.Keycode
			var state uint16
			isDown := false
			if kp, ok := e.(xproto.KeyPressEvent); ok {
				detail, state, isDown = kp.Detail, kp.State, true
			} else if kr, ok := e.(xproto.KeyReleaseEvent); ok {
				detail, state, isDown = kr.Detail, kr.State, false
			}

			keysym := h.getKeysym(detail)
			vk := keysymToVK(uint32(keysym))
			char := rune(0)
			if keysym < 0x80 && keysym >= 0x20 {
				char = rune(keysym)
			} else if keysym >= 0xffb0 && keysym <= 0xffb9 {
				// Numpad digits 0-9
				char = rune('0' + (keysym - 0xffb0))
			} else {
				// Special characters from Numpad
				switch keysym {
				case 0xffaa: char = '*'
				case 0xffab: char = '+'
				case 0xffad: char = '-'
				case 0xffae: char = '.'
				case 0xffaf: char = '/'
				case 0xff8d: char = '\r'
				}
			}

			h.reader.NativeEventChan <- &vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         isDown,
				VirtualKeyCode:  vk,
				Char:            char,
				ControlKeyState: h.translateModifiers(state),
				InputSource:     "x11",
			}

		case xproto.ButtonPressEvent, xproto.ButtonReleaseEvent:
			if h.reader == nil || h.reader.NativeEventChan == nil {
				continue
			}
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
				Type:            vtinput.MouseEventType,
				MouseX:          uint16(int(bx) / h.cellW),
				MouseY:          uint16(int(by) / h.cellH),
				KeyDown:         isDown,
				ControlKeyState: h.translateModifiers(state),
				InputSource:     "x11",
			}

			switch detail {
			case 1:
				event.ButtonState = vtinput.FromLeft1stButtonPressed
			case 2:
				event.ButtonState = vtinput.FromLeft2ndButtonPressed
			case 3:
				event.ButtonState = vtinput.RightmostButtonPressed
			case 4: // Wheel Up
				if isDown {
					event.WheelDirection = 1
				} else {
					continue
				}
			case 5: // Wheel Down
				if isDown {
					event.WheelDirection = -1
				} else {
					continue
				}
			}
			h.reader.NativeEventChan <- event

		case xproto.MotionNotifyEvent:
			if h.reader == nil || h.reader.NativeEventChan == nil {
				continue
			}
			h.reader.NativeEventChan <- &vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          uint16(int(e.EventX) / h.cellW),
				MouseY:          uint16(int(e.EventY) / h.cellH),
				MouseEventFlags: vtinput.MouseMoved,
				ControlKeyState: h.translateModifiers(e.State),
				InputSource:     "x11",
			}

		case xproto.ClientMessageEvent:
			// Handle WM_DELETE_WINDOW to quit gracefully
			if e.Type == h.atomDelete || (len(e.Data.Data32) > 0 && xproto.Atom(e.Data.Data32[0]) == h.atomDelete) {
				if FrameManager != nil {
					FrameManager.EmitCommand(CmQuit, nil)
				}
			}
		}
	}
}

// flushImage converts the Go image.RGBA into an X11 PutImage request.
// In Phase 3, this will be replaced with MIT-SHM for zero-copy performance.
func (h *X11Host) flushImage() {
	h.mu.Lock()
	defer h.mu.Unlock()

	b := h.imgBuf.Bounds()
	w, h2 := b.Dx(), b.Dy()
	totalBytes := w * h2 * 4

	if len(h.bgraBuf) != totalBytes {
		h.bgraBuf = make([]byte, totalBytes)
	}

	pix := h.imgBuf.Pix
	bgra := h.bgraBuf
	lineStride := w * 4

	// 1. Process dirty lines: Convert only what changed
	anyDirty := false
	for y := 0; y < h2; y++ {
		if h.dirtyLines[y] {
			anyDirty = true
			off := y * lineStride
			for x := 0; x < lineStride; x += 4 {
				p := off + x
				bgra[p], bgra[p+1], bgra[p+2], bgra[p+3] = pix[p+2], pix[p+1], pix[p], 0xFF
			}
		}
	}

	if !anyDirty {
		return
	}

	// 2. Find and send contiguous spans of dirty lines
	maxReq := int(xproto.Setup(h.conn).MaximumRequestLength) * 4
	rowsPerReqLimit := (maxReq - 24) / lineStride
	if rowsPerReqLimit < 1 {
		rowsPerReqLimit = 1
	}

	for y := 0; y < h2; {
		if !h.dirtyLines[y] {
			y++
			continue
		}

		// Found start of dirty span
		spanStart := y
		spanEnd := y
		for spanEnd < h2 && h.dirtyLines[spanEnd] && (spanEnd-spanStart) < rowsPerReqLimit {
			h.dirtyLines[spanEnd] = false // Reset dirty flag as we process it
			spanEnd++
		}

		rows := uint16(spanEnd - spanStart)
		data := bgra[spanStart*lineStride : spanEnd*lineStride]
		xproto.PutImage(h.conn, xproto.ImageFormatZPixmap, xproto.Drawable(h.wid), h.gc,
			uint16(w), rows, 0, int16(spanStart), 0, 24, data)
		y = spanEnd
	}
}