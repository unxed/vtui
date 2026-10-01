package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
	"time"
)

func TestMouseHandler_GetClickType_LeftClick(t *testing.T) {
	mh := NewMouseHandler()

	ev := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}

	clickType := mh.GetClickType(ev)
	if clickType != MouseClickLeft {
		t.Errorf("Expected MouseClickLeft, got %d", clickType)
	}
}

func TestMouseHandler_GetClickType_MiddleClick(t *testing.T) {
	mh := NewMouseHandler()

	ev := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft2ndButtonPressed,
		KeyDown:     true,
	}

	clickType := mh.GetClickType(ev)
	if clickType != MouseClickMiddle {
		t.Errorf("Expected MouseClickMiddle, got %d", clickType)
	}
}

func TestMouseHandler_GetClickType_RightClick(t *testing.T) {
	mh := NewMouseHandler()

	ev := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.RightmostButtonPressed,
		KeyDown:     true,
	}

	clickType := mh.GetClickType(ev)
	if clickType != MouseClickRight {
		t.Errorf("Expected MouseClickRight, got %d", clickType)
	}
}

func TestMouseHandler_GetClickType_DoubleClick(t *testing.T) {
	mh := NewMouseHandler()

	// First click
	ev1 := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	mh.GetClickType(ev1)

	// Immediate second click at same location
	ev2 := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	clickType := mh.GetClickType(ev2)
	if clickType != MouseClickDoubleLeft {
		t.Errorf("Expected MouseClickDoubleLeft, got %d", clickType)
	}
}

func TestMouseHandler_GetClickType_DoubleClickThreshold(t *testing.T) {
	mh := NewMouseHandler()
	mh.DoubleClickThreshold = 100 * time.Millisecond

	// First click
	ev1 := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	mh.GetClickType(ev1)

	// Wait past the threshold
	time.Sleep(150 * time.Millisecond)

	// Second click should be single, not double
	ev2 := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	clickType := mh.GetClickType(ev2)
	if clickType != MouseClickLeft {
		t.Errorf("Expected MouseClickLeft (not double due to timing), got %d", clickType)
	}
}

func TestMouseHandler_GetClickType_DoubleClickDistance(t *testing.T) {
	mh := NewMouseHandler()
	mh.DoubleClickDistance = 2

	// First click
	ev1 := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	mh.GetClickType(ev1)

	// Second click too far away
	ev2 := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      15,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	}
	clickType := mh.GetClickType(ev2)
	if clickType != MouseClickLeft {
		t.Errorf("Expected MouseClickLeft (not double due to distance), got %d", clickType)
	}
}

func TestMouseHandler_BeginDrag_UpdateDrag_EndDrag(t *testing.T) {
	mh := NewMouseHandler()

	// Begin drag
	mh.BeginDrag(10, 5, vtinput.FromLeft1stButtonPressed)
	if !mh.DragState.IsActive {
		t.Error("Expected drag to be active after BeginDrag")
	}
	if mh.IsDragging {
		t.Error("Expected IsDragging to be false before threshold exceeded")
	}

	// Small movement (below threshold)
	if mh.UpdateDrag(11, 5) {
		t.Error("Expected UpdateDrag to return false for small movement")
	}

	// Large movement (exceeds threshold)
	if !mh.UpdateDrag(20, 5) {
		t.Error("Expected UpdateDrag to return true after threshold exceeded")
	}

	// End drag
	mh.EndDrag()
	if mh.DragState.IsActive {
		t.Error("Expected drag to be inactive after EndDrag")
	}
}

func TestMouseHandler_GetDragSelection(t *testing.T) {
	mh := NewMouseHandler()

	mh.BeginDrag(10, 5, vtinput.FromLeft1stButtonPressed)
	mh.UpdateDrag(20, 15)

	x1, y1, x2, y2 := mh.GetDragSelection()

	if x1 != 10 || y1 != 5 || x2 != 20 || y2 != 15 {
		t.Errorf("Expected (10,5,20,15), got (%d,%d,%d,%d)", x1, y1, x2, y2)
	}
}

func TestMouseHandler_GetDragSelection_Normalized(t *testing.T) {
	mh := NewMouseHandler()

	// Drag backwards (top-right to bottom-left)
	mh.BeginDrag(20, 15, vtinput.FromLeft1stButtonPressed)
	mh.UpdateDrag(10, 5)

	x1, y1, x2, y2 := mh.GetDragSelection()

	// Should be normalized
	if x1 != 10 || y1 != 5 || x2 != 20 || y2 != 15 {
		t.Errorf("Expected normalized (10,5,20,15), got (%d,%d,%d,%d)", x1, y1, x2, y2)
	}
}

