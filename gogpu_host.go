//go:build !freebsd

package vtui

import (
	"io"
	"os"
	"sync"

	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"github.com/unxed/vtinput"
)

var (
	debugLastMouseX, debugLastMouseY float64 = -1, -1
	debugLastCtxW, debugLastCtxH     int     = -1, -1
)
type GogpuHost struct {
	mu          sync.Mutex
	app         *gogpu.App
	reader      *vtinput.Reader
	scr         *ScreenBuf
	cols        int
	rows        int
	cellW       int
	cellH       int
	lastW       int
	lastH       int
	ctx         *gogpu.Context
	mouseBtn    uint32
	currentMods vtinput.ControlKeyState
}

func RunGogpuHost(cols, rows int, setupApp func()) error {
	fontSize := 18.0
	face, cellW, cellH := loadGogpuFont(fontSize)

	DebugLog("GOGPU_HOST: Starting RunGogpuHost %dx%d (Cell: %dx%d)", cols, rows, cellW, cellH)
	config := gogpu.DefaultConfig().
		WithTitle(AppName).
		WithSize(cols*cellW, rows*cellH)

	app := gogpu.NewApp(config)
	host := &GogpuHost{
		app:   app,
		cols:  cols,
		rows:  rows,
		cellW: cellW,
		cellH: cellH,
	}
	DebugLog("GOGPU: Init Host %dx%d Cells. Font metrics: W=%d H=%d", cols, rows, cellW, cellH)

	scr := NewScreenBuf()
	host.scr = scr
	scr.AllocBuf(cols, rows)
	renderer := NewGogpuRenderer(host, face, cellW, cellH)
	scr.Renderer = renderer

	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr)
	if reader.NativeEventChan == nil {
		reader.NativeEventChan = make(chan *vtinput.InputEvent, 1024)
	}
	host.reader = reader

	app.OnClose(func() {
		FrameManager.EmitCommand(CmQuit, nil)
	})

	app.EventSource().OnKeyPress(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		vk := gogpuKeyToVK(key)

		host.mu.Lock()
		if vk == vtinput.VK_SHIFT || vk == vtinput.VK_LSHIFT || vk == vtinput.VK_RSHIFT {
			host.currentMods |= vtinput.ShiftPressed
		}
		if vk == vtinput.VK_CONTROL || vk == vtinput.VK_LCONTROL || vk == vtinput.VK_RCONTROL {
			host.currentMods |= vtinput.LeftCtrlPressed
		}
		if vk == vtinput.VK_MENU || vk == vtinput.VK_LMENU || vk == vtinput.VK_RMENU {
			host.currentMods |= vtinput.LeftAltPressed
		}
		currMods := host.currentMods
		host.mu.Unlock()

		if vk == 0 { return }
		char := gogpuKeyToChar(key, (currMods & vtinput.ShiftPressed) != 0)

		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  vk,
			Char:            char,
			ControlKeyState: currMods,
		}
	})

	app.EventSource().OnKeyRelease(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		vk := gogpuKeyToVK(key)

		host.mu.Lock()
		if vk == vtinput.VK_SHIFT || vk == vtinput.VK_LSHIFT || vk == vtinput.VK_RSHIFT {
			host.currentMods &^= vtinput.ShiftPressed
		}
		if vk == vtinput.VK_CONTROL || vk == vtinput.VK_LCONTROL || vk == vtinput.VK_RCONTROL {
			host.currentMods &^= vtinput.LeftCtrlPressed
		}
		if vk == vtinput.VK_MENU || vk == vtinput.VK_LMENU || vk == vtinput.VK_RMENU {
			host.currentMods &^= vtinput.LeftAltPressed
		}
		currMods := host.currentMods
		host.mu.Unlock()

		if vk == 0 { return }
		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         false,
			VirtualKeyCode:  vk,
			ControlKeyState: currMods,
		}
	})

	app.EventSource().OnMouseMove(func(x, y float64) {
		host.mu.Lock()
		btn := host.mouseBtn
		host.mu.Unlock()

		if x != debugLastMouseX || y != debugLastMouseY {
			// Логируем только если координата изменилась более чем на 2 пикселя
			DebugLog("GOGPU_MOUSE: RawXY=%.1f,%.1f Cell=%d,%d Btn=%d", x, y, int(x)/cellW, int(y)/cellH, btn)
			debugLastMouseX, debugLastMouseY = x, y
		}

		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          uint16(int(x) / cellW),
			MouseY:          uint16(int(y) / cellH),
			MouseEventFlags: vtinput.MouseMoved,
			ButtonState:     btn,
		}
	})

	app.EventSource().OnMousePress(func(button gpucontext.MouseButton, x, y float64) {
		btn := uint32(vtinput.FromLeft1stButtonPressed)

		host.mu.Lock()
		host.mouseBtn = btn
		DebugLog("PROBE_CLICK: MouseRaw=%.1f,%.1f CtxSize=%dx%d LogSize=%dx%d", x, y, debugLastCtxW, debugLastCtxH, host.cols*host.cellW, host.rows*host.cellH)
		host.mu.Unlock()

		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      uint16(int(x) / cellW),
			MouseY:      uint16(int(y) / cellH),
			KeyDown:     true,
			ButtonState: btn,
		}
	})

	app.EventSource().OnMouseRelease(func(button gpucontext.MouseButton, x, y float64) {
		host.mu.Lock()
		host.mouseBtn = 0
		host.mu.Unlock()

		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      uint16(int(x) / cellW),
			MouseY:      uint16(int(y) / cellH),
			KeyDown:     false,
			ButtonState: 0,
		}
	})

	app.OnDraw(func(dc *gogpu.Context) {
		host.mu.Lock()
		host.ctx = dc
		host.mu.Unlock()

		host.scr.Renderer.Flush()

		host.mu.Lock()
		host.ctx = nil
		host.mu.Unlock()
	})

	GetTerminalSize = func() (int, int, error) {
		return host.cols, host.rows, nil
	}

	setupApp()

	go func() {
		DebugLog("GOGPU_HOST: FrameManager starting...")
		FrameManager.Run(reader)
		DebugLog("GOGPU_HOST: FrameManager exited. Forcing app shutdown to prevent blue screen hang.")
		os.Exit(0)
	}()

	return app.Run()
}

