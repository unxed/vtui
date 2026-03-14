package vtui
import (
	"github.com/unxed/vtinput"
)

// Button represents an interactive button.
type Button struct {
	ScreenObject
	text          string
	OnClick       func()
}

func NewButton(x, y int, text string) *Button {
	// Buttons in Far always look like "[ Text ]"
	fullText := string(boxSymbols[24]) + " " + text + " " + string(boxSymbols[25])
	b := &Button{
		text: fullText,
	}
	b.canFocus = true
	b.SetPosition(x, y, x+len([]rune(fullText))-1, y)
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