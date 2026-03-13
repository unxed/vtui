package vtui

// Text представляет собой простую статическую текстовую надпись.
type Text struct {
	ScreenObject
	content string
	color   uint64
}

func NewText(x, y int, content string, color uint64) *Text {
	t := &Text{content: content, color: color}
	t.SetPosition(x, y, x+len([]rune(content))-1, y)
	return t
}

func (t *Text) Show(scr *ScreenBuf) {
	t.ScreenObject.Show(scr)
	t.DisplayObject(scr)
}

func (t *Text) DisplayObject(scr *ScreenBuf) {
	if !t.IsVisible() { return }
	scr.Write(t.X1, t.Y1, stringToCharInfo(t.content, t.color))
}