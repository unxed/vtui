//go:build windows

package vtui

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/unxed/vtinput"
	"golang.org/x/sys/windows"
)

const logPixelsX = 88

var (
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW   = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procShowWindow         = user32.NewProc("ShowWindow")
	procUpdateWindow       = user32.NewProc("UpdateWindow")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procPostMessageW       = user32.NewProc("PostMessageW")
	procBeginPaint         = user32.NewProc("BeginPaint")
	procEndPaint           = user32.NewProc("EndPaint")
	procInvalidateRect     = user32.NewProc("InvalidateRect")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procSetWindowPos       = user32.NewProc("SetWindowPos")
	procSetWindowTextW     = user32.NewProc("SetWindowTextW")
	procAdjustWindowRectEx = user32.NewProc("AdjustWindowRectEx")
	procLoadCursorW        = user32.NewProc("LoadCursorW")
	procSetCursor          = user32.NewProc("SetCursor")
	procSetCapture         = user32.NewProc("SetCapture")
	procReleaseCapture     = user32.NewProc("ReleaseCapture")
	procGetKeyState        = user32.NewProc("GetKeyState")
	procScreenToClient     = user32.NewProc("ScreenToClient")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procFillRect           = user32.NewProc("FillRect")

	gdi32DLL             = syscall.NewLazyDLL("gdi32.dll")
	procStretchDIBits    = gdi32DLL.NewProc("StretchDIBits")
	procGetDeviceCaps    = gdi32DLL.NewProc("GetDeviceCaps")
	procGetStockObject   = gdi32DLL.NewProc("GetStockObject")
	procCreateCompatDC   = gdi32DLL.NewProc("CreateCompatibleDC")
	procCreateCompatBmp  = gdi32DLL.NewProc("CreateCompatibleBitmap")
	procCreateDIBSection = gdi32DLL.NewProc("CreateDIBSection")
	procSelectObject     = gdi32DLL.NewProc("SelectObject")
	procSetDIBits        = gdi32DLL.NewProc("SetDIBits")
	procBitBlt           = gdi32DLL.NewProc("BitBlt")
	procDeleteDC         = gdi32DLL.NewProc("DeleteDC")
	procDeleteObject     = gdi32DLL.NewProc("DeleteObject")
	procGdiFlush         = gdi32DLL.NewProc("GdiFlush")

	shell32DLL          = syscall.NewLazyDLL("shell32.dll")
	procDragAcceptFiles = shell32DLL.NewProc("DragAcceptFiles")
	procDragQueryFileW  = shell32DLL.NewProc("DragQueryFileW")
	procDragQueryPoint  = shell32DLL.NewProc("DragQueryPoint")
	procDragFinish      = shell32DLL.NewProc("DragFinish")
)

func getWin32DPI() float64 {
	hdc, _, _ := procGetDC.Call(0)
	if hdc == 0 {
		return 96.0
	}
	defer procReleaseDC.Call(0, hdc)
	r, _, _ := procGetDeviceCaps.Call(hdc, logPixelsX)
	dpi := float64(r)
	if dpi <= 0 {
		return 96.0
	}
	return dpi
}

var (
	win32GuiClassRegistered bool
	win32GuiClassMu         sync.Mutex
	win32GuiActiveHosts     sync.Map
)

type win32WndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     syscall.Handle
	hIcon         syscall.Handle
	hCursor       syscall.Handle
	hbrBackground syscall.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       syscall.Handle
}

