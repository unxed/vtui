package vtui

import "github.com/mattn/go-runewidth"

// Text represents a simple static text label.
type Text struct {
	ScreenObject
	FocusLink UIElement // If a hotkey is set, focus will be passed to this element
	content   string
	color     uint64
}

func NewText(x, y int, content string, color uint64) *Text {
	t := &Text{color: color}
	t.X1, t.Y1 = x, y
	t.Y2 = y // Single line height
	t.SetText(content)
	// For simple labels, width is always text length
	t.X2 = t.X1 + runewidth.StringWidth(t.cleanText) - 1
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
	p.DrawHighlightedText(t.X1, t.Y1, t.cleanText, t.hotkeyPos, attr, highAttr)
}
func (t *Text) GetFocusLink() UIElement {
	return t.FocusLink
}
