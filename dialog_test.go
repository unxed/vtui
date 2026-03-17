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
