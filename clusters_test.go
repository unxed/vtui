package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestRadioGroup_Navigation(t *testing.T) {
	rg := NewRadioGroup(0, 0, 1, []string{"&One", "T&wo", "&Three"})

	// 1. Initial
	if rg.Selected != 0 || rg.focusIdx != 0 { t.Errorf("Initial state fail") }

	// 2. Down: moves focus, NOT selection
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if rg.focusIdx != 1 { t.Errorf("Expected focus 1, got %d", rg.focusIdx) }
	if rg.Selected != 0 { t.Error("Selection should not change on arrows") }

	// 3. Space: changes selection
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE})
	if rg.Selected != 1 { t.Error("Space should change selection") }

	// 4. Hotkey: changes both
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 't'})
	if rg.Selected != 2 || rg.focusIdx != 2 { t.Error("Hotkey failed") }
}

func TestRadioGroup_MultiColumn(t *testing.T) {
	// 3 items, 2 cols:
	// 0 1
	// 2
	rg := NewRadioGroup(0, 0, 2, []string{"A", "B", "C"})

	rg.focusIdx = 0
	// 1. Right to B (index 1)
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if rg.focusIdx != 1 {
		t.Errorf("Right nav failed, expected focus 1, got %d", rg.focusIdx)
	}

	// 2. Down from B (index 1). "Snake" logic: move to top of next col or swallow.
	// Since B is at col 1, row 0 and it's the last column, Down should be swallowed
	// because index 1 is not the absolute end of items (index 2 is).
	handled := rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if !handled {
		t.Error("VK_DOWN at index 1 should be swallowed")
	}
}

func TestRadioGroup_SnakeNavigation(t *testing.T) {
	// 4 items, 2 cols:
	// 0 1
	// 2 3
	rg := NewRadioGroup(0, 0, 2, []string{"0", "1", "2", "3"})

	// 1. Down from 2 (bottom of col 0) -> should go to 1 (top of col 1)
	rg.focusIdx = 2
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if rg.focusIdx != 1 {
		t.Errorf("Snake Down failed: expected 1, got %d", rg.focusIdx)
	}

	// 2. Up from 1 (top of col 1) -> should go back to 2 (bottom of col 0)
	rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if rg.focusIdx != 2 {
		t.Errorf("Snake Up failed: expected 2, got %d", rg.focusIdx)
	}
}

func TestRadioGroup_BoundarySwallowing(t *testing.T) {
	// 3 items: 0, 1, 2
	rg := NewRadioGroup(0, 0, 1, []string{"0", "1", "2"})

	// At middle index, arrows should be swallowed (stay in group)
	rg.focusIdx = 1
	arrows := []uint16{vtinput.VK_UP, vtinput.VK_DOWN}
	for _, a := range arrows {
		if !rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: a}) {
			t.Errorf("Arrow %d should be swallowed at index 1", a)
		}
	}

	// At index 0, UP should NOT be swallowed (exit to prev)
	rg.focusIdx = 0
	if rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) {
		t.Error("VK_UP at index 0 should not be handled")
	}

	// At index 2, DOWN should NOT be swallowed (exit to next)
	rg.focusIdx = 2
	if rg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}) {
		t.Error("VK_DOWN at index 2 should not be handled")
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

func TestGridSnakeNavigation(t *testing.T) {
	// 5 items, 2 cols:
	// 0 1
	// 2 3
	// 4
	items := []string{"0", "1", "2", "3", "4"}
	count := len(items)

	// Right from col 1 (idx 1) -> idx 2 (row 1 start)
	idx, ok := gridNav(1, count, 2, vtinput.VK_RIGHT)
	if !ok || idx != 2 { t.Errorf("Snake Right row 0 fail: %d", idx) }

	// Down from bottom of col 0 (idx 4) -> idx 1 (col 1 top)
	idx, ok = gridNav(4, count, 2, vtinput.VK_DOWN)
	if !ok || idx != 1 { t.Errorf("Snake Down col 0 fail: %d", idx) }

	// Left from idx 2 (row 1 start) -> idx 1 (row 0 end)
	idx, ok = gridNav(2, count, 2, vtinput.VK_LEFT)
	if !ok || idx != 1 { t.Errorf("Snake Left row 1 fail: %d", idx) }
}
func TestCheckGroup_SnakeNavigation(t *testing.T) {
	cg := NewCheckGroup(0, 0, 2, []string{"A", "B", "C", "D"})
	// 0(A) 1(B)
	// 2(C) 3(D)

	cg.focusIdx = 1 // At 'B'
	// Right from B -> should snake to C (idx 2)
	cg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if cg.focusIdx != 2 { t.Errorf("CheckGroup snake RIGHT failed, got %d", cg.focusIdx) }

	cg.focusIdx = 2 // At 'C'
	// Left from C -> should snake back to B (idx 1)
	cg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if cg.focusIdx != 1 { t.Errorf("CheckGroup snake LEFT failed, got %d", cg.focusIdx) }
}