func translateGogpuMods(m gpucontext.Modifiers) vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	/*
	if m.IsShift() {
		mods |= vtinput.ShiftPressed
	}
	if m.IsCtrl() {
		mods |= vtinput.LeftCtrlPressed
	}
	if m.IsAlt() {
		mods |= vtinput.LeftAltPressed
	}
	if m.IsSuper() {
		mods |= vtinput.EnhancedKey
	}
	*/
	return mods
}

func gogpuKeyToVK(k gpucontext.Key) uint16 {
	switch k {
	case gpucontext.KeyEscape: return vtinput.VK_ESCAPE
	case gpucontext.KeyF1: return vtinput.VK_F1
	case gpucontext.KeyF2: return vtinput.VK_F2
	case gpucontext.KeyF3: return vtinput.VK_F3
	case gpucontext.KeyF4: return vtinput.VK_F4
	case gpucontext.KeyF5: return vtinput.VK_F5
	case gpucontext.KeyF6: return vtinput.VK_F6
	case gpucontext.KeyF7: return vtinput.VK_F7
	case gpucontext.KeyF8: return vtinput.VK_F8
	case gpucontext.KeyF9: return vtinput.VK_F9
	case gpucontext.KeyF10: return vtinput.VK_F10
	case gpucontext.KeyF11: return vtinput.VK_F11
	case gpucontext.KeyF12: return vtinput.VK_F12
	case gpucontext.KeyInsert: return vtinput.VK_INSERT
	case gpucontext.KeyDelete: return vtinput.VK_DELETE
	case gpucontext.KeyHome: return vtinput.VK_HOME
	case gpucontext.KeyEnd: return vtinput.VK_END
	case gpucontext.KeyPageUp: return vtinput.VK_PRIOR
	case gpucontext.KeyPageDown: return vtinput.VK_NEXT
	case gpucontext.KeyUp: return vtinput.VK_UP
	case gpucontext.KeyDown: return vtinput.VK_DOWN
	case gpucontext.KeyLeft: return vtinput.VK_LEFT
	case gpucontext.KeyRight: return vtinput.VK_RIGHT
	case gpucontext.KeyBackspace: return vtinput.VK_BACK
	case gpucontext.KeyEnter: return vtinput.VK_RETURN
	case gpucontext.KeyTab: return vtinput.VK_TAB
	case gpucontext.KeySpace: return vtinput.VK_SPACE
	case gpucontext.KeyLeftControl, gpucontext.KeyRightControl: return vtinput.VK_CONTROL
	case gpucontext.KeyLeftShift, gpucontext.KeyRightShift: return vtinput.VK_SHIFT
	case gpucontext.KeyLeftAlt, gpucontext.KeyRightAlt: return vtinput.VK_MENU
	case gpucontext.KeyA: return vtinput.VK_A
	case gpucontext.KeyB: return vtinput.VK_B
	case gpucontext.KeyC: return vtinput.VK_C
	case gpucontext.KeyD: return vtinput.VK_D
	case gpucontext.KeyE: return vtinput.VK_E
	case gpucontext.KeyF: return vtinput.VK_F
	case gpucontext.KeyG: return vtinput.VK_G
	case gpucontext.KeyH: return vtinput.VK_H
	case gpucontext.KeyI: return vtinput.VK_I
	case gpucontext.KeyJ: return vtinput.VK_J
	case gpucontext.KeyK: return vtinput.VK_K
	case gpucontext.KeyL: return vtinput.VK_L
	case gpucontext.KeyM: return vtinput.VK_M
	case gpucontext.KeyN: return vtinput.VK_N
	case gpucontext.KeyO: return vtinput.VK_O
	case gpucontext.KeyP: return vtinput.VK_P
	case gpucontext.KeyQ: return vtinput.VK_Q
	case gpucontext.KeyR: return vtinput.VK_R
	case gpucontext.KeyS: return vtinput.VK_S
	case gpucontext.KeyT: return vtinput.VK_T
	case gpucontext.KeyU: return vtinput.VK_U
	case gpucontext.KeyV: return vtinput.VK_V
	case gpucontext.KeyW: return vtinput.VK_W
	case gpucontext.KeyX: return vtinput.VK_X
	case gpucontext.KeyY: return vtinput.VK_Y
	case gpucontext.KeyZ: return vtinput.VK_Z
	case gpucontext.Key0: return vtinput.VK_0
	case gpucontext.Key1: return vtinput.VK_1
	case gpucontext.Key2: return vtinput.VK_2
	case gpucontext.Key3: return vtinput.VK_3
	case gpucontext.Key4: return vtinput.VK_4
	case gpucontext.Key5: return vtinput.VK_5
	case gpucontext.Key6: return vtinput.VK_6
	case gpucontext.Key7: return vtinput.VK_7
	case gpucontext.Key8: return vtinput.VK_8
	case gpucontext.Key9: return vtinput.VK_9
	case gpucontext.KeyMinus: return vtinput.VK_OEM_MINUS
	case gpucontext.KeyEqual: return vtinput.VK_OEM_PLUS
	case gpucontext.KeyLeftBracket: return vtinput.VK_OEM_4
	case gpucontext.KeyRightBracket: return vtinput.VK_OEM_6
	case gpucontext.KeyBackslash: return vtinput.VK_OEM_5
	case gpucontext.KeySemicolon: return vtinput.VK_OEM_1
	case gpucontext.KeyApostrophe: return vtinput.VK_OEM_7
	case gpucontext.KeyComma: return vtinput.VK_OEM_COMMA
	case gpucontext.KeyPeriod: return vtinput.VK_OEM_PERIOD
	case gpucontext.KeySlash: return vtinput.VK_OEM_2
	}
	return 0
}

