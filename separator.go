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
	p := NewPainter(scr)
	attr := Palette[ColDialogBox]

	p.DrawLine(s.X1, s.Y1, s.X2, s.Y1, boxSymbols[bsH], attr, s.ConnectLeft, s.ConnectRight)
}

