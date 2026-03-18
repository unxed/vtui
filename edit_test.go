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

func TestVMenu_ScrollbarMouseClick(t *testing.T) {
	SetDefaultPalette()
	m := NewVMenu("Title")
	// Добавляем 20 элементов, чтобы меню скроллилось
	for i := 0; i < 20; i++ {
		m.AddItem("Item")
	}
	m.SetPosition(0, 0, 10, 6) // Высота 7, данные 5 (Y1+1..Y2-1)

	// Начальное состояние: SelectPos 0
	
	// 1. Клик по нижней стрелке (X = X2 = 10, Y = Y2-1 = 5)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 5, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.selectPos != 1 {
		t.Errorf("VMenu down arrow click failed, pos %d", m.selectPos)
	}

	// 2. Клик по верхней стрелке (Y = Y1+1 = 1)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 1, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.selectPos != 0 {
		t.Errorf("VMenu up arrow click failed, pos %d", m.selectPos)
	}

	// 3. Page Down клик (Y = 4)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 4, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.selectPos != 5 { // 0 + height (5) = 5
		t.Errorf("VMenu PageDown click failed, pos %d", m.selectPos)
	}

	// 4. Page Up клик (Y = 2)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 2, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.selectPos != 0 { // 5 - height (5) = 0
		t.Errorf("VMenu PageUp click failed, pos %d", m.selectPos)
	}
}
func TestVMenu_Hotkeys(t *testing.T) {
	m := NewVMenu("Menu")
	m.AddItem("Open &File")
	m.AddItem("&Save")
	m.AddItem("E&xit")

	// 1. Нажимаем 's' (хоткей второго пункта)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 's'})

	if m.selectPos != 1 {
		t.Errorf("Expected selectPos 1 for 'Save', got %d", m.selectPos)
	}
	if !m.IsDone() || m.exitCode != 1 {
		t.Error("Menu should be finished with exitCode 1")
	}
}
