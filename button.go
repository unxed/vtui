package vtui
import (
	"github.com/unxed/vtinput"
)

// Button представляет собой интерактивную кнопку.
type Button struct {
	ScreenObject
	text       string
	colorNormal   uint64
	colorFocused  uint64
}

func NewButton(x, y int, text string) *Button {
	// Кнопка в Far всегда имеет вид "[ Text ]"
	fullText := string(boxSymbols[24]) + " " + text + " " + string(boxSymbols[25])
	b := &Button{
		text:         fullText,
		colorNormal:  SetRGBBoth(0, 0xCCCCCC, 0x0000A0), // Серый на синем
		colorFocused: SetRGBBoth(0, 0x000000, 0x00AAAA), // Черный на бирюзовом
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
	attr := b.colorNormal
	if b.IsFocused() {
		attr = b.colorFocused
	}
	scr.Write(b.X1, b.Y1, stringToCharInfo(b.text, attr))
}

// Простая заглушка для реализации интерфейса focusable
func (b *Button) ProcessKey(e *vtinput.InputEvent) bool {
	return false
}