package vtui

// Window is a non-modal container for UI elements.
type Window struct {
	BaseWindow
}

func NewWindow(x1, y1, x2, y2 int, title string) *Window {
	w := &Window{
		BaseWindow: *NewBaseWindow(x1, y1, x2, y2, title),
	}
	w.ShowClose = true
	w.ShowZoom = true
	// Re-link the root group to the actual Window pointer
	w.rootGroup.SetOwner(w)
	w.frame.SetOwner(w)
	return w
}

func (w *Window) IsModal() bool { return false }
func (w *Window) GetType() FrameType { return TypeUser }
func (w *Window) GetTitle() string { return w.frame.title }
func (w *Window) GetProgress() int {
	return w.progress
}

func (w *Window) SetProgress(p int) {
	w.progress = p
}
