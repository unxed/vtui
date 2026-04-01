package vtui

// Window is a container for UI elements. It can be modal (Dialog) or non-modal.
type Window struct {
	BaseWindow
}

func NewWindow(x1, y1, x2, y2 int, title string) *Window {
	w := &Window{
		BaseWindow: *NewBaseWindow(x1, y1, x2, y2, title),
	}
	w.ShowClose = true
	w.ShowZoom = true
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
	x1 := (scrW - width) / 2
	y1 := (scrH - height) / 2
	return NewDialog(x1, y1, x1+width-1, y1+height-1, title)
}

func (w *Window) GetType() FrameType {
	if w.Modal {
		return TypeDialog
	}
	return TypeUser
}

func (w *Window) GetTitle() string { return w.frame.title }
func (w *Window) GetProgress() int { return w.progress }
func (w *Window) SetProgress(p int) { w.progress = p }
