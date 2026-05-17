//go:build !freebsd

package vtui

import (
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/gogpu/gg/text"
	"github.com/gogpu/gogpu"
	"github.com/gogpu/gpucontext"
	"github.com/unxed/vtinput"
)

var (
	debugLastMouseX, debugLastMouseY float64 = -1, -1
	debugLastCtxW, debugLastCtxH     int     = -1, -1
)

func getX11ManualScale() float64 {
	if runtime.GOOS == "linux" && os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") != "" {
		dpi := 96.0
		tempConn, err := xgb.NewConn()
		if err == nil && tempConn != nil {
			setup := xproto.Setup(tempConn)
			screen := setup.DefaultScreen(tempConn)
			if screen.WidthInMillimeters > 0 {
				dpi = (float64(screen.WidthInPixels) * 25.4) / float64(screen.WidthInMillimeters)
			}
			tempConn.Close()
		}
		if dpi > 120 {
			return 2.0
		}
	}
	return 1.0
}

type GogpuHost struct {
	mu                   sync.Mutex
	app                  *gogpu.App
	reader               *vtinput.Reader
	scr                  *ScreenBuf
	cols, rows           int
	physCellW, physCellH int
	logCellW, logCellH   int
	face                 text.Face // Physical face for rendering
	gogpuScale           float64
	lastW, lastH         int
	ctx                  *gogpu.Context
	mouseBtn             uint32
	currentMods          vtinput.ControlKeyState
	pendingKeyEvent      *vtinput.InputEvent
	pendingKeyTimer      *time.Timer
}

func (h *GogpuHost) syncMods(vk uint16, mods gpucontext.Modifiers, isDown bool) vtinput.ControlKeyState {
	var sysMods vtinput.ControlKeyState
	if mods.HasShift() { sysMods |= vtinput.ShiftPressed }
	if mods.HasControl() { sysMods |= vtinput.LeftCtrlPressed }
	if mods.HasAlt() { sysMods |= vtinput.LeftAltPressed }

	if isDown {
		if vk == vtinput.VK_SHIFT { sysMods |= vtinput.ShiftPressed }
		if vk == vtinput.VK_CONTROL { sysMods |= vtinput.LeftCtrlPressed }
		if vk == vtinput.VK_MENU { sysMods |= vtinput.LeftAltPressed }
	} else {
		if vk == vtinput.VK_SHIFT { sysMods &^= vtinput.ShiftPressed }
		if vk == vtinput.VK_CONTROL { sysMods &^= vtinput.LeftCtrlPressed }
		if vk == vtinput.VK_MENU { sysMods &^= vtinput.LeftAltPressed }
	}

	h.currentMods = sysMods
	return sysMods
}
func (h *GogpuHost) updateSizedAssets(scale float64) {
	baseFontSize := 18.0

	// Get logical cell size for TUI grid calculations
	logFace, logW, logH := loadGogpuFont(baseFontSize)
	if logFace == nil { // fallback
		h.logCellW, h.logCellH = 8, 16
		h.physCellW, h.physCellH = int(8*scale), int(16*scale)
		h.face = nil
		return
	}
	h.logCellW, h.logCellH = logW, logH

	// Get physical font face and cell size for crisp rendering
	physFace, physW, physH := loadGogpuFont(baseFontSize * scale)
	h.physCellW, h.physCellH = physW, physH
	h.face = physFace

	DebugLog("GOGPU_HOST: Sized assets updated for scale %.1f. Logical cell: %dx%d, Physical cell: %dx%d",
		scale, h.logCellW, h.logCellH, h.physCellW, h.physCellH)
}

