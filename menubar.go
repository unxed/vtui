package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

type MenuBarItem struct {
	Label string
}

type MenuBar struct {
	ScreenObject
	Items     []MenuBarItem
	SelectPos int
	Active    bool
}

func NewMenuBar(items []string) *MenuBar {
	mb := &MenuBar{
		Items: make([]MenuBarItem, len(items)),
	}
	for i, label := range items {
		mb.Items[i] = MenuBarItem{Label: label}
	}
	mb.canFocus = true
	return mb
}

func (mb *MenuBar) Show(scr *ScreenBuf) {
	mb.ScreenObject.Show(scr)
	mb.DisplayObject(scr)
}

func (mb *MenuBar) DisplayObject(scr *ScreenBuf) {
	if !mb.IsVisible() { return }

	attr := Palette[ColMenuBarItem]
	scr.FillRect(mb.X1, mb.Y1, mb.X2, mb.Y2, ' ', attr)

	currX := mb.X1 + 2
	for i, item := range mb.Items {
		itemAttr := attr
		if mb.Active && i == mb.SelectPos {
			itemAttr = Palette[ColMenuBarSelected]
		}

		text := "  " + item.Label + "  "
		scr.Write(currX, mb.Y1, StringToCharInfo(text, itemAttr))
		currX += runewidth.StringWidth(text)
	}
}

// GetItemX returns the X coordinate of the item at the given index.
func (mb *MenuBar) GetItemX(index int) int {
	x := mb.X1 + 2
	for i := 0; i < index; i++ {
		x += runewidth.StringWidth("  " + mb.Items[i].Label + "  ")
	}
	return x
}

func (mb *MenuBar) ProcessKey(e *vtinput.InputEvent) bool {
	if !mb.Active || !e.KeyDown { return false }

	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if mb.SelectPos > 0 {
			mb.SelectPos--
		} else {
			mb.SelectPos = len(mb.Items) - 1
		}
		return true
	case vtinput.VK_RIGHT:
		if mb.SelectPos < len(mb.Items)-1 {
			mb.SelectPos++
		} else {
			mb.SelectPos = 0
		}
		return true
	}
	return false
}