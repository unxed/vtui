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

// StringToCharInfo converts a string into a slice of CharInfo cells,
// correctly handling double-width characters by inserting WideCharFillers.
// It currently ignores zero-width characters to keep cell alignment strict.
// ParseAmpersandString parses a string with ampersands, removes utility &,
// processes && as &, and returns the clean string, the hotkey, and its position (in runes).
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

func StringToCharInfo(s string, attr uint64) []CharInfo {
	var res []CharInfo
	res = FillCharInfo(res, []byte(s), attr)
	return res
}

// FillCharInfo fills a slice of CharInfo with data from a byte slice without extra allocations.
// It grows the target slice if necessary.
func FillCharInfo(target []CharInfo, data []byte, attr uint64) []CharInfo {
	target = target[:0]
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		data = data[size:]

		// Sanitize control characters to prevent terminal cursor corruption
		if r < 0x20 || r == 0x7F {
			r = '·' // Use middle dot for control chars
		}

		w := 1
		if r >= 0x7F {
			w = runewidth.RuneWidth(r)
		}
		if w <= 0 {
			w = 1 // Ensure even zero-width chars are visible in binary files
		}
		target = append(target, CharInfo{Char: uint64(r), Attributes: attr})
		for i := 1; i < w; i++ {
			target = append(target, CharInfo{Char: WideCharFiller, Attributes: attr})
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

		// Sanitize control characters
		if r < 0x20 || r == 0x7F {
			r = '·'
		}

		attr := defaultAttr
		absPos := fragStartOffset + currByte
		if absPos >= selMin && absPos < selMax {
			attr = selAttr
		}
		currByte += size

		w := 1
		if r >= 0x7F {
			w = runewidth.RuneWidth(r)
		}
		if w <= 0 {
			w = 1
		}
		target = append(target, CharInfo{Char: uint64(r), Attributes: attr})
		for i := 1; i < w; i++ {
			target = append(target, CharInfo{Char: WideCharFiller, Attributes: attr})
		}
	}
	return target
}

func RunesToCharInfo(runes []rune, attr uint64) []CharInfo {
	return StringToCharInfo(string(runes), attr)
}
