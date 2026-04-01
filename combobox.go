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
		Edit: NewEdit(x, y, width-1, ""), // Leave 1 character on the right for the arrow
		Menu: NewVMenu(""),
	}
	cb.canFocus = true

	for _, item := range items {
		cb.Menu.AddItem(MenuItem{Text: item})
	}

	// Set menu behavior
	cb.Menu.SetOwner(cb)
	cb.Menu.OnAction = func(idx int) {
		cb.Edit.SetText(cb.Menu.items[idx].Text)
	}

	cb.SetPosition(x, y, x+width-1, y)
	return cb
}

func (cb *ComboBox) SetPosition(x1, y1, x2, y2 int) {
	cb.ScreenObject.SetPosition(x1, y1, x2, y2)
	cb.Edit.SetPosition(x1, y1, x2-1, y1)
}

func (cb *ComboBox) Show(scr *ScreenBuf) {
	cb.ScreenObject.Show(scr)
	cb.DisplayObject(scr)
}

func (cb *ComboBox) DisplayObject(scr *ScreenBuf) {
	if !cb.IsVisible() { return }

	// Rendering edit field
	cb.Edit.focused = cb.focused
	cb.Edit.Show(scr)

	// Rendering dropdown arrow
	attr := Palette[ColDialogText]
	if cb.focused {
		attr = Palette[ColDialogSelectedButton]
	}
	if cb.IsDisabled() {
		attr = DimColor(attr)
	}
	// Use symbol ↓ (U+2193)
	scr.Write(cb.X2, cb.Y1, StringToCharInfo("↓", attr))
}

func (cb *ComboBox) ProcessKey(e *vtinput.InputEvent) bool {

	if !e.KeyDown { return false }
	if cb.IsDisabled() { return false }

	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

	// Alt+Down opens the list
	if e.VirtualKeyCode == vtinput.VK_DOWN && alt {
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
	// Calculate menu position below combo box
	h := len(cb.Menu.items) + 2
	if h > 10 { h = 10 } // Limit height

	// If little space below combo box, open upwards (simplified)
	cb.Menu.SetPosition(cb.X1, cb.Y1+1, cb.X2, cb.Y1+h)
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
