package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestScrollBar_WidgetMouse(t *testing.T) {
	val := -1
	sb := NewScrollBar(0, 0, 10) // height 10, Y: 0..9
	sb.SetParams(5, 0, 20)
	sb.SetVisible(true) // Required for internal hit-testing to pass
	sb.SetOnScroll(func(v int) { val = v })

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

	// 3. Page Up area (Value 5, Track 1-8, Thumb 2-4. Click at Y=1)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 1,
	})
	if val != 0 { t.Errorf("PageUp click failed (5-10=-5 -> 0), got %d", val) }

	// 4. Page Down area (Value 5, Click at Y=7)
	sb.SetParams(5, 0, 20)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 7,
	})
	if val != 15 { t.Errorf("PageDown click failed (5+10=15), got %d", val) }
}

func TestScrollBar_OnStep(t *testing.T) {
	stepVal := 0
	sb := NewScrollBar(0, 0, 10) // Y: 0..9
	sb.SetParams(5, 0, 20)
	sb.SetVisible(true)
	sb.SetOnStep(func(s int) { stepVal = s })

	// 1. Click Up Arrow (Y=0)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 0, MouseX: 0,
	})
	if stepVal != -1 { t.Errorf("OnStep Up failed, got %d", stepVal) }

	// 2. Click Down Arrow (Y=9)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 9, MouseX: 0,
	})
	if stepVal != 1 { t.Errorf("OnStep Down failed, got %d", stepVal) }
}

func TestScrollBar_Dragging(t *testing.T) {
	scrolledVal := -1
	sb := NewScrollBar(0, 0, 10) // Track length = 8 (1-8)
	sb.SetParams(0, 0, 100)
	sb.SetVisible(true)
	sb.SetOnScroll(func(v int) { scrolledVal = v })

	// 1. Initial click on thumb (TopPos 0, thumb is at Y=1)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 1, MouseX: 0,
	})
	if !sb.isDragging { t.Fatal("Dragging should start") }

	// 2. Move mouse to Y=5 (middle of the track)
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseY: 5, MouseX: 0,
	})

	if scrolledVal <= 0 {
		t.Errorf("Dragging failed to trigger scroll, value: %d", scrolledVal)
	}

	// 3. Release
	sb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false, ButtonState: 0,
		MouseY: 5, MouseX: 0,
	})
	if sb.isDragging { t.Error("Dragging should stop on release") }
}
