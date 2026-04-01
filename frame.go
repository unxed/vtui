package vtui

// BorderedFrame represents a frame container that can have a title.
// It embeds ScreenObject for position and visibility management.
type BorderedFrame struct {
	ScreenObject
	title              string
	boxType            int
	ColorBoxIdx        int
	ColorTitleIdx      int
	ColorBackgroundIdx int
	ShowClose          bool
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
// IsBorderClick returns true if the coordinates hit the frame border.
func (f *BorderedFrame) IsBorderClick(x, y int) bool {
	if f.boxType == NoBox {
		return false
	}
	// Check if click is on any of the four borders
	onHoriz := (y == f.Y1 || y == f.Y2) && (x >= f.X1 && x <= f.X2)
	onVert := (x == f.X1 || x == f.X2) && (y >= f.Y1 && y <= f.Y2)
	return onHoriz || onVert
}

// Show saves the background and calls the object's drawing method.
func (f *BorderedFrame) Show(scr *ScreenBuf) {
	if f.IsLocked() {
		return
	}
	f.ScreenObject.Show(scr) // Call embedded structure method
	f.DisplayObject(scr)
}

// DisplayObject renders the frame and title using a Painter.
func (f *BorderedFrame) DisplayObject(scr *ScreenBuf) {
	if f.boxType == NoBox { return }
	p := NewPainter(scr)

	p.Fill(f.X1, f.Y1, f.X2, f.Y2, ' ', Palette[f.ColorBackgroundIdx])
	p.DrawBox(f.X1, f.Y1, f.X2, f.Y2, Palette[f.ColorBoxIdx], f.boxType)
	p.DrawTitle(f.X1, f.Y1, f.X2, f.title, Palette[f.ColorTitleIdx])

	if f.ShowClose {
		p.DrawCloseButton(f.X2, f.Y1, Palette[f.ColorBoxIdx])
	}
}