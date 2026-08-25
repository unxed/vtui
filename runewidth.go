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
	res := make([]CharInfo, 0, len(clean))
	ForEachVisualCluster(clean, func(cluster string, w, _, runeIdx int) {
		attr := normalAttr
		if hkPos >= runeIdx && hkPos < runeIdx+utf8.RuneCountInString(cluster) {
			attr = highAttr
		}
		res = AppendCluster(res, cluster, w, attr)
	})
	return res, hk
}

// SanitizeRune ensures the rune is printable and handles its visual width.
// It looks at one rune in isolation, so a combining mark reaching it has
// already been separated from the character it belongs to and can only be
// shown as a placeholder. Code that has the surrounding text should call
// SanitizeCluster instead.
func SanitizeRune(r rune) (rune, int) {
	if r == '\n' || r == '\r' {
		return 0, 0
	}
	if r == '\uFFFD' {
		return '?', 1
	}
	if isControlRune(r) {
		return '·', 1
	}
	w := runewidth.RuneWidth(r)
	if w <= 0 {
		return '·', 1 // Visible placeholder for zero-width/invalid
	}
	return r, w
}

func StringToCharInfo(s string, attr uint64) []CharInfo {
	s = VisualString(s)
	res := make([]CharInfo, 0, len(s))
	forEachDisplayCluster(s, func(cluster string, w, _, _ int) {
		res = AppendCluster(res, cluster, w, attr)
	})
	return res
}

func FillCharInfo(target []CharInfo, data []byte, attr uint64) []CharInfo {
	target = target[:0]
	forEachDisplayCluster(string(data), func(cluster string, w, _, _ int) {
		target = AppendCluster(target, cluster, w, attr)
	})
	return target
}

// FillCharInfoString fills target with CharInfo for s with attr, reusing target capacity.
func FillCharInfoString(target []CharInfo, s string, attr uint64) []CharInfo {
	s = VisualString(s)
	target = target[:0]
	forEachDisplayCluster(s, func(cluster string, w, _, _ int) {
		target = AppendCluster(target, cluster, w, attr)
	})
	return target
}

// FillCharInfoWithSelection combines FillCharInfo and selection highlighting in a single pass.
// Selection bounds are byte offsets into the whole line; a cluster is selected
// when the byte its first rune starts at falls inside them.
func FillCharInfoWithSelection(target []CharInfo, data []byte, defaultAttr, selAttr uint64, fragStartOffset, selMin, selMax int) []CharInfo {
	target = target[:0]
	forEachDisplayCluster(string(data), func(cluster string, w, offset, _ int) {
		attr := defaultAttr
		absPos := fragStartOffset + offset
		if absPos >= selMin && absPos < selMax {
			attr = selAttr
		}
		target = AppendCluster(target, cluster, w, attr)
	})
	return target
}

func RunesToCharInfo(runes []rune, attr uint64) []CharInfo {
	return StringToCharInfo(string(runes), attr)
}

// FillCharInfoAligned fills target with CharInfo for s with width and alignment under attr.
func FillCharInfoAligned(target []CharInfo, text string, width int, align Alignment, attr uint64) []CharInfo {
	if width <= 0 {
		return target[:0]
	}
	isASCII := true
	for i := 0; i < len(text); i++ {
		if text[i] >= 0x80 || text[i] < 0x20 {
			isASCII = false
			break
		}
	}
	if isASCII {
		vLen := len(text)
		if vLen > width {
			vLen = width
			text = text[:width]
		}
		space := width - vLen
		var leftSpace, rightSpace int
		switch align {
		case AlignLeft:
			rightSpace = space
		case AlignRight:
			leftSpace = space
		case AlignCenter:
			leftSpace = space / 2
			rightSpace = space - leftSpace
		}
		if cap(target) < width {
			target = make([]CharInfo, width)
		} else {
			target = target[:width]
		}
		idx := 0
		for i := 0; i < leftSpace; i++ {
			target[idx] = CharInfo{Char: ' ', Attributes: attr}
			idx++
		}
		for i := 0; i < vLen; i++ {
			target[idx] = CharInfo{Char: uint64(text[i]), Attributes: attr}
			idx++
		}
		for i := 0; i < rightSpace; i++ {
			target[idx] = CharInfo{Char: ' ', Attributes: attr}
			idx++
		}
		return target
	}

	truncated, vLen := truncateStringWidth(text, width, "")
	if vLen >= width {
		return FillCharInfoString(target, truncated, attr)
	}

	space := width - vLen
	var leftSpace, rightSpace int
	switch align {
	case AlignLeft:
		rightSpace = space
	case AlignRight:
		leftSpace = space
	case AlignCenter:
		leftSpace = space / 2
		rightSpace = space - leftSpace
	}

	target = target[:0]
	for i := 0; i < leftSpace; i++ {
		target = append(target, CharInfo{Char: ' ', Attributes: attr})
	}
	s := VisualString(truncated)
	forEachDisplayCluster(s, func(cluster string, w, _, _ int) {
		target = AppendCluster(target, cluster, w, attr)
	})
	for i := 0; i < rightSpace; i++ {
		target = append(target, CharInfo{Char: ' ', Attributes: attr})
	}
	return target
}
