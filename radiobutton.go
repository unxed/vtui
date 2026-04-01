package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// RadioButton represents a toggle switch in a group.
type RadioButton struct {
	ScreenObject
	Text     string
	Selected bool
}

func NewRadioButton(x, y int, text string) *RadioButton {
	rb := &RadioButton{
		Text: text,
	}
	clean, hk, _ := ParseAmpersandString(text)
	rb.hotkey = hk
	rb.canFocus = true
	// Format: "( ) Text" or "(•) Text"
	vLen := 4 + runewidth.StringWidth(clean)
	rb.SetPosition(x, y, x+vLen-1, y)
	return rb
}

func (rb *RadioButton) Show(scr *ScreenBuf) {
	rb.ScreenObject.Show(scr)
	rb.DisplayObject(scr)
}

func (rb *RadioButton) DisplayObject(scr *ScreenBuf) {
	if !rb.IsVisible() { return }

	attr, highAttr := rb.ResolveColors(ColDialogText, ColDialogSelectedButton, ColDialogHighlightText, ColDialogHighlightSelectedButton)

	state := "( ) "
	if rb.Selected {
		state = "(•) "
	}

	cells, _ := StringToCharInfoHighlighted(state+rb.Text, attr, highAttr)
	scr.Write(rb.X1, rb.Y1, cells)
}

func (rb *RadioButton) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }
	if rb.IsDisabled() { return false }
	if e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_RETURN {
		// The button itself doesn't change state directly; Dialog will handle that.
		return false // Return false so Dialog catches the event and updates the group
	}
	return false
}

func (rb *RadioButton) ProcessMouse(e *vtinput.InputEvent) bool {
	if rb.IsDisabled() { return false }
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		return false // Let the dialog handle the click and update the group
	}
	return false
}