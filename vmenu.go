package vtui

import (
	"github.com/unxed/vtinput"
)

// MenuItem represents a single menu item.
type MenuItem struct {
	Text      string
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
	selectPos int // Selected item index
	topPos    int // Index of the first visible item (for scrolling)

}

// NewVMenu creates a new vertical menu instance.
func NewVMenu(title string) *VMenu {
	m := &VMenu{
		title:     title,
		items:     []MenuItem{},
		selectPos: 0,
	}
	m.canFocus = true
	return m
}

// AddItem adds a new item to the menu.
func (m *VMenu) AddItem(text string) {
	m.items = append(m.items, MenuItem{Text: text})
	if len(m.items) == 1 {
		m.SetSelectPos(0, 1)
	}
}

// AddSeparator adds a separator line.
func (m *VMenu) AddSeparator() {
	m.items = append(m.items, MenuItem{Separator: true})
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
	case vtinput.VK_ESCAPE:
		m.SetExitCode(-1)
		return true
	case vtinput.VK_RETURN:
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
	return false
}

func (m *VMenu) ResizeConsole(w, h int) {
	// For standalone VMenus, we might want to keep them centered
}

func (m *VMenu) GetType() FrameType {
	return TypeMenu
}

func (m *VMenu) SetExitCode(code int) {
	m.done = true
	m.exitCode = code
}

func (m *VMenu) IsDone() bool {
	return m.done
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
			// Calculation of the index taking into account the presence/absence of a frame
			offset := 1
			// TODO: detect NoBox mode properly
			clickedIdx := m.topPos + (my - m.Y1 - offset)
			if clickedIdx >= 0 && clickedIdx < len(m.items) && !m.items[clickedIdx].Separator {
				m.SetSelectPos(clickedIdx, 1)
				// Here in the future there will be a call to OnSelect
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
	frame.DisplayObject(scr)

	// 2. Clearing the background
	scr.FillRect(m.X1+1, m.Y1+1, m.X2-1, m.Y2-1, ' ', colText)

	fullWidth := m.X2 - m.X1 + 1
	interiorWidth := fullWidth - 2
	height := m.Y2 - m.Y1 - 1

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
			// Padded menu item
			textRunes := make([]rune, interiorWidth)
			for j := range textRunes {
				textRunes[j] = ' '
			}

			contentRunes := []rune(item.Text)
			if len(contentRunes) > interiorWidth-2 {
				contentRunes = contentRunes[:interiorWidth-2]
			}
			copy(textRunes[1:], contentRunes)

			scr.Write(m.X1+1, currY, RunesToCharInfo(textRunes, attr))
		}
	}
}

func (m *VMenu) SetFocus(f bool) {
	DebugLog("  VMenu(%s): SetFocus(%v)", m.title, f)
	m.focused = f
}

func RunesToCharInfo(runes []rune, attr uint64) []CharInfo {
	res := make([]CharInfo, len(runes))
	for i, r := range runes {
		res[i] = CharInfo{Char: uint64(r), Attributes: attr}
	}
	return res
}

func StringToCharInfo(s string, attr uint64) []CharInfo {
	return RunesToCharInfo([]rune(s), attr)
}