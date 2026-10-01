//go:build windows

package vtui

import (
	"fmt"

	"github.com/unxed/vtinput"
	"github.com/unxed/winkeys"
)

// Win32KeyEvent represents a decoded Windows key event with scan code and key name.
type Win32KeyEvent struct {
	VirtualKeyCode  uint16 // Virtual key code from Windows
	ScanCode        uint16 // Scan code (USB HID code)
	KeyName         string // Human-readable key name
	Char            rune   // Character representation
	KeyDown         bool   // true for key press, false for key release
	Shift           bool   // Shift modifier
	Ctrl            bool   // Ctrl modifier
	Alt             bool   // Alt modifier
	RawControlState winkeys.ControlKeyState
}

// VKToNameScanTable maps Virtual Key codes to key names and scan codes
// Scan codes follow USB HID usage table conventions
var VKToNameScanTable = map[uint16][2]interface{}{ // [2]{keyName, scanCode}
	winkeys.VK_ESCAPE:     {"Escape", uint16(0x29)},
	winkeys.VK_1:          {"1", uint16(0x1E)},
	winkeys.VK_2:          {"2", uint16(0x1F)},
	winkeys.VK_3:          {"3", uint16(0x20)},
	winkeys.VK_4:          {"4", uint16(0x21)},
	winkeys.VK_5:          {"5", uint16(0x22)},
	winkeys.VK_6:          {"6", uint16(0x23)},
	winkeys.VK_7:          {"7", uint16(0x24)},
	winkeys.VK_8:          {"8", uint16(0x25)},
	winkeys.VK_9:          {"9", uint16(0x26)},
	winkeys.VK_0:          {"0", uint16(0x27)},
	winkeys.VK_OEM_MINUS:  {"Minus", uint16(0x2D)},
	winkeys.VK_OEM_PLUS:   {"Equal", uint16(0x2E)},
	winkeys.VK_BACK:       {"Backspace", uint16(0x2A)},
	winkeys.VK_TAB:        {"Tab", uint16(0x2B)},
	winkeys.VK_Q:          {"q", uint16(0x14)},
	winkeys.VK_W:          {"w", uint16(0x1A)},
	winkeys.VK_E:          {"e", uint16(0x08)},
	winkeys.VK_R:          {"r", uint16(0x15)},
	winkeys.VK_T:          {"t", uint16(0x17)},
	winkeys.VK_Y:          {"y", uint16(0x1C)},
	winkeys.VK_U:          {"u", uint16(0x18)},
	winkeys.VK_I:          {"i", uint16(0x0C)},
	winkeys.VK_O:          {"o", uint16(0x12)},
	winkeys.VK_P:          {"p", uint16(0x13)},
	winkeys.VK_OEM_4:      {"LeftBracket", uint16(0x2F)},
	winkeys.VK_OEM_6:      {"RightBracket", uint16(0x30)},
	winkeys.VK_RETURN:     {"Return", uint16(0x28)},
	winkeys.VK_CONTROL:    {"Control", uint16(0xE0)},
	winkeys.VK_A:          {"a", uint16(0x04)},
	winkeys.VK_S:          {"s", uint16(0x16)},
	winkeys.VK_D:          {"d", uint16(0x07)},
	winkeys.VK_F:          {"f", uint16(0x09)},
	winkeys.VK_G:          {"g", uint16(0x0A)},
	winkeys.VK_H:          {"h", uint16(0x0B)},
	winkeys.VK_J:          {"j", uint16(0x0D)},
	winkeys.VK_K:          {"k", uint16(0x0E)},
	winkeys.VK_L:          {"l", uint16(0x0F)},
	winkeys.VK_OEM_1:      {"Semicolon", uint16(0x33)},
	winkeys.VK_OEM_7:      {"Apostrophe", uint16(0x34)},
	winkeys.VK_OEM_3:      {"Grave", uint16(0x35)},
	winkeys.VK_SHIFT:      {"Shift", uint16(0xE1)},
	winkeys.VK_OEM_5:      {"Backslash", uint16(0x31)},
	winkeys.VK_Z:          {"z", uint16(0x1D)},
	winkeys.VK_X:          {"x", uint16(0x1B)},
	winkeys.VK_C:          {"c", uint16(0x06)},
	winkeys.VK_V:          {"v", uint16(0x19)},
	winkeys.VK_B:          {"b", uint16(0x05)},
	winkeys.VK_N:          {"n", uint16(0x11)},
	winkeys.VK_M:          {"m", uint16(0x10)},
	winkeys.VK_OEM_COMMA:  {"Comma", uint16(0x36)},
	winkeys.VK_OEM_PERIOD: {"Period", uint16(0x37)},
	winkeys.VK_OEM_2:      {"Slash", uint16(0x38)},
	winkeys.VK_MENU:       {"Alt", uint16(0xE2)},
	winkeys.VK_SPACE:      {"Space", uint16(0x39)},
	winkeys.VK_CAPITAL:    {"CapsLock", uint16(0x39)},
	winkeys.VK_F1:         {"F1", uint16(0x3A)},
	winkeys.VK_F2:         {"F2", uint16(0x3B)},
	winkeys.VK_F3:         {"F3", uint16(0x3C)},
	winkeys.VK_F4:         {"F4", uint16(0x3D)},
	winkeys.VK_F5:         {"F5", uint16(0x3E)},
	winkeys.VK_F6:         {"F6", uint16(0x3F)},
	winkeys.VK_F7:         {"F7", uint16(0x40)},
	winkeys.VK_F8:         {"F8", uint16(0x41)},
	winkeys.VK_F9:         {"F9", uint16(0x42)},
	winkeys.VK_F10:        {"F10", uint16(0x43)},
	winkeys.VK_F11:        {"F11", uint16(0x44)},
	winkeys.VK_F12:        {"F12", uint16(0x45)},
	winkeys.VK_NUMLOCK:    {"NumLock", uint16(0x53)},
	winkeys.VK_SCROLL:     {"ScrollLock", uint16(0x47)},
	winkeys.VK_NUMPAD7:    {"Numpad7", uint16(0x5F)},
	winkeys.VK_NUMPAD8:    {"Numpad8", uint16(0x60)},
	winkeys.VK_NUMPAD9:    {"Numpad9", uint16(0x61)},
	winkeys.VK_SUBTRACT:   {"NumpadSubtract", uint16(0x56)},
	winkeys.VK_NUMPAD4:    {"Numpad4", uint16(0x5C)},
	winkeys.VK_NUMPAD5:    {"Numpad5", uint16(0x5D)},
	winkeys.VK_NUMPAD6:    {"Numpad6", uint16(0x5E)},
	winkeys.VK_ADD:        {"NumpadAdd", uint16(0x57)},
	winkeys.VK_NUMPAD1:    {"Numpad1", uint16(0x59)},
	winkeys.VK_NUMPAD2:    {"Numpad2", uint16(0x5A)},
	winkeys.VK_NUMPAD3:    {"Numpad3", uint16(0x5B)},
	winkeys.VK_NUMPAD0:    {"Numpad0", uint16(0x62)},
	winkeys.VK_DECIMAL:    {"NumpadDecimal", uint16(0x63)},
	winkeys.VK_F13:        {"F13", uint16(0x68)},
	winkeys.VK_F14:        {"F14", uint16(0x69)},
	winkeys.VK_F15:        {"F15", uint16(0x6A)},
	winkeys.VK_F16:        {"F16", uint16(0x6B)},
	winkeys.VK_F17:        {"F17", uint16(0x6C)},
	winkeys.VK_F18:        {"F18", uint16(0x6D)},
	winkeys.VK_F19:        {"F19", uint16(0x6E)},
	winkeys.VK_F20:        {"F20", uint16(0x6F)},
	winkeys.VK_F21:        {"F21", uint16(0x70)},
	winkeys.VK_F22:        {"F22", uint16(0x71)},
	winkeys.VK_F23:        {"F23", uint16(0x72)},
	winkeys.VK_F24:        {"F24", uint16(0x73)},
	// Extended keys (E0 prefix in scan code)
	winkeys.VK_PRIOR:      {"PageUp", uint16(0x49)},
	winkeys.VK_NEXT:       {"PageDown", uint16(0x51)},
	winkeys.VK_END:        {"End", uint16(0x4F)},
	winkeys.VK_HOME:       {"Home", uint16(0x4A)},
	winkeys.VK_LEFT:       {"Left", uint16(0x50)},
	winkeys.VK_UP:         {"Up", uint16(0x52)},
	winkeys.VK_RIGHT:      {"Right", uint16(0x4F)},
	winkeys.VK_DOWN:       {"Down", uint16(0x51)},
	winkeys.VK_INSERT:     {"Insert", uint16(0x49)},
	winkeys.VK_DELETE:     {"Delete", uint16(0x4C)},
	winkeys.VK_LWIN:       {"LeftMeta", uint16(0xE3)},
	winkeys.VK_RWIN:       {"RightMeta", uint16(0xE7)},
	winkeys.VK_APPS:       {"Menu", uint16(0x65)},
	winkeys.VK_LSHIFT:     {"LeftShift", uint16(0xE1)},
	winkeys.VK_RSHIFT:     {"RightShift", uint16(0xE5)},
	winkeys.VK_LCONTROL:   {"LeftControl", uint16(0xE0)},
	winkeys.VK_RCONTROL:   {"RightControl", uint16(0xE4)},
	winkeys.VK_LMENU:      {"LeftAlt", uint16(0xE2)},
	winkeys.VK_RMENU:      {"RightAlt", uint16(0xE6)},
}

