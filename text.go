package vtui

import (
	"github.com/mattn/go-runewidth"
)

// Text represents a simple static text label.
type Text struct {
	ScreenObject
	FocusLink UIElement // If a hotkey is set, focus will be passed to this element
	content   string
	color     uint64
}

func NewText(x, y int, content string, color uint64) *Text {
	clean, hk, _ := ParseAmpersandString(content)
	t := &Text{content: content, color: color}
	t.hotkey = hk
	vLen := runewidth.StringWidth(clean)
	t.SetPosition(x, y, x+vLen-1, y)
	return t
}

func (t *Text) Show(scr *ScreenBuf) {
	t.ScreenObject.Show(scr)
	t.DisplayObject(scr)
}

func (t *Text) DisplayObject(scr *ScreenBuf) {
	if !t.IsVisible() { return }
	attr, highAttr := t.ResolveColors(ColDialogText, ColDialogText, ColDialogHighlightText, ColDialogHighlightText)
	if t.color != 0 && !t.IsDisabled() { attr = t.color }

	p := NewPainter(scr)
	p.DrawStringHighlighted(t.X1, t.Y1, t.content, attr, highAttr)
}

func (t *Text) SetText(text string) {
	t.content = text
}
func (t *Text) GetFocusLink() UIElement {
	return t.FocusLink
}
