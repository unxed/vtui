package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestRadioGroup_Navigation(t *testing.T) {
	// Use unique hotkeys: O, W, T
	rg := NewRadioGroup(0, 0, 1, []string{"&One", "T&wo", "&Three"})

	// 1. Initial
	if rg.Selected != 0 { t.Errorf("Expected 0, got %d", rg.Selected) }

	// 2. Down
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if rg.Selected != 1 { t.Errorf("Expected 1, got %d", rg.Selected) }

	// Boundary check (should return false when trying to go out of bounds)
	rg.Selected = 2
	handled := rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if handled { t.Errorf("Should return false on bottom boundary") }

	// 3. Hotkey 't' for 'Three'
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 't'})
	if rg.Selected != 2 { t.Errorf("Hotkey failed, got %d", rg.Selected) }
}

func TestRadioGroup_MultiColumn(t *testing.T) {
	rg := NewRadioGroup(0, 0, 2, []string{"A", "B", "C"})
	// Grid is 2x2:
	// A B
	// C

	rg.Selected = 0
	// 1. Right to B
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if rg.Selected != 1 { t.Errorf("Right nav failed, got %d", rg.Selected) }

	// 2. Down from B to nothing? Should snap to C (last item) if out of bounds on the row below
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if rg.Selected != 2 { t.Errorf("Down snap failed, got %d", rg.Selected) }
}

func TestCheckGroup_Toggle(t *testing.T) {
	cg := NewCheckGroup(0, 0, 1, []string{"A", "B"})

	// 1. Space to toggle first
	cg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if !cg.States[0] { t.Error("Toggle 0 failed") }

	// 2. Mouse click on second item
	cg.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 1, MouseX: 0,
	})
	if !cg.States[1] { t.Error("Mouse toggle 1 failed") }
}

func TestListBox_MultiSelect(t *testing.T) {
	lb := NewListBox(0, 0, 10, 5, []string{"A", "B", "C"})
	lb.MultiSelect = true

	// 1. Space on 'A'
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	// 2. Down and Space on 'B'
	lb.SelectPos = 1
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})

	sel := lb.GetSelectedIndices()
	if len(sel) != 2 || sel[0] != 0 || sel[1] != 1 {
		t.Errorf("MultiSelect failed, got indices: %v", sel)
	}
}

func TestGroupBox_Rendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(20, 10)

	gb := NewGroupBox(2, 2, 10, 6, "Group")
	gb.Show(scr)

	// Check title
	// " Group " starts at X=2+2=4.
	// We use ColDialogHighlightText for GroupBox titles to make them stand out.
	checkCell(t, scr, 4, 2, ' ', Palette[ColDialogHighlightText])
	checkCell(t, scr, 5, 2, 'G', Palette[ColDialogHighlightText])
}