type win32Msg struct {
	hwnd    syscall.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type win32Rect struct {
	left, top, right, bottom int32
}

type win32PaintStruct struct {
	hdc         syscall.Handle
	fErase      int32
	rcPaint     win32Rect
	fRestore    int32
	fIncUpdate  int32
	rgbReserved [32]byte
}

type win32Point struct {
	x int32
	y int32
}

type Win32GuiHost struct {
	mu                                   sync.Mutex
	mouseCoalesceMu                      sync.Mutex
	lastMouseSent                        time.Time
	pendingMouse                         *vtinput.InputEvent
	hwnd                                 syscall.Handle
	hCursor                              syscall.Handle
	renderer                             *Win32GuiRenderer
	reader                               *vtinput.Reader
	scr                                  *ScreenBuf
	cols, rows                           int
	cellW, cellH                         int
	scale                                int
	winW, winH                           int
	mouseBtn                             uint32
	closeChan                            chan struct{}
	closed                               bool
	pendingDrag                          *win32DragRequest
	pendingResizeCols, pendingResizeRows int

	// dropTarget is the IDropTarget registered for this window, held as the
	// bare interface pointer so this struct stays free of a type that only
	// exists on the architectures which implement it.
	dropTarget uintptr

	// paintPending is set by Invalidate() and cleared only by a WM_PAINT
	// that actually put pixels on the screen. BeginPaint() validates the
	// update region unconditionally, so a WM_PAINT served before the first
	// frame exists (or one whose StretchDIBits fails) would otherwise throw
	// the invalidation away for good: the renderer only invalidates when a
	// row changes, so nothing would ever ask for that paint again and the
	// window would stay unpainted -- i.e. white -- forever. Flush() re-arms
	// the invalidation while this is set. See f4 issue #514.
	paintPending bool
	// everPainted records whether a frame has ever reached the window. Until
	// it has, WM_ERASEBKGND is left to DefWindowProc so the class background
	// brush (black) is used instead of the undefined -- in practice white --
	// initial content of the window's redirection surface.
	everPainted bool
}

type win32DragRequest struct {
	paths   []string
	allowed DropAction
	done    chan win32DragResult
}

type win32DragResult struct {
	action DropAction
	err    error
}

func (h *Win32GuiHost) AcceptsDrops() bool { return true }
func (h *Win32GuiHost) StartDrag(payload DragPayload, allowed DropAction) (DropAction, error) {
	if len(payload.Paths) == 0 {
		return DropNone, ErrDragNoData
	}
	h.mu.Lock()
	hwnd := h.hwnd
	h.mu.Unlock()
	if hwnd == 0 {
		return DropNone, ErrDragUnsupported
	}

	DebugLog("WIN32_DND: StartDrag queuing to main thread with %d path(s): %q, allowed=%s", len(payload.Paths), payload.Paths, allowed)
	req := &win32DragRequest{
		paths:   payload.Paths,
		allowed: allowed,
		done:    make(chan win32DragResult, 1),
	}

	h.mu.Lock()
	h.pendingDrag = req
	h.mu.Unlock()

	procPostMessageW.Call(uintptr(hwnd), wmPerformDragDrop, 0, 0)
	res := <-req.done
	DebugLog("WIN32_DND: StartDrag finished -> action=%s, err=%v", res.action, res.err)
	return res.action, res.err
}

func (h *Win32GuiHost) SetTitle(title string) {
	h.mu.Lock()
	hwnd := h.hwnd
	h.mu.Unlock()
	if hwnd != 0 {
		u16, err := syscall.UTF16PtrFromString(WindowTitleWithBackend(title))
		if err == nil {
			procSetWindowTextW.Call(uintptr(hwnd), uintptr(unsafe.Pointer(u16)))
		}
	}
}

func (h *Win32GuiHost) ResizeGrid(cols, rows int) {
	if h == nil || cols <= 0 || rows <= 0 {
		return
	}
	h.mu.Lock()
	hwnd := h.hwnd
	if hwnd == 0 {
		// This path keeps the renderer testable before a native window exists.
		h.cols = cols
		h.rows = rows
		h.mu.Unlock()
		return
	}
	h.pendingResizeCols = cols
	h.pendingResizeRows = rows
	h.mu.Unlock()

	// The Win32 window and its message pump live on the locked GUI thread,
	// while FrameManager dispatches actions from its own goroutine. Post the
	// resize just like DoDragDrop is posted, so SetWindowPos and the resulting
	// WM_SIZE are handled by the window's owning thread.
	procPostMessageW.Call(uintptr(hwnd), wmPerformResize, 0, 0)
}

// WindowPosition returns the top-left screen position of the native window.
// The bool is false while the host has not created a window or when Windows
// cannot provide its rectangle.
func (h *Win32GuiHost) WindowPosition() (x, y int, ok bool) {
	h.mu.Lock()
	hwnd := h.hwnd
	h.mu.Unlock()
	if hwnd == 0 {
		return 0, 0, false
	}

	var rect win32Rect
	ret, _, _ := procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rect)))
	if ret == 0 {
		return 0, 0, false
	}
	return int(rect.left), int(rect.top), true
}

// SetWindowPosition moves the native window without resizing or activating
// it. The call is also safe before the window is shown, which lets callers
// restore a saved position during GUI startup.
func (h *Win32GuiHost) SetWindowPosition(x, y int) {
	h.mu.Lock()
	hwnd := h.hwnd
	h.mu.Unlock()
	if hwnd == 0 {
		return
	}
	procSetWindowPos.Call(
		uintptr(hwnd), 0,
		uintptr(uint32(int32(x))), uintptr(uint32(int32(y))),
		0, 0,
		uintptr(swpNoSize|swpNoZOrder|swpNoActivate),
	)
}

func (r *Win32GuiRenderer) WindowPosition() (x, y int, ok bool) {
	if r == nil || r.host == nil {
		return 0, 0, false
	}
	return r.host.WindowPosition()
}

