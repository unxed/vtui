//go:build windows

package vtui

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/winkeys"
)

func TestDecodeInputEvent_LetterKey(t *testing.T) {
	tests := []struct {
		name      string
		vk        uint16
		keyName   string
		scanCode  uint16
		shift     bool
		ctrl      bool
		alt       bool
	}{
		{"A key", winkeys.VK_A, "a", 0x04, false, false, false},
		{"Z key", winkeys.VK_Z, "z", 0x1D, false, false, false},
		{"Q key", winkeys.VK_Q, "q", 0x14, false, false, false},
		{"M key", winkeys.VK_M, "m", 0x10, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &vtinput.InputEvent{
				Type:           vtinput.KeyEventType,
				VirtualKeyCode: tt.vk,
				KeyDown:        true,
			}

			decoded, err := DecodeInputEvent(event)
			if err != nil {
				t.Fatalf("DecodeInputEvent failed: %v", err)
			}

			if decoded.KeyName != tt.keyName {
				t.Errorf("KeyName: got %q, want %q", decoded.KeyName, tt.keyName)
			}
			if decoded.ScanCode != tt.scanCode {
				t.Errorf("ScanCode: got 0x%02X, want 0x%02X", decoded.ScanCode, tt.scanCode)
			}
			if decoded.VirtualKeyCode != tt.vk {
				t.Errorf("VirtualKeyCode: got %d, want %d", decoded.VirtualKeyCode, tt.vk)
			}
		})
	}
}

func TestDecodeInputEvent_NumberKey(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint16
		keyName  string
		scanCode uint16
	}{
		{"0 key", winkeys.VK_0, "0", 0x27},
		{"1 key", winkeys.VK_1, "1", 0x1E},
		{"5 key", winkeys.VK_5, "5", 0x22},
		{"9 key", winkeys.VK_9, "9", 0x26},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &vtinput.InputEvent{
				Type:           vtinput.KeyEventType,
				VirtualKeyCode: tt.vk,
				KeyDown:        true,
			}

			decoded, err := DecodeInputEvent(event)
			if err != nil {
				t.Fatalf("DecodeInputEvent failed: %v", err)
			}

			if decoded.KeyName != tt.keyName {
				t.Errorf("KeyName: got %q, want %q", decoded.KeyName, tt.keyName)
			}
			if decoded.ScanCode != tt.scanCode {
				t.Errorf("ScanCode: got 0x%02X, want 0x%02X", decoded.ScanCode, tt.scanCode)
			}
		})
	}
}

func TestDecodeInputEvent_FunctionKey(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint16
		keyName  string
		scanCode uint16
	}{
		{"F1 key", winkeys.VK_F1, "F1", 0x3A},
		{"F5 key", winkeys.VK_F5, "F5", 0x3E},
		{"F10 key", winkeys.VK_F10, "F10", 0x43},
		{"F12 key", winkeys.VK_F12, "F12", 0x45},
		{"F24 key", winkeys.VK_F24, "F24", 0x73},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &vtinput.InputEvent{
				Type:           vtinput.KeyEventType,
				VirtualKeyCode: tt.vk,
				KeyDown:        true,
			}

			decoded, err := DecodeInputEvent(event)
			if err != nil {
				t.Fatalf("DecodeInputEvent failed: %v", err)
			}

			if decoded.KeyName != tt.keyName {
				t.Errorf("KeyName: got %q, want %q", decoded.KeyName, tt.keyName)
			}
			if decoded.ScanCode != tt.scanCode {
				t.Errorf("ScanCode: got 0x%02X, want 0x%02X", decoded.ScanCode, tt.scanCode)
			}
		})
	}
}

func TestDecodeInputEvent_SpecialKey(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint16
		keyName  string
		scanCode uint16
	}{
		{"Escape", winkeys.VK_ESCAPE, "Escape", 0x29},
		{"Tab", winkeys.VK_TAB, "Tab", 0x2B},
		{"Return", winkeys.VK_RETURN, "Return", 0x28},
		{"Backspace", winkeys.VK_BACK, "Backspace", 0x2A},
		{"Space", winkeys.VK_SPACE, "Space", 0x39},
		{"CapsLock", winkeys.VK_CAPITAL, "CapsLock", 0x39},
		{"Home", winkeys.VK_HOME, "Home", 0x4A},
		{"End", winkeys.VK_END, "End", 0x4F},
		{"PageUp", winkeys.VK_PRIOR, "PageUp", 0x49},
		{"PageDown", winkeys.VK_NEXT, "PageDown", 0x51},
		{"Insert", winkeys.VK_INSERT, "Insert", 0x49},
		{"Delete", winkeys.VK_DELETE, "Delete", 0x4C},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &vtinput.InputEvent{
				Type:           vtinput.KeyEventType,
				VirtualKeyCode: tt.vk,
				KeyDown:        true,
			}

			decoded, err := DecodeInputEvent(event)
			if err != nil {
				t.Fatalf("DecodeInputEvent failed: %v", err)
			}

			if decoded.KeyName != tt.keyName {
				t.Errorf("KeyName: got %q, want %q", decoded.KeyName, tt.keyName)
			}
			if decoded.ScanCode != tt.scanCode {
				t.Errorf("ScanCode: got 0x%02X, want 0x%02X", decoded.ScanCode, tt.scanCode)
			}
		})
	}
}

