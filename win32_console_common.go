package vtui

// Win32 Console color attribute flags (WinCompat/Windows Console API).
const (
	Win32FgBlue      uint16 = 0x0001
	Win32FgGreen     uint16 = 0x0002
	Win32FgRed       uint16 = 0x0004
	Win32FgIntensity uint16 = 0x0008

	Win32BgBlue      uint16 = 0x0010
	Win32BgGreen     uint16 = 0x0020
	Win32BgRed       uint16 = 0x0040
	Win32BgIntensity uint16 = 0x0080

	Win32CommonLvbLeadingByte    uint16 = 0x0100
	Win32CommonLvbTrailingByte   uint16 = 0x0200
	Win32CommonLvbGridHorizontal uint16 = 0x0400
	Win32CommonLvbGridLVertical  uint16 = 0x0800
	Win32CommonLvbGridRVertical  uint16 = 0x1000
	Win32CommonLvbReverseVideo   uint16 = 0x4000
	Win32CommonLvbUnderscore     uint16 = 0x8000
)

// win32Coord and win32SmallRect aliases for Win32 Console API structures.
type win32Coord = Coord
type win32SmallRect = SmallRect

// ansiToWin32ColorMap maps 0..15 ANSI/XTerm palette index to 0..15 Win32 IRGB attribute.
var ansiToWin32ColorMap = [16]uint16{
	0,                                       // 0: Black
	Win32FgRed,                              // 1: Red
	Win32FgGreen,                            // 2: Green
	Win32FgRed | Win32FgGreen,               // 3: Yellow (Brown)
	Win32FgBlue,                             // 4: Blue
	Win32FgRed | Win32FgBlue,                // 5: Magenta
	Win32FgGreen | Win32FgBlue,              // 6: Cyan
	Win32FgRed | Win32FgGreen | Win32FgBlue, // 7: Light Gray
	Win32FgIntensity,                        // 8: Dark Gray
	Win32FgIntensity | Win32FgRed,           // 9: Bright Red
	Win32FgIntensity | Win32FgGreen,         // 10: Bright Green
	Win32FgIntensity | Win32FgRed | Win32FgGreen,               // 11: Bright Yellow
	Win32FgIntensity | Win32FgBlue,                             // 12: Bright Blue
	Win32FgIntensity | Win32FgRed | Win32FgBlue,                // 13: Bright Magenta
	Win32FgIntensity | Win32FgGreen | Win32FgBlue,              // 14: Bright Cyan
	Win32FgIntensity | Win32FgRed | Win32FgGreen | Win32FgBlue, // 15: Bright White
}

// AnsiIndexToWin32Color converts an ANSI 16-color index (0-15) to Win32 Console IRGB attribute.
func AnsiIndexToWin32Color(idx uint8) uint16 {
	return ansiToWin32ColorMap[idx&15]
}

// AttrToWin32Attr exports attrToWin32Attr for renderers outside this package
// that need to paint using the Windows Console API directly (e.g. f4's
// console-view popup overlay, which draws with WriteConsoleOutputW instead
// of going through ScreenBuf) and want colors that match the active theme
// instead of hardcoded attribute bytes.
func AttrToWin32Attr(attr uint64, activePal *[256]uint32) uint16 {
	return attrToWin32Attr(attr, activePal)
}

// attrToWin32Attr maps 64-bit CharInfo attributes to 16-bit Win32 Console attributes.
func attrToWin32Attr(attr uint64, activePal *[256]uint32) uint16 {
	var fgIdx uint8
	if attr&IsFgRGB != 0 {
		fgIdx = findNearestColor(GetRGBFore(attr), activePal, 16)
	} else {
		idx := GetIndexFore(attr)
		if idx >= 16 && activePal != nil {
			fgIdx = findNearestColor(activePal[idx], activePal, 16)
		} else {
			fgIdx = idx & 15
		}
	}
	win32Fg := AnsiIndexToWin32Color(fgIdx)

	var bgIdx uint8
	if attr&IsBgRGB != 0 {
		bgIdx = findNearestColor(GetRGBBack(attr), activePal, 16)
	} else {
		idx := GetIndexBack(attr)
		if idx >= 16 && activePal != nil {
			bgIdx = findNearestColor(activePal[idx], activePal, 16)
		} else {
			bgIdx = idx & 15
		}
	}
	win32Bg := AnsiIndexToWin32Color(bgIdx) << 4

	res := win32Fg | win32Bg

	if attr&ForegroundIntensity != 0 {
		res |= Win32FgIntensity
	}
	if attr&CommonLvbUnderscore != 0 {
		res |= Win32CommonLvbUnderscore
	}
	if attr&CommonLvbReverse != 0 {
		res |= Win32CommonLvbReverseVideo
	}

	// Workaround for Wine conhost bug (conhost.c set_tty_attr):
	// Wine emits "\x1b[m" when FG is 7 (Light Gray), which inadvertently
	// resets the terminal background to black. If the cell has a non-black
	// background, promote FG from 7 (Light Gray) to 15 (Bright White) so Wine
	// emits "\x1b[97m" instead of "\x1b[m", keeping the background intact.
	if isWineOS() && (res&0x00F0) != 0 && (res&0x000F) == (Win32FgRed|Win32FgGreen|Win32FgBlue) {
		res |= Win32FgIntensity
	}

	return res
}

// win32CharInfo represents a native Win32 Console CHAR_INFO struct.
type win32CharInfo struct {
	UnicodeChar uint16
	Attributes  uint16
}

func charInfoToWin32(ci CharInfo, activePal *[256]uint32) win32CharInfo {
	var uc uint16
	if ci.Char == 0 || ci.Char == WideCharFiller {
		uc = ' '
	} else if IsCompChar(ci.Char) || IsSymChar(ci.Char) {
		uc = uint16(CellBaseRune(ci.Char))
	} else if ci.Char < 0x10000 {
		uc = uint16(ci.Char)
	} else {
		uc = '?'
	}

	return win32CharInfo{
		UnicodeChar: uc,
		Attributes:  attrToWin32Attr(ci.Attributes, activePal),
	}
}

// IsWine reports whether the current process is running under Wine.
func IsWine() bool {
	return isWineOS()
}

// DefaultConsoleBackend returns the default console backend name ("winapi" or "ansi").
// Under Wine and legacy Windows (Windows 7/8/8.1 without VT support):
// - If running inside a Win32 Console, it defaults to "winapi".
// - If running directly from a terminal with VT processing support, it defaults to "ansi".
func DefaultConsoleBackend() string {
	if isWineOS() {
		if hasConsoleBufferOS() {
			return "winapi"
		}
		return "ansi"
	}
	if hasConsoleBufferOS() && !hasVTConsoleSupportOS() {
		return "winapi"
	}
	return "ansi"
}