// DecodeInputEvent converts a Win32 InputEvent to a Win32KeyEvent with scan code
// and decoded modifiers according to the kitty protocol specification.
func DecodeInputEvent(event *vtinput.InputEvent) (*Win32KeyEvent, error) {
	if event.Type != vtinput.KeyEventType {
		return nil, fmt.Errorf("not a key event: type=%d", event.Type)
	}

	vk := event.VirtualKeyCode
	decoded := &Win32KeyEvent{
		VirtualKeyCode:  vk,
		Char:            event.Char,
		KeyDown:         event.KeyDown,
		RawControlState: event.ControlKeyState,
	}

	// Decode key name and scan code from table
	if entry, exists := VKToNameScanTable[vk]; exists {
		if keyName, ok := entry[0].(string); ok {
			decoded.KeyName = keyName
		}
		if scanCode, ok := entry[1].(uint16); ok {
			decoded.ScanCode = scanCode
		}
	}

	// If not in table, use the virtual key code as fallback scan code
	if decoded.ScanCode == 0 && vk > 0 {
		decoded.ScanCode = vk
	}

	// Decode modifier flags for kitty protocol
	decoded.Shift = event.ControlKeyState.Contains(winkeys.ShiftPressed)
	decoded.Ctrl = event.ControlKeyState.Contains(winkeys.LeftCtrlPressed | winkeys.RightCtrlPressed)
	decoded.Alt = event.ControlKeyState.Contains(winkeys.LeftAltPressed | winkeys.RightAltPressed)

	if decoded.KeyName == "" {
		decoded.KeyName = fmt.Sprintf("VK_%d", vk)
	}

	return decoded, nil
}

