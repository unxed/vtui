package vtui

import (
	"github.com/unxed/vtinput"
)

// Checkbox represents a flag with 2 or 3 states.
type Checkbox struct {
	ScreenObject
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
	// Prefix "[x] " is 4 columns wide. The label is measured with vtui's own
	// StringWidth, the number DisplayObject then paints: go-runewidth counts
	// runes and knows nothing of grapheme clusters, so on a Bengali or Hindi
	// label it reported fewer columns than the checkbox draws and the widget
	// painted outside its own box (unxed/f4#546).
	cb.X2 = cb.X1 + 4 + StringWidth(cb.cleanText) - 1
	return cb
}

func (cb *Checkbox) Show(scr *ScreenBuf) {
	cb.ScreenObject.Show(scr)
	cb.DisplayObject(scr)
}

func (cb *Checkbox) DisplayObject(scr *ScreenBuf) {
	if !cb.IsVisible() {
		return
	}
	n, h := cb.GetStateAttrs(ColDialogText, ColDialogSelectedButton, ColDialogHighlightText, ColDialogHighlightSelectedButton)

	mark := " "
	if cb.State == 1 {
		mark = "x"
	} else if cb.State == 2 {
		mark = "?"
	}
	prefix := "[" + mark + "] "

	p := NewPainter(scr)
	p.DrawString(cb.X1, cb.Y1, prefix, n)
	p.DrawHighlightedText(cb.X1+StringWidth(prefix), cb.Y1, cb.cleanText, cb.hotkeyPos, n, h)
}

func (cb *Checkbox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	if cb.IsDisabled() {
		return false
	}

	if e.VirtualKeyCode == vtinput.VK_SPACE || e.Char == ' ' {
		cb.Toggle()
		return true
	}
	return false
}

func (cb *Checkbox) ProcessMouse(e *vtinput.InputEvent) bool {
	if cb.IsDisabled() {
		return false
	}
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
	cb.NotifyChange()
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