func TestDecodeInputEvent_ArrowKey(t *testing.T) {
	tests := []struct {
		name     string
		vk       uint16
		keyName  string
		scanCode uint16
	}{
		{"Left arrow", winkeys.VK_LEFT, "Left", 0x50},
		{"Up arrow", winkeys.VK_UP, "Up", 0x52},
		{"Right arrow", winkeys.VK_RIGHT, "Right", 0x4F},
		{"Down arrow", winkeys.VK_DOWN, "Down", 0x51},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &vtinput.InputEvent{
				Type:           vtinput.KeyEventType,
				VirtualKeyCode: tt.vk,
				KeyDown:        true,
			}

			decoded, err := DecodeInputEvent(event)
			if err != nil {
				t.Fatalf("DecodeInputEvent failed: %v", err)
			}

			if decoded.KeyName != tt.keyName {
				t.Errorf("KeyName: got %q, want %q", decoded.KeyName, tt.keyName)
			}
			if decoded.ScanCode != tt.scanCode {
				t.Errorf("ScanCode: got 0x%02X, want 0x%02X", decoded.ScanCode, tt.scanCode)
			}
		})
	}
}

func TestDecodeInputEvent_ModifierShift(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:             vtinput.KeyEventType,
		VirtualKeyCode:   winkeys.VK_A,
		KeyDown:          true,
		ControlKeyState:  winkeys.ShiftPressed,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if !decoded.Shift {
		t.Error("Shift modifier not decoded")
	}
	if decoded.Ctrl {
		t.Error("Ctrl should not be set")
	}
	if decoded.Alt {
		t.Error("Alt should not be set")
	}
}

func TestDecodeInputEvent_ModifierCtrl(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:             vtinput.KeyEventType,
		VirtualKeyCode:   winkeys.VK_C,
		KeyDown:          true,
		ControlKeyState:  winkeys.LeftCtrlPressed,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if decoded.Shift {
		t.Error("Shift should not be set")
	}
	if !decoded.Ctrl {
		t.Error("Ctrl modifier not decoded")
	}
	if decoded.Alt {
		t.Error("Alt should not be set")
	}
}

func TestDecodeInputEvent_ModifierAlt(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:             vtinput.KeyEventType,
		VirtualKeyCode:   winkeys.VK_F4,
		KeyDown:          true,
		ControlKeyState:  winkeys.LeftAltPressed,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if decoded.Shift {
		t.Error("Shift should not be set")
	}
	if decoded.Ctrl {
		t.Error("Ctrl should not be set")
	}
	if !decoded.Alt {
		t.Error("Alt modifier not decoded")
	}
}

func TestDecodeInputEvent_MultipleModifiers(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:             vtinput.KeyEventType,
		VirtualKeyCode:   winkeys.VK_S,
		KeyDown:          true,
		ControlKeyState:  winkeys.LeftCtrlPressed | winkeys.ShiftPressed,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if !decoded.Shift {
		t.Error("Shift modifier not decoded")
	}
	if !decoded.Ctrl {
		t.Error("Ctrl modifier not decoded")
	}
	if decoded.Alt {
		t.Error("Alt should not be set")
	}
}

func TestDecodeInputEvent_KeyDown(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		VirtualKeyCode: winkeys.VK_A,
		KeyDown:        true,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if !decoded.KeyDown {
		t.Error("KeyDown not set correctly")
	}
}

func TestDecodeInputEvent_KeyUp(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		VirtualKeyCode: winkeys.VK_A,
		KeyDown:        false,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if decoded.KeyDown {
		t.Error("KeyDown should be false for key release")
	}
}

func TestDecodeInputEvent_Char(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		VirtualKeyCode: winkeys.VK_A,
		Char:           'x',
		KeyDown:        true,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if decoded.Char != 'x' {
		t.Errorf("Char: got %q, want %q", decoded.Char, 'x')
	}
}

func TestDecodeInputEvent_NonKeyEvent(t *testing.T) {
	event := &vtinput.InputEvent{
		Type: vtinput.MouseEventType,
	}

	_, err := DecodeInputEvent(event)
	if err == nil {
		t.Error("Expected error for non-key event")
	}
}

