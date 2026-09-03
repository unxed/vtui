package vtui

import "github.com/unxed/vtinput"

// isSpecialOrModifiedKey reports whether a key press should be delivered as a
// key event immediately, rather than held back to be paired with the text the
// platform may report for it.
//
// This deliberately carries no build tag. It is a pure function of a virtual
// key code and a modifier state -- no GPU, no FFI, nothing platform-specific
// -- but it has callers under three different sets of constraints: the gogpu
// host and the Ebitengine host, both limited to amd64/arm64, and the Win32 GUI
// backend, which is built for every Windows architecture. Living in
// gogpu_host.go left it undefined on windows/386, where the Win32 backend is
// built and the gogpu one is not, and that broke the build rather than
// degrading it.
func isSpecialOrModifiedKey(vk uint16, mods vtinput.ControlKeyState) bool {
	if (mods & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0 {
		return true
	}
	switch vk {
	case vtinput.VK_ESCAPE, vtinput.VK_RETURN, vtinput.VK_TAB, vtinput.VK_BACK, vtinput.VK_DELETE, vtinput.VK_INSERT,
		vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_LEFT, vtinput.VK_RIGHT,
		vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_PRIOR, vtinput.VK_NEXT,
		// Keypad 5 with NumLock off. It produces no character, so waiting for
		// text that never comes would only delay it by the pairing timeout.
		vtinput.VK_CLEAR,
		vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LWIN, vtinput.VK_RWIN, vtinput.VK_APPS,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return true
	}
	if vk >= vtinput.VK_F1 && vk <= vtinput.VK_F24 {
		return true
	}
	return false
}
