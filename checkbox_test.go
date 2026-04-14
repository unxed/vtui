package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestCheckbox_Toggle(t *testing.T) {
	// 1. Test 2 states
	cb2 := NewCheckbox(0, 0, "2-state", false)
	if cb2.State != 0 { t.Error("Should start unchecked") }
	cb2.Toggle()
	if cb2.State != 1 { t.Error("Should be checked (1)") }
	cb2.Toggle()
	if cb2.State != 0 { t.Error("Should be unchecked again (0)") }

	// 2. Test 3 states
	cb3 := NewCheckbox(0, 0, "3-state", true)
	cb3.Toggle() // 0 -> 1
	if cb3.State != 1 { t.Error("3-state: expected 1") }
	cb3.Toggle() // 1 -> 2
	if cb3.State != 2 { t.Error("3-state: expected 2") }
	cb3.Toggle() // 2 -> 0
	if cb3.State != 0 { t.Error("3-state: expected 0") }
}
func TestCheckbox_NoReturnToggle(t *testing.T) {
	cb := NewCheckbox(0, 0, "Test", false)
	cb.State = 0

	// Enter should NOT toggle checkbox (it should bubble up to dialog)
	handled := cb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if handled {
		t.Error("Checkbox should not handle Return key")
	}
	if cb.State != 0 {
		t.Error("Checkbox state changed on Return")
	}
}

func TestCheckbox_HotkeyRendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 1)

	cb := NewCheckbox(0, 0, "Enable &AI", false)
	cb.Show(scr)

	// "[ ] Enable AI". Letter 'A' should be highlighted.
	// Indices: 0:'[', 1:' ', 2:']', 3:' ', 4:'E', 5:'n', 6:'a', 7:'b', 8:'l', 9:'e', 10:' ', 11:'A'
	checkCell(t, scr, 11, 0, 'A', Palette[ColDialogHighlightText])
	// Check neighbor letter, it should be normal
	checkCell(t, scr, 12, 0, 'I', Palette[ColDialogText])
}
