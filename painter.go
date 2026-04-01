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

// DrawString draws a raw string with given attributes.
func (p *Painter) DrawString(x, y int, text string, attr uint64) {
	p.scr.Write(x, y, StringToCharInfo(text, attr))
}

// DrawHighlightedText draws a pre-parsed string with a specific hotkey position.
func (p *Painter) DrawHighlightedText(x, y int, cleanText string, hkPos int, normAttr, highAttr uint64) {
	cells := make([]CharInfo, 0, len(cleanText))
	currRuneIdx := 0
	for _, r := range cleanText {
		attr := normAttr
		if currRuneIdx == hkPos {
			attr = highAttr
		}

		sr, w := SanitizeRune(r)
		cells = append(cells, CharInfo{Char: uint64(sr), Attributes: attr})
		for j := 1; j < w; j++ {
			cells = append(cells, CharInfo{Char: WideCharFiller, Attributes: attr})
		}
		currRuneIdx++
	}
	p.scr.Write(x, y, cells)
}
// DrawStringHighlighted draws a string, highlighting the character after the '&' symbol.
// This is used for dynamic strings that are not stored in a ScreenObject.
func (p *Painter) DrawStringHighlighted(x, y int, text string, normAttr, highAttr uint64) {
	cells, _ := StringToCharInfoHighlighted(text, normAttr, highAttr)
	p.scr.Write(x, y, cells)
}
// DrawLine draws a horizontal line segment, optionally with connectors.
func (p *Painter) DrawLine(x1, y1, x2, y2 int, char rune, attr uint64, connectLeft, connectRight bool) {
	if x1 > x2 || y1 > y2 { return } // Only horizontal for now

	lineRunes := make([]rune, x2-x1+1)
	for i := range lineRunes {
		lineRunes[i] = char
	}

	if connectLeft { lineRunes[0] = boxSymbols[bsVMenuHCrossLeft] }
	if connectRight { lineRunes[len(lineRunes)-1] = boxSymbols[bsVMenuHCrossRight] }

	p.scr.Write(x1, y1, RunesToCharInfo(lineRunes, attr))
}
