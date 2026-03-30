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
	UserData  any
	Separator bool
}

// VMenu implements a vertical menu with navigation support.
type VMenu struct {
	ScreenObject
	title     string
	items     []MenuItem
	done      bool
	exitCode  int
	OnSelect  func(int)
	OnClose   func()
	OnLeft    func()
	OnRight   func()
	selectPos int // Selected item index
	topPos    int // Index of the first visible item (for scrolling)
	HideShadow bool
}

// NewVMenu creates a new vertical menu instance.
func NewVMenu(title string) *VMenu {
	clean, _, _ := ParseAmpersandString(title)
	m := &VMenu{
		title:     clean,
		items:     []MenuItem{},
		selectPos: 0,
	}
	m.canFocus = true
	return m
}

// AddItem adds a new item to the menu.
func (m *VMenu) AddItem(item MenuItem) {
	m.items = append(m.items, item)
	if len(m.items) == 1 {
		m.SetSelectPos(0, 1)
	}
}

// AddSeparator adds a separator line.
func (m *VMenu) AddSeparator() {
	m.items = append(m.items, MenuItem{Separator: true})
}
// GetItemCount returns the number of items (including separators) in the menu.
func (m *VMenu) GetItemCount() int {
	return len(m.items)
}

// SetSelectPos sets the currently selected item and manages scrolling.
func (m *VMenu) SetSelectPos(pos int, direct int) {
	count := len(m.items)
	if count == 0 { return }

	newPos := pos
	if newPos < 0 { newPos = count - 1 }
	if newPos >= count { newPos = 0 }

	// Skip separators
	if m.items[newPos].Separator {
		if direct == 0 {
			direct = 1
		}
		searchPos := newPos
		for i := 0; i < count; i++ {
			if !m.items[searchPos].Separator {
				newPos = searchPos
				break
			}
			searchPos += direct
			if searchPos < 0 {
				searchPos = count - 1
			} else if searchPos >= count {
				searchPos = 0
			}
		}
	}
	m.selectPos = newPos

	// Scrolling
	h := m.Y2 - m.Y1 - 1
	if h <= 0 { return }
	if m.selectPos < m.topPos {
		m.topPos = m.selectPos
	} else if m.selectPos >= m.topPos+h {
		m.topPos = m.selectPos - h + 1
	}
}

// ProcessKey processes navigation keys.
func (m *VMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_F1:
		m.ShowHelp()
		return true
	case vtinput.VK_LEFT:
		if m.OnLeft != nil {
			m.OnLeft() // Swapping logic is handled by the callback
			return true
		}
	case vtinput.VK_RIGHT:
		if m.OnRight != nil {
			m.OnRight()
			return true
		}
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		m.SetExitCode(-1)
		// If we are pushed as a Frame, we handle the key to prevent bubbling
		// to background frames (e.g. MenuBar). If we are a widget, bubble up.
		return FrameManager.GetTopFrame() == Frame(m)
	case vtinput.VK_RETURN:
		DebugLog("VMENU: Return pressed at %d", m.selectPos)
		// Turbo Vision style: emit command BEFORE setting exit code
		if m.selectPos >= 0 && m.selectPos < len(m.items) {
			if cmd := m.items[m.selectPos].Command; cmd != 0 {
				DebugLog("VMENU: Emitting command %d", cmd)
				FrameManager.EmitCommand(cmd, m.items[m.selectPos].UserData)
			} else {
				DebugLog("VMENU: No command attached to item %d", m.selectPos)
			}
		}

		if m.OnSelect != nil {
			m.OnSelect(m.selectPos)
		}
		m.SetExitCode(m.selectPos)
		return true

	case vtinput.VK_UP:
		m.SetSelectPos(m.selectPos-1, -1)
		return true
	case vtinput.VK_DOWN:
		m.SetSelectPos(m.selectPos+1, 1)
		return true
	case vtinput.VK_HOME:
		m.SetSelectPos(0, 1)
		return true
	case vtinput.VK_END:
		m.SetSelectPos(len(m.items)-1, -1)
		return true
	}

	// Quick jump by hotkey
	if e.Char != 0 {
		charLower := unicode.ToLower(e.Char)
		for i, item := range m.items {
			if item.Separator { continue }
			_, hk, _ := ParseAmpersandString(item.Text)
			if hk == charLower {
				m.SetSelectPos(i, 1)
				if m.OnSelect != nil {
					m.OnSelect(i)
				}
				m.SetExitCode(i)

				if cmd := m.items[i].Command; cmd != 0 {
					FrameManager.EmitCommand(cmd, m.items[i].UserData)
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
	if code == -1 && m.OnClose != nil {
		m.OnClose()
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
	if e.Type != vtinput.MouseEventType {
		return false
	}

	// Wheel scrolling
	if e.WheelDirection > 0 {
		m.SetSelectPos(m.selectPos-1, -1)
		return true
	} else if e.WheelDirection < 0 {
		m.SetSelectPos(m.selectPos+1, 1)
		return true
	}

	// Left button click
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)

		// Checking whether we fit inside menu frame
		if mx >= m.X1 && mx <= m.X2 && my >= m.Y1 && my <= m.Y2 {
			height := m.Y2 - m.Y1 - 1

			// Process scrollbar click
			if len(m.items) > height && mx == m.X2 {
				startY := m.Y1 + 1
				if my == startY {
					m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
					return true
				} else if my == startY+height-1 {
					m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
					return true
				} else if my > startY && my < startY+height-1 {
					// PageUp / PageDown
					if my < startY+height/2 {
						m.SetSelectPos(m.selectPos-height, -1)
					} else {
						m.SetSelectPos(m.selectPos+height, 1)
					}
					return true
				}
			}

			// Calculation of the index taking into account the presence/absence of a frame
			offset := 1
			// TODO: detect NoBox mode properly
			clickedIdx := m.topPos + (my - m.Y1 - offset)
			if clickedIdx >= 0 && clickedIdx < len(m.items) && !m.items[clickedIdx].Separator {
				m.SetSelectPos(clickedIdx, 1)
				if m.OnSelect != nil {
					m.OnSelect(clickedIdx)
				}
				m.SetExitCode(clickedIdx)
				return true
			}
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

	colHigh := Palette[ColMenuHighlight]
	colSelHigh := Palette[ColMenuSelectedHighlight]

	// 3. Rendering items
	for i := 0; i < height; i++ {
		itemIdx := i + m.topPos
		currY := m.Y1 + 1 + i
		if currY >= m.Y2 {
			break
		}

		if itemIdx >= len(m.items) {
			continue
		}

		item := m.items[itemIdx]
		attr := colText
		if itemIdx == m.selectPos {
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
			if itemIdx == m.selectPos {
				finalHighAttr = colSelHigh
			}

			cells, _ := StringToCharInfoHighlighted(fullItemText, attr, finalHighAttr)
			scr.Write(m.X1+1, currY, cells)
		}
	}

	// 4. Scrollbar
	if len(m.items) > height {
		DrawScrollBar(scr, m.X2, m.Y1+1, height, m.topPos, len(m.items), colBox)
	}
}

func (m *VMenu) SetFocus(f bool) {
	DebugLog("  VMenu(%s): SetFocus(%v)", m.title, f)
	m.focused = f
}

// `RunesToCharInfo` and `StringToCharInfo` are now in `runewidth.go`