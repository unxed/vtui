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
	OnChange   func(int)
}


func NewCheckbox(x, y int, text string, threeState bool) *Checkbox {
	cb := &Checkbox{ThreeState: threeState}
	cb.X1, cb.Y1 = x, y
	cb.Y2 = y
	cb.canFocus = true
	cb.SetText(text)
	// Prefix "[x] " is 4 columns wide
	cb.X2 = cb.X1 + 4 + runewidth.StringWidth(cb.cleanText) - 1
	return cb
}

func (cb *Checkbox) Show(scr *ScreenBuf) {
	cb.ScreenObject.Show(scr)
	cb.DisplayObject(scr)
}

func (cb *Checkbox) DisplayObject(scr *ScreenBuf) {
	if !cb.IsVisible() { return }

	attr, highAttr := cb.ResolveColors(ColDialogText, ColDialogSelectedButton, ColDialogHighlightText, ColDialogHighlightSelectedButton)

	char := " "
	switch cb.State {
	case 1:
		char = "x"
	case 2:
		char = "?"
	}

	p := NewPainter(scr)
	prefix := "[" + char + "] "
	p.DrawString(cb.X1, cb.Y1, prefix, attr)

	p.DrawHighlightedText(cb.X1+runewidth.StringWidth(prefix), cb.Y1, cb.cleanText, cb.hotkeyPos, attr, highAttr)
}

func (cb *Checkbox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }
	if cb.IsDisabled() { return false }

	if e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_RETURN {
		cb.Toggle()
		return true
	}
	return false
}

func (cb *Checkbox) ProcessMouse(e *vtinput.InputEvent) bool {
	if cb.IsDisabled() { return false }
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
	var onClick func()
	if cb.OnChange != nil {
		onClick = func() { cb.OnChange(cb.State) }
	}
	cb.FireAction(onClick, cb.State)
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
