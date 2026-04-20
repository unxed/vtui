//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package vtui

import (
	"image"
	"sync"
	"fmt"
	"unicode"

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
	cols, rows   int
	closeChan    chan struct{}
	keyMap       []xproto.Keysym
	keysPerCode  byte
	minKeyCode   xproto.Keycode
	atomDelete     xproto.Atom
	dirtyLines     []bool
	lCtrl, rCtrl   bool
	lAlt, rAlt     bool
	lShift, rShift bool
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
		cols:      cols,
		rows:      rows,
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

	// Set WM_CLASS for better WM compatibility (instance, class)
	wmClassAtom, _ := xproto.InternAtom(conn, false, 8, "WM_CLASS").Reply()
	if wmClassAtom != nil {
		xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, wmClassAtom.Atom,
			xproto.AtomString, 8, 6, []byte("f4\x00f4\x00"))
	}

	// Request maximized state via EWMH before mapping the window
	wmStateAtom, _ := xproto.InternAtom(conn, false, 13, "_NET_WM_STATE").Reply()
	wmMaxVertAtom, _ := xproto.InternAtom(conn, false, 28, "_NET_WM_STATE_MAXIMIZED_VERT").Reply()
	wmMaxHorzAtom, _ := xproto.InternAtom(conn, false, 28, "_NET_WM_STATE_MAXIMIZED_HORZ").Reply()

	if wmStateAtom != nil && wmMaxVertAtom != nil && wmMaxHorzAtom != nil {
		data := make([]byte, 8)
		xgb.Put32(data, uint32(wmMaxVertAtom.Atom))
		xgb.Put32(data[4:], uint32(wmMaxHorzAtom.Atom))
		xproto.ChangeProperty(conn, xproto.PropModeReplace, host.wid, wmStateAtom.Atom,
			xproto.AtomAtom, 32, 2, data)
	}

	// Create GC
	host.gc, err = xproto.NewGcontextId(conn)
	if err == nil {
		xproto.CreateGC(conn, host.gc, xproto.Drawable(host.wid),
			xproto.GcForeground|xproto.GcBackground,
			[]uint32{screen.BlackPixel, screen.WhitePixel})
	}

	// Create backing image buffer
	host.imgBuf = image.NewRGBA(image.Rect(0, 0, int(host.width), int(host.height)))


	// Fetch keyboard mapping
	host.minKeyCode = setup.MinKeycode
	maxKeyCode := setup.MaxKeycode
	mapLen := byte(maxKeyCode - host.minKeyCode + 1)
	kmReply, err := xproto.GetKeyboardMapping(conn, host.minKeyCode, mapLen).Reply()
	if err == nil {
		host.keyMap = kmReply.Keysyms
		host.keysPerCode = kmReply.KeysymsPerKeycode
	}


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

func (h *X11Host) translateModifiers(state uint16, vk uint16, isDown bool) vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState

	// Sync internal state with X11 state bitmask to prevent "stuck" modifiers
	// (happens if Release event is swallowed by system shortcuts)
	if state&xproto.ModMaskControl == 0 {
		h.lCtrl = false
		h.rCtrl = false
	}
	if state&xproto.ModMask1 == 0 {
		h.lAlt = false
		h.rAlt = false
	}
	if state&xproto.ModMaskShift == 0 {
		h.lShift = false
		h.rShift = false
	}

	if h.lCtrl { mods |= vtinput.LeftCtrlPressed }
	if h.rCtrl { mods |= vtinput.RightCtrlPressed | vtinput.EnhancedKey }
	if h.lAlt { mods |= vtinput.LeftAltPressed }
	if h.rAlt { mods |= vtinput.RightAltPressed | vtinput.EnhancedKey }
	if h.lShift || h.rShift { mods |= vtinput.ShiftPressed }

	// Fallbacks for desync (e.g. window lost focus during key release).
	// Only apply fallback if we are NOT actively releasing a key of this category.
	isReleasingShift := !isDown && (vk == vtinput.VK_LSHIFT || vk == vtinput.VK_RSHIFT)
	if state&xproto.ModMaskShift != 0 && !h.lShift && !h.rShift && !isReleasingShift {
		mods |= vtinput.ShiftPressed
	}

	isReleasingCtrl := !isDown && (vk == vtinput.VK_LCONTROL || vk == vtinput.VK_RCONTROL)
	if state&xproto.ModMaskControl != 0 && !h.lCtrl && !h.rCtrl && !isReleasingCtrl {
		mods |= vtinput.LeftCtrlPressed
	}

	isReleasingAlt := !isDown && (vk == vtinput.VK_LMENU || vk == vtinput.VK_RMENU)
	if state&xproto.ModMask1 != 0 && !h.lAlt && !h.rAlt && !isReleasingAlt { // Usually Alt
		mods |= vtinput.LeftAltPressed
	}

	if state&xproto.ModMaskLock != 0 { // CapsLock
		mods |= vtinput.CapsLockOn
	}
	if state&xproto.ModMask2 != 0 { // Usually NumLock
		mods |= vtinput.NumLockOn
	}

	return mods
}

