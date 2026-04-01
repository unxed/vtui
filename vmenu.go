package vtui

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

// MenuItem represents a single menu item.
type MenuItem struct {
	Text      string
	Shortcut  string // Optional right-aligned hotkey hint (e.g. "F3")
	Command   int    // TV-style Command ID to emit when selected
	OnClick   func() // Closure called when selected
	UserData  any
	Separator bool
}

// VMenu implements a vertical menu with navigation support.
type VMenu struct {
	ScreenObject
	ListViewer
	title      string
	items      []MenuItem
	done       bool
	exitCode   int
	Command    int
	OnAction   func(int)
	HideShadow bool
	ScrollBar  *ScrollBar
}


// NewVMenu creates a new vertical menu instance.
func NewVMenu(title string) *VMenu {
	clean, _, _ := ParseAmpersandString(title)
	m := &VMenu{
		title: clean,
		items: []MenuItem{},
	}
	m.canFocus = true
	m.Wrap = true
	m.IsSelectable = func(i int) bool {
		return i >= 0 && i < len(m.items) && !m.items[i].Separator
	}
	m.ScrollBar = NewScrollBar(0, 0, 0)
	m.ScrollBar.SetOwner(m)
	m.ScrollBar.OnScroll = func(v int) {
		m.TopPos = v
	}
	return m
}

func (m *VMenu) SetPosition(x1, y1, x2, y2 int) {
	m.ScreenObject.SetPosition(x1, y1, x2, y2)
	m.ViewHeight = y2 - y1 - 1
	if m.ScrollBar == nil {
		m.ScrollBar = NewScrollBar(m.X2, m.Y1+1, m.ViewHeight)
		m.ScrollBar.SetOwner(m)
		m.ScrollBar.OnScroll = func(v int) {
			m.TopPos = v
		}
	} else {
		m.ScrollBar.SetPosition(m.X2, m.Y1+1, m.X2, m.Y2-1)
	}
}

// AddItem adds a new item to the menu.
func (m *VMenu) AddItem(item MenuItem) {
	m.items = append(m.items, item)
	m.ItemCount = len(m.items)
	if len(m.items) == 1 {
		if m.items[0].Separator {
			m.SelectPos = 0
		} else {
			m.SetSelectPos(0)
		}
	}
}

// AddSeparator adds a separator line.
func (m *VMenu) AddSeparator() {
	m.items = append(m.items, MenuItem{Separator: true})
	m.ItemCount = len(m.items)
}

func (m *VMenu) GetItemCount() int { return len(m.items) }

// ProcessKey processes navigation keys.
func (m *VMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || !e.KeyDown { return false }
	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		FrameManager.EmitCommand(CmMenuLeft, nil); return true
	case vtinput.VK_RIGHT:
		FrameManager.EmitCommand(CmMenuRight, nil); return true
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		m.SetExitCode(-1); return FrameManager.GetTopFrame() == Frame(m)
	case vtinput.VK_RETURN:
		if m.SelectPos >= 0 && m.SelectPos < m.ItemCount {
			item := m.items[m.SelectPos]
			if !item.Separator && FrameManager.DisabledCommands.IsDisabled(item.Command) { return true }
			if item.OnClick != nil {
				item.OnClick()
			} else if item.Command != 0 {
				FrameManager.EmitCommand(item.Command, item.UserData)
			}
		}
		if m.OnAction != nil {
			m.OnAction(m.SelectPos)
		} else if m.Command != 0 {
			m.HandleCommand(m.Command, m.SelectPos)
		}
		m.SetExitCode(m.SelectPos); return true
	case vtinput.VK_UP: m.MoveRelative(-1); return true
	case vtinput.VK_DOWN: m.MoveRelative(1); return true
	case vtinput.VK_HOME: m.SetSelectPos(0); return true
	case vtinput.VK_END: m.SetSelectPos(m.ItemCount-1); return true
	case vtinput.VK_PRIOR: m.MoveRelative(-m.ViewHeight); return true
	case vtinput.VK_NEXT: m.MoveRelative(m.ViewHeight); return true
	}
	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		for i, item := range m.items {
			if item.Separator { continue }
			_, hk, _ := ParseAmpersandString(item.Text)
			if hk == charLower {
				if FrameManager.DisabledCommands.IsDisabled(item.Command) { return true }
				m.SetSelectPos(i)
				if m.OnAction != nil {
					m.OnAction(i)
				} else if m.Command != 0 {
					m.HandleCommand(m.Command, i)
				}
				m.SetExitCode(i)
				if item.OnClick != nil {
					item.OnClick()
				} else if item.Command != 0 {
					FrameManager.EmitCommand(item.Command, item.UserData)
				}
				return true
			}
		}
	}
	return false
}

func (m *VMenu) ResizeConsole(w, h int) {
	// For standalone VMenus, we might want to keep them centered
}
func (m *VMenu) GetTitle() string {
	return m.title
}
func (m *VMenu) GetProgress() int {
	return -1
}

func (m *VMenu) GetType() FrameType {
	return TypeMenu
}

func (m *VMenu) SetExitCode(code int) {
	m.done = true
	m.exitCode = code
	if code == -1 {
		FrameManager.EmitCommand(CmMenuClose, nil)
	}
}

