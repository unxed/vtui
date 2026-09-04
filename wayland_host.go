//go:build linux && !android && (amd64 || arm64)

package vtui

import (
	"fmt"
	"image"
	"io"
	"math"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neurlang/wayland/window"
	"github.com/neurlang/wayland/wl"
	"github.com/unxed/vtinput"
)

// waylandPresentWake coalesces redraw requests until the Wayland event loop
// has processed a sync callback. Flush can be called by FrameManager's
// goroutine, while ScheduleRedraw must run on the DisplayRun goroutine.
type waylandPresentWake struct {
	mu          sync.Mutex
	pending     bool
	wakePending bool
	closed      bool
}

func (w *waylandPresentWake) request(send func() error) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.pending = true
	if w.wakePending {
		w.mu.Unlock()
		return
	}
	w.wakePending = true
	w.mu.Unlock()

	if err := send(); err != nil {
		w.mu.Lock()
		w.wakePending = false
		w.mu.Unlock()
	}
}

func (w *waylandPresentWake) done() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	w.wakePending = false
	pending := w.pending
	w.pending = false
	return pending
}

func (w *waylandPresentWake) close() {
	w.mu.Lock()
	w.closed = true
	w.pending = false
	w.wakePending = false
	w.mu.Unlock()
}

// WaylandHost encapsulates the connection to the Wayland compositor.
type WaylandHost struct {
	mu      sync.Mutex
	display *window.Display
	win     *window.Window
	widget  *window.Widget

	// exiting is set just before the host asks the display loop to stop.
	// If DisplayRun returns without it, the loop stopped on its own --
	// the compositor closed the connection -- and that is reported.
	exiting atomic.Bool
	reader  *vtinput.Reader
	present waylandPresentWake

	imgBuf     *image.RGBA
	cols, rows int
	cellW      int
	cellH      int
	scale      float64
	fontName   string
	fontSize   float64
	screen     *ScreenBuf
	renderer   *WaylandRenderer

	mouseX         int
	mouseY         int
	mouseBtn       uint32
	lastMouseCellX int
	lastMouseCellY int
	mouseCellKnown bool

	axisValue             [2]float64
	axisDiscrete          [2]int32
	axisValue120          [2]int32
	axisStopped           [2]bool
	axisPixelRemainder    [2]float64
	axisValue120Remainder [2]int32

	isRepeating    bool
	repeatVK       uint16
	repeatChar     rune
	repeatMods     vtinput.ControlKeyState
	repeatNext     time.Time
	currentMods    vtinput.ControlKeyState
	numLockOn      bool
	numLockKnown   bool
	lCtrl, rCtrl   bool
	lAlt, rAlt     bool
	lShift, rShift bool
}

func runInWaylandWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	d, err := window.DisplayCreate([]string{})
	if err != nil {
		return err
	}

	if fontSize <= 0 {
		fontSize = 18.0
	}
	// The host starts at 1x and updates itself from the physical dimensions
	// supplied by the Wayland surface. Fractional-scale compositors report a
	// 1.5x backing buffer for a 150% output, for example.
	dpi := 72.0
	face, cellW, cellH := loadBestFont(fontName, fontSize, dpi)

	host := &WaylandHost{
		display:  d,
		cols:     cols,
		rows:     rows,
		cellW:    cellW,
		cellH:    cellH,
		scale:    1,
		fontName: fontName,
		fontSize: fontSize,
		imgBuf:   image.NewRGBA(image.Rect(0, 0, cols*cellW, rows*cellH)),
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
	host.screen = scr
	host.renderer = NewWaylandRenderer(host, face)
	scr.Renderer = host.renderer
	scr.Graphics().SetProtocol(GraphicsNative)
	scr.Graphics().SetCellSize(cellW, cellH)
	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	host.widget.ScheduleResize(logicalWaylandPixels(cols*cellW, host.scale), logicalWaylandPixels(rows*cellH, host.scale))

	setupApp()
	// After setupApp: the application installs the debug log sink during
	// setup, so a backend announced before it is logged nowhere.
	SetActiveBackend("wayland")

	// FrameManager must run in a goroutine because Wayland's DisplayRun blocks the main thread
	go func() {
		FrameManager.Run(reader)
		host.Close()
		// On exit, close Wayland display
		host.exiting.Store(true)
		host.display.Exit()
	}()

	// Blocks until application exit
	window.DisplayRun(d)

	// DisplayRun also returns when the compositor closes the connection,
	// which is what a Wayland protocol error looks like from the client
	// side: the library prints the read error with fmt.Println (stdout)
	// and returns, and the application then exits normally, with nothing
	// in the crash log to say why the window vanished. Say so here, on
	// stderr, where the crash log is.
	if !host.exiting.Load() {
		msg := "wayland: the compositor closed the connection (a protocol error on our side, or the compositor went away); the read error, if any, is printed on stdout just above."
		fmt.Fprintln(os.Stderr, msg)
		DebugLog("WAYLAND: %s", msg)
	}

	host.widget.Destroy()
	host.win.Destroy()
	d.Destroy()

	return nil
}

// -- window.WidgetHandler Implementation --

func (h *WaylandHost) Resize(widget *window.Widget, width int32, height int32, pwidth int32, pheight int32) {
	// xdg_toplevel is allowed to send an initial 0x0 configure while the
	// surface is being mapped. It is not a usable pixel size: accepting it
	// would replace the backing image with an empty one and permanently zero
	// the terminal grid before the first real configure arrives.
	if !hasWaylandPixelSize(width, height, pwidth, pheight) {
		DebugLog("WAYLAND: ignoring zero-sized configure logical=%dx%d pixels=%dx%d", width, height, pwidth, pheight)
		return
	}

	h.mu.Lock()
	targetCols, targetRows := h.cols, h.rows
	scaleChanged := h.updateScaleLocked(waylandScaleFromDimensions(width, height, pwidth, pheight))
	scale, cellW, cellH := h.scale, h.cellW, h.cellH
	cols, rows := h.cols, h.rows
	pixelSizeChanged := int(pwidth) != h.imgBuf.Rect.Dx() || int(pheight) != h.imgBuf.Rect.Dy()
	if pixelSizeChanged {
		h.imgBuf = image.NewRGBA(image.Rect(0, 0, int(pwidth), int(pheight)))
		if !scaleChanged {
			h.cols = int(pwidth) / h.cellW
			h.rows = int(pheight) / h.cellH
		}
		cols, rows = h.cols, h.rows
		h.mu.Unlock()

		if h.reader != nil {
			h.reader.EventChan <- &vtinput.InputEvent{Type: vtinput.ResizeEventType}
		}
	} else {
		h.mu.Unlock()
	}
	DebugLog("WAYLAND: resize logical=%dx%d pixels=%dx%d scale=%.2f cell=%dx%d grid=%dx%d", width, height, pwidth, pheight, scale, cellW, cellH, cols, rows)
	widget.SetAllocation(0, 0, width, height)
	if scaleChanged && targetCols > 0 && targetRows > 0 {
		widget.ScheduleResize(logicalWaylandPixels(targetCols*cellW, scale), logicalWaylandPixels(targetRows*cellH, scale))
	}
	// The first call to Resize confirms the window is mapped and ready.
	// This is the correct place to trigger the initial draw.
	if FrameManager != nil {
		if pixelSizeChanged {
			// The new backing buffer is blank. Force every cell to be painted
			// again instead of relying on the renderer's dirty-cell shadow.
			FrameManager.HardRefresh()
		} else {
			FrameManager.Redraw()
		}
	}
}

func hasWaylandPixelSize(width, height, pwidth, pheight int32) bool {
	return width > 0 && height > 0 && pwidth > 0 && pheight > 0
}

