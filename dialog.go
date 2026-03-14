package vtui

import "github.com/unxed/vtinput"

// UIElement is the interface that all dialog elements must implement.
type UIElement interface {
	GetPosition() (int, int, int, int)
	Show(scr *ScreenBuf)
	Hide(scr *ScreenBuf)
	SetFocus(bool)
	IsFocused() bool
	CanFocus() bool
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
}

// Dialog is a container for UI elements that manages focus and events.
type Dialog struct {
	ScreenObject
	items    []UIElement
	focusIdx int
	frame    *BorderedFrame
	done     bool
	exitCode int
}

func NewDialog(x1, y1, x2, y2 int, title string) *Dialog {
	d := &Dialog{
		items:    []UIElement{},
		focusIdx: -1,
		frame:    NewBorderedFrame(x1, y1, x2, y2, DoubleBox, title),
	}
	d.SetPosition(x1, y1, x2, y2)
	return d
}

// AddItem adds an element to the dialog.
func (d *Dialog) AddItem(item UIElement) {
	d.items = append(d.items, item)
	// If this is the first focusable element, give it focus
	if d.focusIdx == -1 && item.CanFocus() {
		d.focusIdx = len(d.items) - 1
		item.SetFocus(true)
	}
}

// Show renders the dialog and all its elements.
func (d *Dialog) Show(scr *ScreenBuf) {
	d.ScreenObject.Show(scr)
	d.frame.DisplayObject(scr)
	for _, item := range d.items {
		item.Show(scr)
	}
}

// ProcessKey manages focus switching and passes events to elements.
func (d *Dialog) ProcessKey(e *vtinput.InputEvent) bool {

	// 1. Pass the event to the active element first (allows elements to override Tab/Esc if needed)
	// We don't filter KeyDown here, as elements might want to handle KeyUp.
	if d.focusIdx != -1 {
		if d.items[d.focusIdx].ProcessKey(e) {
			return true
		}
	}

	// 2. Handle global dialog keys
	if !e.KeyDown { return false }

	DebugLog("Dialog.ProcessKey: VK=%X Char=%d FocusIdx=%d", e.VirtualKeyCode, e.Char, d.focusIdx)

	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		DebugLog("Dialog: Close signal")
		d.SetExitCode(-1)
		return true
	case vtinput.VK_TAB:
		oldIdx := d.focusIdx
		d.nextFocus()
		DebugLog("Dialog: TAB pressed. Focus changed: %d -> %d", oldIdx, d.focusIdx)
		return true
	}

	return false
}

func (d *Dialog) ResizeConsole(w, h int) {
	// Center the dialog on the new screen size
	dw, dh := d.X2-d.X1+1, d.Y2-d.Y1+1
	x1 := (w - dw) / 2
	y1 := (h - dh) / 2
	d.SetPosition(x1, y1, x1+dw-1, y1+dh-1)
	// Important: We'd need to reposition all internal items here too,
	// but for now we focus on the Panels.
}

func (d *Dialog) GetType() FrameType {
	return TypeDialog
}

func (d *Dialog) SetExitCode(code int) {
	d.done = true
	d.exitCode = code
}

func (d *Dialog) IsDone() bool {
	return d.done
}

func (d *Dialog) nextFocus() {
	if len(d.items) == 0 { return }

	DebugLog("Dialog.nextFocus: Starting from index %d", d.focusIdx)

	// 1. Remove focus from current
	if d.focusIdx != -1 {
		d.items[d.focusIdx].SetFocus(false)
	}

	// 2. Find next focusable
	startIdx := d.focusIdx
	for {
		d.focusIdx = (d.focusIdx + 1) % len(d.items)
		can := d.items[d.focusIdx].CanFocus()
		DebugLog("  Checking item index %d: CanFocus=%v", d.focusIdx, can)
		if can || d.focusIdx == startIdx {
			break
		}
	}

	DebugLog("Dialog.nextFocus: Landed on index %d", d.focusIdx)
	d.items[d.focusIdx].SetFocus(true)
}

// ProcessMouse handles mouse events, passing them to the appropriate element.
func (d *Dialog) ProcessMouse(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)

	// We check whether the click hit any element of the dialog.
	// We iterate over the elements in reverse order (Z-order: top first)
	for i := len(d.items) - 1; i >= 0; i-- {
		item := d.items[i]
		x1, y1, x2, y2 := item.GetPosition()
		if mx >= x1 && mx <= x2 && my >= y1 && my <= y2 {

			// If it's a left button click, change focus.
			if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				if item.CanFocus() && d.focusIdx != i {
					if d.focusIdx != -1 {
						d.items[d.focusIdx].SetFocus(false)
					}
					d.focusIdx = i
					item.SetFocus(true)
				}
			}

			// Always propagate an event to the element under the mouse
			if item.ProcessMouse(e) {
				return true
			}

			// If an element absorbs a click (even if it returns false),
			// prevent clicks through it (important for overlapping elements)
			return true
		}
	}

	return false
}
