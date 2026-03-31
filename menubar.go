package vtui

import (
	"unicode"
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

type MenuBarItem struct {
	Label    string
	SubItems []MenuItem
	Command  int
}

type MenuBar struct {
	Bar
	Items         []MenuBarItem
	SelectPos     int
	Active        bool
	activeSubMenu Frame
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

func (mb *MenuBar) HandleCommand(cmd int, args any) bool {
	switch cmd {
	case CmMenuLeft:
		newIdx := mb.SelectPos - 1
		if newIdx < 0 { newIdx = len(mb.Items) - 1 }
		mb.ActivateSubMenu(newIdx)
		return true
	case CmMenuRight:
		newIdx := (mb.SelectPos + 1) % len(mb.Items)
		mb.ActivateSubMenu(newIdx)
		return true
	case CmMenuClose:
		mb.activeSubMenu = nil
		mb.Active = false
		return true
	}
	return mb.Bar.HandleCommand(cmd, args)
}
func (mb *MenuBar) Show(scr *ScreenBuf) {
	mb.ScreenObject.Show(scr)
	mb.DisplayObject(scr)
}

func (mb *MenuBar) DisplayObject(scr *ScreenBuf) {
	if !mb.IsVisible() { return }

	attr := Palette[ColMenuBarItem]
	mb.DrawBackground(scr, attr)

	currX := mb.X1 + 2
	for i, item := range mb.Items {
		itemAttr := attr
		hiAttr := Palette[ColMenuBarHighlight]

		// Check if ALL subitems are disabled (simplified logic for top-level)
		allDisabled := len(item.SubItems) > 0
		for _, si := range item.SubItems {
			if !si.Separator && !FrameManager.DisabledCommands.IsDisabled(si.Command) {
				allDisabled = false
				break
			}
		}

		if i == mb.SelectPos && mb.Active {
			itemAttr = Palette[ColMenuBarSelected]
			hiAttr = Palette[ColMenuBarSelectedHighlight]
		} else if allDisabled {
			// Dim the top level menu if it leads to nothing useful
			itemAttr = SetRGBFore(itemAttr, GetRGBFore(itemAttr)/2)
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
		// Use RemoveFrame instead of Pop to ensure the menu is gone
		// even if a dialog popped up on top of it.
		FrameManager.RemoveFrame(mb.activeSubMenu)
		mb.activeSubMenu = nil
	}
}

// ActivateSubMenu creates and pushes a VMenu for the given top-level item index.
func (mb *MenuBar) ActivateSubMenu(index int) {
	mb.closeSub()
	mb.SelectPos = index

	if mb.Items[index].Command != 0 {
		FrameManager.EmitCommand(mb.Items[index].Command, index)
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

	// Dynamically calculate required width for the submenu
	maxWidth := 24
	for _, itm := range items {
		if !itm.Separator {
			clean, _, _ := ParseAmpersandString(" " + itm.Text)
			w := runewidth.StringWidth(clean)
			if itm.Shortcut != "" {
				w += runewidth.StringWidth(itm.Shortcut + " ")
			}
			w += 4 // Minimum visual padding between text and shortcut/border
			if w > maxWidth {
				maxWidth = w
			}
		}
	}

	m.SetPosition(x, mb.Y1+1, x+maxWidth-1, mb.Y1+1+m.GetItemCount()+1)

	m.SetOnSelect(func(itmIdx int) {
		mb.Active = false
	})

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

func (mb *MenuBar) ProcessMouse(e *vtinput.InputEvent) bool {
	if mb.IsDisabled() { return false }
	if e.Type == vtinput.MouseEventType && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		my, mx := int(e.MouseY), int(e.MouseX)
		if my == mb.Y1 && mx >= mb.X1 && mx <= mb.X2 {
			for i := range mb.Items {
				x1 := mb.GetItemX(i)
				var x2 int
				if i < len(mb.Items)-1 {
					x2 = mb.GetItemX(i+1) - 1
				} else {
					clean, _, _ := ParseAmpersandString(mb.Items[i].Label)
					x2 = x1 + runewidth.StringWidth("  " + clean + "  ") - 1
				}
				if mx >= x1 && mx <= x2 {
					mb.Active = true
					mb.ActivateSubMenu(i)
					return true
				}
			}
		}
	}
	return false
}