func waylandScaleFromDimensions(width, height, pwidth, pheight int32) float64 {
	scale := 0.0
	if width > 0 && pwidth > 0 {
		scale = math.Max(scale, float64(pwidth)/float64(width))
	}
	if height > 0 && pheight > 0 {
		scale = math.Max(scale, float64(pheight)/float64(height))
	}
	if scale <= 0 {
		return 1
	}
	return scale
}

func logicalWaylandPixels(physical int, scale float64) int32 {
	if scale <= 0 {
		scale = 1
	}
	logical := int32(math.Round(float64(physical) / scale))
	if physical > 0 && logical < 1 {
		return 1
	}
	return logical
}

func (h *WaylandHost) updateScaleLocked(scale float64) bool {
	if scale <= 0 || math.Abs(h.scale-scale) < 0.001 {
		return false
	}
	face, cellW, cellH := loadBestFont(h.fontName, h.fontSize, 72.0*float64(scale))
	h.scale = scale
	h.cellW = cellW
	h.cellH = cellH
	if h.renderer != nil {
		h.renderer.setFace(face)
	}
	if h.screen != nil {
		h.screen.Graphics().SetCellSize(cellW, cellH)
	}
	return true
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
				dst[dIdx] = src.Pix[sIdx+2]   // B
				dst[dIdx+1] = src.Pix[sIdx+1] // G
				dst[dIdx+2] = src.Pix[sIdx]   // R
				dst[dIdx+3] = 255             // A
			}
		}
		surface.Destroy() // Commits the buffer
	}
	// Note: Normal redraws are driven by vtui.Flush() calling widget.ScheduleRedraw().
	// However, during key repeat, we intentionally spin the event loop here
	// to prevent it from sleeping, which allows the timer to tick safely on the main thread.
	if h.isRepeating {
		now := time.Now()
		if now.After(h.repeatNext) || now.Equal(h.repeatNext) {
			h.repeatNext = now.Add(40 * time.Millisecond)
			if h.reader != nil {
				// Non-blocking send to prevent deadlocks
				select {
				case h.reader.EventChan <- &vtinput.InputEvent{
					Type:            vtinput.KeyEventType,
					KeyDown:         true,
					VirtualKeyCode:  h.repeatVK,
					Char:            h.repeatChar,
					ControlKeyState: h.repeatMods,
				}:
				default:
				}
			}
		}
		widget.ScheduleRedraw()
	}
}

func (h *WaylandHost) Close() {
	h.mu.Lock()
	h.reader = nil
	h.mu.Unlock()
	h.present.close()
}

// requestPresent asks the compositor to send a callback. The callback wakes
// DisplayRun from its blocking socket read and schedules the deferred redraw
// on the display goroutine, avoiding a cross-goroutine toolkit call.
func (h *WaylandHost) requestPresent() {
	h.mu.Lock()
	var display *wl.Display
	if h.display != nil {
		display = h.display.Display
	}
	h.mu.Unlock()
	if display == nil {
		return
	}

	h.present.request(func() error {
		ctx := display.Context()
		if ctx == nil {
			return wl.ErrContextNil
		}
		callback := wl.NewCallback(ctx)
		// Register before sending the request: DisplayRun may receive the
		// callback as soon as the compositor processes the sync request.
		callback.AddDoneHandler(h)
		if err := ctx.SendRequest(display, 0, callback); err != nil {
			callback.RemoveDoneHandler(h)
			callback.Unregister()
			return err
		}
		return nil
	})
}

// HandleCallbackDone implements wl.CallbackDoneHandler.
func (h *WaylandHost) HandleCallbackDone(event wl.CallbackDoneEvent) {
	if event.C != nil {
		event.C.Unregister()
	}
	if !h.present.done() {
		return
	}

	h.mu.Lock()
	widget := h.widget
	h.mu.Unlock()
	if widget != nil {
		widget.ScheduleRedraw()
	}
}

