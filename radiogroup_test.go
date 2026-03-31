package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestRadioGroup_ProcessMouse(t *testing.T) {
	rg := NewRadioGroup(0, 0, 2, []string{"A", "B", "C"})
	changed := false
	rg.ChangeCommand = rg.AddCallback(func(args any) { changed = true })

	// Click on B (index 1), Col 1, Row 0
	// X1 is 0, Col 0 is 6 wide, so Col 1 starts at 6. Y is 0.
	handled := rg.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 7, MouseY: 0, // Column 0 is 7 chars wide (4 prefix + 1 char + 2 padding)
	})

	if !handled {
		t.Error("RadioGroup should handle valid mouse click")
	}
	if rg.Selected != 1 {
		t.Errorf("Expected B (1) to be selected, got %d", rg.Selected)
	}
	if !changed {
		t.Error("OnChange should be called")
	}
}
func TestRadioGroup_MouseEdgeCases(t *testing.T) {
	rg := NewRadioGroup(0, 0, 1, []string{"A"})

	// Click completely outside the RadioGroup
	handled := rg.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: 10, MouseY: 10,
	})

	if handled {
		t.Error("RadioGroup should ignore out-of-bounds clicks")
	}
}
