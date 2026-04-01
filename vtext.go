package vtui

import (
	"github.com/mattn/go-runewidth"
)

// VText represents a vertical text label.
type VText struct {
	ScreenObject
	Content string
	Color   uint64
}

func NewVText(x, y int, content string, color uint64) *VText {
	vt := &VText{Content: content, Color: color}
	// Height is the number of characters, width is the maximum character width (usually 1)
	runes := []rune(content)
	height := len(runes)
	width := 0
	for _, r := range runes {
		rw := runewidth.RuneWidth(r)
		if rw > width {
			width = rw
		}
	}
	vt.SetPosition(x, y, x+width-1, y+height-1)
	return vt
}

func (vt *VText) Show(scr *ScreenBuf) {
	vt.ScreenObject.Show(scr)
	vt.DisplayObject(scr)
}

func (vt *VText) DisplayObject(scr *ScreenBuf) {
	if !vt.IsVisible() { return }
	attr := vt.ResolveColor(ColDialogText, ColDialogText)
	if vt.Color != 0 && !vt.IsDisabled() { attr = vt.Color }

	runes := []rune(vt.Content)
	for i, r := range runes {
		scr.Write(vt.X1, vt.Y1+i, StringToCharInfo(string(r), attr))
	}
}