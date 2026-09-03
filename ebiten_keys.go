//go:build (linux || windows || darwin) && !android && (amd64 || arm64)

package vtui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/unxed/vtinput"
)

// ebitenKeyToVK translates an Ebitengine key code into the Win32 virtual key
// code that vtinput speaks. Zero means "no equivalent": the caller must drop
// the event rather than send VK 0, which the framework would read as a real
// key.
//
// Ebitengine reports physical keys, so the letter and digit rows map by
// position and not by the character the layout produces. That character
// arrives separately through AppendInputChars, which is why the host pairs
// each keydown with the text input of the same frame instead of deriving a
// rune from the key code here.
func isNumLockEffective(mods vtinput.ControlKeyState) bool {
	numLock := (mods & vtinput.NumLockOn) != 0
	shift := (mods & vtinput.ShiftPressed) != 0
	return numLock != shift
}

// isEbitenEnhancedNavKey reports whether k is part of the "enhanced"
// navigation cluster, so a consumer can tell it from the numpad arrows that
// reuse the same virtual key codes with NumLock off.
func isEbitenEnhancedNavKey(k ebiten.Key) bool {
	switch k {
	case ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeyArrowLeft, ebiten.KeyArrowRight,
		ebiten.KeyHome, ebiten.KeyEnd, ebiten.KeyPageUp, ebiten.KeyPageDown,
		ebiten.KeyInsert, ebiten.KeyDelete:
		return true
	}
	return false
}

func ebitenKeyToVK(k ebiten.Key, mods vtinput.ControlKeyState) uint16 {
	switch k {
	// Editing and navigation.
	case ebiten.KeyEscape:
		return vtinput.VK_ESCAPE
	case ebiten.KeyEnter:
		return vtinput.VK_RETURN
	case ebiten.KeyNumpadEnter:
		return vtinput.VK_RETURN
	case ebiten.KeyTab:
		return vtinput.VK_TAB
	case ebiten.KeyBackspace:
		return vtinput.VK_BACK
	case ebiten.KeyDelete:
		return vtinput.VK_DELETE
	case ebiten.KeyInsert:
		return vtinput.VK_INSERT
	case ebiten.KeyHome:
		return vtinput.VK_HOME
	case ebiten.KeyEnd:
		return vtinput.VK_END
	case ebiten.KeyPageUp:
		return vtinput.VK_PRIOR
	case ebiten.KeyPageDown:
		return vtinput.VK_NEXT
	case ebiten.KeyArrowUp:
		return vtinput.VK_UP
	case ebiten.KeyArrowDown:
		return vtinput.VK_DOWN
	case ebiten.KeyArrowLeft:
		return vtinput.VK_LEFT
	case ebiten.KeyArrowRight:
		return vtinput.VK_RIGHT
	case ebiten.KeySpace:
		return vtinput.VK_SPACE

	// Modifiers. vtui distinguishes sides, so we keep them apart.
	case ebiten.KeyControlLeft:
		return vtinput.VK_LCONTROL
	case ebiten.KeyControlRight:
		return vtinput.VK_RCONTROL
	case ebiten.KeyShiftLeft:
		return vtinput.VK_LSHIFT
	case ebiten.KeyShiftRight:
		return vtinput.VK_RSHIFT
	case ebiten.KeyAltLeft:
		return vtinput.VK_LMENU
	case ebiten.KeyAltRight:
		return vtinput.VK_RMENU
	case ebiten.KeyMetaLeft:
		return vtinput.VK_LWIN
	case ebiten.KeyMetaRight:
		return vtinput.VK_RWIN
	case ebiten.KeyContextMenu:
		return vtinput.VK_APPS

	// Locks.
	case ebiten.KeyCapsLock:
		return vtinput.VK_CAPITAL
	case ebiten.KeyNumLock:
		return vtinput.VK_NUMLOCK
	case ebiten.KeyScrollLock:
		return vtinput.VK_SCROLL
	case ebiten.KeyPause:
		return vtinput.VK_PAUSE
	case ebiten.KeyPrintScreen:
		return vtinput.VK_SNAPSHOT

	// Function keys.
	case ebiten.KeyF1:
		return vtinput.VK_F1
	case ebiten.KeyF2:
		return vtinput.VK_F2
	case ebiten.KeyF3:
		return vtinput.VK_F3
	case ebiten.KeyF4:
		return vtinput.VK_F4
	case ebiten.KeyF5:
		return vtinput.VK_F5
	case ebiten.KeyF6:
		return vtinput.VK_F6
	case ebiten.KeyF7:
		return vtinput.VK_F7
	case ebiten.KeyF8:
		return vtinput.VK_F8
	case ebiten.KeyF9:
		return vtinput.VK_F9
	case ebiten.KeyF10:
		return vtinput.VK_F10
	case ebiten.KeyF11:
		return vtinput.VK_F11
	case ebiten.KeyF12:
		return vtinput.VK_F12

	// Numeric keypad.
	case ebiten.KeyNumpad0:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD0
		}
		return vtinput.VK_INSERT
	case ebiten.KeyNumpad1:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD1
		}
		return vtinput.VK_END
	case ebiten.KeyNumpad2:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD2
		}
		return vtinput.VK_DOWN
	case ebiten.KeyNumpad3:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD3
		}
		return vtinput.VK_NEXT
	case ebiten.KeyNumpad4:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD4
		}
		return vtinput.VK_LEFT
	case ebiten.KeyNumpad5:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD5
		}
		return vtinput.VK_CLEAR
	case ebiten.KeyNumpad6:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD6
		}
		return vtinput.VK_RIGHT
	case ebiten.KeyNumpad7:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD7
		}
		return vtinput.VK_HOME
	case ebiten.KeyNumpad8:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD8
		}
		return vtinput.VK_UP
	case ebiten.KeyNumpad9:
		if isNumLockEffective(mods) {
			return vtinput.VK_NUMPAD9
		}
		return vtinput.VK_PRIOR
	case ebiten.KeyNumpadAdd:
		return vtinput.VK_ADD
	case ebiten.KeyNumpadSubtract:
		return vtinput.VK_SUBTRACT
	case ebiten.KeyNumpadMultiply:
		return vtinput.VK_MULTIPLY
	case ebiten.KeyNumpadDivide:
		return vtinput.VK_DIVIDE
	case ebiten.KeyNumpadDecimal:
		if isNumLockEffective(mods) {
			return vtinput.VK_DECIMAL
		}
		return vtinput.VK_DELETE

	// Punctuation. These are the US positions; the layout-dependent character
	// still comes from AppendInputChars, so a non-US layout gets the right
	// text even though the VK reflects the physical key.
	case ebiten.KeyMinus:
		return vtinput.VK_OEM_MINUS
	case ebiten.KeyEqual:
		return vtinput.VK_OEM_PLUS
	case ebiten.KeyComma:
		return vtinput.VK_OEM_COMMA
	case ebiten.KeyPeriod:
		return vtinput.VK_OEM_PERIOD
	case ebiten.KeySlash:
		return vtinput.VK_OEM_2
	case ebiten.KeySemicolon:
		return vtinput.VK_OEM_1
	case ebiten.KeyQuote:
		return vtinput.VK_OEM_7
	case ebiten.KeyBracketLeft:
		return vtinput.VK_OEM_4
	case ebiten.KeyBracketRight:
		return vtinput.VK_OEM_6
	case ebiten.KeyBackslash:
		return vtinput.VK_OEM_5
	case ebiten.KeyBackquote:
		return vtinput.VK_OEM_3
	case ebiten.KeyIntlBackslash:
		return vtinput.VK_OEM_102
	}

	// Letters and digits are contiguous in both encodings, so a range check
	// beats another sixty case labels.
	if k >= ebiten.KeyA && k <= ebiten.KeyZ {
		return vtinput.VK_A + uint16(k-ebiten.KeyA)
	}
	if k >= ebiten.KeyDigit0 && k <= ebiten.KeyDigit9 {
		return vtinput.VK_0 + uint16(k-ebiten.KeyDigit0)
	}

	return 0
}

