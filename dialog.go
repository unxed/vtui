package vtui

import (
	"unicode"

	"github.com/unxed/vtinput"
)

// UIElement is the interface that all dialog elements must implement.
type UIElement interface {
	GetPosition() (int, int, int, int)
	SetPosition(int, int, int, int)
	GetGrowMode() GrowMode
	Show(scr *ScreenBuf)
	Hide(scr *ScreenBuf)
	SetFocus(bool)
	IsFocused() bool
	CanFocus() bool
	GetHotkey() rune
	GetHelp() string
	ProcessKey(e *vtinput.InputEvent) bool
	ProcessMouse(e *vtinput.InputEvent) bool
}

// Dialog is a container for UI elements that manages focus and events.
type Dialog struct {
	ScreenObject
	items      []UIElement
	focusIdx   int
	frame      *BorderedFrame
	done       bool
	exitCode   int
	isDragging bool
	isResizing bool
	dragOffX   int
	dragOffY   int
	lastW      int
	lastH      int
	MinW       int
	MinH       int
}

func NewDialog(x1, y1, x2, y2 int, title string) *Dialog {
	d := &Dialog{
		items:    []UIElement{},
		focusIdx: -1,
		frame:    NewBorderedFrame(x1, y1, x2, y2, DoubleBox, title),
		MinW:     10,
		MinH:     3,
	}
	d.SetPosition(x1, y1, x2, y2)
	d.lastW = x2 - x1 + 1
	d.lastH = y2 - y1 + 1
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
	d.drawShadow(scr)
	d.frame.DisplayObject(scr)
	for _, item := range d.items {
		item.Show(scr)
	}
}

func (d *Dialog) drawShadow(scr *ScreenBuf) {
	// Shadow in Far is offset +2 on X and +1 on Y.
	// Draw only if shadow fits within buffer boundaries.
	shAttr := Palette[ColShadow]

	// Vertical part of the shadow (right)
	// From Y1+1 to Y2+1, in columns X2+1 and X2+2
	scr.FillRect(d.X2+1, d.Y1+1, d.X2+2, d.Y2+1, ' ', shAttr)

	// Horizontal part of the shadow (bottom)
	// From X1+2 to X2, in row Y2+1
	scr.FillRect(d.X1+2, d.Y2+1, d.X2, d.Y2+1, ' ', shAttr)
}

// ProcessKey manages focus switching and passes events to elements.
func (d *Dialog) ProcessKey(e *vtinput.InputEvent) bool {

	// 1. Pass the event to the active element first
	if d.focusIdx != -1 {
		if d.items[d.focusIdx].ProcessKey(e) {
			return true
		}

		// Special RadioButton handling: if element didn't consume Space/Enter,
		// but it's a radio button, we activate it and reset others.
		if e.KeyDown && (e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_RETURN) {
			if rb, ok := d.items[d.focusIdx].(*RadioButton); ok {
				d.selectRadio(rb)
				return true
			}
		}
	}

	// 2. Handle global dialog keys
	if !e.KeyDown { return false }

	DebugLog("Dialog.ProcessKey: VK=%X Char=%d FocusIdx=%d", e.VirtualKeyCode, e.Char, d.focusIdx)

	// --- Hotkey handling (Alt+Char or just Char) ---
	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

		// In Far dialogs, hotkeys always trigger on Alt+Letter.
		// And just by Letter — only if focus is NOT on an edit field.
		allowWithoutAlt := true
		if d.focusIdx != -1 {
			if _, isEdit := d.items[d.focusIdx].(*Edit); isEdit {
				allowWithoutAlt = false
			} else if cb, isCombo := d.items[d.focusIdx].(*ComboBox); isCombo && !cb.DropdownOnly {
				allowWithoutAlt = false
			}
		}

		if alt || allowWithoutAlt {
			for i, item := range d.items {
				hk := item.GetHotkey()
				if hk != 0 && hk == charLower {
					target := item
					targetIdx := i

					// Recursively traverse FocusLink links until terminal element found
					for hops := 0; hops < len(d.items); hops++ {
						if txt, ok := target.(*Text); ok && txt.FocusLink != nil {
							target = txt.FocusLink
							for j, other := range d.items {
								if other == target {
									targetIdx = j
									break
								}
							}
							continue
						}
						break
					}

					if target.CanFocus() {
						if d.focusIdx != -1 {
							d.items[d.focusIdx].SetFocus(false)
						}
						d.focusIdx = targetIdx
						target.SetFocus(true)
					}

					// Emulate click
					if _, isBtn := target.(*Button); isBtn {
						target.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
					} else if _, isChk := target.(*Checkbox); isChk {
						target.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
					} else if rb, isRad := target.(*RadioButton); isRad {
						d.selectRadio(rb)
					}
					return true
				}
			}
		}
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_F1:
		d.ShowHelp()
		return true
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		DebugLog("Dialog: Close signal")
		d.SetExitCode(-1)
		return true
	case vtinput.VK_TAB:
		shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
		if shift {
			d.changeFocus(-1) // Back
		} else {
			d.changeFocus(1) // Forward
		}
		return true
	}

	return false
}

