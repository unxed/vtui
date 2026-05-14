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

type GogpuHost struct {
	mu       sync.Mutex
	app      *gogpu.App
	reader   *vtinput.Reader
	scr      *ScreenBuf
	cols     int
	rows     int
	ctx      *gogpu.Context
	mouseBtn uint32
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
		app:  app,
		cols: cols,
		rows: rows,
	}

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
		if vk == 0 { return }
		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  vk,
			Char:            gogpuKeyToChar(key, false /*mods.IsShift()*/),
			ControlKeyState: translateGogpuMods(mods),
		}
	})

	app.EventSource().OnKeyRelease(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		vk := gogpuKeyToVK(key)
		if vk == 0 { return }
		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         false,
			VirtualKeyCode:  vk,
			ControlKeyState: translateGogpuMods(mods),
		}
	})

	app.EventSource().OnMouseMove(func(x, y float64) {
		host.mu.Lock()
		btn := host.mouseBtn
		host.mu.Unlock()
		DebugLog("GOGPU_HOST: Raw Mouse Move: %f, %f -> Cell: %d, %d", x, y, int(x)/cellW, int(y)/cellH)
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

	go FrameManager.Run(reader)

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
	}

	if k >= gpucontext.KeyA && k <= gpucontext.KeyZ {
		return uint16(k)
	}
	if k >= gpucontext.Key0 && k <= gpucontext.Key9 {
		return uint16(k)
	}

	return 0
}

func gogpuKeyToChar(k gpucontext.Key, shift bool) rune {
	if k >= gpucontext.KeyA && k <= gpucontext.KeyZ {
		if shift { return rune(k) }
		return rune(k + 32)
	}
	if k >= gpucontext.Key0 && k <= gpucontext.Key9 {
		if shift {
			chars := ")!@#$%^&*("
			return rune(chars[k-gpucontext.Key0])
		}
		return rune(k)
	}
	switch k {
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
	//case gpucontext.KeyGraveAccent: if shift { return '~' } else { return '`' }
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