// -- Pointer & Keyboard Handlers --

func (h *WaylandHost) Enter(w *window.Widget, input *window.Input, x float32, y float32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mouseX, h.mouseY = int(x), int(y)
}
func (h *WaylandHost) Leave(w *window.Widget, input *window.Input) {
	h.mu.Lock()
	h.isRepeating = false
	h.mu.Unlock()
}

func (h *WaylandHost) Motion(w *window.Widget, input *window.Input, time uint32, x float32, y float32) int {
	h.mu.Lock()
	h.mouseX, h.mouseY = int(x), int(y)
	mouseX, mouseY := h.mouseCellLocked()
	mouseBtn := h.mouseBtn
	moved := !h.mouseCellKnown || mouseX != h.lastMouseCellX || mouseY != h.lastMouseCellY
	if mouseBtn != 0 && moved {
		h.lastMouseCellX, h.lastMouseCellY = mouseX, mouseY
		h.mouseCellKnown = true
	}
	h.mu.Unlock()

	// VTUI consumes mouse coordinates in cells. Sending every pixel-level
	// Wayland motion can fill the input queue faster than a drag can be drawn,
	// so only report a held-button move after the pointer enters another cell.
	if h.reader != nil && mouseBtn != 0 && moved {
		h.reader.EventChan <- &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			KeyDown:         true,
			MouseX:          int16(mouseX),
			MouseY:          int16(mouseY),
			MouseEventFlags: vtinput.MouseMoved,
			ButtonState:     mouseBtn,
			ControlKeyState: h.modsForPointer(input),
		}
	}
	return window.CursorLeftPtr
}

func (h *WaylandHost) Button(w *window.Widget, input *window.Input, time uint32, button uint32, state wl.PointerButtonState, handler window.WidgetHandler) {
	isDown := state == wl.PointerButtonStatePressed
	bs := uint32(0)

	// Wayland standard button codes (linux/input-event-codes.h)
	switch button {
	case 272:
		bs = vtinput.FromLeft1stButtonPressed // BTN_LEFT
	case 273:
		bs = vtinput.RightmostButtonPressed // BTN_RIGHT
	case 274:
		bs = vtinput.FromLeft2ndButtonPressed // BTN_MIDDLE
	}

	h.mu.Lock()
	if isDown {
		h.mouseBtn |= bs
	} else {
		h.mouseBtn &^= bs
	}
	bs = h.mouseBtn
	mouseX, mouseY := h.mouseCellLocked()
	h.lastMouseCellX, h.lastMouseCellY = mouseX, mouseY
	h.mouseCellKnown = true
	h.mu.Unlock()

	if h.reader != nil {
		h.reader.EventChan <- &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			KeyDown:         isDown,
			MouseX:          int16(mouseX),
			MouseY:          int16(mouseY),
			ButtonState:     bs,
			ControlKeyState: h.modsForPointer(input),
		}
	}
}

func (h *WaylandHost) mouseCellLocked() (int, int) {
	if h.cellW <= 0 || h.cellH <= 0 {
		return 0, 0
	}
	return int(float64(h.mouseX) * h.scale / float64(h.cellW)), int(float64(h.mouseY) * h.scale / float64(h.cellH))
}

func (h *WaylandHost) AxisDiscrete(w *window.Widget, input *window.Input, axis uint32, discrete int32) {
	if axis >= uint32(len(h.axisDiscrete)) {
		return
	}
	h.mu.Lock()
	h.axisDiscrete[axis] += discrete
	h.mu.Unlock()
}

// AxisValue120 receives high-resolution wheel information from wl_pointer
// version 8 and newer. It is an optional extension exposed by the Wayland
// window package, so older window versions and non-wheel devices continue to
// work through AxisDiscrete or Axis respectively.
func (h *WaylandHost) AxisValue120(w *window.Widget, input *window.Input, axis uint32, value120 int32) {
	if axis >= uint32(len(h.axisValue120)) {
		return
	}
	h.mu.Lock()
	h.axisValue120[axis] += value120
	h.mu.Unlock()
}

