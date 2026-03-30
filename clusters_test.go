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
	// 3 items, 2 cols:
	// 0 1
	// 2
	rg := NewRadioGroup(0, 0, 2, []string{"A", "B", "C"})

	rg.Selected = 0
	// 1. Right to B (index 1)
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if rg.Selected != 1 {
		t.Errorf("Right nav failed, got %d", rg.Selected)
	}

	// 2. Down from B (index 1). There is nothing below B, and it's the last column.
	// According to the "exit only at absolute boundaries" rule, it should be swallowed
	// because index 1 is not the last element of the group (index 2 is).
	handled := rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if !handled {
		t.Error("VK_DOWN at index 1 should be swallowed (not absolute end of group)")
	}
}

func TestRadioGroup_SnakeNavigation(t *testing.T) {
	// 4 items, 2 cols:
	// 0 1
	// 2 3
	rg := NewRadioGroup(0, 0, 2, []string{"0", "1", "2", "3"})

	// 1. Down from 2 (bottom of col 0) to 1 (top of col 1)
	rg.Selected = 2
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if rg.Selected != 1 {
		t.Errorf("Snake Down failed: expected 1, got %d", rg.Selected)
	}

	// 2. Up from 1 (top of col 1) back to 2 (bottom of col 0)
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if rg.Selected != 2 {
		t.Errorf("Snake Up failed: expected 2, got %d", rg.Selected)
	}
}

func TestRadioGroup_BoundarySwallowing(t *testing.T) {
	// 3 items: 0, 1, 2
	rg := NewRadioGroup(0, 0, 1, []string{"0", "1", "2"})

	// At index 1, ANY arrow should be swallowed
	rg.Selected = 1
	arrows := []uint16{vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT}
	for _, a := range arrows {
		if !rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: a}) {
			t.Errorf("Arrow %d should be swallowed at index 1", a)
		}
	}

	// At index 0, UP and LEFT should NOT be swallowed (exit to prev)
	rg.Selected = 0
	if rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) {
		t.Error("VK_UP at index 0 should not be handled")
	}
	if rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT}) {
		t.Error("VK_LEFT at index 0 should not be handled")
	}

	// At index 2, DOWN and RIGHT should NOT be swallowed (exit to next)
	rg.Selected = 2
	if rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}) {
		t.Error("VK_DOWN at index 2 should not be handled")
	}
	if rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}) {
		t.Error("VK_RIGHT at index 2 should not be handled")
	}
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
	// Calculated title start for " Group " is at X=3 (X1+1).
	checkCell(t, scr, 3, 2, ' ', Palette[ColDialogHighlightText])
	checkCell(t, scr, 4, 2, 'G', Palette[ColDialogHighlightText])
	checkCell(t, scr, 5, 2, 'r', Palette[ColDialogHighlightText])
}