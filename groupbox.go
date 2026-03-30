package vtui

// GroupBox is a decorative titled frame used to visually group elements.
type GroupBox struct {
	ScreenObject
	Title string
}

func NewGroupBox(x1, y1, x2, y2 int, title string) *GroupBox {
	gb := &GroupBox{Title: title}
	gb.SetPosition(x1, y1, x2, y2)
	return gb
}

func (gb *GroupBox) Show(scr *ScreenBuf) {
	gb.ScreenObject.Show(scr)
	gb.DisplayObject(scr)
}

func (gb *GroupBox) DisplayObject(scr *ScreenBuf) {
	if !gb.IsVisible() { return }
	attr := Palette[ColDialogBox]
	sym := getBoxSymbols(SingleBox)

	w := gb.X2 - gb.X1 + 1

	// Top with Title
	topLine := make([]CharInfo, w)
	for i := range topLine { topLine[i] = CharInfo{Char: uint64(sym[bsH]), Attributes: attr} }
	topLine[0] = CharInfo{Char: uint64(sym[bsTL]), Attributes: attr}
	topLine[w-1] = CharInfo{Char: uint64(sym[bsTR]), Attributes: attr}

	scr.Write(gb.X1, gb.Y1, topLine)

	if gb.Title != "" {
		tStr := " " + gb.Title + " "
		// Use highlight color for GroupBox titles to make them stand out
		scr.Write(gb.X1+2, gb.Y1, StringToCharInfo(tStr, Palette[ColDialogHighlightText]))
	}

	// Bottom
	botLine := make([]CharInfo, w)
	for i := range botLine { botLine[i] = CharInfo{Char: uint64(sym[bsH]), Attributes: attr} }
	botLine[0] = CharInfo{Char: uint64(sym[bsBL]), Attributes: attr}
	botLine[w-1] = CharInfo{Char: uint64(sym[bsBR]), Attributes: attr}
	scr.Write(gb.X1, gb.Y2, botLine)

	// Sides
	side := []CharInfo{{Char: uint64(sym[bsV]), Attributes: attr}}
	for y := gb.Y1 + 1; y < gb.Y2; y++ {
		scr.Write(gb.X1, y, side)
		scr.Write(gb.X2, y, side)
	}
}

func (gb *GroupBox) CanFocus() bool { return false }