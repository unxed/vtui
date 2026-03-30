package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// Checkbox represents a flag with 2 or 3 states.
type Checkbox struct {
	ScreenObject
	Text       string
	State      int  // 0 - Unchecked, 1 - Checked, 2 - Undefined (3-state)
	ThreeState bool // Enable support for the third state
}

func NewCheckbox(x, y int, text string, threeState bool) *Checkbox {
	cb := &Checkbox{
		Text:       text,
		ThreeState: threeState,
	}
	clean, hk, _ := ParseAmpersandString(text)
	cb.hotkey = hk
	cb.canFocus = true
	// Format: "[x] Text"
	vLen := 4 + runewidth.StringWidth(clean)
	cb.SetPosition(x, y, x+vLen-1, y)
	return cb
}

func (cb *Checkbox) Show(scr *ScreenBuf) {
	cb.ScreenObject.Show(scr)
	cb.DisplayObject(scr)
}

func (cb *Checkbox) DisplayObject(scr *ScreenBuf) {
	if !cb.IsVisible() { return }

	attr := Palette[ColDialogText]
	highAttr := Palette[ColDialogHighlightText]
	if cb.IsFocused() {
		attr = Palette[ColDialogSelectedButton]
		highAttr = Palette[ColDialogHighlightSelectedButton]
	}

	char := " "
	switch cb.State {
	case 1:
		char = "x"
	case 2:
		char = "?" // Symbol for undefined state
	}

	cells, _ := StringToCharInfoHighlighted("["+char+"] "+cb.Text, attr, highAttr)
	scr.Write(cb.X1, cb.Y1, cells)
}

func (cb *Checkbox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }

	if e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_RETURN {
		cb.Toggle()
		return true
	}
	return false
}

func (cb *Checkbox) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		cb.Toggle()
		return true
	}
	return false
}

func (cb *Checkbox) Toggle() {
	if cb.ThreeState {
		cb.State = (cb.State + 1) % 3
	} else {
		if cb.State == 0 {
			cb.State = 1
		} else {
			cb.State = 0
		}
	}
}

func (cb *Checkbox) GetData() any {
	return cb.State
}

func (cb *Checkbox) SetData(val any) {
	if i, ok := val.(int); ok {
		cb.State = i
	} else if b, ok := val.(bool); ok {
		if b {
			cb.State = 1
		} else {
			cb.State = 0
		}
	}
}
