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
	return gb
}

func (gb *GroupBox) Show(scr *ScreenBuf) {
	gb.ScreenObject.Show(scr)
	gb.DisplayObject(scr)
}

func (gb *GroupBox) DisplayObject(scr *ScreenBuf) {
	if !gb.IsVisible() {
		return
	}

	// 1. Draw the frame and title
	frame := NewBorderedFrame(gb.X1, gb.Y1, gb.X2, gb.Y2, SingleBox, gb.Title)
	frame.ColorBoxIdx = gb.ColorBoxIdx
	frame.ColorTitleIdx = gb.ColorTitleIdx
	frame.ColorBackgroundIdx = gb.ColorBackgroundIdx
	// A groupbox doesn't fill its background, it's transparent to the dialog's bg
	// So we only draw the border part of the frame.
	frame.DisplayObject(scr)

	// 2. Draw the children inside the frame
	gb.Group.DisplayObject(scr)
}

// Override CanFocus for GroupBox itself. Focus is handled by its children.
func (gb *GroupBox) CanFocus() bool {
	// A GroupBox can be a focus target if it contains focusable children.
	// The changeFocus logic will handle this.
	for _, item := range gb.items {
		if item.CanFocus() && !item.IsDisabled() {
			return true
		}
	}
	return false
}

