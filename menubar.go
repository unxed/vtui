package vtui

import (
	"unicode"
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

type MenuBarItem struct {
	Label    string
	SubItems []MenuItem
}

type MenuBar struct {
	ScreenObject
	Items          []MenuBarItem
	SelectPos      int
	Active         bool
	activeSubMenu  Frame
	OnActivate     func(index int, subItems []MenuItem)
	OnCommand      func(menuIdx, itemIdx int)
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
		hiAttr := Palette[ColMenuBarHighlight]
		if i == mb.SelectPos && mb.Active {
			itemAttr = Palette[ColMenuBarSelected]
			hiAttr = Palette[ColMenuBarSelectedHighlight]
		}

		cells, _ := StringToCharInfoHighlighted("  "+item.Label+"  ", itemAttr, hiAttr)
		scr.Write(currX, mb.Y1, cells)

		clean, _, _ := ParseAmpersandString(item.Label)
		currX += runewidth.StringWidth("  " + clean + "  ")
	}
}

// GetItemX returns the X coordinate of the item at the given index.
func (mb *MenuBar) GetItemX(index int) int {
	x := mb.X1 + 2
	for i := 0; i < index; i++ {
		clean, _, _ := ParseAmpersandString(mb.Items[i].Label)
		x += runewidth.StringWidth("  " + clean + "  ")
	}
	return x
}

// SetSubMenu associates an open VMenu with the bar so it can be auto-closed later.
func (mb *MenuBar) SetSubMenu(f Frame) {
	mb.activeSubMenu = f
}

func (mb *MenuBar) closeSub() {
	if mb.activeSubMenu != nil {
		if FrameManager.GetTopFrameType() == TypeMenu {
			FrameManager.Pop()
		}
		mb.activeSubMenu = nil
	}
}

// ActivateSubMenu creates and pushes a VMenu for the given top-level item index.
func (mb *MenuBar) ActivateSubMenu(index int) {
	mb.closeSub()
	mb.SelectPos = index
	
	// If application provided a custom activator, use it
	if mb.OnActivate != nil {
		mb.OnActivate(index, mb.Items[index].SubItems)
		return
	}

	items := mb.Items[index].SubItems
	if len(items) == 0 { return }

	m := NewVMenu(mb.Items[index].Label)
	mb.activeSubMenu = m
	for _, itm := range items {
		if itm.Separator {
			m.AddSeparator()
		} else {
			m.AddItem(itm)
		}
	}

	x := mb.GetItemX(index)
	// Default positioning and width for submenus
	m.SetPosition(x, mb.Y1+1, x+24, mb.Y1+1+m.GetItemCount()+1)

	m.OnLeft = func() {
		newIdx := mb.SelectPos - 1
		if newIdx < 0 { newIdx = len(mb.Items) - 1 }
		mb.ActivateSubMenu(newIdx)
	}
	m.OnRight = func() {
		newIdx := (mb.SelectPos + 1) % len(mb.Items)
		mb.ActivateSubMenu(newIdx)
	}
	m.OnSelect = func(itmIdx int) {
		mb.Active = false
		// Backward compatibility callback
		if mb.OnCommand != nil {
			mb.OnCommand(mb.SelectPos, itmIdx)
		}
		// The actual EmitCommand happens inside VMenu.ProcessKey now.
	}
	m.OnClose = func() {
		mb.activeSubMenu = nil
	}

	FrameManager.Push(m)
}

func (mb *MenuBar) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }

	// Helper to close current submenu before switching or closing
	closeSub := func() {
		if mb.activeSubMenu != nil {
			if FrameManager.GetTopFrameType() == TypeMenu {
				FrameManager.Pop()
			}
			mb.activeSubMenu = nil
		}
	}

	// 1. Logic when menu is already active
	if mb.Active {
		switch e.VirtualKeyCode {
		case vtinput.VK_LEFT:
			closeSub()
			if mb.SelectPos > 0 { mb.SelectPos-- } else { mb.SelectPos = len(mb.Items) - 1 }
			return true
		case vtinput.VK_RIGHT:
			closeSub()
			if mb.SelectPos < len(mb.Items)-1 { mb.SelectPos++ } else { mb.SelectPos = 0 }
			return true
		case vtinput.VK_DOWN, vtinput.VK_RETURN:
			mb.ActivateSubMenu(mb.SelectPos)
			return true
		}

		// Handle hotkeys without Alt when active
		if e.Char != 0 {
			charLower := unicode.ToLower(e.Char)
			for i, item := range mb.Items {
				_, hk, _ := ParseAmpersandString(item.Label)
				if hk == charLower {
					mb.ActivateSubMenu(i)
					return true
				}
			}
		}
	}

	// 2. Alt+Hotkey handling (Trigger activation)
	alt := (e.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	if alt && e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		for i, item := range mb.Items {
			_, hk, _ := ParseAmpersandString(item.Label)
			if hk == charLower {
				mb.Active = true
				mb.ActivateSubMenu(i)
				return true
			}
		}
	}

	return false
}