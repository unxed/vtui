package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestScrollBar_WidgetMouse(t *testing.T) {
	val := -1
	sb := NewScrollBar(0, 0, 10) // height 10, Y: 0..9
	sb.SetParams(5, 0, 20)
	sb.OnScroll = func(v int) { val = v }

	// 1. Click top arrow (Y=0)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 0,
	})
	if val != 4 { t.Errorf("Top arrow click failed, got %d", val) }

	// 2. Click bottom arrow (Y=9)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 9,
	})
	if val != 5 { t.Errorf("Bottom arrow click failed, want 5, got %d", val) }

	// 3. Page Up area (e.g., Y=2)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 1,
	})
	if val != 0 { t.Errorf("PageUp click failed (5-10=-5 -> 0), want 0, got %d", val) }

	// 4. Page Down area (e.g., Y=7)
	sb.SetParams(5, 0, 20)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 7,
	})
	if val != 15 { t.Errorf("PageDown click failed (5+10=15), got %d", val) }
}

func TestScrollBar_Clamping(t *testing.T) {
	sb := NewScrollBar(0, 0, 10)
	sb.SetParams(5, 0, 10)

	val := -1
	sb.OnScroll = func(v int) { val = v }

	// Try scroll above max
	sb.scroll(100)
	if val != 10 { t.Errorf("Expected clamp to 10, got %d", val) }

	// Try scroll below min
	sb.scroll(-100)
	if val != 0 { t.Errorf("Expected clamp to 0, got %d", val) }
}
func TestScrollBar_Dragging(t *testing.T) {
	val := -1
	sb := NewScrollBar(0, 0, 12) // trackLen = 10
	sb.SetParams(0, 0, 100)
	sb.OnScroll = func(v int) { val = v }

	// 1. Initial click on thumb (TopPos=0, thumb is at the top)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 1,
	})
	if !sb.dragging { t.Fatal("Should be dragging") }

	// 2. Drag to middle (Y=6)
	// newVal = (relY * max) / (trackLen - 1) = (5 * 100) / 9 = 55
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 6,
	})
	if val < 50 || val > 60 {
		t.Errorf("Dragging failed, expected around 55, got %d", val)
	}

	// 3. Release button
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false, ButtonState: 0,
		MouseY: 6,
	})
	if sb.dragging { t.Error("Should stop dragging on release") }
}