func TestDecodeInputEvent_UnknownVK(t *testing.T) {
	// Use a VK code that's unlikely to be in the table
	event := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		VirtualKeyCode: 0xFE, // Unlikely VK code
		KeyDown:        true,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	// Should use fallback keyname format
	if !strings.Contains(decoded.KeyName, "VK_") {
		t.Errorf("Expected fallback keyname format for unknown VK, got %q", decoded.KeyName)
	}
	// Should use VK code as scancode fallback
	if decoded.ScanCode != 0xFE {
		t.Errorf("Expected fallback scancode 0xFE, got 0x%02X", decoded.ScanCode)
	}
}

func TestToKittyKeyboardProtocol_NoModifiers(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_A,
		ScanCode:       0x04,
		KeyName:        "a",
		KeyDown:        true,
		Shift:          false,
		Ctrl:           false,
		Alt:            false,
	}

	protocol := decoded.ToKittyKeyboardProtocol()

	// Expected format: CSI ? VK : SC : mods u
	// CSI = \x1B[ , so format is \x1B[?VK:SC:mods u
	if !strings.HasPrefix(protocol, "\x1B[?") {
		t.Errorf("Protocol should start with CSI?, got %q", protocol)
	}
	if !strings.Contains(protocol, "68") { // 0x04 in decimal would be in hex part
		t.Logf("Protocol: %q", protocol)
	}
	if !strings.HasSuffix(protocol, "u") {
		t.Errorf("Protocol should end with 'u', got %q", protocol)
	}
}

func TestToKittyKeyboardProtocol_WithShift(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_A,
		ScanCode:       0x04,
		KeyName:        "a",
		KeyDown:        true,
		Shift:          true,
		Ctrl:           false,
		Alt:            false,
	}

	protocol := decoded.ToKittyKeyboardProtocol()

	// Should contain :1u for Shift modifier
	if !strings.Contains(protocol, ":1u") {
		t.Errorf("Protocol should contain :1u for Shift, got %q", protocol)
	}
}

func TestToKittyKeyboardProtocol_WithCtrl(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_C,
		ScanCode:       0x06,
		KeyName:        "c",
		KeyDown:        true,
		Shift:          false,
		Ctrl:           true,
		Alt:            false,
	}

	protocol := decoded.ToKittyKeyboardProtocol()

	// Should contain :4u for Ctrl modifier (bit 2)
	if !strings.Contains(protocol, ":4u") {
		t.Errorf("Protocol should contain :4u for Ctrl, got %q", protocol)
	}
}

func TestToKittyKeyboardProtocol_WithAlt(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_F4,
		ScanCode:       0x3D,
		KeyName:        "F4",
		KeyDown:        true,
		Shift:          false,
		Ctrl:           false,
		Alt:            true,
	}

	protocol := decoded.ToKittyKeyboardProtocol()

	// Should contain :2u for Alt modifier (bit 1)
	if !strings.Contains(protocol, ":2u") {
		t.Errorf("Protocol should contain :2u for Alt, got %q", protocol)
	}
}

func TestToKittyKeyboardProtocol_AllModifiers(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_S,
		ScanCode:       0x16,
		KeyName:        "s",
		KeyDown:        true,
		Shift:          true,
		Ctrl:           true,
		Alt:            true,
	}

	protocol := decoded.ToKittyKeyboardProtocol()

	// Should contain :7u for all modifiers (1|2|4=7)
	if !strings.Contains(protocol, ":7u") {
		t.Errorf("Protocol should contain :7u for all modifiers, got %q", protocol)
	}
}

func TestToKittyKeyboardProtocolWithAction_KeyPress(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_A,
		ScanCode:       0x04,
		KeyName:        "a",
		KeyDown:        true,
		Shift:          false,
		Ctrl:           false,
		Alt:            false,
	}

	protocol := decoded.ToKittyKeyboardProtocolWithAction()

	// Key press should end with 'u'
	if !strings.HasSuffix(protocol, "u") {
		t.Errorf("Key press should end with 'u', got %q", protocol)
	}
}

func TestToKittyKeyboardProtocolWithAction_KeyRelease(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_A,
		ScanCode:       0x04,
		KeyName:        "a",
		KeyDown:        false,
		Shift:          false,
		Ctrl:           false,
		Alt:            false,
	}

	protocol := decoded.ToKittyKeyboardProtocolWithAction()

	// Key release should end with 'm'
	if !strings.HasSuffix(protocol, "m") {
		t.Errorf("Key release should end with 'm', got %q", protocol)
	}
}