func TestMouseHandler_GetWheelDirection_Up(t *testing.T) {
	mh := NewMouseHandler()

	ev := &vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		WheelDirection: 1,
	}

	dir := mh.GetWheelDirection(ev)
	if dir != MouseWheelUp {
		t.Errorf("Expected MouseWheelUp, got %d", dir)
	}
}

func TestMouseHandler_GetWheelDirection_Down(t *testing.T) {
	mh := NewMouseHandler()

	ev := &vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		WheelDirection: -1,
	}

	dir := mh.GetWheelDirection(ev)
	if dir != MouseWheelDown {
		t.Errorf("Expected MouseWheelDown, got %d", dir)
	}
}

func TestMouseHandler_GetWheelLines(t *testing.T) {
	mh := NewMouseHandler()
	mh.WheelScrollLines = 5

	ev := &vtinput.InputEvent{
		Type:           vtinput.MouseEventType,
		WheelDirection: -1,
	}

	lines := mh.GetWheelLines(ev)
	if lines != -5 {
		t.Errorf("Expected -5 lines, got %d", lines)
	}
}

func TestMouseHandler_UpdateMousePosition(t *testing.T) {
	mh := NewMouseHandler()

	if mh.MousePositionKnown {
		t.Error("Expected MousePositionKnown to be false initially")
	}

	ev := &vtinput.InputEvent{
		Type:   vtinput.MouseEventType,
		MouseX: 42,
		MouseY: 17,
	}

	mh.UpdateMousePosition(ev)

	if !mh.MousePositionKnown {
		t.Error("Expected MousePositionKnown to be true after update")
	}
	if mh.CurrentMouseX != 42 || mh.CurrentMouseY != 17 {
		t.Errorf("Expected (42,17), got (%d,%d)", mh.CurrentMouseX, mh.CurrentMouseY)
	}
}

func TestMouseHandler_IsClickInBounds(t *testing.T) {
	mh := NewMouseHandler()
	mh.CurrentMouseX = 15
	mh.CurrentMouseY = 10
	mh.MousePositionKnown = true

	// Point inside bounds
	if !mh.IsClickInBounds(10, 5, 20, 15) {
		t.Error("Expected click to be within bounds")
	}

	// Point outside bounds
	if mh.IsClickInBounds(20, 15, 30, 25) {
		t.Error("Expected click to be outside bounds")
	}
}

func TestMouseHandler_IsDragInBounds(t *testing.T) {
	mh := NewMouseHandler()

	mh.BeginDrag(10, 5, vtinput.FromLeft1stButtonPressed)
	mh.UpdateDrag(20, 15)

	// Overlapping bounds
	if !mh.IsDragInBounds(5, 0, 25, 20) {
		t.Error("Expected drag to overlap with bounds")
	}

	// Non-overlapping bounds
	if mh.IsDragInBounds(25, 20, 35, 30) {
		t.Error("Expected drag to not overlap with bounds")
	}
}

func TestMouseHandler_Reset(t *testing.T) {
	mh := NewMouseHandler()

	// Set some state
	mh.BeginDrag(10, 5, vtinput.FromLeft1stButtonPressed)
	mh.GetClickType(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      10,
		MouseY:      5,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	})
	mh.UpdateMousePosition(&vtinput.InputEvent{
		Type:   vtinput.MouseEventType,
		MouseX: 42,
		MouseY: 17,
	})

	// Reset
	mh.Reset()

	if mh.DragState.IsActive {
		t.Error("Expected drag state to be inactive after reset")
	}
	if mh.ClickCount != 0 {
		t.Error("Expected click count to be 0 after reset")
	}
	if mh.MousePositionKnown {
		t.Error("Expected MousePositionKnown to be false after reset")
	}
}

func TestMouseHandler_DragWithinBounds(t *testing.T) {
	mh := NewMouseHandler()
	mh.DragStartThreshold = 1 // Low threshold for testing

	// Simulate a drag from (5,5) to (25,25)
	mh.BeginDrag(5, 5, vtinput.FromLeft1stButtonPressed)

	// Move to trigger drag start
	mh.UpdateDrag(7, 7)

	// Verify we're in drag mode
	if !mh.IsDragging {
		t.Error("Expected to be in dragging state")
	}

	// Check bounds
	if !mh.IsDragInBounds(0, 0, 30, 30) {
		t.Error("Expected drag to be within large bounds")
	}

	if mh.IsDragInBounds(10, 10, 15, 15) {
		t.Error("Expected drag to extend beyond small bounds")
	}
}
