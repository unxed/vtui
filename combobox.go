package vtui

import (
	"github.com/unxed/vtinput"
)

// ComboBox combines an edit field and a dropdown menu.
type ComboBox struct {
	ScreenObject
	Edit         *Edit
	Menu         *VMenu
	DropdownOnly bool // If true, manual text entry is not allowed
}

func NewComboBox(x, y, width int, items []string) *ComboBox {
	cb := &ComboBox{
		Edit: NewEdit(0, 0, width-1, ""),
		Menu: NewVMenu(""),
	}
	cb.canFocus = true

	for _, item := range items {
		cb.Menu.AddItem(MenuItem{Text: item})
	}

	cb.Menu.SetOwner(cb)
	cb.Menu.OnAction = func(idx int) {
		cb.Edit.SetText(cb.Menu.Items[idx].Text)
	}

	cb.SetPosition(x, y, x+width-1, y)
	return cb
}

func (cb *ComboBox) SetPosition(x1, y1, x2, y2 int) {
	cb.ScreenObject.SetPosition(x1, y1, x2, y2)
	cb.applyLayout()
}

func (cb *ComboBox) applyLayout() {
	hbox := NewHBoxLayout(cb.X1, cb.Y1, cb.X2-cb.X1+1, 1)
	hbox.Spacing = 0
	hbox.Add(cb.Edit, Margins{}, AlignFill)
	// Add a dummy text element for the arrow to participate in the layout math
	arrow := NewText(0, 0, "↓", 0)
	hbox.Add(arrow, Margins{}, AlignTop)
	hbox.Apply()
}

func (cb *ComboBox) Show(scr *ScreenBuf) {
	cb.ScreenObject.Show(scr)
	cb.DisplayObject(scr)
}

func (cb *ComboBox) DisplayObject(scr *ScreenBuf) {
	if !cb.IsVisible() {
		return
	}

	cb.Edit.focused = cb.focused
	if cb.DropdownOnly {
		cb.Edit.HideCursor = true
		if cb.focused {
			// Visually highlight the entire field as selected when focused
			oldStart, oldEnd := cb.Edit.selStart, cb.Edit.selEnd
			cb.Edit.selStart = 0
			cb.Edit.selEnd = len(cb.Edit.text)
			cb.Edit.Show(scr)
			cb.Edit.selStart, cb.Edit.selEnd = oldStart, oldEnd
		} else {
			cb.Edit.Show(scr)
		}
	} else {
		cb.Edit.HideCursor = false
		cb.Edit.Show(scr)
	}

	attr := Palette[ColDialogText]
	if cb.focused {
		attr = Palette[ColDialogSelectedButton]
	}
	if cb.IsDisabled() {
		attr = DimColor(attr)
	}
	scr.Write(cb.X2, cb.Y1, StringToCharInfo("↓", attr))
}

func (cb *ComboBox) ProcessKey(e *vtinput.InputEvent) bool {

	if !e.KeyDown { return false }
	if cb.IsDisabled() { return false }

	ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0

	// Ctrl+Down opens the list
	if e.VirtualKeyCode == vtinput.VK_DOWN && ctrl {
		cb.Open()
		return true
	}

	// In DropdownOnly mode Enter opens the list
	if e.VirtualKeyCode == vtinput.VK_RETURN && cb.DropdownOnly {
		cb.Open()
		return true
	}

	// If not DropdownOnly, pass keys to Edit
	if !cb.DropdownOnly {
		if cb.Edit.ProcessKey(e) {
			return true
		}
	}

	return false
}

func (cb *ComboBox) ProcessMouse(e *vtinput.InputEvent) bool {
	if cb.IsDisabled() { return false }
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx := int(e.MouseX)
		// If arrow clicked
		if mx == cb.X2 {
			cb.Open()
			return true
		}
	}
	return cb.Edit.ProcessMouse(e)
}

func (cb *ComboBox) Open() {
	if cb.IsDisabled() {
		return
	}
	// Calculate menu position below combo box
	h := len(cb.Menu.Items) + 2
	if h > 10 { h = 10 } // Limit height

	y := cb.Y1 + 1
	if FrameManager != nil && FrameManager.scr != nil {
		// If it doesn't fit below, and there is more space above, flip it
		if y+h > FrameManager.scr.height && cb.Y1 >= h {
			y = cb.Y1 - h
		}
	}
	cb.Menu.SetPosition(cb.X1, y, cb.X2, y+h-1)
	cb.Menu.ClearDone()
	cb.Menu.HideShadow = true
	FrameManager.Push(cb.Menu)
}

func (cb *ComboBox) SetFocus(f bool) {
	cb.focused = f
	cb.Edit.SetFocus(f)
}

func (cb *ComboBox) SetDisabled(d bool) {
	cb.ScreenObject.SetDisabled(d)
	cb.Edit.SetDisabled(d)
}
func (cb *ComboBox) WantsChars() bool {
	return !cb.DropdownOnly
}
