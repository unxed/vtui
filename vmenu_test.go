package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestVMenu_SeparatorSkipping(t *testing.T) {
	m := NewVMenu("Separator Test")
	m.AddItem(MenuItem{Text: "0"})
	m.AddSeparator()
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "3"})
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "5"})

	// 1. Start at 0, move Down -> should land on 3, skipping two separators
	m.SetSelectPos(0, 1)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if m.SelectPos != 3 {
		t.Errorf("Failed to skip double separator Down: got %d, want 3", m.SelectPos)
	}

	// 2. Move Down again -> land on 5
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if m.SelectPos != 5 {
		t.Errorf("Failed to move to 5: got %d", m.SelectPos)
	}

	// 3. Move Down from 5 -> wrap to 0 (skipping separator at 4)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if m.SelectPos != 0 {
		t.Errorf("Failed to wrap-skip separator Down: got %d, want 0", m.SelectPos)
	}

	// 4. Move Up from 0 -> wrap to 5 (skipping separator at 4)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if m.SelectPos != 5 {
		t.Errorf("Failed to wrap-skip separator Up: got %d, want 5", m.SelectPos)
	}
}