package vtui

import (
	"github.com/unxed/vtinput"
	"unicode"
)

type Edit struct {
	ScreenObject
	text           []rune
	curPos         int  // Логическая позиция в строке runes
	leftPos        int  // Визуальное смещение (скроллинг)
	selStart       int  // -1 если нет выделения
	selEnd         int
	overtype       bool
	clearFlag      bool // Если true, первый ввод удалит текст
	colorNormal    uint64
	colorSelected  uint64
	colorUnchanged uint64
}

func NewEdit(x, y, width int, defaultText string) *Edit {
	e := &Edit{
		text:           []rune(defaultText),
		selStart:       -1,
		clearFlag:      true,
		colorNormal:    SetRGBBoth(0, 0xFFFFFF, 0x000000), // Белый на черном
		colorSelected:  SetRGBBoth(0, 0x000000, 0x00AAAA), // Черный на бирюзовом
		colorUnchanged: SetRGBBoth(0, 0xAAAAAA, 0x000000), // Серый на черном
	}
	e.canFocus = true
	e.curPos = len(e.text)
	e.SetPosition(x, y, x+width-1, y)
	return e
}

func (e *Edit) Show(scr *ScreenBuf) {
	e.ScreenObject.Show(scr)
	e.DisplayObject(scr)
	if e.IsFocused() {
		scr.SetCursorVisible(true)
		scr.SetCursorPos(e.X1+(e.curPos-e.leftPos), e.Y1)
	}
}

func (e *Edit) DisplayObject(scr *ScreenBuf) {
	if !e.IsVisible() { return }

	width := e.X2 - e.X1 + 1
	// Автоматическая корректировка LeftPos (скроллинг)
	if e.curPos < e.leftPos {
		e.leftPos = e.curPos
	} else if e.curPos-e.leftPos >= width {
		e.leftPos = e.curPos - width + 1
	}

	for i := 0; i < width; i++ {
		strIdx := i + e.leftPos
		char := ' '
		attr := e.colorNormal

		if e.clearFlag {
			attr = e.colorUnchanged
		}

		if strIdx < len(e.text) {
			char = e.text[strIdx]
			// Проверка выделения
			if e.selStart != -1 && strIdx >= e.selStart && strIdx < e.selEnd {
				attr = e.colorSelected
			}
		}
		scr.Write(e.X1+i, e.Y1, []CharInfo{{Char: uint64(char), Attributes: attr}})
	}
}

func (e *Edit) ProcessKey(event *vtinput.InputEvent) bool {
	if !event.KeyDown { return false }

	// Навигация со сбросом или установкой выделения
	shift := (event.ControlKeyState & vtinput.ShiftPressed) != 0

	switch event.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if shift { e.beginSelection() } else { e.selStart = -1 }
		if e.curPos > 0 { e.curPos-- }
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_RIGHT:
		if shift { e.beginSelection() } else { e.selStart = -1 }
		if e.curPos < len(e.text) { e.curPos++ }
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_HOME:
		if shift { e.beginSelection() } else { e.selStart = -1 }
		e.curPos = 0
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_END:
		if shift { e.beginSelection() } else { e.selStart = -1 }
		e.curPos = len(e.text)
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_BACK:
		if e.selStart != -1 {
			e.DeleteBlock()
		} else if e.curPos > 0 {
			e.text = append(e.text[:e.curPos-1], e.text[e.curPos:]...)
			e.curPos--
		}
		e.clearFlag = false
		return true

	case vtinput.VK_DELETE:
		if e.selStart != -1 {
			e.DeleteBlock()
		} else if e.curPos < len(e.text) {
			e.text = append(e.text[:e.curPos], e.text[e.curPos+1:]...)
		}
		e.clearFlag = false
		return true

	case vtinput.VK_INSERT:
		e.overtype = !e.overtype
		return true
	}

	// Ввод текста
	if event.Char != 0 && (unicode.IsGraphic(event.Char) || event.Char == ' ') {
		if e.clearFlag {
			e.text = []rune{}
			e.curPos = 0
			e.clearFlag = false
		}

		if e.selStart != -1 {
			e.DeleteBlock()
		}

		if e.overtype && e.curPos < len(e.text) {
			e.text[e.curPos] = event.Char
		} else {
			newText := make([]rune, 0, len(e.text)+1)
			newText = append(newText, e.text[:e.curPos]...)
			newText = append(newText, event.Char)
			newText = append(newText, e.text[e.curPos:]...)
			e.text = newText
		}
		e.curPos++
		return true
	}

	return false
}

func (e *Edit) beginSelection() {
	if e.selStart == -1 {
		e.selStart = e.curPos
		e.selEnd = e.curPos
	}
}

func (e *Edit) endSelection() {
	if e.selStart != -1 {
		// Всегда храним selStart < selEnd
		start, end := e.selStart, e.curPos
		if start > end {
			e.selStart, e.selEnd = end, start
		} else {
			e.selStart, e.selEnd = start, end
		}
		if e.selStart == e.selEnd {
			e.selStart = -1
		}
	}
}

func (e *Edit) DeleteBlock() {
	if e.selStart != -1 {
		e.text = append(e.text[:e.selStart], e.text[e.selEnd:]...)
		e.curPos = e.selStart
		e.selStart = -1
	}
}