// ToKittyKeyboardProtocol converts the decoded event to kitty keyboard protocol format.
// Returns the ANSI escape sequence that represents this key event.
// Format: CSI ? VK : SC : <modifiers> u
// where modifiers is a bitmask: 1=Shift, 2=Alt, 4=Ctrl, 8=Meta
func (e *Win32KeyEvent) ToKittyKeyboardProtocol() string {
	// Calculate modifier bitmask for kitty protocol
	// Bit 0: Shift (1)
	// Bit 1: Alt (2)
	// Bit 2: Ctrl (4)
	// Bit 3: Meta (8)
	mods := uint8(0)
	if e.Shift {
		mods |= 1
	}
	if e.Alt {
		mods |= 2
	}
	if e.Ctrl {
		mods |= 4
	}

	// For now, we don't track meta key separately
	// Format: CSI ? keycode : scancode : modifiers u
	return fmt.Sprintf("\x1B[?%d:%d:%d%c",
		e.VirtualKeyCode, e.ScanCode, mods, 'u')
}

// ToKittyKeyboardProtocolWithAction is like ToKittyKeyboardProtocol but includes key action (press/release).
// If the event is a key release, it appends the appropriate suffix.
func (e *Win32KeyEvent) ToKittyKeyboardProtocolWithAction() string {
	protocol := e.ToKittyKeyboardProtocol()
	if !e.KeyDown {
		// Key release - append "m" suffix according to kitty spec
		// Remove the trailing 'u' and add release indicator
		protocol = protocol[:len(protocol)-1] + "m"
	}
	return protocol
}

// String returns a human-readable representation of the decoded key event.
func (e *Win32KeyEvent) String() string {
	action := "press"
	if !e.KeyDown {
		action = "release"
	}
	mods := ""
	if e.Shift {
		mods += "Shift+"
	}
	if e.Ctrl {
		mods += "Ctrl+"
	}
	if e.Alt {
		mods += "Alt+"
	}
	charStr := ""
	if e.Char > 0 && e.Char != '\x00' {
		charStr = fmt.Sprintf(" Char:'%c'", e.Char)
	}
	return fmt.Sprintf("Key{%s%s SC:0x%02X VK:0x%02X %s%s}",
		mods, e.KeyName, e.ScanCode, e.VirtualKeyCode, action, charStr)
}