func gogpuKeyToChar(k gpucontext.Key, shift bool) rune {
	switch k {
	case gpucontext.KeyA: if shift { return 'A' } else { return 'a' }
	case gpucontext.KeyB: if shift { return 'B' } else { return 'b' }
	case gpucontext.KeyC: if shift { return 'C' } else { return 'c' }
	case gpucontext.KeyD: if shift { return 'D' } else { return 'd' }
	case gpucontext.KeyE: if shift { return 'E' } else { return 'e' }
	case gpucontext.KeyF: if shift { return 'F' } else { return 'f' }
	case gpucontext.KeyG: if shift { return 'G' } else { return 'g' }
	case gpucontext.KeyH: if shift { return 'H' } else { return 'h' }
	case gpucontext.KeyI: if shift { return 'I' } else { return 'i' }
	case gpucontext.KeyJ: if shift { return 'J' } else { return 'j' }
	case gpucontext.KeyK: if shift { return 'K' } else { return 'k' }
	case gpucontext.KeyL: if shift { return 'L' } else { return 'l' }
	case gpucontext.KeyM: if shift { return 'M' } else { return 'm' }
	case gpucontext.KeyN: if shift { return 'N' } else { return 'n' }
	case gpucontext.KeyO: if shift { return 'O' } else { return 'o' }
	case gpucontext.KeyP: if shift { return 'P' } else { return 'p' }
	case gpucontext.KeyQ: if shift { return 'Q' } else { return 'q' }
	case gpucontext.KeyR: if shift { return 'R' } else { return 'r' }
	case gpucontext.KeyS: if shift { return 'S' } else { return 's' }
	case gpucontext.KeyT: if shift { return 'T' } else { return 't' }
	case gpucontext.KeyU: if shift { return 'U' } else { return 'u' }
	case gpucontext.KeyV: if shift { return 'V' } else { return 'v' }
	case gpucontext.KeyW: if shift { return 'W' } else { return 'w' }
	case gpucontext.KeyX: if shift { return 'X' } else { return 'x' }
	case gpucontext.KeyY: if shift { return 'Y' } else { return 'y' }
	case gpucontext.KeyZ: if shift { return 'Z' } else { return 'z' }
	case gpucontext.Key0: if shift { return ')' } else { return '0' }
	case gpucontext.Key1: if shift { return '!' } else { return '1' }
	case gpucontext.Key2: if shift { return '@' } else { return '2' }
	case gpucontext.Key3: if shift { return '#' } else { return '3' }
	case gpucontext.Key4: if shift { return '$' } else { return '4' }
	case gpucontext.Key5: if shift { return '%' } else { return '5' }
	case gpucontext.Key6: if shift { return '^' } else { return '6' }
	case gpucontext.Key7: if shift { return '&' } else { return '7' }
	case gpucontext.Key8: if shift { return '*' } else { return '8' }
	case gpucontext.Key9: if shift { return '(' } else { return '9' }
	case gpucontext.KeySpace: return ' '
	case gpucontext.KeyMinus: if shift { return '_' } else { return '-' }
	case gpucontext.KeyEqual: if shift { return '+' } else { return '=' }
	case gpucontext.KeyLeftBracket: if shift { return '{' } else { return '[' }
	case gpucontext.KeyRightBracket: if shift { return '}' } else { return ']' }
	case gpucontext.KeyBackslash: if shift { return '|' } else { return '\\' }
	case gpucontext.KeySemicolon: if shift { return ':' } else { return ';' }
	case gpucontext.KeyApostrophe: if shift { return '"' } else { return '\'' }
	case gpucontext.KeyComma: if shift { return '<' } else { return ',' }
	case gpucontext.KeyPeriod: if shift { return '>' } else { return '.' }
	case gpucontext.KeySlash: if shift { return '?' } else { return '/' }
	}
	return 0
}

func loadGogpuFont(size float64) (text.Face, int, int) {
	candidates :=[]string{
		"C:\\Windows\\Fonts\\consola.ttf",
		"C:\\Windows\\Fonts\\arial.ttf",
		"/usr/share/fonts/truetype/ubuntu/UbuntuMono-R.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf",
		"/System/Library/Fonts/Monaco.ttf",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			src, err := text.NewFontSourceFromFile(p)
			if err == nil {
				face := src.Face(size)
				metrics := face.Metrics()
				cellH := int(metrics.Ascent + metrics.Descent + 0.5)
				cellW := int(face.Advance("A") + 0.5)
				if cellW == 0 { cellW = 8 }
				if cellH == 0 { cellH = 16 }
				DebugLog("GOGPU_HOST: Loaded font %s (Cell size: %dx%d)", p, cellW, cellH)
				return face, cellW, cellH
			}
		}
	}
	return nil, 8, 16
}