func RunGogpuHost(cols, rows int, setupApp func()) error {
	baseFontSize := 18.0
	manualScale := getX11ManualScale()
	_, logW, logH := loadGogpuFont(baseFontSize)

	DebugLog("GOGPU_HOST: Starting RunGogpuHost %dx%d (Logical Cell: %dx%d, X11 Scale Hint: %.1f)", cols, rows, logW, logH, manualScale)

	config := gogpu.DefaultConfig().
		WithTitle(AppName).
		WithSize(int(float64(cols*logW)*manualScale), int(float64(rows*logH)*manualScale))

	app := gogpu.NewApp(config)

	host := &GogpuHost{
		app:        app,
		cols:       cols,
		rows:       rows,
		gogpuScale: app.ScaleFactor(),
	}

	host.updateSizedAssets(host.gogpuScale)

	scr := NewScreenBuf()
	host.scr = scr
	scr.AllocBuf(cols, rows)
	renderer := NewGogpuRenderer(host, host.face, host.physCellW, host.physCellH)
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
		if vk != 0 {
			DebugLog("GOGPU_HOST_EVENT: OnKeyPress key=%v, vk=%d", key, vk)
		}

		host.mu.Lock()
		currMods := host.syncMods(vk, mods, true)

		// Сбрасываем предыдущее событие, если оно зависло
		if host.pendingKeyEvent != nil {
			if host.pendingKeyTimer != nil {
				host.pendingKeyTimer.Stop()
			}
			host.reader.NativeEventChan <- host.pendingKeyEvent
			host.pendingKeyEvent = nil
		}

		if vk != 0 {
			host.pendingKeyEvent = &vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				VirtualKeyCode:  vk,
				ControlKeyState: currMods,
			}
			// Отложенная отправка: если OnText не придет в течение 2мс, отправляем только код клавиши
			host.pendingKeyTimer = time.AfterFunc(2*time.Millisecond, func() {
				host.mu.Lock()
				defer host.mu.Unlock()
				if host.pendingKeyEvent != nil {
					host.reader.NativeEventChan <- host.pendingKeyEvent
					host.pendingKeyEvent = nil
				}
			})
		}
		host.mu.Unlock()
	})

	app.EventSource().OnTextInput(func(text string) {
		DebugLog("GOGPU_HOST_EVENT: OnTextInput text=%q", text)
		host.mu.Lock()
		defer host.mu.Unlock()

		runes := []rune(text)
		if len(runes) == 0 {
			return
		}

		if host.pendingKeyEvent != nil {
			if host.pendingKeyTimer != nil {
				host.pendingKeyTimer.Stop()
			}
			// Сливаем первый символ с ожидающим событием OnKeyPress
			host.pendingKeyEvent.Char = runes[0]
			host.reader.NativeEventChan <- host.pendingKeyEvent
			host.pendingKeyEvent = nil

			// Остальные символы отправляем отдельными событиями
			for i := 1; i < len(runes); i++ {
				host.reader.NativeEventChan <- &vtinput.InputEvent{
					Type:            vtinput.KeyEventType,
					KeyDown:         true,
					Char:            runes[i],
					ControlKeyState: host.currentMods,
				}
			}
		} else {
			// Если OnText пришел без OnKeyPress, просто отправляем текст
			for _, r := range runes {
				host.reader.NativeEventChan <- &vtinput.InputEvent{
					Type:            vtinput.KeyEventType,
					KeyDown:         true,
					Char:            r,
					ControlKeyState: host.currentMods,
				}
			}
		}
	})

	app.EventSource().OnKeyRelease(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		vk := gogpuKeyToVK(key)

		host.mu.Lock()
		currMods := host.syncMods(vk, mods, false)

		// Принудительно сбрасываем залипшее нажатие перед отпусканием
		if host.pendingKeyEvent != nil {
			if host.pendingKeyTimer != nil {
				host.pendingKeyTimer.Stop()
			}
			host.reader.NativeEventChan <- host.pendingKeyEvent
			host.pendingKeyEvent = nil
		}
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
		cW := host.physCellW
		cH := host.physCellH
		host.mu.Unlock()

		if x != debugLastMouseX || y != debugLastMouseY {
			debugLastMouseX, debugLastMouseY = x, y
		}

		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          uint16(int(x) / cW),
			MouseY:          uint16(int(y) / cH),
			MouseEventFlags: vtinput.MouseMoved,
			ButtonState:     btn,
		}
	})

	app.EventSource().OnMousePress(func(button gpucontext.MouseButton, x, y float64) {
		btn := uint32(vtinput.FromLeft1stButtonPressed)

		host.mu.Lock()
		host.mouseBtn = btn
		cW := host.physCellW
		cH := host.physCellH
		host.mu.Unlock()

		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      uint16(int(x) / cW),
			MouseY:      uint16(int(y) / cH),
			KeyDown:     true,
			ButtonState: btn,
		}
	})

	app.EventSource().OnMouseRelease(func(button gpucontext.MouseButton, x, y float64) {
		host.mu.Lock()
		host.mouseBtn = 0
		cW := host.physCellW
		cH := host.physCellH
		host.mu.Unlock()

		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      uint16(int(x) / cW),
			MouseY:      uint16(int(y) / cH),
			KeyDown:     false,
			ButtonState: 0,
		}
	})

	var infoLogged sync.Once
	app.OnDraw(func(dc *gogpu.Context) {
		host.mu.Lock()
		currentScale := dc.ScaleFactor()
		if host.gogpuScale != currentScale {
			DebugLog("GOGPU_HOST: Scale changed %.1f -> %.1f. Recalculating assets.", host.gogpuScale, currentScale)
			host.gogpuScale = currentScale
			host.updateSizedAssets(currentScale)
			if r, ok := host.scr.Renderer.(*GogpuRenderer); ok {
				r.mu.Lock()
				r.face = host.face
				r.cellW = host.physCellW
				r.cellH = host.physCellH
				r.mu.Unlock()
			}
			if host.reader != nil && host.reader.NativeEventChan != nil {
				host.reader.NativeEventChan <- &vtinput.InputEvent{Type: vtinput.ResizeEventType}
			}
		}
		host.mu.Unlock()

		infoLogged.Do(func() {
			if provider := app.GPUContextProvider(); provider != nil {
				info := provider.AdapterInfo()
				DebugLog("GOGPU_HOST_ON_DRAW: Adapter confirmed: %q, Type: %v", info.Name, info.Type)
			}
		})

		startDraw := time.Now()
		host.mu.Lock()
		host.ctx = dc
		host.mu.Unlock()

		host.scr.Renderer.Flush()

		host.mu.Lock()
		host.ctx = nil
		host.mu.Unlock()

		dur := time.Since(startDraw)
		if dur > 5*time.Millisecond {
			DebugLog("GOGPU_HOST: OnDraw execution time: %v", dur)
		}
	})

	GetTerminalSize = func() (int, int, error) {
		fw, fh := app.PhysicalSize()
		s := app.ScaleFactor()
		if s <= 0 {
			s = 1.0
		}
		w, h := int(float64(fw)/s), int(float64(fh)/s)

		host.mu.Lock()
		defer host.mu.Unlock()
		if host.logCellW > 0 && host.logCellH > 0 && w > 0 && h > 0 {
			c := w / host.logCellW
			r := h / host.logCellH
			if c != host.cols || r != host.rows {
				host.cols = c
				host.rows = r
			}
		}
		return host.cols, host.rows, nil
	}

	setupApp()

	go func() {
		w, h := app.Size()
		fw, fh := app.PhysicalSize()
		DebugLog("GOGPU_HOST: Before Run(). App Size (Log): %dx%d. App PhysicalSize: %dx%d. ScaleFactor: %f", w, h, fw, fh, app.ScaleFactor())

		provider := app.GPUContextProvider()
		if provider != nil {
			info := provider.AdapterInfo()
			DebugLog("GOGPU_HOST: Adapter: Name=%q, Type=%v", info.Name, info.Type)
		}

		DebugLog("GOGPU_HOST: FrameManager starting...")
		FrameManager.Run(reader)
		DebugLog("GOGPU_HOST: FrameManager exited. Forcing app shutdown to prevent blue screen hang.")
		os.Exit(0)
	}()

	return app.Run()
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
				adv := face.Advance("A")
				cellH := int(metrics.Ascent + metrics.Descent + 0.5)
				cellW := int(adv + 0.5)
				if cellW == 0 { cellW = 8 }
				if cellH == 0 { cellH = 16 }
				DebugLog("GOGPU_DIAG_FONT: File=%s RequestSize=%.1f", p, size)
				DebugLog("GOGPU_DIAG_FONT: Metrics: Ascent=%.2f Descent=%.2f LineGap=%.2f AdvanceA=%.2f",
					float64(metrics.Ascent), float64(metrics.Descent), float64(metrics.LineGap), adv)
				DebugLog("GOGPU_DIAG_FONT: Calculated Cell: %dx%d", cellW, cellH)
				return face, cellW, cellH
			}
		}
	}
	return nil, 8, 16
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