func (h *WaylandHost) Key(win *window.Window, input *window.Input, timeMs uint32, key uint32, notUnicode uint32, state wl.KeyboardKeyState, handler window.WidgetHandler) {
	isDown := state == wl.KeyboardKeyStatePressed
	vk := keysymToVK(notUnicode) // Reuse the XKB keysym to VK mapping from X11

	char := input.GetRune(&notUnicode, key)

	h.mu.Lock()
	if isDown {
		if vk == vtinput.VK_LCONTROL {
			h.lCtrl = true
		}
		if vk == vtinput.VK_RCONTROL {
			h.rCtrl = true
		}
		if vk == vtinput.VK_LMENU {
			h.lAlt = true
		}
		if vk == vtinput.VK_RMENU {
			h.rAlt = true
		}
		if vk == vtinput.VK_LSHIFT {
			h.lShift = true
		}
		if vk == vtinput.VK_RSHIFT {
			h.rShift = true
		}
	} else {
		if vk == vtinput.VK_LCONTROL {
			h.lCtrl = false
		}
		if vk == vtinput.VK_RCONTROL {
			h.rCtrl = false
		}
		if vk == vtinput.VK_LMENU {
			h.lAlt = false
		}
		if vk == vtinput.VK_RMENU {
			h.rAlt = false
		}
		if vk == vtinput.VK_LSHIFT {
			h.lShift = false
		}
		if vk == vtinput.VK_RSHIFT {
			h.rShift = false
		}
	}
	h.mu.Unlock()

	mods := h.getMods(input)
	h.mu.Lock()
	if isDown && vk == vtinput.VK_NUMLOCK && h.numLockKnown {
		h.numLockOn = !h.numLockOn
	}
	if numLockOn, ok := waylandNumLockFromKeysym(notUnicode, mods&vtinput.ShiftPressed != 0); ok {
		h.numLockOn = numLockOn
		h.numLockKnown = true
	}
	if h.numLockKnown && h.numLockOn {
		mods |= vtinput.NumLockOn
	} else {
		mods &^= vtinput.NumLockOn
	}
	h.syncModifierStateLocked(mods, vk, isDown)
	h.currentMods = mods
	h.mu.Unlock()

	mods |= enhancedKeyForX11Keysym(notUnicode)

	h.mu.Lock()
	if isDown && waylandKeyCanRepeat(vk) {
		h.isRepeating = true
		h.repeatVK = vk
		h.repeatChar = char
		h.repeatMods = mods
		h.repeatNext = time.Now().Add(400 * time.Millisecond)
		// Force an immediate redraw to start the spin loop
		h.widget.ScheduleRedraw()
	} else if !isDown && h.repeatVK == vk {
		h.stopKeyRepeatLocked()
	}
	h.mu.Unlock()

	if h.reader != nil {
		h.reader.EventChan <- &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         isDown,
			VirtualKeyCode:  vk,
			Char:            char,
			ControlKeyState: mods,
		}
	}
}

func waylandKeyCanRepeat(vk uint16) bool {
	switch vk {
	case vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_LWIN, vtinput.VK_RWIN,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return false
	default:
		return true
	}
}

func (h *WaylandHost) stopKeyRepeatLocked() {
	h.isRepeating = false
	h.repeatVK = 0
	h.repeatChar = 0
	h.repeatMods = 0
	h.repeatNext = time.Time{}
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
		h.mu.Lock()
		rCtrl := h.rCtrl
		h.mu.Unlock()
		if rCtrl {
			mods |= vtinput.RightCtrlPressed
		} else {
			mods |= vtinput.LeftCtrlPressed
		}
	}
	if m&window.ModAltMask != 0 {
		h.mu.Lock()
		rAlt := h.rAlt
		h.mu.Unlock()
		if rAlt {
			mods |= vtinput.RightAltPressed
		} else {
			mods |= vtinput.LeftAltPressed
		}
	}
	return mods
}

