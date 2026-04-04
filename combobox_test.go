package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestComboBox_Selection(t *testing.T) {
	items := []string{"One", "Two", "Three"}
	cb := NewComboBox(0, 0, 20, items)

	// Initially text is empty
	if cb.Edit.GetText() != "" {
		t.Errorf("Expected empty text, got %q", cb.Edit.GetText())
	}

	// Simulate selecting the second item ("Two") in menu
	if cb.Menu.OnAction != nil {
		cb.Menu.OnAction(1)
	}

	if cb.Edit.GetText() != "Two" {
		t.Errorf("Expected 'Two', got %q", cb.Edit.GetText())
	}
}

func TestComboBox_DropdownOnly(t *testing.T) {
	cb := NewComboBox(0, 0, 20, []string{"A", "B"})
	cb.DropdownOnly = true

	// Attempting to enter text 'X'
	cb.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    'X',
	})

	if cb.Edit.GetText() == "X" {
		t.Error("DropdownOnly ComboBox should not allow manual text entry")
	}
}
func TestComboBox_DropdownOnly_Enter(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())

	cb := NewComboBox(0, 0, 20, []string{"A", "B"})
	cb.DropdownOnly = true

	// Press Enter in DropdownOnly mode should open the menu
	cb.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	top := fm.GetTopFrame()
	if top == nil || top.GetType() != TypeMenu {
		t.Error("Enter should open dropdown menu when DropdownOnly is true")
	}
}
func TestComboBox_OpenFlip(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 10) // Small height
	FrameManager.Init(scr)

	cb := NewComboBox(0, 8, 20, []string{"Item 1", "Item 2"})

	// ComboBox is at Y=8. Default open is downwards (Y=9).
	// But screen height is 10, so Y=9 is the last line.
	// With 2 items + border, menu height is 4.
	// It MUST flip upwards to fit.
	cb.Open()

	top := FrameManager.GetTopFrame()
	if top == nil || top.GetType() != TypeMenu {
		t.Fatal("Menu not opened")
	}

	_, y1, _, _ := top.GetPosition()
	// ComboBox is at Y=8. Upward flip with height 4 should start at 8-4 = 4.
	if y1 >= 8 {
		t.Errorf("ComboBox menu did not flip upwards. Y1=%d, ComboBoxY=%d", y1, cb.Y1)
	}
}

func TestComboBox_DisabledState(t *testing.T) {
	FrameManager.Init(NewSilentScreenBuf())
	cb := NewComboBox(0, 0, 20, []string{"A", "B"})
	
	// 1. Initially enabled
	if cb.IsDisabled() || cb.Edit.IsDisabled() {
		t.Error("ComboBox and its Edit should be enabled by default")
	}
	
	// 2. Disable ComboBox
	cb.SetDisabled(true)
	
	if !cb.IsDisabled() {
		t.Error("ComboBox failed to set disabled flag")
	}
	if !cb.Edit.IsDisabled() {
		t.Error("SetDisabled failed to propagate to underlying Edit control")
	}
	
	// 3. Try to open menu while disabled
	cb.Open()
	if FrameManager.GetTopFrameType() == TypeMenu {
		t.Error("Disabled ComboBox should not allow opening its menu")
	}
}

func TestComboBox_WantsChars(t *testing.T) {
	cb := NewComboBox(0, 0, 20, []string{"A"})
	
	// Normal mode: should want chars (pass to edit)
	if !cb.WantsChars() {
		t.Error("Standard ComboBox should want chars for editing")
	}
	
	// DropdownOnly: should NOT want chars
	cb.DropdownOnly = true
	if cb.WantsChars() {
		t.Error("DropdownOnly ComboBox should not want chars")
	}
}
