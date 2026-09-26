package vtui

import (
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

// RadioButton represents an individual radio button widget.
type RadioButton struct {
	ScreenObject
	Selected bool
	OnChange func(bool)
}

func NewRadioButton(x, y int, text string, selected bool) *RadioButton {
	rb := &RadioButton{Selected: selected}
	rb.X1, rb.Y1 = x, y
	rb.Y2 = y
	rb.canFocus = true
	rb.SetText(text)
	// "(•) " is 4 columns wide
	rb.X2 = rb.X1 + 4 + runewidth.StringWidth(rb.cleanText) - 1
	return rb
}

func (rb *RadioButton) Show(scr *ScreenBuf) {
	rb.ScreenObject.Show(scr)
	rb.DisplayObject(scr)
}

func (rb *RadioButton) DisplayObject(scr *ScreenBuf) {
	if !rb.IsVisible() {
		return
	}
	n, h := rb.GetStateAttrs(ColDialogText, ColDialogSelectedButton, ColDialogHighlightText, ColDialogHighlightSelectedButton)

	sym := SymRadioOff
	if rb.Selected {
		sym = SymRadioOn
	}

	// The indicator ("( )", 3 cells) and the space after it are drawn
	// separately so the indicator can carry DialogIndicatorAttr while the
	// space keeps the plain text attribute, exactly as the previous literal
	// "( ) " prefix did when its first 3 cells were redrawn on top.
	p := NewPainter(scr)
	p.DrawSymGlyph(rb.X1, rb.Y1, sym, DialogIndicatorAttr(n, rb.IsFocused()))
	p.DrawString(rb.X1+3, rb.Y1, " ", n)
	p.DrawHighlightedText(rb.X1+4, rb.Y1, rb.cleanText, rb.hotkeyPos, n, h)
}

func (rb *RadioButton) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || rb.IsDisabled() {
		return false
	}
	if e.VirtualKeyCode == vtinput.VK_SPACE || e.Char == ' ' {
		rb.Select()
		return true
	}
	return false
}

func (rb *RadioButton) ProcessMouse(e *vtinput.InputEvent) bool {
	if rb.IsDisabled() {
		return false
	}
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && IsMousePress(e) {
		rb.Select()
		return true
	}
	return false
}

func (rb *RadioButton) Select() {
	if !rb.Selected {
		rb.Selected = true
		if rb.OnChange != nil {
			rb.OnChange(true)
		}
		rb.FireAction(nil, true)
		rb.NotifyChange()
	}
}

func (rb *RadioButton) GetData() any {
	return rb.Selected
}

func (rb *RadioButton) SetData(val any) {
	if b, ok := val.(bool); ok {
		rb.Selected = b
	}
}
