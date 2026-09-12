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
