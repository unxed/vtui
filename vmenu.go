package vtui

import (
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
	ScrollView
	title      string
	Items      []MenuItem
	done       bool
	exitCode   int
	OnAction   func(int)
	OnKeyDown  func(*vtinput.InputEvent) bool
	HideShadow bool
}


// NewVMenu creates a new vertical menu instance.
func NewVMenu(title string) *VMenu {
	clean, _, _ := ParseAmpersandString(title)
	m := &VMenu{
		title: clean,
		Items: []MenuItem{},
	}
	m.canFocus = true
	m.Wrap = true
	m.IsSelectable = func(i int) bool {
		return i >= 0 && i < len(m.Items) && !m.Items[i].Separator
	}
	m.ShowScrollBar = true
	m.MarginTop = 1
	m.MarginBottom = 1
	m.InitScrollBar(m)
	return m
}

// AddItem adds a new item to the menu.
func (m *VMenu) AddItem(item MenuItem) {
	m.Items = append(m.Items, item)
	m.ItemCount = len(m.Items)
	if len(m.Items) == 1 {
		if m.Items[0].Separator {
			m.SelectPos = 0
		} else {
			m.SetSelectPos(0)
		}
	}
}

// AddSeparator adds a separator line.
func (m *VMenu) AddSeparator() {
	m.Items = append(m.Items, MenuItem{Separator: true})
	m.ItemCount = len(m.Items)
}

func (m *VMenu) GetItemCount() int { return len(m.Items) }

// ProcessKey processes navigation keys.
func (m *VMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if m.IsDisabled() || !e.KeyDown { return false }

	if m.OnKeyDown != nil && m.OnKeyDown(e) {
		return true
	}

	isSubMenu := false
	if m.owner != nil {
		_, isSubMenu = m.owner.(*MenuBar)
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if isSubMenu {
			FrameManager.EmitCommand(CmMenuLeft, nil)
			return true
		}
		return false // Boundary exit
	case vtinput.VK_RIGHT:
		if isSubMenu {
			FrameManager.EmitCommand(CmMenuRight, nil)
			return true
		}
		// If last item in standalone menu, let focus cycle
		if m.SelectPos == m.ItemCount-1 { return false }
		return m.HandleKey(e)
	case vtinput.VK_UP:
		if m.SelectPos == 0 && !isSubMenu { return false }
		return m.HandleKey(e)
	case vtinput.VK_DOWN:
		if m.SelectPos == m.ItemCount-1 && !isSubMenu { return false }
		return m.HandleKey(e)
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		m.SetExitCode(-1); return FrameManager.GetTopFrame() == Frame(m)
	case vtinput.VK_RETURN:
		if m.SelectPos >= 0 && m.SelectPos < m.ItemCount {
			item := m.Items[m.SelectPos]
			if item.Separator { return true }
			if FrameManager.DisabledCommands.IsDisabled(item.Command) { return true }

			// 1. Fire the actual action (bubbles through owner)
			oldCmd := m.Command
			m.Command = item.Command
			m.FireAction(item.OnClick, item.UserData)
			m.Command = oldCmd

			// 2. Notify listener (may close the menu)
			if m.OnAction != nil { m.OnAction(m.SelectPos) }

			m.SetExitCode(m.SelectPos)
			return true
		}
		return true
	}

	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		for i, item := range m.Items {
			if item.Separator { continue }
			if ExtractHotkey(item.Text) == charLower {
				if FrameManager.DisabledCommands.IsDisabled(item.Command) {
					return true
				}
				m.SetSelectPos(i)

				oldCmd := m.Command
				m.Command = item.Command
				m.FireAction(item.OnClick, item.UserData)
				m.Command = oldCmd

				if m.OnAction != nil { m.OnAction(i) }

				m.SetExitCode(i)
				return true
			}
		}
	}

	return m.HandleKey(e)
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
	if m.HandleMouseScroll(e) { return true }

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		clickIdx := m.GetClickIndex(int(e.MouseY))
		if clickIdx != -1 && !m.Items[clickIdx].Separator {
			m.SetSelectPos(clickIdx)
			item := m.Items[clickIdx]
			if FrameManager.DisabledCommands.IsDisabled(item.Command) {
				return true
			}

			// Fire Action BEFORE calling OnAction/SetExitCode
			oldCmd := m.Command
			m.Command = item.Command
			m.FireAction(item.OnClick, item.UserData)
			m.Command = oldCmd

			if m.OnAction != nil {
				m.OnAction(clickIdx)
			}
			m.SetExitCode(clickIdx)
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
	if !m.IsVisible() { return }
	p := NewPainter(scr)

	// 1. Frame and Background
	p.Fill(m.X1, m.Y1, m.X2, m.Y2, ' ', Palette[ColMenuText])
	p.DrawBox(m.X1, m.Y1, m.X2, m.Y2, Palette[ColMenuBox], DoubleBox)

	titleAttr := Palette[ColMenuTitle]
	if m.IsFocused() {
		titleAttr = Palette[ColDialogHighlightBoxTitle]
	}
	p.DrawTitle(m.X1, m.Y1, m.X2, m.title, titleAttr)

	colText := Palette[ColMenuText]
	colSel := Palette[ColMenuSelectedText]
	colBox := Palette[ColMenuBox]
	height := m.Y2 - m.Y1 - 1
	if height < 0 { height = 0 }

	colHigh := Palette[ColMenuHighlight]
	colSelHigh := Palette[ColMenuSelectedHighlight]

	// 3. Rendering items
	for i := 0; i < height; i++ {
		itemIdx := i + m.TopPos
		currY := m.Y1 + 1 + i
		if currY >= m.Y2 { break }
		if itemIdx >= len(m.Items) { continue }

		item := m.Items[itemIdx]
		isDisabled := !item.Separator && FrameManager.DisabledCommands.IsDisabled(item.Command)

		attr := colText
		if isDisabled {
			attr = DimColor(attr)
		} else if itemIdx == m.SelectPos {
			attr = colSel
		}

		if item.Separator {
			p.DrawLine(m.X1, currY, m.X2, currY, boxSymbols[bsH], colBox, true, true)
			continue
		}

		// Resolve item colors
		isSel := itemIdx == m.SelectPos
		isDisabled = FrameManager.DisabledCommands.IsDisabled(item.Command)

		itemAttr := colText
		hiAttr := colHigh
		if isSel {
			itemAttr, hiAttr = colSel, colSelHigh
		}
		if isDisabled {
			itemAttr, hiAttr = DimColor(itemAttr), DimColor(hiAttr)
		}

		// Calculate layout
		//clean, _, _ := ParseAmpersandString(item.Text)
		//vLenText := runewidth.StringWidth(clean) + 1 // +1 for leading space
		shortcutText := ""
		vLenShortcut := 0
		if item.Shortcut != "" {
			shortcutText = item.Shortcut + " "
			vLenShortcut = runewidth.StringWidth(shortcutText)
		}

		// Draw background and text
		p.Fill(m.X1+1, currY, m.X2-1, currY, ' ', itemAttr)
		p.DrawControlText(m.X1+1, currY, " "+item.Text, itemAttr, hiAttr)
		if shortcutText != "" {
			p.DrawString(m.X2-vLenShortcut, currY, shortcutText, itemAttr)
		}
	}

	// 4. Scrollbar
	m.DrawScrollBar(scr)
}

