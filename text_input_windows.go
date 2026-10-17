//go:build windows

package vtui

import (
	"unsafe"

	"github.com/unxed/vtinput"
	"golang.org/x/sys/windows"
)

var (
	textGetKeyboardState  = user32.NewProc("GetKeyboardState")
	textGetKeyboardLayout = user32.NewProc("GetKeyboardLayout")
	textMapVirtualKeyEx   = user32.NewProc("MapVirtualKeyExW")
	textToUnicodeEx       = user32.NewProc("ToUnicodeEx")
	textProbeSupported    = windows.RtlGetVersion().BuildNumber >= 14393
)

// guiKeyTranslation is called on the native event thread, never for remote
// terminal input. Flag 4 leaves Windows' dead-key composition state untouched.
// A nonzero result reserves this key for the later character callback.
func guiKeyTranslation(vk, scan uint16, mods vtinput.ControlKeyState) int {
	if !textProbeSupported || vk == 0 || isGUIModifierKeyCode(vk) {
		return 0
	}
	var state [256]byte
	ok, _, _ := textGetKeyboardState.Call(uintptr(unsafe.Pointer(&state[0])))
	if ok == 0 {
		return 0
	}
	// Use the same modifiers that will be attached to this native key event.
	for _, key := range []uint16{vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU, vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT} {
		state[key] &^= 0x80
	}
	if mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0 {
		state[vtinput.VK_CONTROL] |= 0x80
	}
	if mods&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
		state[vtinput.VK_MENU] |= 0x80
	}
	if mods&vtinput.ShiftPressed != 0 {
		state[vtinput.VK_SHIFT] |= 0x80
	}
	state[vtinput.VK_CAPITAL] = 0
	state[vtinput.VK_NUMLOCK] = 0
	if mods&vtinput.CapsLockOn != 0 {
		state[vtinput.VK_CAPITAL] = 1
	}
	if mods&vtinput.NumLockOn != 0 {
		state[vtinput.VK_NUMLOCK] = 1
	}
	state[vk] |= 0x80
	layout, _, _ := textGetKeyboardLayout.Call(0)
	if scan == 0 {
		mapped, _, _ := textMapVirtualKeyEx.Call(uintptr(vk), 0, layout)
		scan = uint16(mapped)
	}
	var chars [8]uint16
	result, _, _ := textToUnicodeEx.Call(uintptr(vk), uintptr(scan), uintptr(unsafe.Pointer(&state[0])),
		uintptr(unsafe.Pointer(&chars[0])), uintptr(len(chars)), 4, layout)
	n := int32(result)
	if n < 0 {
		return -1
	}
	if n > 0 && chars[0] >= ' ' && chars[0] != 0x7f {
		return int(n)
	}
	return 0
}

func isGUIModifierKeyCode(vk uint16) bool {
	switch vk {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LWIN, vtinput.VK_RWIN, vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return true
	}
	return false
}
