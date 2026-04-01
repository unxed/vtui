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
		Title:              title,
		ColorBoxIdx:        ColDialogBox,
		ColorTitleIdx:      ColDialogHighlightText,
		ColorBackgroundIdx: ColDialogText,
	}
	// The GroupBox itself handles position, the inner group is relative
	gb.ScreenObject.SetPosition(x1, y1, x2, y2)
	// We don't call gb.Group.SetOwner(gb) here because GroupBox IS the Group.
	// Its owner will be set when the GroupBox itself is added to a Dialog.
	return gb
}

func (gb *GroupBox) Show(scr *ScreenBuf) {
	gb.ScreenObject.Show(scr)
	gb.DisplayObject(scr)
}

func (gb *GroupBox) DisplayObject(scr *ScreenBuf) {
	if !gb.IsVisible() { return }
	p := NewPainter(scr)

	// GroupBox is typically transparent to the dialog background,
	// so we don't call p.Fill() here, only draw the border and title.
	p.DrawBox(gb.X1, gb.Y1, gb.X2, gb.Y2, Palette[gb.ColorBoxIdx], SingleBox)
	p.DrawTitle(gb.X1, gb.Y1, gb.X2, gb.Title, Palette[gb.ColorTitleIdx])

	gb.Group.DisplayObject(scr)
}

// CanFocus returns true if the groupbox contains at least one focusable child.
func (gb *GroupBox) CanFocus() bool {
	for _, item := range gb.items {
		if item.CanFocus() && !item.IsDisabled() {
			return true
		}
	}
	return false
}