func (r *Win32GuiRenderer) SetWindowPosition(x, y int) {
	if r != nil && r.host != nil {
		r.host.SetWindowPosition(x, y)
	}
}

func (h *Win32GuiHost) Invalidate() {
	h.mu.Lock()
	hwnd := h.hwnd
	if hwnd != 0 {
		h.paintPending = true
	}
	h.mu.Unlock()
	if hwnd != 0 {
		procInvalidateRect.Call(uintptr(hwnd), 0, 0)
		procPostMessageW.Call(uintptr(hwnd), 0, 0, 0)
	}
}

// paintOutstanding reports that an invalidation was issued but no frame has
// reached the window since. The renderer's Flush() re-issues the invalidation
// while this holds, which makes a dropped or empty WM_PAINT self-healing on
// the next frame instead of permanent.
func (h *Win32GuiHost) paintOutstanding() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.paintPending && h.hwnd != 0
}

func (h *Win32GuiHost) PostQuit() {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		close(h.closeChan)
	}
	hwnd := h.hwnd
	h.mu.Unlock()
	if hwnd != 0 {
		procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	}
}

func (h *Win32GuiHost) sendEvent(ev *vtinput.InputEvent) {
	h.flushPendingMouse()
	h.mu.Lock()
	closed := h.closed || h.reader == nil || h.reader.EventChan == nil
	h.mu.Unlock()
	if closed {
		return
	}

	defer func() {
		recover()
	}()

	select {
	case h.reader.EventChan <- ev:
	case <-h.closeChan:
	default:
		if ev.Type == vtinput.MouseEventType && (ev.MouseEventFlags&vtinput.MouseMoved) != 0 {
			// Channel full: keep only the freshest move instead of
			// dropping it, otherwise the drag lags behind the cursor.
			h.mouseCoalesceMu.Lock()
			h.pendingMouse = ev
			h.mouseCoalesceMu.Unlock()
			return
		}
		select {
		case h.reader.EventChan <- ev:
		case <-h.closeChan:
		default:
		}
	}
}

// win32MouseMinInterval caps how often a drag mouse-move is enqueued for
// rendering. The freshest position is always retained in pendingMouse, so
// capping the enqueue rate only bounds the (expensive) full-frame render
// frequency -- keeping CPU sane during a drag without losing cursor tracking.
const win32MouseMinInterval = 16 * time.Millisecond

// postMouseMove coalesces drag mouse-move events through a single pending
// slot so a backed-up event channel never loses the latest cursor position.
func (h *Win32GuiHost) postMouseMove(ev *vtinput.InputEvent) {
	h.mouseCoalesceMu.Lock()
	h.pendingMouse = ev
	since := time.Since(h.lastMouseSent)
	h.mouseCoalesceMu.Unlock()
	if since < win32MouseMinInterval {
		// Keep the latest position but don't enqueue another render yet;
		// the pending move is flushed when the interval elapses or on the
		// next sendEvent (e.g. mouse-up), bounding render/CPU load.
		return
	}
	h.flushPendingMouse()
}

// flushPendingMouse pushes any coalesced mouse-move into the event channel.
// Safe to call from anywhere; a no-op when nothing is pending or when the
// channel is still full (the move stays pending and is retried on the next
// sendEvent).
func (h *Win32GuiHost) flushPendingMouse() {
	h.mouseCoalesceMu.Lock()
	ev := h.pendingMouse
	if ev != nil {
		h.pendingMouse = nil
	}
	h.mouseCoalesceMu.Unlock()
	if ev == nil {
		return
	}
	h.mu.Lock()
	closed := h.closed || h.reader == nil || h.reader.EventChan == nil
	h.mu.Unlock()
	if closed {
		return
	}
	select {
	case h.reader.EventChan <- ev:
		h.mouseCoalesceMu.Lock()
		h.lastMouseSent = time.Now()
		h.mouseCoalesceMu.Unlock()
	case <-h.closeChan:
	default:
		h.mouseCoalesceMu.Lock()
		if h.pendingMouse == nil {
			h.pendingMouse = ev
		}
		h.mouseCoalesceMu.Unlock()
	}
}

func (h *Win32GuiHost) getModifiers() vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	if isWin32KeyDown(0x10) { // VK_SHIFT
		mods |= vtinput.ShiftPressed
	}
	if isWin32KeyDown(0x11) { // VK_CONTROL
		if isWin32KeyDown(0xA3) { // VK_RCONTROL
			mods |= vtinput.RightCtrlPressed
		} else {
			mods |= vtinput.LeftCtrlPressed
		}
	}
	if isWin32KeyDown(0x12) { // VK_MENU (ALT)
		if isWin32KeyDown(0xA5) { // VK_RMENU
			mods |= vtinput.RightAltPressed
		} else {
			mods |= vtinput.LeftAltPressed
		}
	}
	if isWin32KeyToggled(0x14) { // VK_CAPITAL
		mods |= vtinput.CapsLockOn
	}
	if isWin32KeyToggled(0x90) { // VK_NUMLOCK
		mods |= vtinput.NumLockOn
	}
	if isWin32KeyToggled(0x91) { // VK_SCROLL
		mods |= vtinput.ScrollLockOn
	}
	return mods
}

