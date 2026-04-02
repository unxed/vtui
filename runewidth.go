package vtui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// WideCharFiller is a special marker indicating that this cell in ScreenBuf
// is occupied by the right half of a full-width character (like CJK or Emoji).
const WideCharFiller = ^uint64(0)

// ExtractHotkey quickly finds the hotkey rune in a string without allocating memory.
func ExtractHotkey(s string) rune {
	idx := 0
	for {
		i := strings.IndexByte(s[idx:], '&')
		if i == -1 {
			return 0
		}
		idx += i
		if idx+1 < len(s) {
			if s[idx+1] == '&' {
				idx += 2
				continue
			}
			r, _ := utf8.DecodeRuneInString(s[idx+1:])
			return unicode.ToLower(r)
		}
		return 0
	}
}
// StringToCharInfo converts a string into a slice of CharInfo cells,
// correctly handling double-width characters by inserting WideCharFillers.
// It currently ignores zero-width characters to keep cell alignment strict.
// ParseAmpersandString parses a string with ampersands, removes utility &,
// processes && as &, and returns the clean string, the hotkey, and its position (in runes).
func ParseAmpersandString(s string) (clean string, hotkey rune, hotkeyPos int) {
	if s == "" || strings.IndexByte(s, '&') == -1 {
		return s, 0, -1
	}

	var sb strings.Builder
	hotkeyPos = -1
	runeCount := 0
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		if runes[i] == '&' && i+1 < len(runes) {
			if runes[i+1] == '&' {
				sb.WriteRune('&')
				runeCount++
				i++
			} else {
				if hotkeyPos == -1 {
					hotkey = unicode.ToLower(runes[i+1])
					hotkeyPos = runeCount
				}
				sb.WriteRune(runes[i+1])
				runeCount++
				i++
			}
		} else {
			sb.WriteRune(runes[i])
			runeCount++
		}
	}
	return sb.String(), hotkey, hotkeyPos
}

// StringToCharInfoHighlighted works like StringToCharInfo but highlights the letter after &.
func StringToCharInfoHighlighted(s string, normalAttr, highAttr uint64) ([]CharInfo, rune) {
	clean, hk, hkPos := ParseAmpersandString(s)
	var res []CharInfo
	currRuneIdx := 0

	for _, r := range clean {
		attr := normalAttr
		if currRuneIdx == hkPos {
			attr = highAttr
		}
		w := runewidth.RuneWidth(r)
		if w > 0 {
			res = append(res, CharInfo{Char: uint64(r), Attributes: attr})
			for j := 1; j < w; j++ {
				res = append(res, CharInfo{Char: WideCharFiller, Attributes: attr})
			}
		}
		currRuneIdx++
	}
	return res, hk
}

// SanitizeRune ensures the rune is printable and handles its visual width.
func SanitizeRune(r rune) (rune, int) {
	if r == '\n' || r == '\r' {
		return 0, 0
	}
	if r < 0x20 || r == 0x7F {
		return '·', 1
	}
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return r, 1 // Visible placeholder for zero-width/invalid
	}
	return r, w
}

func StringToCharInfo(s string, attr uint64) []CharInfo {
	return FillCharInfo(nil, []byte(s), attr)
}

func FillCharInfo(target []CharInfo, data []byte, attr uint64) []CharInfo {
	target = target[:0]
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		data = data[size:]

		sr, w := SanitizeRune(r)
		if w > 0 {
			target = append(target, CharInfo{Char: uint64(sr), Attributes: attr})
			for i := 1; i < w; i++ {
				target = append(target, CharInfo{Char: WideCharFiller, Attributes: attr})
			}
		}
	}
	return target
}

// FillCharInfoWithSelection combines FillCharInfo and selection highlighting in a single pass.
func FillCharInfoWithSelection(target []CharInfo, data []byte, defaultAttr, selAttr uint64, fragStartOffset, selMin, selMax int) []CharInfo {
	target = target[:0]
	currByte := 0
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		data = data[size:]

		sr, w := SanitizeRune(r)
		attr := defaultAttr
		absPos := fragStartOffset + currByte
		if absPos >= selMin && absPos < selMax {
			attr = selAttr
		}
		currByte += size

		if w > 0 {
			target = append(target, CharInfo{Char: uint64(sr), Attributes: attr})
			for i := 1; i < w; i++ {
				target = append(target, CharInfo{Char: WideCharFiller, Attributes: attr})
			}
		}
	}
	return target
}

func RunesToCharInfo(runes []rune, attr uint64) []CharInfo {
	return StringToCharInfo(string(runes), attr)
}
