package vtui

import "github.com/unxed/vtinput"

// Button represents an interactive button.
type Button struct {
	ScreenObject
	OnClick      func()
	IsDefault    bool
	caption      string
	mouseArmed   bool
	mousePressed bool
}

func NewButton(x, y int, text string) *Button {
	b := &Button{}
	b.X1, b.Y1 = x, y
	b.Y2 = y
	b.canFocus = true
	// Buttons in Far always look like "[ Text ]"
	b.SetText(text)
	// Calculate width based on the parsed clean text
	b.X2 = b.X1 + StringWidth(b.cleanText) - 1
	return b
}

// SetText assigns the button caption. The stored text is always decorated
// with the Far-style brackets, while the bare caption is kept aside, so the
// semantic export and external tooling can use it as is.
func (b *Button) SetText(text string) {
	b.caption, _, _ = ParseAmpersandString(text)
	b.ScreenObject.SetText(string(UIStrings.ButtonBrackets[0]) + " " + text + " " + string(UIStrings.ButtonBrackets[1]))
}

// GetCaption returns the button caption without the decorating brackets
// and without the ampersand hotkey marker.
func (b *Button) GetCaption() string {
	return b.caption
}

func (b *Button) Show(scr *ScreenBuf) {
	b.ScreenObject.Show(scr)
	b.DisplayObject(scr)
}

func (b *Button) DisplayObject(scr *ScreenBuf) {
	if !b.IsVisible() {
		return
	}
	normalIdx := ColDialogButton
	if b.IsDefault {
		normalIdx = ColDialogHighlightButton
	}
	n, h := b.GetStateAttrs(normalIdx, ColDialogSelectedButton, ColDialogHighlightButton, ColDialogHighlightSelectedButton)
	if b.mousePressed {
		n, h = b.GetStateAttrs(ColDialogSelectedButton, ColDialogSelectedButton, ColDialogHighlightSelectedButton, ColDialogHighlightSelectedButton)
	}
	NewPainter(scr).DrawHighlightedText(b.X1, b.Y1, b.cleanText, b.hotkeyPos, n, h)
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
	if b.IsDisabled() {
		b.mouseArmed = false
		b.mousePressed = false
		return false
	}

	if b.mouseArmed {
		inside := b.HitTest(int(e.MouseX), int(e.MouseY))
		if e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 {
			b.mousePressed = inside
			return true
		}

		activate := b.mousePressed && inside
		b.mouseArmed = false
		b.mousePressed = false
		if activate {
			b.FireAction(b.OnClick, nil)
		}
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown && e.MouseEventFlags&vtinput.MouseMoved == 0 && b.HitTest(int(e.MouseX), int(e.MouseY)) {
		b.mouseArmed = true
		b.mousePressed = true
		return true
	}
	return false
}

func (b *Button) SetDisabled(d bool) {
	b.ScreenObject.SetDisabled(d)
	if d {
		b.mouseArmed = false
		b.mousePressed = false
	}
}

func (b *Button) SizeSpecH() SizeSpec {
	if b.sizeSpecH != nil {
		return *b.sizeSpecH
	}
	w := StringWidth(b.cleanText)
	if w < 8 {
		w = 8
	}
	return SizeSpec{
		Hint:    w,
		Min:     6,
		Policy:  PolicyFixed,
		Stretch: 1,
	}
}

func (b *Button) SizeSpecV() SizeSpec {
	if b.sizeSpecV != nil {
		return *b.sizeSpecV
	}
	return SizeSpec{
		Hint:    1,
		Min:     1,
		Policy:  PolicyFixed,
		Stretch: 1,
	}
}