func isWaylandModifierVK(vk uint16) bool {
	switch vk {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LWIN, vtinput.VK_RWIN, vtinput.VK_APPS,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return true
	default:
		return false
	}
}

// syncModifierStateLocked repairs the side-tracking flags from Wayland's
// aggregate modifier mask. The window package can lose a modifier key release
// when keyboard focus changes, and it can also report the aggregate mask
// before the corresponding key event updates it. Keep the modifier key's own
// transition authoritative, while using ordinary events to recover a missing
// side and inactive masks to clear stale sides.
func (h *WaylandHost) syncModifierStateLocked(mods vtinput.ControlKeyState, vk uint16, isDown bool) {
	assumeActive := !isWaylandModifierVK(vk)
	keepShift := isDown && (vk == vtinput.VK_SHIFT || vk == vtinput.VK_LSHIFT || vk == vtinput.VK_RSHIFT)
	keepCtrl := isDown && (vk == vtinput.VK_CONTROL || vk == vtinput.VK_LCONTROL || vk == vtinput.VK_RCONTROL)
	keepAlt := isDown && (vk == vtinput.VK_MENU || vk == vtinput.VK_LMENU || vk == vtinput.VK_RMENU)

	if mods&vtinput.ShiftPressed == 0 && !keepShift {
		h.lShift, h.rShift = false, false
	} else if mods&vtinput.ShiftPressed != 0 && assumeActive && !h.lShift && !h.rShift {
		h.lShift = true
	}
	if mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) == 0 && !keepCtrl {
		h.lCtrl, h.rCtrl = false, false
	} else if mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0 && assumeActive && !h.lCtrl && !h.rCtrl {
		h.lCtrl = true
	}
	if mods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) == 0 && !keepAlt {
		h.lAlt, h.rAlt = false, false
	} else if mods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 && assumeActive && !h.lAlt && !h.rAlt {
		h.lAlt = true
	}
}

func (h *WaylandHost) modsForPointer(input *window.Input) vtinput.ControlKeyState {
	mods := h.getMods(input)
	if input == nil {
		// A nil callback input carries no modifier snapshot; do not treat it as
		// an assertion that all currently tracked modifiers are released.
		return mods
	}
	h.mu.Lock()
	h.syncModifierStateLocked(mods, 0, false)
	h.mu.Unlock()
	return mods
}

func (h *WaylandHost) sendFocusEvent(focused bool) {
	h.mu.Lock()
	reader := h.reader
	h.mu.Unlock()
	if reader == nil || reader.EventChan == nil {
		return
	}
	defer func() {
		// The reader may close concurrently with the display loop during exit.
		_ = recover()
	}()
	reader.EventChan <- &vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: focused}
}

func (h *WaylandHost) Focus(w *window.Window, device *window.Input) {
	if device != nil {
		h.sendFocusEvent(true)
		return
	}

	// Wayland does not send key releases to a surface after keyboard focus has
	// left it. Drop all locally tracked state so a held modifier or repeat key
	// cannot remain active indefinitely when the window later regains focus.
	h.mu.Lock()
	h.stopKeyRepeatLocked()
	h.currentMods = 0
	h.numLockOn = false
	h.numLockKnown = false
	h.lCtrl, h.rCtrl = false, false
	h.lAlt, h.rAlt = false, false
	h.lShift, h.rShift = false, false
	h.mu.Unlock()
	h.sendFocusEvent(false)
}

