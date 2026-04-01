package vtui

// GroupBox is a decorative titled frame used to visually group elements.
// It embeds a Group to manage child elements.
type GroupBox struct {
	Group
	Title              string
	ColorBoxIdx        int
	ColorTitleIdx      int
	ColorBackgroundIdx int
}

func NewGroupBox(x1, y1, x2, y2 int, title string) *GroupBox {
	gb := &GroupBox{
		Group:              *NewGroup(x1+1, y1+1, x2-x1-1, y2-y1-1),
		ColorBoxIdx:        ColDialogBox,
		ColorTitleIdx:      ColDialogHighlightText,
		ColorBackgroundIdx: ColDialogText,
	}
	// Use manual assignment to avoid 'visible = false' side effect of SetPosition in constructor
	gb.X1, gb.Y1, gb.X2, gb.Y2 = x1, y1, x2, y2
	gb.SetText(title)
	return gb
}

func (gb *GroupBox) Show(scr *ScreenBuf) {
	gb.ScreenObject.Show(scr)
	gb.DisplayObject(scr)
}

func (gb *GroupBox) DisplayObject(scr *ScreenBuf) {
	if !gb.IsVisible() { return }
	p := NewPainter(scr)

	p.DrawBox(gb.X1, gb.Y1, gb.X2, gb.Y2, Palette[gb.ColorBoxIdx], SingleBox)
	// DrawTitle is also simplified now as it can use cleanText
	p.DrawTitle(gb.X1, gb.Y1, gb.X2, gb.cleanText, Palette[gb.ColorTitleIdx])

	gb.Group.DisplayObject(scr)
}