func isWin32KeyDown(vk int) bool {
	r, _, _ := procGetKeyState.Call(uintptr(vk))
	return int16(r) < 0
}

func isWin32KeyToggled(vk int) bool {
	r, _, _ := procGetKeyState.Call(uintptr(vk))
	return (r & 1) != 0
}

func win32GuiWndProc(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	val, ok := win32GuiActiveHosts.Load(hwnd)
	if !ok {
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r
	}
	host := val.(*Win32GuiHost)
	return host.handleMessage(hwnd, msg, wParam, lParam)
}

func (h *Win32GuiHost) handleMessage(hwnd syscall.Handle, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmPerformDragDrop:
		h.mu.Lock()
		req := h.pendingDrag
		h.pendingDrag = nil
		h.mu.Unlock()
		if req != nil {
			action, err := win32DoDragDrop(req.paths, req.allowed)
			req.done <- win32DragResult{action: action, err: err}
		}
		return 0

	case wmPerformResize:
		h.mu.Lock()
		cols, rows := h.pendingResizeCols, h.pendingResizeRows
		h.pendingResizeCols, h.pendingResizeRows = 0, 0
		cellW, cellH := h.cellW, h.cellH
		h.mu.Unlock()
		if cols <= 0 || rows <= 0 || cellW <= 0 || cellH <= 0 {
			return 0
		}

		// ResizeGrid takes logical cells, but SetWindowPos takes the outer
		// window size. Use the exact same style/ex-style pair as creation so
		// the requested client area remains cols*cellW by rows*cellH.
		var rc win32Rect
		rc.right = int32(cols * cellW)
		rc.bottom = int32(rows * cellH)
		procAdjustWindowRectEx.Call(
			uintptr(unsafe.Pointer(&rc)),
			uintptr(wsOverlappedWindow),
			0,
			uintptr(wsExAcceptFiles|wsExAppWindow),
		)
		outerW := rc.right - rc.left
		outerH := rc.bottom - rc.top
		if outerW <= 0 || outerH <= 0 {
			return 0
		}
		procSetWindowPos.Call(
			uintptr(hwnd),
			0,
			0,
			0,
			uintptr(outerW),
			uintptr(outerH),
			swpNoMove|swpNoZOrder|swpNoActivate,
		)
		return 0

	case wmEraseBkgnd:
		// Suppressing the erase eliminates flicker, but only once there is a
		// frame to cover the window with. Before the first successful blit,
		// let DefWindowProc erase with the class brush so the window shows
		// black rather than the uninitialised white of a fresh redirection
		// surface.
		h.mu.Lock()
		painted := h.everPainted
		h.mu.Unlock()
		if painted {
			return 1
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r

	case wmPaint:
		var ps win32PaintStruct
		hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		painted := false
		if hdc != 0 {
			if h.renderer != nil {
				painted = h.renderer.blitTo(hdc)
			}
			if !painted {
				// BeginPaint has already validated the update region, so
				// leaving it untouched would show whatever the surface
				// happened to contain. Paint it black and keep paintPending
				// set so the next Flush() asks for this paint again.
				fillRectBlack(hdc, &ps.rcPaint)
				// Self-heal: immediately ask for another paint instead of
				// waiting for the next 250ms Flush() heartbeat.
				procInvalidateRect.Call(uintptr(hwnd), 0, 0)
			}
			procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
		}
		if painted {
			h.mu.Lock()
			h.paintPending = false
			h.everPainted = true
			h.mu.Unlock()
		}
		return 0

	case wmSize:
		newW := int(lParam & 0xFFFF)
		newH := int((lParam >> 16) & 0xFFFF)
		if wParam != 1 && newW > 0 && newH > 0 {
			h.mu.Lock()
			newCols := newW / h.cellW
			newRows := newH / h.cellH
			if newCols < 1 {
				newCols = 1
			}
			if newRows < 1 {
				newRows = 1
			}
			sizeChanged := newCols != h.cols || newRows != h.rows
			h.cols, h.rows = newCols, newRows
			h.winW, h.winH = newW, newH
			h.mu.Unlock()

			if sizeChanged && h.reader != nil && h.reader.EventChan != nil {
				h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType})
			}
			// Explicitly ask for a repaint: CS_HREDRAW|CS_VREDRAW should do
			// this, but a DWM-presented window can stay black if the paint
			// arrives before the FrameManager recomposes for the new size.
			procInvalidateRect.Call(uintptr(hwnd), 0, 0)
		}
		return 0

	case wmKeyDown, wmSysKeyDown:
		vk := uint16(wParam)
		mods := h.getModifiers()
		if isSpecialOrModifiedKey(vk, mods) {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				VirtualKeyCode:  vk,
				Char:            defaultRuneForVK(vk),
				ControlKeyState: mods,
			})
			if msg == wmSysKeyDown && vk != vtinput.VK_LMENU && vk != vtinput.VK_RMENU {
				return 0
			}
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r

	case wmKeyUp, wmSysKeyUp:
		vk := uint16(wParam)
		mods := h.getModifiers()
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         false,
			VirtualKeyCode:  vk,
			ControlKeyState: mods,
		})
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r

	case wmChar, wmSysChar:
		r := rune(wParam)
		mods := h.getModifiers()
		if r >= ' ' && r != 0x7F {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				Char:            r,
				ControlKeyState: mods,
			})
			return 0
		}
		res, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return res

	case wmLButtonDown, wmRButtonDown, wmMButtonDown:
		procSetCapture.Call(uintptr(hwnd))
		x := int16(int32(int16(lParam & 0xFFFF)))
		y := int16(int32(int16(lParam >> 16)))
		cellX := int16(int(x) / h.cellW)
		cellY := int16(int(y) / h.cellH)
		var btn uint32
		switch msg {
		case wmLButtonDown:
			btn = uint32(vtinput.FromLeft1stButtonPressed)
		case wmRButtonDown:
			btn = uint32(vtinput.RightmostButtonPressed)
		case wmMButtonDown:
			btn = uint32(vtinput.FromLeft2ndButtonPressed)
		}
		h.mu.Lock()
		h.mouseBtn |= btn
		currBtn := h.mouseBtn
		h.mu.Unlock()
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          cellX,
			MouseY:          cellY,
			KeyDown:         true,
			ButtonState:     currBtn,
			ControlKeyState: h.getModifiers(),
		})
		return 0

	case wmLButtonUp, wmRButtonUp, wmMButtonUp:
		x := int16(int32(int16(lParam & 0xFFFF)))
		y := int16(int32(int16((lParam >> 16) & 0xFFFF)))
		cellX := int16(int(x) / h.cellW)
		cellY := int16(int(y) / h.cellH)
		var btn uint32
		switch msg {
		case wmLButtonUp:
			btn = uint32(vtinput.FromLeft1stButtonPressed)
		case wmRButtonUp:
			btn = uint32(vtinput.RightmostButtonPressed)
		case wmMButtonUp:
			btn = uint32(vtinput.FromLeft2ndButtonPressed)
		}
		h.mu.Lock()
		h.mouseBtn &^= btn
		currBtn := h.mouseBtn
		h.mu.Unlock()
		if currBtn == 0 {
			procReleaseCapture.Call()
		}
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          cellX,
			MouseY:          cellY,
			KeyDown:         false,
			ButtonState:     currBtn,
			ControlKeyState: h.getModifiers(),
		})
		return 0

	case wmLButtonDblClk, wmRButtonDblClk, wmMButtonDblClk:
		x := int16(int32(int16(lParam & 0xFFFF)))
		y := int16(int32(int16((lParam >> 16) & 0xFFFF)))
		cellX := int16(int(x) / h.cellW)
		cellY := int16(int(y) / h.cellH)
		var btn uint32
		switch msg {
		case wmLButtonDblClk:
			btn = uint32(vtinput.FromLeft1stButtonPressed)
		case wmRButtonDblClk:
			btn = uint32(vtinput.RightmostButtonPressed)
		case wmMButtonDblClk:
			btn = uint32(vtinput.FromLeft2ndButtonPressed)
		}
		h.mu.Lock()
		h.mouseBtn |= btn
		currBtn := h.mouseBtn
		h.mu.Unlock()
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          cellX,
			MouseY:          cellY,
			KeyDown:         true,
			ButtonState:     currBtn,
			MouseEventFlags: vtinput.DoubleClick,
			ControlKeyState: h.getModifiers(),
		})
		return 0

	case wmMouseMove:
		x := int16(int32(int16(lParam & 0xFFFF)))
		y := int16(int32(int16((lParam >> 16) & 0xFFFF)))
		cellX := int16(int(x) / h.cellW)
		cellY := int16(int(y) / h.cellH)
		h.mu.Lock()
		btn := h.mouseBtn
		h.mu.Unlock()
		if btn != 0 {
			h.postMouseMove(&vtinput.InputEvent{
				Type:            vtinput.MouseEventType,
				MouseX:          cellX,
				MouseY:          cellY,
				MouseEventFlags: vtinput.MouseMoved,
				ButtonState:     btn,
				ControlKeyState: h.getModifiers(),
			})
		}
		return 0

	case wmMouseWheel:
		zDelta := int16((wParam >> 16) & 0xFFFF)
		dir := 1
		if zDelta < 0 {
			dir = -1
		}
		var pt win32Point
		pt.x = int32(int16(lParam & 0xFFFF))
		pt.y = int32(int16((lParam >> 16) & 0xFFFF))
		procScreenToClient.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
		cellX := int16(int(pt.x) / h.cellW)
		cellY := int16(int(pt.y) / h.cellH)
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          cellX,
			MouseY:          cellY,
			WheelDirection:  dir,
			ControlKeyState: h.getModifiers(),
		})
		return 0

	case wmDropFiles:
		hDrop := syscall.Handle(wParam)
		var pt win32Point
		procDragQueryPoint.Call(uintptr(hDrop), uintptr(unsafe.Pointer(&pt)))
		cellX := int(pt.x) / h.cellW
		cellY := int(pt.y) / h.cellH

		countRet, _, _ := procDragQueryFileW.Call(uintptr(hDrop), 0xFFFFFFFF, 0, 0)
		fileCount := int(countRet)
		var paths []string
		for i := 0; i < fileCount; i++ {
			var buf [1024]uint16
			procDragQueryFileW.Call(uintptr(hDrop), uintptr(i), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			paths = append(paths, syscall.UTF16ToString(buf[:]))
		}
		procDragFinish.Call(uintptr(hDrop))

		if len(paths) > 0 {
			// This is the fallback for sources that do not speak OLE. When
			// a drop target is registered the drop arrives through it
			// instead, with real enter / over phases; here the whole
			// gesture has already happened by the time we hear about it,
			// so the three phases are synthesised back to back.
			DebugLog("WIN32_DND: WM_DROPFILES at cell (%d,%d), count=%d: %q", cellX, cellY, len(paths), paths)
			payload := DragPayload{Kinds: []string{"text/uri-list"}, Paths: paths}
			mods := h.getModifiers()
			var lastAction DropAction
			for _, phase := range []DragPhase{DragEnter, DragOver, DragDrop} {
				lastAction = DeliverDragEvent(&DragEvent{
					Phase:     phase,
					X:         cellX,
					Y:         cellY,
					Modifiers: mods,
					Allowed:   DropCopy | DropMove | DropLink,
					Suggested: DropCopy,
					Payload:   payload,
				})
			}
			DebugLog("WIN32_DND: WM_DROPFILES delivery result -> %s", lastAction)
		}
		return 0

	case wmSetCursor:
		// lParam's low word is the hit-test result Windows computed for the
		// pointer. Over the non-client frame (the resizeable edges/corners)
		// we must let DefWindowProc set the matching resize cursor, so only
		// override the cursor when the hit is inside the client area.
		const htClient = 1
		if int(lParam&0xFFFF) == htClient && h.hCursor != 0 {
			procSetCursor.Call(uintptr(h.hCursor))
			return 1
		}
		r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
		return r

	case wmSetFocus:
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: true})
		return 0

	case wmKillFocus:
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
		return 0

	case wmClose:
		if h.closed || FrameManager.IsShutdown() {
			procDestroyWindow.Call(uintptr(hwnd))
			return 0
		}
		postQuitCommand()
		return 0

	case wmDestroy:
		win32RevokeDropTarget(h, hwnd)
		win32GuiActiveHosts.Delete(hwnd)
		procPostQuitMessage.Call(0)
		return 0
	}

	r, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wParam, lParam)
	return r
}

