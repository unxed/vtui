//go:build linux || openbsd || netbsd || dragonfly

package vtui

import (
	"image"
	"io"
	"sync"

	"github.com/neurlang/wayland/wl"
	window "github.com/neurlang/wayland/windowtrace"
	"github.com/unxed/vtinput"
)

// WaylandHost encapsulates the connection to the Wayland compositor.
type WaylandHost struct {
	mu         sync.Mutex
	display    *window.Display
	win        *window.Window
	widget     *window.Widget
	reader     *vtinput.Reader

	imgBuf     *image.RGBA
	cols, rows int
	cellW      int
	cellH      int
	scale      int

	mouseX     int
	mouseY     int
	mouseBtn   uint32
}

func runInWaylandWindow(cols, rows int, setupApp func()) error {
	d, err := window.DisplayCreate([]string{})
	if err != nil {
		return err
	}

	fontSize := 12.0
	dpi := 96.0 // Wayland scaling is usually handled by the compositor buffers, assuming 96 base
	face, cellW, cellH := loadBestFont(fontSize, dpi)

	host := &WaylandHost{
		display: d,
		cols:    cols,
		rows:    rows,
		cellW:   cellW,
		cellH:   cellH,
		scale:   1, // Will be updated if output scale changes
		imgBuf:  image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH)),
	}

	host.win = window.Create(d)
	host.widget = host.win.AddWidget(host)
	host.win.SetTitle(AppName + " (Wayland)")
	host.win.SetBufferType(window.BufferTypeShm)

	// Set handlers
	host.win.SetKeyboardHandler(host)
	host.widget.SetUserDataWidgetHandler(host)

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = NewWaylandRenderer(host, face)
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

	host.widget.ScheduleResize(int32(cols*cellW), int32(rows*cellH))

	setupApp()

	// FrameManager must run in a goroutine because Wayland's DisplayRun blocks the main thread
	go func() {
		FrameManager.Run(reader)
		// On exit, close Wayland display
		host.display.Exit()
	}()

	// Blocks until application exit
	window.DisplayRun(d)

	host.widget.Destroy()
	host.win.Destroy()
	d.Destroy()

	return nil
}

// -- window.WidgetHandler Implementation --

func (h *WaylandHost) Resize(widget *window.Widget, width int32, height int32, pwidth int32, pheight int32) {
	h.mu.Lock()
	if int(pwidth) != h.imgBuf.Rect.Dx() || int(pheight) != h.imgBuf.Rect.Dy() {
		h.imgBuf = image.NewRGBA(image.Rect(0, 0, int(pwidth), int(pheight)))
		h.cols = int(pwidth) / h.cellW
		h.rows = int(pheight) / h.cellH
		h.mu.Unlock()

		if h.reader != nil {
			h.reader.NativeEventChan <- &vtinput.InputEvent{Type: vtinput.ResizeEventType}
		}
	} else {
		h.mu.Unlock()
	}
	widget.SetAllocation(0, 0, pwidth, pheight)
	// The first call to Resize confirms the window is mapped and ready.
	// This is the correct place to trigger the initial draw.
	if FrameManager != nil {
		FrameManager.Redraw()
	}
}

func (h *WaylandHost) Redraw(widget *window.Widget) {
	h.mu.Lock()
	defer h.mu.Unlock()

	surface := h.win.WindowGetSurface()
	if surface != nil {
		dst := surface.ImageSurfaceGetData()
		stride := surface.ImageSurfaceGetStride()
		width := surface.ImageSurfaceGetWidth()
		height := surface.ImageSurfaceGetHeight()

		src := h.imgBuf
		for y := 0; y < height && y < src.Rect.Dy(); y++ {
			dstOff := y * stride
			srcOff := y * src.Stride
			for x := 0; x < width && x < src.Rect.Dx(); x++ {
				// Cairo format: ARGB32 native (BGRA in memory on little endian)
				dIdx := dstOff + x*4
				sIdx := srcOff + x*4
				dst[dIdx]   = src.Pix[sIdx+2] // B
				dst[dIdx+1] = src.Pix[sIdx+1] // G
				dst[dIdx+2] = src.Pix[sIdx]   // R
				dst[dIdx+3] = 255             // A
			}
		}
		surface.Destroy() // Commits the buffer
	}
	// Note: We DO NOT call widget.ScheduleRedraw() here, otherwise it spins at 100% CPU.
	// Redraws are driven by vtui.Flush() calling widget.ScheduleRedraw().
}

