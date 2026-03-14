package vtui
import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// Button represents an interactive button.
type Button struct {
	ScreenObject
	text          string
	OnClick       func()
}

func NewButton(x, y int, text string) *Button {
	// Buttons in Far always look like "[ Text ]"
	fullText := string(UIStrings.ButtonBrackets[0]) + " " + text + " " + string(UIStrings.ButtonBrackets[1])
	b := &Button{
		text: fullText,
	}
	b.canFocus = true
	vLen := runewidth.StringWidth(fullText)
	b.SetPosition(x, y, x+vLen-1, y)
	return b
}

func (b *Button) Show(scr *ScreenBuf) {
	b.ScreenObject.Show(scr)
	b.DisplayObject(scr)
}

func (b *Button) DisplayObject(scr *ScreenBuf) {
	if !b.IsVisible() { return }
	attr := Palette[ColDialogButton]
	if b.IsFocused() {
		attr = Palette[ColDialogSelectedButton]
	}
	scr.Write(b.X1, b.Y1, StringToCharInfo(b.text, attr))
}

func (b *Button) SetFocus(f bool) {
	DebugLog("  Button(%s): SetFocus(%v)", b.text, f)
	b.focused = f
}

func (b *Button) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	if e.VirtualKeyCode == vtinput.VK_RETURN || e.VirtualKeyCode == vtinput.VK_SPACE {
		if b.OnClick != nil {
			b.OnClick()
		}
		return true
	}
	return false
}

func (b *Button) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		if b.OnClick != nil {
			b.OnClick()
		}
		return true
	}
	return false
}