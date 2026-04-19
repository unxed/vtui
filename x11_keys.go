package vtui

import "github.com/unxed/vtinput"

// x11KeysymToVK мапит стандартные X11 Keysyms в Windows Virtual Key Codes.
var x11KeysymToVK = map[uint32]uint16{
	0xff08: vtinput.VK_BACK,
	0xff09: vtinput.VK_TAB,
	0xff0d: vtinput.VK_RETURN,
	0xff1b: vtinput.VK_ESCAPE,
	0xff50: vtinput.VK_HOME,
	0xff51: vtinput.VK_LEFT,
	0xff52: vtinput.VK_UP,
	0xff53: vtinput.VK_RIGHT,
	0xff54: vtinput.VK_DOWN,
	0xff55: vtinput.VK_PRIOR, // PgUp
	0xff56: vtinput.VK_NEXT,  // PgDn
	0xff57: vtinput.VK_END,
	0xff63: vtinput.VK_INSERT,
	0xffff: vtinput.VK_DELETE,
	0xffbe: vtinput.VK_F1,
	0xffbf: vtinput.VK_F2,
	0xffc0: vtinput.VK_F3,
	0xffc1: vtinput.VK_F4,
	0xffc2: vtinput.VK_F5,
	0xffc3: vtinput.VK_F6,
	0xffc4: vtinput.VK_F7,
	0xffc5: vtinput.VK_F8,
	0xffc6: vtinput.VK_F9,
	0xffc7: vtinput.VK_F10,
	0xffc8: vtinput.VK_F11,
	0xffc9: vtinput.VK_F12,
	0xffe1: vtinput.VK_SHIFT,   // Left Shift
	0xffe2: vtinput.VK_SHIFT,   // Right Shift
	0xffe3: vtinput.VK_CONTROL, // Left Ctrl
	0xffe4: vtinput.VK_CONTROL, // Right Ctrl
	0xffe9: vtinput.VK_MENU,    // Left Alt
	0xffea: vtinput.VK_MENU,    // Right Alt
	0x0020: vtinput.VK_SPACE,
}

func keysymToVK(keysym uint32) uint16 {
	// 1. Прямой маппинг спецклавиш
	if vk, ok := x11KeysymToVK[keysym]; ok {
		return vk
	}
	// 2. Цифры
	if keysym >= 0x0030 && keysym <= 0x0039 {
		return uint16(keysym)
	}
	// 3. Латиница (в Keysym латиница совпадает с ASCII)
	if keysym >= 0x0061 && keysym <= 0x007a { // a-z
		return uint16(keysym - 0x20) // to A-Z
	}
	if keysym >= 0x0041 && keysym <= 0x005a { // A-Z
		return uint16(keysym)
	}
	return 0
}