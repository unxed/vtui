//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris

package vtui

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"unicode"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/unxed/vtinput"
	"github.com/unxed/xkb-go"
)

var (
	keyDeclRegexp  = regexp.MustCompile(`(?i)key\s+<[^>]+>`)
	emptyKeyRegexp = regexp.MustCompile(`(?i)key\s+<\s*>`)
)

// isXkbcompOutputValid выполняет эвристическую проверку полноты карты клавиатуры.
func isXkbcompOutputValid(output []byte) bool {
	if len(output) == 0 {
		return false
	}
	str := string(output)

	// Проверка 0: Наличие базовых секций (исключает обрезанный вывод)
	if !strings.Contains(str, "xkb_symbols") || !strings.Contains(str, "xkb_keycodes") {
		return false
	}

	// Проверка 1: Ищем пустые маркеры-заглушки (часто отправляются XQuartz)
	if emptyKeyRegexp.MatchString(str) {
		return false
	}

	// Проверка 2: Наличие фундаментальных клавиш управления
	if !strings.Contains(str, "<ESC>") || !strings.Contains(str, "<RTRN>") {
		return false
	}

	// Проверка 3: Подсчет общего количества задекларированных клавиш
	matches := keyDeclRegexp.FindAllStringIndex(str, -1)
	if len(matches) < 10 {
		return false
	}

	return true
}

type rawRequest []byte

func (r rawRequest) Bytes() []byte { return r }

// XkbState хранит полный ответ от XkbGetState
type XkbState struct {
	BaseMods     byte
	LatchedMods  byte
	LockedMods   byte
	BaseGroup    int16
	LatchedGroup int16
	LockedGroup  byte
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
	buf[1] = 4                // XkbGetState
	xgb.Put16(buf[2:], 2)     // Length
	xgb.Put16(buf[4:], 0x0100) // deviceSpec = XkbUseCoreKbd

	cookie := X.NewCookie(true, true)
	X.NewRequest(rawRequest(buf), cookie)
	reply, err := cookie.Reply()
	if err != nil {
		return nil, err
	}
	if len(reply) < 18 { // Минимальная длина для полей, которые нас интересуют
		return nil, fmt.Errorf("XKB reply too short")
	}

	return &XkbState{
		BaseMods:     reply[9],
		LatchedMods:  reply[10],
		LockedMods:   reply[11],
		LockedGroup:  reply[13],
		BaseGroup:    int16(xgb.Get16(reply[14:])),
		LatchedGroup: int16(xgb.Get16(reply[16:])),
	}, nil
}

const (
	modMaskShift  = 1 << 0
	modMaskLock   = 1 << 1
	modMaskAltGr3 = 1 << 5 // Mod3
	modMaskAltGr5 = 1 << 7 // Mod5
)

type CoreKeymap struct {
	MinKeycode int
	MaxKeycode int
	SymsPerKey int
	Syms       []xproto.Keysym
}

func loadCoreKeymap(conn *xgb.Conn, setup *xproto.SetupInfo) (*CoreKeymap, error) {
	min := setup.MinKeycode
	max := setup.MaxKeycode
	count := byte(max - min + 1)

	reply, err := xproto.GetKeyboardMapping(conn, min, count).Reply()
	if err != nil {
		return nil, err
	}

	return &CoreKeymap{
		MinKeycode: int(min),
		MaxKeycode: int(max),
		SymsPerKey: int(reply.KeysymsPerKeycode),
		Syms:       reply.Keysyms,
	}, nil
}

