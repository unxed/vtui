package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// RadioButton представляет собой переключатель в группе.
type RadioButton struct {
	ScreenObject
	Text     string
	Selected bool
}

func NewRadioButton(x, y int, text string) *RadioButton {
	rb := &RadioButton{
		Text: text,
	}
	rb.canFocus = true
	// Формат: "( ) Текст" или "(•) Текст"
	vLen := 4 + runewidth.StringWidth(text)
	rb.SetPosition(x, y, x+vLen-1, y)
	return rb
}

func (rb *RadioButton) Show(scr *ScreenBuf) {
	rb.ScreenObject.Show(scr)
	rb.DisplayObject(scr)
}

func (rb *RadioButton) DisplayObject(scr *ScreenBuf) {
	if !rb.IsVisible() { return }

	attr := Palette[ColDialogText]
	if rb.IsFocused() {
		attr = Palette[ColDialogSelectedButton] // Используем стиль активного элемента
	}

	state := "( ) "
	if rb.Selected {
		state = "(•) "
	}

	scr.Write(rb.X1, rb.Y1, StringToCharInfo(state+rb.Text, attr))
}

func (rb *RadioButton) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }
	if e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_RETURN {
		// Сама кнопка не меняет состояние напрямую, это сделает Dialog
		return false // Возвращаем false, чтобы Dialog поймал событие и обновил группу
	}
	return false
}

func (rb *RadioButton) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		return false // Даем диалогу обработать клик и обновить группу
	}
	return false
}