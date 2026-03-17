package vtui

import (
	"testing"
)

func TestRadioButton_GroupLogic(t *testing.T) {
	SetDefaultPalette()
	d := NewDialog(0, 0, 40, 10, "Test")
	rb1 := NewRadioButton(1, 1, "R1")
	rb2 := NewRadioButton(1, 2, "R2")

	rb1.Selected = true
	d.AddItem(rb1)
	d.AddItem(rb2)

	if !rb1.Selected || rb2.Selected {
		t.Fatal("Initial selection state invalid")
	}

	// Имитируем клик по второй кнопке (индекс 1 в диалоге)
	// Для теста вызовем внутренний метод выбора
	d.selectRadio(rb2)

	if rb1.Selected {
		t.Error("rb1 should be deselected after rb2 chosen")
	}
	if !rb2.Selected {
		t.Error("rb2 should be selected")
	}
}

func TestRadioButton_Rendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(20, 5)

	rb := NewRadioButton(0, 0, "Item")
	rb.Selected = true
	rb.Show(scr)

	// Проверяем наличие точки (•) в начале (StringToCharInfo превращает её в rune)
	// Координаты: (1, 0) внутри "(•) "
	checkCell(t, scr, 1, 0, '•', Palette[ColDialogText])
}