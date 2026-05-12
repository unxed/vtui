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
	cols     int
	rows     int
	ctx      *gogpu.Context
	mouseBtn uint32
}

func RunGogpuHost(cols, rows int, setupApp func()) error {
	DebugLog("GOGPU_HOST: Starting RunGogpuHost %dx%d (font scaling: 10x20)", cols, rows)
	config := gogpu.DefaultConfig().
		WithTitle(AppName).
		WithSize(cols*10, rows*20)

	app := gogpu.NewApp(config)
	host := &GogpuHost{
		app:  app,
		cols: cols,
		rows: rows,
	}

	fontSize := 18.0
	face, cellW, cellH := loadGogpuFont(fontSize)

	scr := NewScreenBuf()
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
		DebugLog("GOGPU_HOST: Hardware KeyPress: %v", key)
		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         true,
			VirtualKeyCode:  uint16(key),
			ControlKeyState: translateGogpuMods(mods),
		}
	})

	app.EventSource().OnKeyRelease(func(key gpucontext.Key, mods gpucontext.Modifiers) {
		host.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         false,
			VirtualKeyCode:  uint16(key),
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
		host.ctx = dc
		DebugLog("GOGPU_HOST: OnDraw callback from GPU driver")
		FrameManager.renderPhase()
		host.ctx = nil
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
	*/
	return mods
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