// -- Pointer & Keyboard Handlers --

func (h *WaylandHost) Enter(w *window.Widget, input *window.Input, x float32, y float32) {
	h.mouseX, h.mouseY = int(x), int(y)
}
func (h *WaylandHost) Leave(w *window.Widget, input *window.Input) {}

func (h *WaylandHost) Motion(w *window.Widget, input *window.Input, time uint32, x float32, y float32) int {
	h.mouseX, h.mouseY = int(x), int(y)
	if h.reader != nil {
		h.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          uint16(h.mouseX / h.cellW),
			MouseY:          uint16(h.mouseY / h.cellH),
			MouseEventFlags: vtinput.MouseMoved,
			ButtonState:     h.mouseBtn,
			ControlKeyState: h.getMods(input),
		}
	}
	return window.CursorLeftPtr
}

func (h *WaylandHost) Button(w *window.Widget, input *window.Input, time uint32, button uint32, state wl.PointerButtonState, handler window.WidgetHandler) {
	isDown := state == wl.PointerButtonStatePressed
	bs := uint32(0)

	// Wayland standard button codes (linux/input-event-codes.h)
	switch button {
	case 272: bs = vtinput.FromLeft1stButtonPressed // BTN_LEFT
	case 273: bs = vtinput.RightmostButtonPressed   // BTN_RIGHT
	case 274: bs = vtinput.FromLeft2ndButtonPressed // BTN_MIDDLE
	}

	h.mouseBtn = bs
	if !isDown {
		bs = 0
	}

	if h.reader != nil {
		h.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			KeyDown:         isDown,
			MouseX:          uint16(h.mouseX / h.cellW),
			MouseY:          uint16(h.mouseY / h.cellH),
			ButtonState:     bs,
			ControlKeyState: h.getMods(input),
		}
	}
}

func (h *WaylandHost) AxisDiscrete(w *window.Widget, input *window.Input, axis uint32, discrete int32) {
	dir := 0
	// Wayland axis 0 is vertical scroll.
	if discrete < 0 {
		dir = 1 // Up
	} else if discrete > 0 {
		dir = -1 // Down
	}

	if dir != 0 && h.reader != nil {
		h.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:           vtinput.MouseEventType,
			WheelDirection: dir,
		}
	}
}

func (h *WaylandHost) Key(win *window.Window, input *window.Input, time uint32, key uint32, notUnicode uint32, state wl.KeyboardKeyState, handler window.WidgetHandler) {
	isDown := state == wl.KeyboardKeyStatePressed
	vk := keysymToVK(notUnicode) // Reuse the XKB keysym to VK mapping from X11

	char := input.GetRune(&notUnicode, key)

	if h.reader != nil {
		h.reader.NativeEventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         isDown,
			VirtualKeyCode:  vk,
			Char:            char,
			ControlKeyState: h.getMods(input),
		}
	}
}

func (h *WaylandHost) getMods(input *window.Input) vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	if input == nil {
		return mods
	}
	m := input.GetModifiers()
	if m&window.ModShiftMask != 0 {
		mods |= vtinput.ShiftPressed
	}
	if m&window.ModControlMask != 0 {
		mods |= vtinput.LeftCtrlPressed
	}
	if m&window.ModAltMask != 0 {
		mods |= vtinput.LeftAltPressed
	}
	return mods
}

// Unused Handlers to satisfy interface
func (h *WaylandHost) Focus(w *window.Window, device *window.Input) {}
func (h *WaylandHost) TouchUp(w *window.Widget, i *window.Input, serial uint32, time uint32, id int32) {}
func (h *WaylandHost) TouchDown(w *window.Widget, i *window.Input, serial uint32, time uint32, id int32, x float32, y float32) {}
func (h *WaylandHost) TouchMotion(w *window.Widget, i *window.Input, time uint32, id int32, x float32, y float32) {}
func (h *WaylandHost) TouchFrame(w *window.Widget, i *window.Input) {}
func (h *WaylandHost) TouchCancel(w *window.Widget, width int32, height int32) {}
func (h *WaylandHost) Axis(w *window.Widget, i *window.Input, time uint32, axis uint32, value float32) {}
func (h *WaylandHost) AxisSource(w *window.Widget, i *window.Input, source uint32) {}
func (h *WaylandHost) AxisStop(w *window.Widget, i *window.Input, time uint32, axis uint32) {}
func (h *WaylandHost) PointerFrame(w *window.Widget, i *window.Input) {}
