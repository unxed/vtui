package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// Checkbox представляет собой флажок с 2 или 3 состояниями.
type Checkbox struct {
	ScreenObject
	Text       string
	State      int  // 0 - Unchecked, 1 - Checked, 2 - Undefined (3-state)
	ThreeState bool // Включить поддержку третьего состояния
}

func NewCheckbox(x, y int, text string, threeState bool) *Checkbox {
	cb := &Checkbox{
		Text:       text,
		ThreeState: threeState,
	}
	cb.canFocus = true
	// Формат: "[x] Текст"
	vLen := 4 + runewidth.StringWidth(text)
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
	if cb.IsFocused() {
		attr = Palette[ColDialogSelectedButton]
	}

	char := " "
	switch cb.State {
	case 1:
		char = "x"
	case 2:
		char = "?" // Символ для неопределенного состояния
	}

	scr.Write(cb.X1, cb.Y1, StringToCharInfo("["+char+"] "+cb.Text, attr))
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