package vtui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// BorderedFrame represents a frame container that can have a title.
// It embeds ScreenObject for position and visibility management.
type BorderedFrame struct {
	ScreenObject
	title              string
	boxType            int
	ColorBoxIdx        int
	ColorTitleIdx      int
	ColorBackgroundIdx int
}

// NewBorderedFrame creates a new BorderedFrame instance.
func NewBorderedFrame(x1, y1, x2, y2 int, boxType int, title string) *BorderedFrame {
	f := &BorderedFrame{
		title:              title,
		boxType:            boxType,
		ColorBoxIdx:        ColDialogBox,
		ColorTitleIdx:      ColDialogBoxTitle,
		ColorBackgroundIdx: ColDialogText,
	}
	f.SetPosition(x1, y1, x2, y2)
	return f
}

// SetTitle sets the title for the frame.
func (f *BorderedFrame) SetTitle(title string) {
	f.title = title
}

// Show saves the background and calls the object's drawing method.
func (f *BorderedFrame) Show(scr *ScreenBuf) {
	if f.IsLocked() {
		return
	}
	f.ScreenObject.Show(scr) // Call embedded structure method
	f.DisplayObject(scr)
}

// DisplayObject renders the frame and title into ScreenBuf.
func (f *BorderedFrame) DisplayObject(scr *ScreenBuf) {
	if f.boxType == NoBox {
		return
	}

	// Сначала закрашиваем всю область фона
	scr.FillRect(f.X1, f.Y1, f.X2, f.Y2, ' ', Palette[f.ColorBackgroundIdx])

	sym := getBoxSymbols(f.boxType)
	w := f.X2 - f.X1 + 1

	// Top and bottom borders
	var topBorder, bottomBorder strings.Builder
	topBorder.WriteRune(sym[bsTL])
	bottomBorder.WriteRune(sym[bsBL])
	for i := 0; i < w-2; i++ {
		topBorder.WriteRune(sym[bsH])
		bottomBorder.WriteRune(sym[bsH])
	}
	topBorder.WriteRune(sym[bsTR])
	bottomBorder.WriteRune(sym[bsBR])

	// Rendering the top border with title
	topRunes := []rune(topBorder.String())
	colBox := Palette[f.ColorBoxIdx]
	colTitle := Palette[f.ColorTitleIdx]
	
	vLen := runewidth.StringWidth(f.title)
	if vLen > 0 {
		titleStr := f.title
		if vLen > w-4 {
			titleStr = runewidth.Truncate(titleStr, w-4, "")
			vLen = runewidth.StringWidth(titleStr)
		}

		titleStr = " " + titleStr + " "
		vLen += 2 // spaces

		start := (w - vLen) / 2

		// First draw full top border
		scr.Write(f.X1, f.Y1, RunesToCharInfo(topRunes, colBox))
		// Then overwrite center with title
		scr.Write(f.X1+start, f.Y1, StringToCharInfo(titleStr, colTitle))
	} else {
		scr.Write(f.X1, f.Y1, RunesToCharInfo(topRunes, colBox))
	}
	scr.Write(f.X1, f.Y2, StringToCharInfo(bottomBorder.String(), colBox))

	// Vertical lines
	vertLine := []CharInfo{{Char: uint64(sym[bsV]), Attributes: colBox}}
	for y := f.Y1 + 1; y < f.Y2; y++ {
		scr.Write(f.X1, y, vertLine)
		scr.Write(f.X2, y, vertLine)
	}
}