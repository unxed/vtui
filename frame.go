package vtui

import (
	"strings"
)

// BorderedFrame represents a frame container that can have a title.
// It embeds ScreenObject for position and visibility management.
type BorderedFrame struct {
	ScreenObject
	title      string
	boxType       int
	ColorBoxIdx   int
	ColorTitleIdx int
}

// NewBorderedFrame creates a new BorderedFrame instance.
func NewBorderedFrame(x1, y1, x2, y2 int, boxType int, title string) *BorderedFrame {
	f := &BorderedFrame{
		title:         title,
		boxType:       boxType,
		ColorBoxIdx:   ColDialogBox,
		ColorTitleIdx: ColDialogBoxTitle,
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

	sym := getBoxSymbols(f.boxType)
	w := f.X2 - f.X1 + 1

	// Clearing background inside the frame (optional but useful)
	// scr.FillRect(f.X1+1, f.Y1+1, f.X2-1, f.Y2-1, ' ', f.borderColor)

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
	titleRunes := []rune(f.title)

	colBox := Palette[f.ColorBoxIdx]
	colTitle := Palette[f.ColorTitleIdx]

	if len(titleRunes) > 0 {
		if len(titleRunes) > w-4 {
			titleRunes = titleRunes[:w-4]
		}
		start := (w - len(titleRunes)) / 2
		// Draw frame and title in a single line for coordinate stability
		titleStr := " " + string(titleRunes) + " "
		fullTopLine := topRunes
		copy(fullTopLine[start-1:], []rune(titleStr))

		// Write the entire line with the border color
		scr.Write(f.X1, f.Y1, RunesToCharInfo(fullTopLine, colBox))
		// Overlay the color only on the title text
		scr.Write(f.X1+start-1, f.Y1, RunesToCharInfo([]rune(titleStr), colTitle))
	} else {
		scr.Write(f.X1, f.Y1, RunesToCharInfo(topRunes, colBox))
	}
	scr.Write(f.X1, f.Y2, strToCharInfo(bottomBorder.String(), colBox, 0, ""))

	// Vertical lines
	vertLine := []CharInfo{{Char: uint64(sym[bsV]), Attributes: colBox}}
	for y := f.Y1 + 1; y < f.Y2; y++ {
		scr.Write(f.X1, y, vertLine)
		scr.Write(f.X2, y, vertLine)
	}
}

// Helper function to convert a string to []CharInfo considering the title color.
func strToCharInfo(str string, borderColor, titleColor uint64, title string) []CharInfo {
	runes := []rune(str)
	info := make([]CharInfo, len(runes))

	titleStart := -1
	if title != "" {
		titleStart = strings.Index(str, title)
	}

	for i, r := range runes {
		isTitle := titleStart != -1 && i >= titleStart && i < titleStart+len(title)
		info[i].Char = uint64(r)
		if isTitle {
			info[i].Attributes = titleColor
		} else {
			info[i].Attributes = borderColor
		}
	}
	return info
}