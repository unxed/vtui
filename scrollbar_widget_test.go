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
	if val != 6 { t.Errorf("Bottom arrow click failed, got %d", val) }

	// 3. Page Up area (e.g., Y=2)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 2,
	})
	if val != 0 { t.Errorf("PageUp click failed (5-10=-5 -> 0), got %d", val) }

	// 4. Page Down area (e.g., Y=7)
	sb.SetParams(5, 0, 20)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 7,
	})
	if val != 15 { t.Errorf("PageDown click failed (5+10=15), got %d", val) }
}