// blackStockBrush returns the BLACK_BRUSH stock object. Stock objects are
// owned by GDI and must not be deleted, so the handle can be reused freely.
func blackStockBrush() syscall.Handle {
	const blackBrush = 4
	hbr, _, _ := procGetStockObject.Call(blackBrush)
	return syscall.Handle(hbr)
}

// fillRectBlack paints rc with the black stock brush. Used when there is no
// frame to blit yet, so that a validated-but-unpainted region reads as an
// empty terminal rather than a white rectangle.
func fillRectBlack(hdc uintptr, rc *win32Rect) {
	hbr := blackStockBrush()
	if hbr == 0 {
		return
	}
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(rc)), uintptr(hbr))
}

// blitTo composes the frame into the offscreen memory-DC bitmap and BitBlts
// it into hdc, holding the renderer lock across the whole sequence (a
// concurrent Render() may resize imgBuf/bgraBuf, and the sizes must stay
// consistent with the DIB/bitmap being built from them).
//
// This goes through an off-screen compatible bitmap (SetDIBits into a memory
// DC, then BitBlt from that memory DC into hdc) rather than calling
// StretchDIBits directly on the window's own HDC. A direct StretchDIBits into
// a DWM-composited window HDC is known to be unreliable on real Windows: it
// can report success (return the number of scanlines copied) while the
// desktop compositor never picks up the new content, leaving the window
// black/blank -- intermittently, and worse right after a resize, when DWM is
// also rebuilding the window's redirection surface. Wine's own presentation
// path doesn't have this failure mode, which is why the backend "works
// perfectly" under Wine while flaking on native Windows 7/8/10. Going through
// a plain GDI memory DC + BitBlt is the standard, reliable pattern for
// presenting a software-rendered bitmap into a window. See f4 issue #514.
//
// It reports whether pixels were handed to the DC. A false return means the
// caller must keep the paint pending rather than treat the window as drawn.
func (r *Win32GuiRenderer) blitTo(hdc uintptr) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	w, h, ok := r.syncBGRALocked()
	if !ok || w <= 0 || h <= 0 {
		return false
	}

	if r.memDC == 0 || r.memW != w || r.memH != h {
		r.releaseMemDCLocked()
		memDC, _, _ := procCreateCompatDC.Call(hdc)
		if memDC == 0 {
			return false
		}
		// A device-INDEPENDENT bitmap (DIB section): unlike a DDB from
		// CreateCompatibleBitmap, its pixel buffer is always writable
		// directly (and SetDIBits into a DDB created off a DWM window DC
		// intermittently fails on larger sizes, leaving the window black --
		// f4 issue #514).
		bmi := makeTopDownDIBInfo(w, h)
		var bits uintptr
		bmp, _, _ := procCreateDIBSection.Call(
			hdc,
			uintptr(unsafe.Pointer(&bmi)),
			dibRGBColors,
			uintptr(unsafe.Pointer(&bits)),
			0, 0,
		)
		if bmp == 0 {
			procDeleteDC.Call(memDC)
			return false
		}
		procSelectObject.Call(memDC, bmp)
		r.memDC, r.memBitmap = memDC, bmp
		r.memBits = bits
		r.memW, r.memH = w, h
	}

	// The DIB section is 32bpp top-down, i.e. BGRA row-major top-to-bottom,
	// exactly the layout syncBGRALocked produced -- so copy pixels straight
	// into the DIB memory and skip SetDIBits entirely.
	if r.memBits == 0 || len(r.bgraBuf) < w*h*4 {
		return false
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(r.memBits)), w*h*4)
	copy(dst, r.bgraBuf[:w*h*4])

	const srcCopyRop = srcCopy
	ret, _, _ := procBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), r.memDC, 0, 0, srcCopyRop)
	procGdiFlush.Call()
	return ret != 0
}

