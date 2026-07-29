package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestListViewer_Logic(t *testing.T) {
	sv := &ScrollView{ItemCount: 10, ViewHeight: 3}

	// 1. Initial
	sv.SetSelectPos(0)
	if sv.TopPos != 0 {
		t.Error("TopPos should be 0")
	}

	// 2. Navigation
	sv.HandleNavKey(vtinput.VK_DOWN) // pos 1
	sv.HandleNavKey(vtinput.VK_DOWN) // pos 2
	if sv.TopPos != 0 {
		t.Error("TopPos should still be 0 at pos 2")
	}

	sv.HandleNavKey(vtinput.VK_DOWN) // pos 3 (scrolls!)
	if sv.TopPos != 1 {
		t.Errorf("Expected TopPos 1, got %d", sv.TopPos)
	}

	// 3. Page Nav
	sv.HandleNavKey(vtinput.VK_NEXT) // 3 + 3 = 6
	if sv.SelectPos != 6 {
		t.Errorf("PgDn failed: got %d", sv.SelectPos)
	}
	if sv.TopPos != 4 {
		t.Errorf("PgDn TopPos failed: got %d", sv.TopPos)
	}

	// 4. Boundaries
	sv.SetSelectPos(9)
	sv.HandleNavKey(vtinput.VK_DOWN)
	if sv.SelectPos != 9 {
		t.Error("Should not exceed ItemCount")
	}

	// 5. Empty list
	sv.ItemCount = 0
	sv.SetSelectPos(5)
	if sv.SelectPos != 0 {
		t.Error("Empty list SelectPos should be 0 (compat)")
	}
}

func TestListViewer_MoveSelection(t *testing.T) {
	sv := &ScrollView{ItemCount: 10, ViewHeight: 5}
	sv.SetSelectPos(2)

	sv.MoveSelection(2)
	if sv.SelectPos != 4 {
		t.Errorf("MoveSelection(2) failed, got %d", sv.SelectPos)
	}

	sv.MoveSelection(-10)
	if sv.SelectPos != 0 {
		t.Errorf("MoveSelection underflow failed, got %d", sv.SelectPos)
	}
}

func TestScrollView_SelectableSkipping(t *testing.T) {
	// Setup a list where index 1 and 3 are unselectable (separators)
	sv := &ScrollView{ItemCount: 5, ViewHeight: 5}
	sv.IsSelectable = func(i int) bool {
		return i != 1 && i != 3
	}

	// 1. Start at 0, move down. Should skip 1 and land on 2.
	sv.SetSelectPos(0)
	sv.MoveSelection(1)
	if sv.SelectPos != 2 {
		t.Errorf("Skipping forward failed: expected 2, got %d", sv.SelectPos)
	}

	// 2. Move down from 2. Should skip 3 and land on 4.
	sv.MoveSelection(1)
	if sv.SelectPos != 4 {
		t.Errorf("Skipping forward (2) failed: expected 4, got %d", sv.SelectPos)
	}

	// 3. Move up from 4. Should skip 3 and land on 2.
	sv.MoveSelection(-1)
	if sv.SelectPos != 2 {
		t.Errorf("Skipping backward failed: expected 2, got %d", sv.SelectPos)
	}

	// 4. Wrapping check
	sv.Wrap = true
	sv.SetSelectPos(4)
	sv.MoveSelection(1) // Should skip 0 (unselectable if we changed the logic, but here 0 is ok)
	if sv.SelectPos != 0 {
		t.Errorf("Wrap move failed: expected 0, got %d", sv.SelectPos)
	}
}
func TestScrollView_MiddleClick(t *testing.T) {
	actionTriggered := false
	var triggeredPos int

	sv := &ScrollView{ItemCount: 5, ViewHeight: 5}
	sv.OnAction = func(pos int) {
		actionTriggered = true
		triggeredPos = pos
	}

	// Simulate middle mouse button click (wheel click) on Y coordinate 2
	event := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseY:      2,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft2ndButtonPressed,
	}

	handled := sv.HandleMouse(event)
	if !handled {
		t.Error("HandleMouse should return true for middle click on a valid item")
	}

	if sv.SelectPos != 2 {
		t.Errorf("Expected SelectPos to be updated to 2, got %d", sv.SelectPos)
	}

	if !actionTriggered {
		t.Error("OnAction should be triggered on middle click")
	}

	if triggeredPos != 2 {
		t.Errorf("Expected action triggered on position under cursor (2), got %d", triggeredPos)
	}
}
