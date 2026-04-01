package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// Button represents an interactive button.
type Button struct {
	ScreenObject
	text      string
	OnClick   func()
	IsDefault bool
}


func NewButton(x, y int, text string) *Button {
	b := &Button{}
	b.X1, b.Y1 = x, y
	b.Y2 = y
	b.canFocus = true
	// Buttons in Far always look like "[ Text ]"
	b.SetText(string(UIStrings.ButtonBrackets[0]) + " " + text + " " + string(UIStrings.ButtonBrackets[1]))
	// Calculate width based on the parsed clean text
	b.X2 = b.X1 + runewidth.StringWidth(b.cleanText) - 1
	return b
}

func (b *Button) Show(scr *ScreenBuf) {
	b.ScreenObject.Show(scr)
	b.DisplayObject(scr)
}

func (b *Button) DisplayObject(scr *ScreenBuf) {
	if !b.IsVisible() { return }
	attr, highAttr := b.ResolveColors(ColDialogButton, ColDialogSelectedButton, ColDialogHighlightButton, ColDialogHighlightSelectedButton)
	p := NewPainter(scr)
	p.DrawHighlightedText(b.X1, b.Y1, b.cleanText, b.hotkeyPos, attr, highAttr)
}


func (b *Button) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	if b.IsDisabled() {
		return false
	}
	if e.VirtualKeyCode == vtinput.VK_RETURN || e.VirtualKeyCode == vtinput.VK_SPACE {
		return b.FireAction(b.OnClick, nil)
	}
	return false
}

func (b *Button) ProcessMouse(e *vtinput.InputEvent) bool {
	if b.IsDisabled() { return false }
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		return b.FireAction(b.OnClick, nil)
	}
	return false
}