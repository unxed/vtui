package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestListBox_Scrolling(t *testing.T) {
	items := []string{"1", "2", "3", "4", "5"}
	// ListBox with a height of 2 lines
	lb := NewListBox(0, 0, 10, 2, items)

	// 1. Initially SelectPos 0, TopPos 0
	if lb.SelectPos != 0 || lb.TopPos != 0 {
		t.Errorf("Initial state error: SelectPos %d, TopPos %d", lb.SelectPos, lb.TopPos)
	}

	// 2. Scroll down twice
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	// SelectPos should be 2, TopPos should become 1 (to see index 2 in a 2-line window)
	if lb.SelectPos != 2 {
		t.Errorf("SelectPos error after Down: %d", lb.SelectPos)
	}
	if lb.TopPos != 1 {
		t.Errorf("TopPos error after Down (scrolling): expected 1, got %d", lb.TopPos)
	}

	// 3. Home test
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	if lb.SelectPos != 0 || lb.TopPos != 0 {
		t.Errorf("Home error: SelectPos %d, TopPos %d", lb.SelectPos, lb.TopPos)
	}
}

func TestListBox_MouseWheel(t *testing.T) {
	items := make([]string, 10)
	lb := NewListBox(0, 0, 10, 3, items)
	lb.TopPos = 2
	lb.SelectPos = 2

	// Scroll Down (WheelDirection < 0)
	lb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	if lb.TopPos != 3 {
		t.Errorf("Mouse wheel down failed: TopPos %d", lb.TopPos)
	}

	// Scroll Up (WheelDirection > 0)
	lb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1})
	if lb.TopPos != 2 {
		t.Errorf("Mouse wheel up failed: TopPos %d", lb.TopPos)
	}
}

func TestListBox_OnChange(t *testing.T) {
	called := false
	newIdx := -1
	lb := NewListBox(0, 0, 10, 5, []string{"A", "B", "C"})
	lb.OnSelect = func(idx int) {
		called = true
		newIdx = idx
	}

	// Down to index 1
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	if !called {
		t.Error("OnChange callback was not called")
	}
	if newIdx != 1 {
		t.Errorf("Expected index 1 in callback, got %d", newIdx)
	}
}

func TestListBox_PageNavigation(t *testing.T) {
	items := make([]string, 20) // 20 items
	lb := NewListBox(0, 0, 10, 5, items) // Height 5

	// 1. End
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})
	if lb.SelectPos != 19 {
		t.Errorf("End failed: expected 19, got %d", lb.SelectPos)
	}

	// 2. Page Up (19 -> 14)
	// ViewHeight is 5. 19 - 5 = 14.
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
	if lb.SelectPos != 14 {
		t.Errorf("PageUp failed: expected 14, got %d", lb.SelectPos)
	}

	// 3. Page Down (14 -> 19)
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if lb.SelectPos != 19 {
		t.Errorf("PageDown failed: expected 19, got %d", lb.SelectPos)
	}
}

func TestListBox_MouseClickItem(t *testing.T) {
	lb := NewListBox(0, 0, 10, 5, []string{"0", "1", "2", "3", "4"})
	lb.TopPos = 1 // Offset by 1 (visible: 1, 2, 3, 4, 5)

	// Re-render to ensure Table internal state (MarginTop) is updated in test environment
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	lb.Show(scr)

	// Click at Y=2 (this is the second row of content since Y1=0 and MarginTop=0)
	// Selected index should be TopPos (1) + 2 = 3.
	lb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		MouseX: 2, MouseY: 2,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})

	if lb.SelectPos != 3 {
		t.Errorf("Mouse click selection failed: expected 3, got %d", lb.SelectPos)
	}
}

func TestListBox_EmptyList(t *testing.T) {
	lb := NewListBox(0, 0, 10, 5, []string{})

	// Navigation attempt should not cause panic
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	if lb.SelectPos != 0 {
		t.Error("SelectPos should be 0 for empty list")
	}
}
func TestListBox_SelectName(t *testing.T) {
	items := []string{"apple", "banana", "cherry"}
	lb := NewListBox(0, 0, 10, 5, items)

	// 1. Select existing item
	lb.SelectName("banana")
	if lb.SelectPos != 1 {
		t.Errorf("SelectName('banana') failed: expected index 1, got %d", lb.SelectPos)
	}

	// 2. Select non-existent item (should do nothing)
	lb.SelectName("dragonfruit")
	if lb.SelectPos != 1 {
		t.Errorf("SelectName should not change position for missing items, got %d", lb.SelectPos)
	}
}
func TestListBox_DynamicLayout(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 10)

	lb := NewListBox(0, 0, 10, 5, []string{"Item1"})

	// 1. Default (No Header): Row 0 is at Y=0
	lb.Show(scr)
	checkCell(t, scr, 0, 0, 'I', Palette[ColTableText])

	// 2. Enable Header: Row 0 must shift to Y=1
	lb.ShowHeader = true
	lb.Show(scr)
	// Cell at 0,0 should now be part of the header (title or spacer)
	// Cell at 0,1 should contain 'I'
	checkCell(t, scr, 0, 1, 'I', Palette[ColTableText])
}
