package vtui

import (
	"strings"
	"github.com/mattn/go-runewidth"
)

// Painter provides high-level drawing primitives on top of a ScreenBuf.
type Painter struct {
	scr *ScreenBuf
}

func NewPainter(scr *ScreenBuf) *Painter {
	return &Painter{scr: scr}
}

// Fill fills a rectangular area with a character and attributes.
func (p *Painter) Fill(x1, y1, x2, y2 int, char rune, attr uint64) {
	p.scr.FillRect(x1, y1, x2, y2, char, attr)
}

// DrawBox draws a frame of specified type (SingleBox, DoubleBox).
func (p *Painter) DrawBox(x1, y1, x2, y2 int, attr uint64, boxType int) {
	if boxType == NoBox { return }
	sym := getBoxSymbols(boxType)
	w := x2 - x1 + 1

	// Horizontal lines
	hLine := string(sym[bsH])
	top := string(sym[bsTL]) + strings.Repeat(hLine, w-2) + string(sym[bsTR])
	bottom := string(sym[bsBL]) + strings.Repeat(hLine, w-2) + string(sym[bsBR])

	p.scr.Write(x1, y1, StringToCharInfo(top, attr))
	p.scr.Write(x1, y2, StringToCharInfo(bottom, attr))

	// Vertical lines
	vSym := sym[bsV]
	for y := y1 + 1; y < y2; y++ {
		p.scr.Write(x1, y, []CharInfo{{Char: uint64(vSym), Attributes: attr}})
		p.scr.Write(x2, y, []CharInfo{{Char: uint64(vSym), Attributes: attr}})
	}
}

// DrawTitle draws a centered title on the top border of a box.
func (p *Painter) DrawTitle(x1, y1, x2 int, title string, attr uint64) {
	if title == "" { return }
	w := x2 - x1 + 1
	vLen := runewidth.StringWidth(title)

	if vLen > w-4 {
		title = runewidth.Truncate(title, w-4, "")
		vLen = runewidth.StringWidth(title)
	}

	titleStr := " " + title + " "
	vLen += 2
	start := (w - vLen) / 2
	p.scr.Write(x1+start, y1, StringToCharInfo(titleStr, attr))
}

// DrawCloseButton draws the [x] button on the top right.
func (p *Painter) DrawCloseButton(x2, y1 int, attr uint64) {
	closeStr := string(UIStrings.CloseBrackets[0]) + string(UIStrings.CloseSymbol) + string(UIStrings.CloseBrackets[1])
	p.scr.Write(x2-4, y1, StringToCharInfo(closeStr, attr))
}