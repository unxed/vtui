package vtui

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// WideCharFiller is a special marker indicating that this cell in ScreenBuf
// is occupied by the right half of a full-width character (like CJK or Emoji).
const WideCharFiller = ^uint64(0)

// StringToCharInfo converts a string into a slice of CharInfo cells,
// correctly handling double-width characters by inserting WideCharFillers.
// It currently ignores zero-width characters to keep cell alignment strict.
// ParseAmpersandString парсит строку с амперсандами, удаляет служебные &,
// обрабатывает && как & и возвращает чистую строку, хоткей и его позицию (в рунах).
func ParseAmpersandString(s string) (clean string, hotkey rune, hotkeyPos int) {
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

// StringToCharInfoHighlighted работает как StringToCharInfo, но подсвечивает букву после &.
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

func StringToCharInfo(s string, attr uint64) []CharInfo {
	var res []CharInfo
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if w > 0 {
			res = append(res, CharInfo{Char: uint64(r), Attributes: attr})
			// Fill the extra cells required by the wide character
			for i := 1; i < w; i++ {
				res = append(res, CharInfo{Char: WideCharFiller, Attributes: attr})
			}
		}
	}
	return res
}

func RunesToCharInfo(runes []rune, attr uint64) []CharInfo {
	return StringToCharInfo(string(runes), attr)
}
