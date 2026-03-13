package vtui

import (
	"github.com/unxed/vtinput"
)

// MenuItem представляет собой один пункт в меню.
type MenuItem struct {
	Text      string
	UserData  any
	Separator bool
}

// VMenu реализует вертикальное меню с поддержкой навигации.
type VMenu struct {
	ScreenObject
	title     string
	items     []MenuItem
	selectPos int // Индекс выбранного пункта
	topPos    int // Индекс первого видимого пункта (для скроллинга)

	// Цвета
	ColorText       uint64
	ColorSelected   uint64
	ColorBorder     uint64
}

// NewVMenu создает новый экземпляр вертикального меню.
func NewVMenu(title string) *VMenu {
	m := &VMenu{
		title:         title,
		items:         []MenuItem{},
		selectPos:     0,
		ColorText:     SetRGBBoth(0, 0xCCCCCC, 0x0000A0), // Серый на синем
		ColorSelected: SetRGBBoth(0, 0x000000, 0x00AAAA), // Черный на бирюзовом
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

// SetSelectPos устанавливает текущий выбранный пункт и управляет скроллингом.
func (m *VMenu) SetSelectPos(pos int, direct int) {
	count := len(m.items)
	if count == 0 { return }

	newPos := pos
	if newPos < 0 { newPos = count - 1 }
	if newPos >= count { newPos = 0 }

	// Пропуск разделителей
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

// Show подготавливает фон и вызывает отрисовку.
func (m *VMenu) Show(scr *ScreenBuf) {
	m.ScreenObject.Show(scr)
	m.DisplayObject(scr)
}

// DisplayObject отрисовывает рамку и пункты меню.
func (m *VMenu) DisplayObject(scr *ScreenBuf) {
	if !m.IsVisible() {
		return
	}

	// 1. Отрисовка рамки
	frame := NewFrame(m.X1, m.Y1, m.X2, m.Y2, DoubleBox, m.title)
	frame.borderColor = m.ColorBorder
	frame.DisplayObject(scr)

	// 2. Очистка фона
	scr.FillRect(m.X1+1, m.Y1+1, m.X2-1, m.Y2-1, ' ', m.ColorText)

	fullWidth := m.X2 - m.X1 + 1
	interiorWidth := fullWidth - 2
	height := m.Y2 - m.Y1 - 1

	// 3. Отрисовка пунктов
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
			// Разделитель: ╟──────╢
			sepRunes := make([]rune, fullWidth)
			sepRunes[0] = boxSymbols[22] // ╟
			for j := 1; j < fullWidth-1; j++ {
				sepRunes[j] = boxSymbols[1] // ─
			}
			sepRunes[fullWidth-1] = boxSymbols[23] // ╢
			scr.Write(m.X1, currY, runesToCharInfo(sepRunes, m.ColorBorder))
		} else {
			// Пункт меню с отступами
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