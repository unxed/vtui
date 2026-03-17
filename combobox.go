package vtui

import (
	"github.com/unxed/vtinput"
)

// ComboBox объединяет поле ввода и выпадающее меню.
type ComboBox struct {
	ScreenObject
	Edit         *Edit
	Menu         *VMenu
	DropdownOnly bool // Если true, нельзя вводить свой текст
}

func NewComboBox(x, y, width int, items []string) *ComboBox {
	cb := &ComboBox{
		Edit: NewEdit(x, y, width-1, ""), // Оставляем 1 символ справа под стрелку
		Menu: NewVMenu(""),
	}
	cb.canFocus = true

	for _, item := range items {
		cb.Menu.AddItem(item)
	}

	// Настраиваем поведение меню
	cb.Menu.OnSelect = func(idx int) {
		cb.Edit.SetText(cb.Menu.items[idx].Text)
	}

	cb.SetPosition(x, y, x+width-1, y)
	return cb
}

func (cb *ComboBox) SetPosition(x1, y1, x2, y2 int) {
	cb.ScreenObject.SetPosition(x1, y1, x2, y2)
	cb.Edit.SetPosition(x1, y1, x2-1, y1)
}

func (cb *ComboBox) Show(scr *ScreenBuf) {
	cb.ScreenObject.Show(scr)
	cb.DisplayObject(scr)
}

func (cb *ComboBox) DisplayObject(scr *ScreenBuf) {
	if !cb.IsVisible() { return }

	// Отрисовка текстового поля
	cb.Edit.focused = cb.focused
	cb.Edit.Show(scr)

	// Отрисовка стрелки выпадающего списка
	attr := Palette[ColDialogText]
	if cb.focused {
		attr = Palette[ColDialogSelectedButton]
	}
	// Используем символ ↓ (U+2193)
	scr.Write(cb.X2, cb.Y1, StringToCharInfo("↓", attr))
}

func (cb *ComboBox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }

	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

	// Alt+Down открывает список
	if e.VirtualKeyCode == vtinput.VK_DOWN && alt {
		cb.Open()
		return true
	}

	// В режиме DropdownOnly Enter открывает список
	if e.VirtualKeyCode == vtinput.VK_RETURN && cb.DropdownOnly {
		cb.Open()
		return true
	}

	// Если не DropdownOnly, прокидываем клавиши в Edit
	if !cb.DropdownOnly {
		if cb.Edit.ProcessKey(e) {
			return true
		}
	}

	return false
}

func (cb *ComboBox) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx := int(e.MouseX)
		// Если кликнули по стрелке
		if mx == cb.X2 {
			cb.Open()
			return true
		}
	}
	return cb.Edit.ProcessMouse(e)
}

func (cb *ComboBox) Open() {
	// Рассчитываем позицию меню под комбобоксом
	h := len(cb.Menu.items) + 2
	if h > 10 { h = 10 } // Ограничиваем высоту

	// Если под комбобоксом мало места, открываем вверх (упрощенно)
	cb.Menu.SetPosition(cb.X1, cb.Y1+1, cb.X2, cb.Y1+h)
	cb.Menu.ClearDone()
	FrameManager.Push(cb.Menu)
}

func (cb *ComboBox) SetFocus(f bool) {
	cb.focused = f
	cb.Edit.SetFocus(f)
}