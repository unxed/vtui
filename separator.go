package vtui

// Separator represents a horizontal line used to divide sections in a dialog.
type Separator struct {
	ScreenObject
	ConnectLeft  bool
	ConnectRight bool
}

func NewSeparator(x, y, w int, connectLeft, connectRight bool) *Separator {
	s := &Separator{ConnectLeft: connectLeft, ConnectRight: connectRight}
	s.SetPosition(x, y, x+w-1, y)
	return s
}

func (s *Separator) Show(scr *ScreenBuf) {
	s.ScreenObject.Show(scr)
	s.DisplayObject(scr)
}

func (s *Separator) DisplayObject(scr *ScreenBuf) {
	if !s.IsVisible() { return }
	attr := Palette[ColDialogBox]

	// Draw horizontal line
	sym := getBoxSymbols(SingleBox)
	line := make([]CharInfo, s.X2-s.X1+1)
	for i := range line {
		line[i] = CharInfo{Char: uint64(sym[bsH]), Attributes: attr}
	}

	// Add connectors if explicitly requested
	if s.ConnectLeft {
		line[0] = CharInfo{Char: uint64(boxSymbols[26]), Attributes: attr}
	}
	if s.ConnectRight {
		line[len(line)-1] = CharInfo{Char: uint64(boxSymbols[27]), Attributes: attr}
	}
	
	scr.Write(s.X1, s.Y1, line)
}

func (s *Separator) CanFocus() bool { return false }