package vtui

import "github.com/unxed/vtinput"

// Window is a container for UI elements. It can be modal (Dialog) or non-modal.
type Window struct {
	BaseWindow
	contentWidth  int
	contentHeight int
	scrollY       int
	scrollMax     int
}

func NewWindow(x1, y1, x2, y2 int, title string) *Window {
	w := &Window{
		BaseWindow: *NewBaseWindow(x1, y1, x2, y2, title),
	}
	w.ShowClose = true
	w.ShowZoom = true
	w.contentWidth = x2 - x1 + 1
	w.contentHeight = y2 - y1 + 1
	w.rootGroup.SetOwner(w)
	w.frame.SetOwner(w)
	return w
}

// NewDialog is a convenience wrapper for creating a modal window.
func NewDialog(x1, y1, x2, y2 int, title string) *Window {
	w := NewWindow(x1, y1, x2, y2, title)
	w.Modal = true
	w.ShowZoom = false
	w.ShowClose = false // Dialogs don't have a close button by default unless specified
	return w
}

// NewCenteredDialog creates a modal dialog automatically centered on the screen.
func NewCenteredDialog(width, height int, title string) *Window {
	scrW, scrH := 80, 25
	if FrameManager != nil {
		scrW = FrameManager.GetScreenSize()
		if FrameManager.scr != nil {
			scrH = FrameManager.scr.height
		}
	}
	viewW, viewH := modalViewportSize(scrW, scrH, width, height)
	x1 := (scrW - viewW) / 2
	y1 := (scrH - viewH) / 2
	dialog := NewDialog(x1, y1, x1+width-1, y1+height-1, title)
	dialog.resizeModalViewport(scrW, scrH)
	return dialog
}

// ResizeConsole keeps modal dialogs inside the usable screen viewport. A
// dialog whose contents are taller than the viewport enables vertical
// scrolling; shrinking fixed controls would make them overlap instead.
func (w *Window) ResizeConsole(screenW, screenH int) {
	if !w.Modal {
		w.BaseWindow.ResizeConsole(screenW, screenH)
		return
	}
	w.resizeModalViewport(screenW, screenH)
}

func modalViewportSize(screenW, screenH, dialogW, dialogH int) (int, int) {
	if dialogW < screenW {
		screenW = dialogW
	}
	if dialogH < screenH {
		screenH = dialogH
	}
	return screenW, screenH
}

func (w *Window) resizeModalViewport(screenW, screenH int) {
	viewW, viewH := modalViewportSize(screenW, screenH, w.contentWidth, w.contentHeight)
	if viewW <= 0 || viewH <= 0 {
		return
	}
	w.MoveRelative((screenW-viewW)/2-w.X1, (screenH-viewH)/2-w.Y1)
	w.setViewportSize(viewW, viewH)
	w.updateScrollBounds()
	w.ensureFocusedItemVisible()
}

func (w *Window) updateScrollBounds() {
	if !w.Modal || w.rootGroup == nil {
		return
	}
	contentBottom := w.Y1 + 1
	for _, item := range w.rootGroup.items {
		_, _, _, y2 := item.GetPosition()
		if bottom := y2 + w.scrollY; bottom > contentBottom {
			contentBottom = bottom
		}
	}
	w.scrollMax = contentBottom - (w.Y2 - 1)
	if w.scrollMax < 0 {
		w.scrollMax = 0
	}
	w.setScroll(w.scrollY)
}

func (w *Window) setScroll(target int) {
	if target < 0 {
		target = 0
	}
	if target > w.scrollMax {
		target = w.scrollMax
	}
	delta := w.scrollY - target
	if delta == 0 || w.rootGroup == nil {
		return
	}
	for _, item := range w.rootGroup.items {
		item.MoveRelative(0, delta)
	}
	w.scrollY = target
}

func (w *Window) ensureFocusedItemVisible() {
	if w.scrollMax == 0 {
		return
	}
	focused := w.GetFocusedItem()
	if focused == nil {
		return
	}
	_, y1, _, y2 := focused.GetPosition()
	top, bottom := w.Y1+1, w.Y2-1
	target := w.scrollY
	if y1 < top {
		target -= top - y1
	}
	if y2 > bottom {
		target += y2 - bottom
	}
	w.setScroll(target)
}

func (w *Window) Show(scr *ScreenBuf) {
	w.updateScrollBounds()
	w.BaseWindow.Show(scr)
}

func (w *Window) ProcessKey(e *vtinput.InputEvent) bool {
	handled := w.BaseWindow.ProcessKey(e)
	if handled && (e.KeyDown || e.Type == vtinput.FocusEventType) {
		w.updateScrollBounds()
		w.ensureFocusedItemVisible()
	}
	return handled
}

func (w *Window) ProcessMouse(e *vtinput.InputEvent) bool {
	// A short modal dialog owns the wheel even when the pointer happens to be
	// above a child control. Otherwise a child can consume the event before
	// the dialog gets a chance to reveal its hidden controls.
	if e.WheelDirection != 0 && w.scrollMax != 0 {
		if e.WheelDirection > 0 {
			w.setScroll(w.scrollY - wheelLinesFor(WheelAreaList, e.WheelDirection))
		} else {
			w.setScroll(w.scrollY + wheelLinesFor(WheelAreaList, e.WheelDirection))
		}
		return true
	}
	if w.BaseWindow.ProcessMouse(e) {
		w.updateScrollBounds()
		w.ensureFocusedItemVisible()
		return true
	}
	return false
}

func (w *Window) GetType() FrameType {
	if w.Modal {
		return TypeDialog
	}
	return TypeUser
}
func (w *Window) GetElementAt(x, y int) UIElement {
	return w.rootGroup.GetElementAt(x, y)
}

func (w *Window) GetChildren() []UIElement {
	return w.rootGroup.GetChildren()
}

func (w *Window) GetProgress() int  { return w.progress }
func (w *Window) SetProgress(p int) { w.progress = p }