func (km *CoreKeymap) Lookup(kc int, state uint16, group int) uint32 {
	if kc < km.MinKeycode || kc > km.MaxKeycode {
		return 0
	}
	offset := (kc - km.MinKeycode) * km.SymsPerKey
	if offset+km.SymsPerKey > len(km.Syms) {
		return 0
	}
	syms := km.Syms[offset : offset+km.SymsPerKey]

	length := len(syms)
	for length > 0 && syms[length-1] == 0 {
		length--
	}
	if length == 0 {
		return 0
	}

	shift := (state & modMaskShift) != 0
	capsLock := (state & modMaskLock) != 0
	altGr := (state&modMaskAltGr5) != 0 || (state&modMaskAltGr3) != 0

	// Эвристика ширины группы (защита от сбоя при SymsPerKey > 4)
	groupWidth := 2
	if km.SymsPerKey > 4 && km.SymsPerKey%4 == 0 {
		groupWidth = 4
	}

	idx := group * groupWidth
	if idx >= length {
		idx = 0
	}

	var symsHex []string
	for _, s := range syms {
		symsHex = append(symsHex, fmt.Sprintf("0x%X", s))
	}

	DebugLog("XKB_DEBUG: Keycode=%d state=0x%X group=%d groupWidth=%d colBase=%d syms=[%s]",
		kc, state, group, groupWidth, idx, strings.Join(symsHex, ", "))

	altGrApplied := false
	if altGr {
		altGrOffset := 0
		if groupWidth == 4 && idx+2 < length && syms[idx+2] != 0 && syms[idx+2] != syms[idx] {
			// Если ширина группы 4 (стандарт XKB), AltGr всегда имеет смещение +2
			altGrOffset = 2
		} else {
			// Динамический поиск для нестандартных таблиц
			offsets := []int{2, 4, 3, 6, 8}
			if km.SymsPerKey > 2 {
				offsets = append([]int{km.SymsPerKey / 2}, offsets...)
			}
			for _, o := range offsets {
				if idx+o < length && syms[idx+o] != 0 && syms[idx+o] != syms[idx] {
					// Защита от ложного Mod3/Mod5 (NumLock) в чужой группе
					if group == 0 || (idx+o < len(syms) && syms[idx+o] != syms[o]) {
						altGrOffset = o
						break
					}
				}
			}
		}
		if altGrOffset != 0 {
			idx += altGrOffset
			altGrApplied = true
		}
	}

	DebugLog("XKB_DEBUG: AltGr=%v altGrApplied=%v resolvedIdx=%d keysym=0x%X",
		altGr, altGrApplied, idx, syms[idx])

	baseSym := uint32(syms[idx])
	shiftedSym := baseSym
	if idx+1 < length && syms[idx+1] != 0 {
		shiftedSym = uint32(syms[idx+1])
	}

	r := xkb.KeysymToUTF32(xkb.Keysym(baseSym))
	isLetter := r != 0 && unicode.IsLetter(r)

	resSym := baseSym
	if shift {
		if capsLock && isLetter {
			resSym = baseSym
		} else {
			resSym = shiftedSym
		}
	} else if capsLock && isLetter {
		resSym = shiftedSym
	}

	return resSym
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

	xkbOpcode  byte
	xkbState   *xkb.State
	coreKeymap *CoreKeymap
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

	var xkbState *xkb.State
	var coreKeymap *CoreKeymap

	var xkbcompPath string
	if p, err := exec.LookPath("xkbcomp"); err == nil {
		xkbcompPath = p
	} else if runtime.GOOS == "windows" {
		// Сохраняем логику поиска xkbcomp на Windows для совместимости,
		// но по умолчанию не используем её, так как серверы VcXsrv/Xming
		// могут отдавать некорректно структурированные карты.
		commonPaths := []string{
			`C:\Program Files\VcXsrv\xkbcomp.exe`,
			`C:\Program Files (x86)\VcXsrv\xkbcomp.exe`,
			`C:\Program Files\Xming\xkbcomp.exe`,
			`C:\Program Files (x86)\Xming\xkbcomp.exe`,
			`C:\cygwin64\bin\xkbcomp.exe`,
			`C:\cygwin\bin\xkbcomp.exe`,
			`C:\msys64\usr\bin\xkbcomp.exe`,
			`C:\msys64\mingw64\bin\xkbcomp.exe`,
			`C:\msys64\mingw32\bin\xkbcomp.exe`,
		}
		for _, p := range commonPaths {
			if _, err := os.Stat(p); err == nil {
				xkbcompPath = p
				break
			}
		}
	}

	if xkbcompPath != "" {
		display := os.Getenv("DISPLAY")
		if display == "" {
			display = ":0"
		}
		cmd := exec.Command(xkbcompPath, display, "-")
		var out bytes.Buffer
		cmd.Stdout = &out
		if err := cmd.Run(); err == nil && out.Len() > 0 {
			if isXkbcompOutputValid(out.Bytes()) {
				xkbCtx := xkb.NewContext(context.Background(), xkb.ContextNoFlags)
				if keymap, kerr := xkbCtx.NewKeymapFromString(out.Bytes(), xkb.KeymapFormatTextV1); kerr == nil {
					xkbState = keymap.NewState()
					DebugLog("XKB: Keymap loaded via xkbcomp (%d bytes)", out.Len())
				} else {
					DebugLog("XKB: Failed to parse xkbcomp keymap: %v", kerr)
				}
			} else {
				DebugLog("XKB: xkbcomp output failed feature detection validation (incomplete or empty map)")
			}
		} else {
			DebugLog("XKB: xkbcomp execution failed or returned empty output")
		}
	}

	if xkbState == nil {
		var err error
		coreKeymap, err = loadCoreKeymap(conn, setup)
		if err != nil {
			return nil, fmt.Errorf("failed to load core keymap: %v", err)
		}
		DebugLog("XKB: Core keymap fallback loaded successfully")
	}

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
		xkbState:   xkbState,
		coreKeymap: coreKeymap,
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

		case xproto.MappingNotifyEvent:
			h.mu.Lock()
			setup := xproto.Setup(h.conn)
			if newKm, err := loadCoreKeymap(h.conn, setup); err == nil {
				h.coreKeymap = newKm
				DebugLog("PUREX11: Keyboard mapping reloaded after MappingNotify")
			} else {
				DebugLog("PUREX11: Failed to reload keyboard mapping: %v", err)
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

func (h *PureX11Host) handleKeyEvent(detail xproto.Keycode, state uint16, isDown bool) {
	if h.xkbState != nil {
		srv, err := queryXkbState(h.conn, h.xkbOpcode)
		if err == nil {
			h.xkbState.UpdateMask(
				xkb.ModMask(srv.BaseMods),
				xkb.ModMask(srv.LatchedMods),
				xkb.ModMask(srv.LockedMods),
				xkb.Group(srv.BaseGroup),
				xkb.Group(srv.LatchedGroup),
				xkb.Group(srv.LockedGroup),
			)
		}

		kc := xkb.Keycode(detail)
		sym := h.xkbState.KeyGetOneSym(kc)
		char := h.xkbState.KeyGetUTF32(kc)
		vk := keysymToVK(uint32(sym))

		// Fallback for positional VK if unknown and we are in an alternate layout
		if vk == 0 && h.xkbState.LockedGroup() != 0 {
			bm, lam, lom := h.xkbState.BaseMods(), h.xkbState.LatchedMods(), h.xkbState.LockedMods()
			bg, lag, log := h.xkbState.BaseGroup(), h.xkbState.LatchedGroup(), h.xkbState.LockedGroup()

			// Temporarily switch to Group 0 with no modifiers
			h.xkbState.UpdateMask(0, 0, 0, 0, 0, 0)
			vkSym := h.xkbState.KeyGetOneSym(kc)
			vk = keysymToVK(uint32(vkSym))

			// Restore original state
			h.xkbState.UpdateMask(bm, lam, lom, bg, lag, log)
		}

		mods := h.translateModifiers(state)

		if h.reader != nil {
			h.reader.NativeEventChan <- &vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         isDown,
				VirtualKeyCode:  vk,
				Char:            char,
				ControlKeyState: mods,
			}
		}
		return
	}

	group := 0
	if h.xkbOpcode != 0 {
		if srv, err := queryXkbState(h.conn, h.xkbOpcode); err == nil {
			group = int(srv.BaseGroup) + int(srv.LockedGroup)
			if group < 0 {
				group = 0
			}
			if group > 3 {
				group = group % 4
			}
			DebugLog("PUREX11_KEYBOARD: Key=%d X11State=0x%X srvGroup=%d (Base=%d Locked=%d)",
				detail, state, srv.BaseGroup+int16(srv.LockedGroup), srv.BaseGroup, srv.LockedGroup)
		} else {
			DebugLog("PUREX11_KEYBOARD: queryXkbState error: %v", err)
		}
	}

	sym := h.coreKeymap.Lookup(int(detail), state, group)
	char := xkb.KeysymToUTF32(xkb.Keysym(sym))
	DebugLog("PUREX11_KEYBOARD: Translated Keycode %d -> Sym=0x%X (Name: %s) Char=%q",
		detail, sym, xkb.KeysymGetName(xkb.Keysym(sym)), string(char))

	vk := keysymToVK(sym)

	// Fallback for positional VK if unknown and we are in an alternate layout
	if vk == 0 && group > 0 {
		baseSym := h.coreKeymap.Lookup(int(detail), state, 0)
		vk = keysymToVK(baseSym)
	}

	mods := h.translateModifiers(state)

	if h.reader != nil {
		h.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         isDown,
			VirtualKeyCode:  vk,
			Char:            char,
			ControlKeyState: mods,
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