// waylandNumLockFromKeysym recovers the XKB NumLock state from a keypad
// keysym. Unlike an ordinary modifier, NumLock is already consumed by XKB
// while selecting the keysym, so the Wayland window package does not expose
// it through Input.GetModifiers. Shift reverses the keypad selection: a
// numeric keysym means NumLock is on without Shift and off with Shift; a
// navigation keysym means the opposite.
func waylandNumLockFromKeysym(keysym uint32, shift bool) (bool, bool) {
	switch {
	case keysym >= 0xffb0 && keysym <= 0xffb9, // XK_KP_0 .. XK_KP_9
		keysym == 0xffae: // XK_KP_Decimal
		return !shift, true
	case keysym >= 0xff95 && keysym <= 0xff9f: // XK_KP_Home .. XK_KP_Delete
		return shift, true
	default:
		return false, false
	}
}

// Unused Handlers to satisfy interface
func (h *WaylandHost) TouchUp(w *window.Widget, i *window.Input, serial uint32, time uint32, id int32) {
}
func (h *WaylandHost) TouchDown(w *window.Widget, i *window.Input, serial uint32, time uint32, id int32, x float32, y float32) {
}
func (h *WaylandHost) TouchMotion(w *window.Widget, i *window.Input, time uint32, id int32, x float32, y float32) {
}
func (h *WaylandHost) TouchFrame(w *window.Widget, i *window.Input)            {}
func (h *WaylandHost) TouchCancel(w *window.Widget, width int32, height int32) {}
func (h *WaylandHost) Axis(w *window.Widget, i *window.Input, time uint32, axis uint32, value float32) {
	if axis >= uint32(len(h.axisValue)) {
		return
	}
	h.mu.Lock()
	h.axisValue[axis] += float64(value)
	h.mu.Unlock()
}
func (h *WaylandHost) AxisSource(w *window.Widget, i *window.Input, source uint32) {}
func (h *WaylandHost) AxisStop(w *window.Widget, i *window.Input, time uint32, axis uint32) {
	if axis >= uint32(len(h.axisStopped)) {
		return
	}
	h.mu.Lock()
	h.axisStopped[axis] = true
	h.mu.Unlock()
}

func (h *WaylandHost) PointerFrame(w *window.Widget, input *window.Input) {
	const verticalAxis = int(wl.PointerAxisVerticalScroll)

	h.mu.Lock()
	raw := h.axisValue[verticalAxis]
	discrete := h.axisDiscrete[verticalAxis]
	value120 := h.axisValue120[verticalAxis]
	stopped := h.axisStopped[verticalAxis]
	for axis := range h.axisValue {
		h.axisValue[axis] = 0
		h.axisDiscrete[axis] = 0
		h.axisValue120[axis] = 0
		h.axisStopped[axis] = false
	}

	steps := 0
	if value120 != 0 {
		h.axisValue120Remainder[verticalAxis] += value120
		steps = int(h.axisValue120Remainder[verticalAxis] / 120)
		h.axisValue120Remainder[verticalAxis] -= int32(steps * 120)
	} else if discrete != 0 {
		steps = int(discrete)
	} else if raw != 0 {
		threshold := float64(h.cellH) / h.scale
		if threshold < 1 {
			threshold = 1
		}
		h.axisPixelRemainder[verticalAxis] += raw
		steps = int(h.axisPixelRemainder[verticalAxis] / threshold)
		h.axisPixelRemainder[verticalAxis] -= float64(steps) * threshold
	}
	if stopped {
		h.axisPixelRemainder[verticalAxis] = 0
	}
	mouseX, mouseY := h.mouseCellLocked()
	mouseBtn := h.mouseBtn
	reader := h.reader
	h.mu.Unlock()

	if steps == 0 || reader == nil {
		return
	}
	direction := -1
	if steps < 0 {
		direction = 1
		steps = -steps
	}
	mods := h.modsForPointer(input)
	for range steps {
		reader.EventChan <- &vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			KeyDown:         mouseBtn != 0,
			MouseX:          int16(mouseX),
			MouseY:          int16(mouseY),
			ButtonState:     mouseBtn,
			WheelDirection:  direction,
			ControlKeyState: mods,
		}
	}
}