func (m *VMenu) IsDone() bool {
	return m.done
}
func (m *VMenu) IsBusy() bool { return false }
func (m *VMenu) IsModal() bool { return true }
func (m *VMenu) GetWindowNumber() int { return 0 }
func (m *VMenu) SetWindowNumber(n int) {}
func (m *VMenu) RequestFocus() bool { return true }
func (m *VMenu) Close() { m.SetExitCode(-1) }
func (m *VMenu) HasShadow() bool { return !m.HideShadow }
// ClearDone resets the menu state, allowing it to be shown again.
func (m *VMenu) ClearDone() {
	m.done = false
	m.exitCode = -1
}

// ProcessMouse handles mouse wheel scrolling and menu item clicks.
func (m *VMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || e.Type != vtinput.MouseEventType { return false }

	if e.WheelDirection != 0 {
		if e.WheelDirection > 0 { m.MoveRelative(-1) } else { m.MoveRelative(1) }
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)
		h := m.ViewHeight
		if mx == m.X2 && m.ItemCount > h {
			startY := m.Y1 + 1
			if my == startY { m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) } else if my == startY+h-1 { m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}) } else {
				if my < startY+h/2 { m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR}) } else { m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT}) }
			}
			return true
		}
		
		clickedIdx := m.TopPos + (my - m.Y1 - 1)
		if mx >= m.X1 && mx < m.X2 && clickedIdx >= 0 && clickedIdx < m.ItemCount && !m.items[clickedIdx].Separator {
			m.SetSelectPos(clickedIdx)
			if m.OnAction != nil {
				m.OnAction(clickedIdx)
			} else if m.Command != 0 {
				m.HandleCommand(m.Command, clickedIdx)
			}
			m.SetExitCode(clickedIdx)
			return true
		}
	}
	return false
}


// Show prepares the background and calls the render method.
func (m *VMenu) Show(scr *ScreenBuf) {
	m.ScreenObject.Show(scr)
	m.DisplayObject(scr)
}

// DisplayObject renders the frame and menu items.
func (m *VMenu) DisplayObject(scr *ScreenBuf) {
	if !m.IsVisible() {
		return
	}

	// 1. Rendering the frame
	frame := NewBorderedFrame(m.X1, m.Y1, m.X2, m.Y2, DoubleBox, m.title)
	// VMenu maps to Menu colors (not dialog listbox) for now
	colText := Palette[ColMenuText]
	colSel := Palette[ColMenuSelectedText]
	colBox := Palette[ColMenuBox]

	frame.ColorBoxIdx = ColMenuBox
	frame.ColorTitleIdx = ColMenuTitle
	frame.ColorBackgroundIdx = ColMenuText
	frame.DisplayObject(scr)

	fullWidth := m.X2 - m.X1 + 1
	interiorWidth := fullWidth - 2
	height := m.Y2 - m.Y1 - 1
	if height < 0 { height = 0 }

	colHigh := Palette[ColMenuHighlight]
	colSelHigh := Palette[ColMenuSelectedHighlight]

	// 3. Rendering items
	for i := 0; i < height; i++ {
		itemIdx := i + m.TopPos
		currY := m.Y1 + 1 + i
		if currY >= m.Y2 { break }
		if itemIdx >= len(m.items) { continue }

		item := m.items[itemIdx]
		isDisabled := !item.Separator && FrameManager.DisabledCommands.IsDisabled(item.Command)

		attr := colText
		if isDisabled {
			attr = DimColor(attr)
		} else if itemIdx == m.SelectPos {
			attr = colSel
		}

		if item.Separator {
			// Separator: ╟──────╢
			sepRunes := make([]rune, fullWidth)
			sepRunes[0] = boxSymbols[22] // ╟
			for j := 1; j < fullWidth-1; j++ {
				sepRunes[j] = boxSymbols[1] // ─
			}
			sepRunes[fullWidth-1] = boxSymbols[23] // ╢
			scr.Write(m.X1, currY, RunesToCharInfo(sepRunes, colBox))
		} else {
			fullItemText := " " + item.Text
			clean, _, _ := ParseAmpersandString(fullItemText)
			vLenText := runewidth.StringWidth(clean)

			shortcutText := ""
			vLenShortcut := 0
			if item.Shortcut != "" {
				shortcutText = item.Shortcut + " "
				vLenShortcut = runewidth.StringWidth(shortcutText)
			}

			padding := interiorWidth - vLenText - vLenShortcut
			if padding > 0 {
				fullItemText += strings.Repeat(" ", padding)
			}
			fullItemText += shortcutText

			finalHighAttr := colHigh
			if itemIdx == m.SelectPos {
				finalHighAttr = colSelHigh
			}

			cells, _ := StringToCharInfoHighlighted(fullItemText, attr, finalHighAttr)
			scr.Write(m.X1+1, currY, cells)
		}
	}

	// 4. Scrollbar
	if m.ScrollBar != nil && height > 0 && m.ItemCount > height {
		m.ScrollBar.SetParams(m.TopPos, 0, m.ItemCount-height)
		m.ScrollBar.Show(scr)
	}
}

