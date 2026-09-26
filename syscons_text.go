package vtui

import (
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Text on FreeBSD syscons: one byte, one cell.
//
// sys/dev/syscons/scterm-teken.c switches its teken instance to 8-bit input
// unless the kernel is built with options TEKEN_UTF8 (GENERIC is not):
//
//	#ifndef TEKEN_UTF8
//		teken_set_8bit(&ts->ts_teken);
//	#endif
//
// and sys/teken/teken.c then hands each output byte to the character
// handler on its own, which gives it width 1:
//
//	if ((c & 0x80) == 0x00 || t->t_stateflags & TS_8BIT) {
//		/* One-byte sequence. */
//
// So a UTF-8 sequence covers as many cells as it has bytes and shows whatever
// glyphs the loaded font has at those byte values: U+2502 is E2 94 82, which
// is "Γöé" with the cp437 font and "Ôöé" with cp850 (unxed/f4#91). Box
// drawing is on cellAdvanceTrusted's list, so the rest of such a row was
// written two cells to the right of where the shadow buffer has it, and a
// glyph in the last column wrapped its tail onto the next row. Runes outside
// that list got a cursor resync, which painted over the tail and left only the
// lead byte: the "Ô" standing in for the sort arrow and the symlink marker.
// vt(4) never calls teken_set_8bit and is not affected.
//
// The font is a bare 256-glyph bitmap loaded by vidcontrol -f; nothing tells
// which code page it follows (cp437, cp850, koi8-r, iso8859-*...), and vtui
// writes UTF-8 whatever the locale says. The one output that is right for
// every font is printable ASCII, so on syscons every cell is written as
// exactly one ASCII byte, or two for a double-width cell. The box drawing and
// scrollbar fallbacks in symbols.go and scrollbar.go already do this for the
// glyphs vtui draws itself; this covers everything else that reaches the
// screen, including glyphs applications put into cells directly.

// boxDrawingASCII maps U+2500..U+257F: horizontal lines to '-', the double
// horizontal line to '=', vertical lines to '|', and every corner, tee, cross
// and arc to '+', matching the boxSymbols fallback.
const boxDrawingASCII = "" +
	"--||--||--||++++" + // U+2500 ─━│┃┄┅┆┇┈┉┊┋┌┍┎┏
	"++++++++++++++++" + // U+2510 ┐┑┒┓└┕┖┗┘┙┚┛├┝┞┟
	"++++++++++++++++" + // U+2520 ┠┡┢┣┤┥┦┧┨┩┪┫┬┭┮┯
	"++++++++++++++++" + // U+2530 ┰┱┲┳┴┵┶┷┸┹┺┻┼┽┾┿
	"++++++++++++--||" + // U+2540 ╀╁╂╃╄╅╆╇╈╉╊╋╌╍╎╏
	"=|++++++++++++++" + // U+2550 ═║╒╓╔╕╖╗╘╙╚╛╜╝╞╟
	"++++++++++++++++" + // U+2560 ╠╡╢╣╤╥╦╧╨╩╪╫╬╭╮╯
	"+/\\X-|-|-|-|-|-|" //  U+2570 ╰╱╲╳╴╵╶╷╸╹╺╻╼╽╾╿

// asciiStandIns covers the non-letter glyphs vtui and its applications use
// that neither the tables above nor a compatibility decomposition reduce to
// ASCII.
var asciiStandIns = map[rune]byte{
	'←': '<', '↑': '^', '→': '>', '↓': 'v', '↔': '-', '↕': '|',
	'▲': '^', '▴': '^', '►': '>', '▶': '>', '▸': '>',
	'▼': 'v', '▾': 'v', '◄': '<', '◀': '<', '◂': '<',
	'■': '#', '▣': '#', '●': '*', '◆': '*', '•': '*', '·': '.', '∙': '.',
	'√': '*', '✓': '*', '✔': '*', '×': 'x', '≡': '=', '°': 'o',
	'–': '-', '—': '-', '―': '-', '−': '-',
	'‘': '\'', '’': '\'', '‚': ',', '“': '"', '”': '"', '„': '"',
	'«': '"', '»': '"', '‹': '<', '›': '>',
	'©': 'c', '®': 'r',
	'ø': 'o', 'Ø': 'O', 'đ': 'd', 'Đ': 'D', 'ł': 'l', 'Ł': 'L', 'ı': 'i', 'ß': 's',
}

// asciiStandIn returns the printable ASCII byte syscons shows for r.
func asciiStandIn(r rune) byte {
	switch {
	case r >= 0x20 && r < 0x7F:
		return byte(r)
	case r >= 0x2500 && r <= 0x257F:
		return boxDrawingASCII[r-0x2500]
	case r == '░':
		return '.'
	case r == '▒':
		return ':'
	case r >= 0x2580 && r <= 0x259F: // the other block elements
		return '#'
	case r >= 0x2800 && r <= 0x28FF: // braille, used for spinners
		return '*'
	}
	if b, ok := asciiStandIns[r]; ok {
		return b
	}
	// A compatibility decomposition takes care of accented Latin letters
	// (é -> e), the no-break and typographic spaces, the ellipsis, ligatures
	// and full-width forms.
	if d := norm.NFKD.String(string(r)); d != "" && d[0] >= 0x20 && d[0] < 0x7F {
		return d[0]
	}
	return '?'
}

// sysconsCell writes the cell ch as syscons must receive it: one ASCII byte,
// and a second one when the cell is double width, so that the terminal cursor
// ends up exactly where the renderer expects it.
func sysconsCell(out *byteBuffer, ch uint64, wide bool) {
	r := rune(ch)
	if IsCompChar(ch) {
		// A grapheme cluster: the base character stands for all of it.
		r, _ = utf8.DecodeRuneInString(CellString(ch))
	} else if IsSymChar(ch) {
		// A checkbox/radio token: stand in for its classic literal rune.
		r = CellBaseRune(ch)
	}
	b := asciiStandIn(r)
	out.WriteByte(b)
	if wide {
		second := byte(' ')
		if b == '?' {
			second = '?'
		}
		out.WriteByte(second)
	}
}
