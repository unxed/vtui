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
	if cb.Menu.SelectCommand != 0 {
		cb.Menu.HandleCommand(cb.Menu.SelectCommand, 1)
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
