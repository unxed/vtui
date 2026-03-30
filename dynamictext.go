package vtui

// DynamicText is a label that updates its content every frame via a callback.
type DynamicText struct {
	Text
	GetValue func() string
}

func NewDynamicText(x, y, w int, color uint64, cb func() string) *DynamicText {
	dt := &DynamicText{
		Text:     *NewText(x, y, "", color),
		GetValue: cb,
	}
	dt.X2 = x + w - 1
	return dt
}

func (dt *DynamicText) Show(scr *ScreenBuf) {
	if dt.GetValue != nil {
		dt.content = dt.GetValue()
	}
	dt.Text.Show(scr)
}