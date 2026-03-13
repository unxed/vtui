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
	selectPos int // Selected item index
	topPos    int // Index of the first visible item (for scrolling)

	// Colors
	ColorText       uint64
	ColorSelected   uint64
	ColorBorder     uint64
}

// NewVMenu creates a new vertical menu instance.
func NewVMenu(title string) *VMenu {
	m := &VMenu{
		title:         title,
		items:         []MenuItem{},
		selectPos:     0,
		ColorText:     SetRGBBoth(0, 0xCCCCCC, 0x0000A0), // Gray on blue
		ColorSelected: SetRGBBoth(0, 0x000000, 0x00AAAA), // Black on cyan
		ColorBorder:   SetRGBBoth(0, 0xCCCCCC, 0x0000A0),
	}
	m.canFocus = true
	return m
}

// AddItem добавляет новый пункт в меню.
func (m *VMenu) AddItem(text string) {
	m.items = append(m.items, MenuItem{Text: text})
	if len(m.items) == 1 {
		m.SetSelectPos(0, 1)
	}
}

// AddSeparator добавляет разделительную линию.
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

	// Скроллинг
	h := m.Y2 - m.Y1 - 1
	if h <= 0 { return }
	if m.selectPos < m.topPos {
		m.topPos = m.selectPos
	} else if m.selectPos >= m.topPos+h {
		m.topPos = m.selectPos - h + 1
	}
}

// ProcessKey обрабатывает клавиши навигации.
func (m *VMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.KeyEventType || !e.KeyDown {
		return false
	}

	switch e.VirtualKeyCode {
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
// ProcessMouse обрабатывает прокрутку колесиком и клики по пунктам меню.
func (m *VMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType {
		return false
	}

	// Прокрутка колесиком
	if e.WheelDirection > 0 {
		m.SetSelectPos(m.selectPos-1, -1)
		return true
	} else if e.WheelDirection < 0 {
		m.SetSelectPos(m.selectPos+1, 1)
		return true
	}

	// Клик левой кнопкой мыши
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)

		// Проверяем попадание внутрь рамки меню
		if mx > m.X1 && mx < m.X2 && my > m.Y1 && my < m.Y2 {
			clickedIdx := m.topPos + (my - m.Y1 - 1)
			if clickedIdx >= 0 && clickedIdx < len(m.items) && !m.items[clickedIdx].Separator {
				m.SetSelectPos(clickedIdx, 1)
				// Здесь в будущем будет вызов OnSelect
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
	frame := NewFrame(m.X1, m.Y1, m.X2, m.Y2, DoubleBox, m.title)
	frame.borderColor = m.ColorBorder
	frame.DisplayObject(scr)

	// 2. Clearing the background
	scr.FillRect(m.X1+1, m.Y1+1, m.X2-1, m.Y2-1, ' ', m.ColorText)

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
		attr := m.ColorText
		if itemIdx == m.selectPos {
			attr = m.ColorSelected
		}

		if item.Separator {
			// Separator: ╟──────╢
			sepRunes := make([]rune, fullWidth)
			sepRunes[0] = boxSymbols[22] // ╟
			for j := 1; j < fullWidth-1; j++ {
				sepRunes[j] = boxSymbols[1] // ─
			}
			sepRunes[fullWidth-1] = boxSymbols[23] // ╢
			scr.Write(m.X1, currY, runesToCharInfo(sepRunes, m.ColorBorder))
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

			scr.Write(m.X1+1, currY, runesToCharInfo(textRunes, attr))
		}
	}
}

func runesToCharInfo(runes []rune, attr uint64) []CharInfo {
	res := make([]CharInfo, len(runes))
	for i, r := range runes {
		res[i] = CharInfo{Char: uint64(r), Attributes: attr}
	}
	return res
}

func stringToCharInfo(s string, attr uint64) []CharInfo {
	return runesToCharInfo([]rune(s), attr)
}