func (d *Dialog) ResizeConsole(w, h int) {
	// 1. Center dialog on the new screen
	dw, dh := d.X2-d.X1+1, d.Y2-d.Y1+1
	nx1 := (w - dw) / 2
	ny1 := (h - dh) / 2

	// If dialog moved entirely, just call MoveRelative
	offX, offY := nx1-d.X1, ny1-d.Y1
	d.MoveRelative(offX, offY)
}
// Center positions the dialog in the middle of the given area (usually screen size).
func (d *Dialog) Center(scrW, scrH int) {
	dw, dh := d.X2-d.X1+1, d.Y2-d.Y1+1
	nx1 := (scrW - dw) / 2
	ny1 := (scrH - dh) / 2
	d.MoveRelative(nx1-d.X1, ny1-d.Y1)
}

// ChangeSize changes dialog size and adapts child positions via GrowMode.
func (d *Dialog) ChangeSize(nw, nh int) {
	dx := nw - d.lastW
	dy := nh - d.lastH
	if dx == 0 && dy == 0 { return }

	d.X2 += dx
	d.Y2 += dy
	d.frame.SetPosition(d.X1, d.Y1, d.X2, d.Y2)

	for _, item := range d.items {
		gm := item.GetGrowMode()
		ix1, iy1, ix2, iy2 := item.GetPosition()

		if (gm & GrowLoX) != 0 { ix1 += dx }
		if (gm & GrowHiX) != 0 { ix2 += dx }
		if (gm & GrowLoY) != 0 { iy1 += dy }
		if (gm & GrowHiY) != 0 { iy2 += dy }

		item.SetPosition(ix1, iy1, ix2, iy2)
	}

	d.lastW = nw
	d.lastH = nh
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
// GetFocusedItem returns the currently focused element in the dialog.
func (d *Dialog) GetFocusedItem() UIElement {
	if d.focusIdx >= 0 && d.focusIdx < len(d.items) {
		return d.items[d.focusIdx]
	}
	return nil
}
func (d *Dialog) IsBusy() bool { return false }

func (d *Dialog) changeFocus(direction int) {
	if len(d.items) == 0 { return }

	// 1. Remove focus from the current element
	if d.focusIdx != -1 {
		d.items[d.focusIdx].SetFocus(false)
	}

	// 2. Search for the next/previous focusable element
	startIdx := d.focusIdx
	for {
		d.focusIdx += direction
		if d.focusIdx < 0 {
			d.focusIdx = len(d.items) - 1
		}
		if d.focusIdx >= len(d.items) {
			d.focusIdx = 0
		}

		if d.items[d.focusIdx].CanFocus() || d.focusIdx == startIdx {
			break
		}
	}

	d.items[d.focusIdx].SetFocus(true)
}

func (d *Dialog) selectRadio(rb *RadioButton) {
	if rb.Selected { return }
	for _, item := range d.items {
		if other, ok := item.(*RadioButton); ok {
			other.Selected = false
		}
	}
	rb.Selected = true
}

// ProcessMouse handles mouse events, passing them to the appropriate element.
func (d *Dialog) ProcessMouse(e *vtinput.InputEvent) bool {
	mx, my := int(e.MouseX), int(e.MouseY)

	// 1. Handle Active Dragging
	if d.isDragging {
		if !e.KeyDown && e.ButtonState == 0 {
			d.isDragging = false
			return true
		}
		// Move the whole dialog including its components
		dx := mx - d.dragOffX
		dy := my - d.dragOffY
		if dx != d.X1 || dy != d.Y1 {
			offX, offY := dx-d.X1, dy-d.Y1
			d.MoveRelative(offX, offY)
		}
		return true
	}

	// 1.5 Handle Active Resizing
	if d.isResizing {
		if !e.KeyDown && e.ButtonState == 0 {
			d.isResizing = false
			return true
		}
		newW := mx - d.X1 + 1
		newH := my - d.Y1 + 1
		if newW < d.MinW { newW = d.MinW }
		if newH < d.MinH { newH = d.MinH }
		d.ChangeSize(newW, newH)
		return true
	}

	// 2. Check elements
	for i := len(d.items) - 1; i >= 0; i-- {
		item := d.items[i]
		x1, y1, x2, y2 := item.GetPosition()
		if mx >= x1 && mx <= x2 && my >= y1 && my <= y2 {
			if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				if item.CanFocus() && d.focusIdx != i {
					if d.focusIdx != -1 {
						d.items[d.focusIdx].SetFocus(false)
					}
					d.focusIdx = i
					item.SetFocus(true)
				}
			}
			if item.ProcessMouse(e) {
				return true
			}
			if rb, ok := item.(*RadioButton); ok && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
				d.selectRadio(rb)
				return true
			}
			return true
		}
	}

	// 3. Initiate Dragging or Resizing (if click on border or background)
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		if mx == d.X2 && my == d.Y2 {
			d.isResizing = true
			return true
		}
		if mx >= d.X1 && mx <= d.X2 && my >= d.Y1 && my <= d.Y2 {
			d.isDragging = true
			d.dragOffX = mx - d.X1
			d.dragOffY = my - d.Y1
			return true
		}
	}

	return false
}

// MoveRelative shifts the dialog and all its children by offset.
func (d *Dialog) MoveRelative(dx, dy int) {
	d.X1 += dx
	d.X2 += dx
	d.Y1 += dy
	d.Y2 += dy
	d.frame.SetPosition(d.X1, d.Y1, d.X2, d.Y2)
	for _, item := range d.items {
		ix1, iy1, ix2, iy2 := item.GetPosition()
		// We need to ensure UIElement has SetPosition or similar. 
		// Since most implement ScreenObject, we can type assert.
		if so, ok := item.(interface{ SetPosition(int, int, int, int) }); ok {
			so.SetPosition(ix1+dx, iy1+dy, ix2+dx, iy2+dy)
		}
	}
}