// releaseMemDCLocked frees the offscreen memory DC and bitmap. Caller must
// hold r.mu.
func (r *Win32GuiRenderer) releaseMemDCLocked() {
	if r.memBitmap != 0 {
		procDeleteObject.Call(r.memBitmap)
		r.memBitmap = 0
	}
	if r.memDC != 0 {
		procDeleteDC.Call(r.memDC)
		r.memDC = 0
	}
	r.memW, r.memH = 0, 0
	r.memBits = 0
}

// releaseGDIResources frees GDI handles owned by the renderer. Called from
// Win32GuiHost.Close() to avoid leaking a memory DC/bitmap per window.
func (r *Win32GuiRenderer) releaseGDIResources() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.releaseMemDCLocked()
}

func (h *Win32GuiHost) Close() {
	h.mu.Lock()
	if !h.closed {
		h.closed = true
		close(h.closeChan)
	}
	hwnd := h.hwnd
	h.hwnd = 0
	renderer := h.renderer
	h.reader = nil
	h.mu.Unlock()
	if renderer != nil {
		renderer.releaseGDIResources()
	}
	if hwnd != 0 {
		win32GuiActiveHosts.Delete(hwnd)
		procDestroyWindow.Call(uintptr(hwnd))
	}
}

func RunWin32GuiHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	oleInit()

	if fontSize <= 0 {
		fontSize = 18.0
	}

	win32DPI := getWin32DPI()
	scaleFactor := win32DPI / 96.0
	if scaleFactor < 1.0 {
		scaleFactor = 1.0
	}
	fontDPI := 72.0 * scaleFactor
	face, cellW, cellH := loadBestFont(fontName, fontSize, fontDPI)
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = int(8*scaleFactor+0.5), int(16*scaleFactor+0.5)
	}

	scale := int(scaleFactor + 0.5)
	if scale < 1 {
		scale = 1
	}

	host := &Win32GuiHost{
		cols:      cols,
		rows:      rows,
		cellW:     cellW,
		cellH:     cellH,
		scale:     scale,
		winW:      cols * cellW,
		winH:      rows * cellH,
		closeChan: make(chan struct{}),
	}

	className, err := syscall.UTF16PtrFromString("VTUI_WIN32_GUI")
	if err != nil {
		return err
	}

	win32GuiClassMu.Lock()
	if !win32GuiClassRegistered {
		hInst, _, _ := procGetModuleHandleW.Call(0)
		hCursor, _, _ := procLoadCursorW.Call(0, 32512) // IDC_ARROW
		host.hCursor = syscall.Handle(hCursor)

		wc := win32WndClassExW{
			cbSize:        uint32(unsafe.Sizeof(win32WndClassExW{})),
			style:         csHRedraw | csVRedraw | csDblClks,
			lpfnWndProc:   syscall.NewCallback(win32GuiWndProc),
			hInstance:     syscall.Handle(hInst),
			hCursor:       host.hCursor,
			hbrBackground: blackStockBrush(),
			lpszClassName: className,
		}
		ret, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if ret == 0 && err != windows.ERROR_CLASS_ALREADY_EXISTS {
			win32GuiClassMu.Unlock()
			return fmt.Errorf("failed to register Win32 GUI window class: %v", err)
		}
		win32GuiClassRegistered = true
	}
	win32GuiClassMu.Unlock()

	style := uint32(wsOverlappedWindow)
	var rc struct{ left, top, right, bottom int32 }
	rc.right = int32(cols * cellW)
	rc.bottom = int32(rows * cellH)
	procAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&rc)), uintptr(style), 0, uintptr(wsExAcceptFiles|wsExAppWindow))
	adjW := rc.right - rc.left
	adjH := rc.bottom - rc.top

	titlePtr, _ := syscall.UTF16PtrFromString(WindowTitleWithBackend(AppName))
	hInst, _, _ := procGetModuleHandleW.Call(0)

	hwndRet, _, err := procCreateWindowExW.Call(
		uintptr(wsExAcceptFiles|wsExAppWindow),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(titlePtr)),
		uintptr(style),
		0x80000000, // CW_USEDEFAULT
		0x80000000,
		uintptr(adjW),
		uintptr(adjH),
		0, 0,
		uintptr(hInst),
		0,
	)
	if hwndRet == 0 {
		return fmt.Errorf("failed to create Win32 GUI window: %v", err)
	}

	host.hwnd = syscall.Handle(hwndRet)
	win32GuiActiveHosts.Store(host.hwnd, host)
	procDragAcceptFiles.Call(hwndRet, 1)
	// This thread is the one that called OleInitialize and the one that
	// pumps this window's messages, which is exactly where a drop target
	// has to be registered.
	win32RegisterDropTarget(host, host.hwnd)

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	renderer := NewWin32GuiRenderer(host, face, cellW, cellH)
	scr.Renderer = renderer
	scr.Graphics().SetProtocol(GraphicsNative)
	scr.Graphics().SetCellSize(cellW, cellH)
	host.renderer = renderer
	host.scr = scr

	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		cols, rows := host.cols, host.rows
		if cols > 500 {
			cols = 500
		}
		if rows > 300 {
			rows = 300
		}
		return cols, rows, nil
	}

	DisableTerminalClipboard()
	SetDragBackend(host)
	setupApp()
	SetActiveBackend("win32", fmt.Sprintf("cell %dx%d, font %q", cellW, cellH, fontName), "GDI SetDIBitsToDevice")
	setWheelNotchLines(getSystemScrollLines())

	// Compose the very first real frame while the window is still hidden.
	// Without this, the window becomes visible with an empty (black) screen
	// and only receives its content after FrameManager.Run's first
	// renderPhase -- which on a cold start (font glyph cache, panel layout,
	// session restore) takes long enough to read as a ~1s black flash.
	FrameManager.renderPhase()

	// Force the initial frame composition so the bitmap buffer is fully populated
	// before the window is made visible and receives its first WM_PAINT.
	scr.Flush()

	// Paint the first frame into the (still hidden) window before showing it:
	// DWM presents the window the instant ShowWindow runs, a frame or two
	// before the first GDI blit would land, so an unpainted window flashes
	// black on cold start.
	procInvalidateRect.Call(hwndRet, 0, 0)
	procUpdateWindow.Call(hwndRet)

	procShowWindow.Call(hwndRet, 1)
	procUpdateWindow.Call(hwndRet)

	go func() {
		defer LogAndRepanic("win32 FrameManager")
		FrameManager.Run(reader)
		host.PostQuit()
	}()

	var msg win32Msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	return nil
}

func runInWin32Window(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return RunWin32GuiHost(cols, rows, fontName, fontSize, setupApp)
}
