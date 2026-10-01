package vtui

import "github.com/unxed/vtinput"

// IsMousePress distinguishes a new press from held-button motion reports.
func IsMousePress(e *vtinput.InputEvent) bool {
	return e.KeyDown && e.ButtonState != 0 && e.WheelDirection == 0 && e.MouseEventFlags&vtinput.MouseMoved == 0
}

// IsMouseRelease accepts both console releases (no buttons) and ANSI/SGR
// releases (the released button remains named, but KeyDown is false).
func IsMouseRelease(e *vtinput.InputEvent) bool {
	return e.WheelDirection == 0 && (e.ButtonState == 0 || !e.KeyDown && e.MouseEventFlags&vtinput.MouseMoved == 0)
}

// IsMouseMotion reports whether the event represents mouse movement without button changes.
func IsMouseMotion(e *vtinput.InputEvent) bool {
	return e.Type == vtinput.MouseEventType && e.MouseEventFlags&vtinput.MouseMoved != 0
}

// IsMouseWheelEvent reports whether the event represents mouse wheel scrolling.
func IsMouseWheelEvent(e *vtinput.InputEvent) bool {
	return e.Type == vtinput.MouseEventType && e.WheelDirection != 0
}

// IsLeftMouseButton reports whether the left mouse button is involved in the event.
func IsLeftMouseButton(e *vtinput.InputEvent) bool {
	return e.ButtonState&vtinput.FromLeft1stButtonPressed != 0
}

// IsMiddleMouseButton reports whether the middle mouse button is involved in the event.
func IsMiddleMouseButton(e *vtinput.InputEvent) bool {
	return e.ButtonState&vtinput.FromLeft2ndButtonPressed != 0
}

// IsRightMouseButton reports whether the right mouse button is involved in the event.
func IsRightMouseButton(e *vtinput.InputEvent) bool {
	return e.ButtonState&vtinput.RightmostButtonPressed != 0
}

// IsDoubleClick reports whether the event is a double-click based on the mouse event flags.
func IsDoubleClick(e *vtinput.InputEvent) bool {
	return e.Type == vtinput.MouseEventType && e.MouseEventFlags&vtinput.DoubleClick != 0
}

// GetMouseCoordinates returns the x and y coordinates of a mouse event.
func GetMouseCoordinates(e *vtinput.InputEvent) (x, y int) {
	return int(e.MouseX), int(e.MouseY)
}

// IsMouseEventInBounds checks if a mouse event occurred within the specified rectangular bounds.
func IsMouseEventInBounds(e *vtinput.InputEvent, x1, y1, x2, y2 int) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}
	mx, my := int(e.MouseX), int(e.MouseY)
	return mx >= x1 && mx <= x2 && my >= y1 && my <= y2
}