// ebitenModifiers reads the current modifier state straight from Ebitengine.
// It is polled per frame rather than accumulated from key events, so a
// modifier released while the window was unfocused cannot get stuck down.
func ebitenModifiers() vtinput.ControlKeyState {
	var mods vtinput.ControlKeyState
	if ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) {
		mods |= vtinput.ShiftPressed
	}
	if ebiten.IsKeyPressed(ebiten.KeyControlLeft) {
		mods |= vtinput.LeftCtrlPressed
	}
	if ebiten.IsKeyPressed(ebiten.KeyControlRight) {
		mods |= vtinput.RightCtrlPressed
	}
	if ebiten.IsKeyPressed(ebiten.KeyAltLeft) {
		mods |= vtinput.LeftAltPressed
	}
	if ebiten.IsKeyPressed(ebiten.KeyAltRight) {
		mods |= vtinput.RightAltPressed
	}
	if ebiten.IsCapsLockOn() {
		mods |= vtinput.CapsLockOn
	}
	if ebiten.IsNumLockOn() {
		mods |= vtinput.NumLockOn
	}
	return mods
}

// isModifierVK reports whether a virtual key is a modifier or a lock.
//
// These are excluded from auto-repeat: holding Shift means one sustained
// modifier state, not a stream of Shift presses, and repeating them would
// flood the queue for the whole time a chord is held down.
func isModifierVK(vk uint16) bool {
	switch vk {
	case vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LWIN, vtinput.VK_RWIN,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return true
	}
	return false
}

// keyRepeatFires decides whether a key held for d ticks should deliver an
// event on this tick, given the loop's tick rate.
//
// Ebitengine reports only the transition into the pressed state, so a held key
// would otherwise fire exactly once. The X11 and Wayland backends get repeat
// from the display server; here it has to be synthesised, using the usual
// desktop feel of roughly half a second before the first repeat and about
// thirty a second after that. Deriving both from tps rather than hardcoding
// tick counts keeps the timing right if the loop rate ever changes.
func keyRepeatFires(d, tps int) bool {
	if d <= 0 {
		return false
	}
	if d == 1 {
		return true
	}
	if tps <= 0 {
		tps = 60
	}
	delay := tps / 2
	if delay < 1 {
		delay = 1
	}
	interval := tps / 30
	if interval < 1 {
		interval = 1
	}
	if d <= delay {
		return false
	}
	return (d-delay)%interval == 0
}
