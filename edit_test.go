package vtui

import "testing"

func TestEdit_PasswordMode(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(10, 1)

	e := NewEdit(0, 0, 10, "abc")
	e.PasswordMode = true
	e.Show(scr)

	// Проверяем, что в буфере вместо 'a' находится '*'
	// Атрибуты должны соответствовать ColDialogEdit
	checkCell(t, scr, 0, 0, '*', Palette[ColDialogEdit])
	checkCell(t, scr, 1, 0, '*', Palette[ColDialogEdit])
	checkCell(t, scr, 2, 0, '*', Palette[ColDialogEdit])
}