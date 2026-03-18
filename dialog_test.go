package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestDialog_FocusCycle(t *testing.T) {
	d := NewDialog(0, 0, 20, 10, "Test")
	b1 := NewButton(1, 1, "B1")
	txt := NewText(1, 2, "Static", 0) // Text is not focusable
	e1 := NewEdit(1, 3, 10, "E1")
	b2 := NewButton(1, 4, "B2")

	d.AddItem(b1)
	d.AddItem(txt)
	d.AddItem(e1)
	d.AddItem(b2)

	// Initial focus should be on the first focusable element (index 0 - b1)
	if d.focusIdx != 0 {
		t.Errorf("Initial focus: expected 0, got %d", d.focusIdx)
	}
	if !b1.IsFocused() {
		t.Error("b1 should be focused initially")
	}

	// 1. Tab -> skips txt (1), goes to e1 (index 2)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if d.focusIdx != 2 {
		t.Errorf("Tab 1: expected index 2 (e1), got %d", d.focusIdx)
	}
	if !e1.IsFocused() || b1.IsFocused() {
		t.Error("Focus state not updated correctly after Tab 1")
	}

	// 2. Tab -> goes to b2 (index 3)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if d.focusIdx != 3 {
		t.Errorf("Tab 2: expected index 3 (b2), got %d", d.focusIdx)
	}

	// 3. Tab -> cycles back to b1 (index 0)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if d.focusIdx != 0 {
		t.Errorf("Tab 3 (cycle): expected index 0 (b1), got %d", d.focusIdx)
	}

	// 4. Shift+Tab -> cycles backward to b2 (index 3)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed})
	if d.focusIdx != 3 {
		t.Errorf("Shift+Tab 1 (cycle back): expected index 3 (b2), got %d", d.focusIdx)
	}

	// 5. Shift+Tab -> goes back to e1 (index 2)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed})
	if d.focusIdx != 2 {
		t.Errorf("Shift+Tab 2: expected index 2 (e1), got %d", d.focusIdx)
	}
}

func TestDialog_RadioButtonIntegration(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Radio Test")
	rb1 := NewRadioButton(1, 1, "R1")
	rb2 := NewRadioButton(1, 2, "R2")
	rb1.Selected = true
	d.AddItem(rb1)
	d.AddItem(rb2)

	// Переходим на rb2 (Tab)
	d.changeFocus(1)

	// Нажимаем Space. Dialog должен перехватить это и обновить группу.
	d.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_SPACE,
	})

	if rb1.Selected {
		t.Error("rb1 should be deselected via ProcessKey(Space)")
	}
	if !rb2.Selected {
		t.Error("rb2 should be selected via ProcessKey(Space)")
	}
}
func TestDialog_Shadow(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(20, 20)

	// Диалог 5x5
	d := NewDialog(2, 2, 7, 7, "Title")
	d.Show(scr)

	// Проверяем ячейку тени.
	// Правый нижний угол диалога - (7, 7).
	// Тень должна быть в (8, 8) и (9, 8).
	shAttr := Palette[ColShadow]
	checkCell(t, scr, 9, 8, ' ', shAttr)
	checkCell(t, scr, 8, 3, ' ', shAttr) // Вертикальная часть тени
}
func TestDialog_MouseFocus(t *testing.T) {
	d := NewDialog(0, 0, 20, 10, "Test")
	b1 := NewButton(1, 1, "B1")
	b2 := NewButton(1, 2, "B2")
	d.AddItem(b1)
	d.AddItem(b2)

	// Initially, b1 (index 0) has focus
	if d.focusIdx != 0 {
		t.Fatalf("Initial focus should be on b1 (0), got %d", d.focusIdx)
	}

	// Simulate a click on b2 at (1, 2)
	d.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      1,
		MouseY:      2,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})

	// Focus should now be on b2 (index 1)
	if d.focusIdx != 1 {
		t.Errorf("Mouse click failed to change focus: expected 1, got %d", d.focusIdx)
	}
	if !b2.IsFocused() {
		t.Error("b2 should be focused after click")
	}
	if b1.IsFocused() {
		t.Error("b1 should lose focus after click on b2")
	}
}

func TestDialog_HotkeyColor(t *testing.T) {
	d := NewDialog(0, 0, 20, 5, "Test")
	btn := NewButton(1, 1, "&Test")
	d.AddItem(btn)

	scr := NewScreenBuf()
	scr.AllocBuf(22, 7)
	SetDefaultPalette()

	// 1. Unfocused state
	btn.SetFocus(false)
	d.Show(scr)
	// Hotkey 'T' should have highlight color
	// Text is "[ &Test ]", clean is "[ Test ]". Hotkey pos is 2.
	checkCell(t, scr, 1+2, 1, 'T', Palette[ColDialogHighlightButton])
	
	// 2. Focused state
	btn.SetFocus(true)
	d.Show(scr)
	// Hotkey 'T' should have SELECTED highlight color
	checkCell(t, scr, 1+2, 1, 'T', Palette[ColDialogHighlightSelectedButton])
}

func TestDialog_HotkeyActivation(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Hotkey Test")

	clicked := false
	btn := NewButton(1, 1, "&Click")
	btn.OnClick = func() { clicked = true }

	e1 := NewEdit(1, 2, 10, "")

	d.AddItem(btn)
	d.AddItem(e1)

	// Переводим фокус на Edit, чтобы кнопка была не в фокусе
	d.focusIdx = 1
	btn.SetFocus(false)
	e1.SetFocus(true)

	// 1. Нажимаем Alt+C (хоткей кнопки)
	d.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'c',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !clicked {
		t.Error("Button should be clicked via Alt+C")
	}
	if d.focusIdx != 0 || !btn.IsFocused() {
		t.Error("Focus should move to the button after hotkey activation")
	}
}

func TestDialog_LabelFocusLink(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "FocusLink Test")

	edit := NewEdit(1, 2, 10, "")
	label := NewText(1, 1, "&Name:", 0)
	label.FocusLink = edit // Привязываем метку к полю ввода

	d.AddItem(label)
	d.AddItem(edit)

	// Изначально фокус на первом элементе (label), но он не canFocus,
	// поэтому Dialog сам выберет edit при добавлении. Сбросим это вручную для теста.
	d.focusIdx = -1
	edit.SetFocus(false)

	// 1. Нажимаем Alt+N (хоткей метки)
	d.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'n',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if d.focusIdx != 1 || !edit.IsFocused() {
		t.Errorf("Focus should move to edit via label hotkey. focusIdx=%d", d.focusIdx)
	}
}
func TestDialog_HotkeyCaseInsensitivity(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Case Test")
	btn := NewButton(1, 1, "&File")
	d.AddItem(btn)

	// Нажимаем заглавную 'F', хотя в строке она может быть любой.
	// Парсер сохраняет 'f', поэтому сравнение должно быть успешным.
	handled := d.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'F',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !handled {
		t.Error("Hotkey should be case-insensitive")
	}
}