func (h *X11Host) getKeysym(detail xproto.Keycode, state uint16) xproto.Keysym {
	if h.keyMap == nil {
		return 0
	}
	baseIdx := int(detail-h.minKeyCode) * int(h.keysPerCode)
	if baseIdx < 0 || baseIdx >= len(h.keyMap) {
		return 0
	}

	shift := state&xproto.ModMaskShift != 0
	//numLock := state&xproto.ModMask2 != 0

	// Extract XKB Group (Layout)
	group := (state >> 13) & 0x03
	// Heuristic: Mod5 (0x80) often indicates secondary layout if XKB group bits are not set
	if group == 0 && state&xproto.ModMask5 != 0 && h.keysPerCode > 2 {
		group = 1
	}

	col := int(group) * 2
	if shift {
		col += 1
	}

	// Safety: if group column exceeds available mapping, fallback to first group
	if col >= int(h.keysPerCode) {
		col = col % 2
	}

	sym := h.keyMap[baseIdx+col]
	if sym == 0 && col > 0 {
		// Fallback to base keysym if specific column is empty
		sym = h.keyMap[baseIdx]
	}

	return sym
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
				newCols := int(e.Width) / h.cellW
				newRows := int(e.Height) / h.cellH
				if newCols < 1 { newCols = 1 }
				if newRows < 1 { newRows = 1 }

				h.mu.Lock()
				h.width = e.Width
				h.height = e.Height
				h.cols = newCols
				h.rows = newRows
				h.imgBuf = image.NewRGBA(image.Rect(0, 0, int(h.width), int(h.height)))
				h.dirtyLines = make([]bool, int(e.Height))
				for i := range h.dirtyLines {
					h.dirtyLines[i] = true
				}
				h.mu.Unlock()

				if h.reader != nil && h.reader.NativeEventChan != nil {
					h.reader.NativeEventChan <- &vtinput.InputEvent{
						Type:        vtinput.ResizeEventType,
						InputSource: "x11",
					}
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

			keysym := h.getKeysym(detail, state)
			baseKeysym := h.getKeysym(detail, 0)
			
			vk := keysymToVK(uint32(keysym))
			if vk == 0 {
				vk = keysymToVK(uint32(baseKeysym))
			}

			char := keysymToRune(uint32(keysym))

			// Heuristic: if we got a control keysym (like Return) but it's used with Alt,
			// some apps might expect the ASCII char equivalent in the event.
			if char == 0 {
				switch keysym {
				case 0xff0d, 0xff8d: char = '\r'
				case 0xff09, 0xff89: char = '\t'
				case 0xff08: char = '\b'
				case 0xff1b: char = 27
				}
			}

			if isDown {
				switch vk {
				case vtinput.VK_LCONTROL: h.lCtrl = true
				case vtinput.VK_RCONTROL: h.rCtrl = true
				case vtinput.VK_LMENU:    h.lAlt = true
				case vtinput.VK_RMENU:    h.rAlt = true
				case vtinput.VK_LSHIFT:   h.lShift = true
				case vtinput.VK_RSHIFT:   h.rShift = true
				}
			} else {
				switch vk {
				case vtinput.VK_LCONTROL: h.lCtrl = false
				case vtinput.VK_RCONTROL: h.rCtrl = false
				case vtinput.VK_LMENU:    h.lAlt = false
				case vtinput.VK_RMENU:    h.rAlt = false
				case vtinput.VK_LSHIFT:   h.lShift = false
				case vtinput.VK_RSHIFT:   h.rShift = false
				}
			}

			mods := h.translateModifiers(state, vk, isDown)

			// Universal CapsLock + Shift logic for alphabetical characters
			if unicode.IsLetter(char) {
				shift := mods.Contains(vtinput.ShiftPressed)
				caps := mods.Contains(vtinput.CapsLockOn)
				if shift != caps {
					char = unicode.ToUpper(char)
				} else {
					char = unicode.ToLower(char)
				}
			}

			typeName := "PRESS"
			if !isDown {
				typeName = "RELEASE"
			}
			DebugLog("X11_KEY: %s code=%d state=0x%04x keysym=0x%04x vk=%s char=%q",
				typeName, detail, state, keysym, vtinput.VKString(vk), char)

			scancode := uint16(0)
			if vk == vtinput.VK_RSHIFT {
				scancode = vtinput.ScanCodeRightShift
			} else if vk == vtinput.VK_LSHIFT {
				scancode = vtinput.ScanCodeLeftShift
			}

			h.reader.NativeEventChan <- &vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         isDown,
				VirtualKeyCode:  vk,
				VirtualScanCode: scancode,
				Char:            char,
				ControlKeyState: mods,
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
				ControlKeyState: h.translateModifiers(state, 0, false),
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
				ControlKeyState: h.translateModifiers(e.State, 0, false),
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
	lineStride := w * 4

	// Проверяем, есть ли вообще изменения
	anyDirty := false
	for y := 0; y < h2; y++ {
		if h.dirtyLines[y] {
			anyDirty = true
			break
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

		spanStart := y
		spanEnd := y
		// Жадное объединение грязных строк в один запрос PutImage
		for spanEnd < h2 && (h.dirtyLines[spanEnd] || (spanEnd+1 < h2 && h.dirtyLines[spanEnd+1])) && (spanEnd-spanStart) < rowsPerReqLimit {
			h.dirtyLines[spanEnd] = false
			spanEnd++
		}

		if spanEnd > spanStart {
			rows := uint16(spanEnd - spanStart)
			data := pix[spanStart*lineStride : spanEnd*lineStride]
			xproto.PutImage(h.conn, xproto.ImageFormatZPixmap, xproto.Drawable(h.wid), h.gc,
				uint16(w), rows, 0, int16(spanStart), 0, 24, data)
		}
		y = spanEnd
	}
}