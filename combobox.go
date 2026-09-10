package vtui

import (
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

// ComboBox combines an edit field and a dropdown menu.
type ComboBox struct {
	ScreenObject
	Edit              *Edit
	Menu              *VMenu
	DropdownOnly      bool // If true, manual text entry is not allowed
	editMouseCaptured bool
	editMouseMoved    bool
}

func NewComboBox(x, y, width int, items []string) *ComboBox {
	cb := &ComboBox{
		Edit: NewEdit(0, 0, width-1, ""),
		Menu: NewVMenu(""),
	}
	cb.canFocus = true

	cb.Edit.ColorTextIdx = ColDialogComboText
	cb.Edit.ColorSelectedIdx = ColDialogComboSelectedText

	for _, item := range items {
		cb.Menu.AddItem(MenuItem{Text: item})
	}

	cb.Menu.ColorTextIdx = ColDialogComboText
	cb.Menu.ColorSelectedTextIdx = ColDialogComboSelectedText
	cb.Menu.ColorHighlightIdx = ColDialogComboHighlight
	cb.Menu.ColorSelectedHighlightIdx = ColDialogComboSelectedHighlight
	cb.Menu.ColorBoxIdx = ColDialogComboBox
	cb.Menu.ColorTitleIdx = ColDialogComboTitle
	cb.Menu.BoxType = SingleBox
	if cb.Menu.ScrollBar != nil {
		cb.Menu.ScrollBar.ColorIdx = ColDialogComboScrollbar
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

func (cb *ComboBox) MoveRelative(dx, dy int) {
	cb.ScreenObject.MoveRelative(dx, dy)
	cb.applyLayout()
}

func (cb *ComboBox) applyLayout() {
	// The arrow is painted by ComboBox.DisplayObject at X2. Keep the edit
	// portion adjacent to it whenever the outer control is resized by a
	// layout. A generic HBox cannot express this one-cell trailing affordance
	// because its AlignFill mode only stretches the cross-axis.
	editX2 := cb.X2
	if cb.X2 > cb.X1 {
		editX2--
	}
	cb.Edit.SetPosition(cb.X1, cb.Y1, editX2, cb.Y1)
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

	bgIdx := cb.Edit.ColorTextIdx
	fgIdx := ColDialogComboHighlight

	if cb.DropdownOnly {
		cb.Edit.HideCursor = true
		if cb.focused {
			bgIdx = cb.Edit.ColorSelectedIdx
			fgIdx = ColDialogComboSelectedHighlight

			oldTextIdx := cb.Edit.ColorTextIdx
			oldStart, oldEnd := cb.Edit.selStart, cb.Edit.selEnd

			cb.Edit.ColorTextIdx = cb.Edit.ColorSelectedIdx
			cb.Edit.selStart = 0
			cb.Edit.selEnd = len(cb.Edit.text)

			cb.Edit.Show(scr)

			cb.Edit.ColorTextIdx = oldTextIdx
			cb.Edit.selStart, cb.Edit.selEnd = oldStart, oldEnd
		} else {
			cb.Edit.Show(scr)
		}
	} else {
		cb.Edit.HideCursor = false
		cb.Edit.Show(scr)
	}

	arrowAttr := withBackground(Palette[fgIdx], Palette[bgIdx])
	if cb.IsDisabled() {
		arrowAttr = DimColor(arrowAttr)
	}
	scr.Write(cb.X2, cb.Y1, StringToCharInfo("↓", arrowAttr))
}

// withBackground keeps the foreground and text attributes from attr while
// taking the background from backgroundAttr. ComboBox arrows are rendered as
// a separate cell, but visually belong to the edit field beside them.
func withBackground(attr, backgroundAttr uint64) uint64 {
	if backgroundAttr&IsBgRGB != 0 {
		attr = SetRGBBack(attr, GetRGBBack(backgroundAttr))
	} else {
		attr = SetIndexBack(attr, GetIndexBack(backgroundAttr))
	}
	return attr&^BackgroundIntensity | backgroundAttr&BackgroundIntensity
}

func (cb *ComboBox) ProcessKey(e *vtinput.InputEvent) bool {

	if !e.KeyDown {
		return false
	}
	if cb.IsDisabled() {
		return false
	}

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
	if cb.IsDisabled() {
		return false
	}
	if cb.editMouseCaptured {
		cb.Edit.ProcessMouse(e)
		if e.MouseEventFlags&vtinput.MouseMoved != 0 && e.ButtonState != 0 {
			cb.editMouseMoved = true
		}
		if IsMouseRelease(e) {
			openMenu := !cb.editMouseMoved
			cb.editMouseCaptured = false
			cb.editMouseMoved = false
			if openMenu {
				cb.Open()
			}
			return true
		}
		return true
	}
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown && e.MouseEventFlags&vtinput.MouseMoved == 0 {
		mx := int(e.MouseX)
		// The arrow and DropdownOnly controls open on press. For an editable
		// control, defer opening until release so a drag can still select text.
		if mx == cb.X2 || cb.DropdownOnly {
			cb.Open()
			cb.Menu.BeginMouseSelection()
			return true
		}
		if cb.Edit.HitTest(mx, int(e.MouseY)) {
			cb.Edit.ProcessMouse(e)
			cb.editMouseCaptured = true
			cb.editMouseMoved = false
			return true
		}
	}
	return cb.Edit.ProcessMouse(e)
}

func (cb *ComboBox) Open() {
	if cb.IsDisabled() {
		return
	}

	// 1. Calculate required width based on items
	maxWidth := cb.X2 - cb.X1 + 1
	for _, itm := range cb.Menu.Items {
		// We add 4 for: 1 leading space, 1 trailing space, 2 for borders
		clean, _, _ := ParseAmpersandString(itm.Text)
		w := runewidth.StringWidth(clean) + 4
		if w > maxWidth {
			maxWidth = w
		}
	}

	// 2. Calculate height and vertical position
	h := len(cb.Menu.Items) + 2
	if h > 10 {
		h = 10
	} // Limit height

	y := cb.Y1 + 1
	if FrameManager != nil && FrameManager.scr != nil {
		// If it doesn't fit below, and there is more space above, flip it
		if y+h > FrameManager.scr.height && cb.Y1 >= h {
			y = cb.Y1 - h
		}
		// Horizontal safety: don't let it go off-screen to the right
		if cb.X1+maxWidth > FrameManager.scr.width {
			// In this case, we just cap it to screen edge
			maxWidth = FrameManager.scr.width - cb.X1
		}
	}

	cb.Menu.SetPosition(cb.X1, y, cb.X1+maxWidth-1, y+h-1)
	cb.Menu.ClearDone()
	cb.Menu.HideShadow = true
	FrameManager.Push(cb.Menu)
}

func (cb *ComboBox) SetFocus(f bool) {
	cb.focused = f
	cb.Edit.SetFocus(f)
	if !f {
		cb.editMouseCaptured = false
		cb.editMouseMoved = false
	}
}

func (cb *ComboBox) SetDisabled(d bool) {
	cb.ScreenObject.SetDisabled(d)
	cb.Edit.SetDisabled(d)
	if d {
		cb.editMouseCaptured = false
		cb.editMouseMoved = false
	}
}
func (cb *ComboBox) WantsChars() bool {
	return !cb.DropdownOnly
}
