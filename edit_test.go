package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)


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

func TestEdit_IgnoreLockKeys(t *testing.T) {
	e := NewEdit(0, 0, 10, "")

	// Имитируем ввод 'x' с включенным NumLock и CapsLock
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            'x',
		ControlKeyState: vtinput.NumLockOn | vtinput.CapsLockOn,
	})

	if e.GetText() != "x" {
		t.Errorf("Expected 'x', got %q. Lock keys probably blocked the input.", e.GetText())
	}
}
