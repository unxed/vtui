package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestButton_OnClick(t *testing.T) {
	b := NewButton(0, 0, "OK")
	clicked := false
	b.OnClick = func() { clicked = true }

	// Test KeyDown Space
	b.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if !clicked {
		t.Error("Button should be clicked on Space")
	}

	clicked = false
	// Test KeyDown Return (Buttons SHOULD still handle Return)
	b.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if !clicked {
		t.Error("Button should be clicked on Return")
	}

	clicked = false
	// Test Mouse Click
	b.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed})
	if !clicked {
		t.Error("Button should be clicked on Left Mouse Button")
	}
}

func TestButton_HotkeyParsing(t *testing.T) {
	b := NewButton(0, 0, "Sa&ve")
	// Check that the constructor correctly extracted 'v' (lowercase)
	if b.GetHotkey() != 'v' {
		t.Errorf("Expected hotkey 'v', got %c", b.GetHotkey())
	}
}