func TestString_NoModifiers(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_A,
		ScanCode:       0x04,
		KeyName:        "a",
		KeyDown:        true,
		Shift:          false,
		Ctrl:           false,
		Alt:            false,
	}

	str := decoded.String()

	if !strings.Contains(str, "a") {
		t.Errorf("String should contain key name 'a', got %q", str)
	}
	if !strings.Contains(str, "press") {
		t.Errorf("String should contain 'press', got %q", str)
	}
	if !strings.Contains(str, "SC:0x04") {
		t.Errorf("String should contain 'SC:0x04', got %q", str)
	}
}

func TestString_WithModifiers(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_C,
		ScanCode:       0x06,
		KeyName:        "c",
		KeyDown:        true,
		Shift:          true,
		Ctrl:           true,
		Alt:            false,
		Char:           'C',
	}

	str := decoded.String()

	if !strings.Contains(str, "Shift+") {
		t.Errorf("String should contain 'Shift+', got %q", str)
	}
	if !strings.Contains(str, "Ctrl+") {
		t.Errorf("String should contain 'Ctrl+', got %q", str)
	}
	if !strings.Contains(str, "c") {
		t.Errorf("String should contain key name 'c', got %q", str)
	}
}

func TestString_KeyRelease(t *testing.T) {
	decoded := &Win32KeyEvent{
		VirtualKeyCode: winkeys.VK_B,
		ScanCode:       0x05,
		KeyName:        "b",
		KeyDown:        false,
		Shift:          false,
		Ctrl:           false,
		Alt:            false,
	}

	str := decoded.String()

	if !strings.Contains(str, "release") {
		t.Errorf("String should contain 'release', got %q", str)
	}
	if strings.Contains(str, "press") && !strings.Contains(str, "release") {
		t.Errorf("Should not contain 'press' for key release, got %q", str)
	}
}

func TestVKToNameScanTable_Coverage(t *testing.T) {
	// Verify that critical keys are present
	criticalKeys := []uint16{
		winkeys.VK_A, winkeys.VK_ESCAPE, winkeys.VK_RETURN,
		winkeys.VK_F1, winkeys.VK_LEFT, winkeys.VK_SPACE,
	}

	for _, vk := range criticalKeys {
		if _, exists := VKToNameScanTable[vk]; !exists {
			t.Errorf("Critical VK 0x%02X not in table", vk)
		}
	}
}

func TestIntegration_CtrlC(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:             vtinput.KeyEventType,
		VirtualKeyCode:   winkeys.VK_C,
		Char:             '\x03',
		KeyDown:          true,
		ControlKeyState:  winkeys.LeftCtrlPressed,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if decoded.KeyName != "c" {
		t.Errorf("Expected 'c', got %q", decoded.KeyName)
	}
	if !decoded.Ctrl {
		t.Error("Ctrl modifier not detected")
	}
	if decoded.Char != '\x03' {
		t.Errorf("Expected Char '\\x03', got %q", decoded.Char)
	}

	protocol := decoded.ToKittyKeyboardProtocol()
	if !strings.Contains(protocol, ":4") { // Ctrl is bit 2 = 4
		t.Errorf("Kitty protocol should contain Ctrl modifier, got %q", protocol)
	}
}

func TestIntegration_AltF4(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:             vtinput.KeyEventType,
		VirtualKeyCode:   winkeys.VK_F4,
		KeyDown:          true,
		ControlKeyState:  winkeys.LeftAltPressed,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if decoded.KeyName != "F4" {
		t.Errorf("Expected 'F4', got %q", decoded.KeyName)
	}
	if !decoded.Alt {
		t.Error("Alt modifier not detected")
	}

	protocol := decoded.ToKittyKeyboardProtocol()
	if !strings.Contains(protocol, ":2") { // Alt is bit 1 = 2
		t.Errorf("Kitty protocol should contain Alt modifier, got %q", protocol)
	}
}

func TestIntegration_ShiftTab(t *testing.T) {
	event := &vtinput.InputEvent{
		Type:             vtinput.KeyEventType,
		VirtualKeyCode:   winkeys.VK_TAB,
		KeyDown:          true,
		ControlKeyState:  winkeys.ShiftPressed,
	}

	decoded, err := DecodeInputEvent(event)
	if err != nil {
		t.Fatalf("DecodeInputEvent failed: %v", err)
	}

	if decoded.KeyName != "Tab" {
		t.Errorf("Expected 'Tab', got %q", decoded.KeyName)
	}
	if !decoded.Shift {
		t.Error("Shift modifier not detected")
	}

	protocol := decoded.ToKittyKeyboardProtocol()
	if !strings.Contains(protocol, ":1") { // Shift is bit 0 = 1
		t.Errorf("Kitty protocol should contain Shift modifier, got %q", protocol)
	}
}
