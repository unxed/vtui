package vtui

import (
	"unicode"

	"github.com/unxed/vtinput"
)

type Edit struct {
	ScreenObject
	text           []rune
	curPos         int  // Logical position in the runes string
	leftPos        int  // Visual offset (scrolling)
	selStart       int  // -1 if no selection
	selEnd         int
	selAnchor      int  // Position where selection started
	overtype       bool
	clearFlag      bool // If true, first input will clear the text
	colorNormal    uint64
	colorSelected  uint64
	colorUnchanged uint64
}

func NewEdit(x, y, width int, defaultText string) *Edit {
	e := &Edit{
		text:           []rune(defaultText),
		selStart:       -1,
		selAnchor:      -1,
		clearFlag:      false,
		colorNormal:    SetRGBBoth(0, 0xFFFFFF, 0x000000), // White on black
		colorSelected:  SetRGBBoth(0, 0x000000, 0x00AAAA), // Black on cyan
		colorUnchanged: SetRGBBoth(0, 0xAAAAAA, 0x000000), // Gray on black
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
	// Automatic LeftPos adjustment (scrolling)
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
			// Check selection
			if e.selStart != -1 && strIdx >= e.selStart && strIdx < e.selEnd {
				attr = e.colorSelected
			}
		}
		scr.Write(e.X1+i, e.Y1, []CharInfo{{Char: uint64(char), Attributes: attr}})
	}
}

func (e *Edit) SetFocus(f bool) {
	DebugLog("  Edit: SetFocus(%v)", f)
	e.focused = f
}

func (e *Edit) ProcessKey(event *vtinput.InputEvent) bool {
	if !event.KeyDown { return false }

	// Navigation with selection reset or set
	DebugLog("  Edit.ProcessKey: VK=%X Char=%d", event.VirtualKeyCode, event.Char)
	shift := (event.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (event.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0

	if ctrl && !shift {
		switch event.VirtualKeyCode {
		case vtinput.VK_C, vtinput.VK_INSERT:
			if e.selStart != -1 {
				e.copySelection()
				return true
			}
		case vtinput.VK_X:
			if e.selStart != -1 {
				e.copySelection()
				e.DeleteBlock()
				e.clearFlag = false
				return true
			}
		}
	}

	if shift && !ctrl {
		switch event.VirtualKeyCode {
		case vtinput.VK_DELETE:
			if e.selStart != -1 {
				e.copySelection()
				e.DeleteBlock()
				e.clearFlag = false
				return true
			}
		}
	}

	switch event.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
		if ctrl {
			for e.curPos > 0 && unicode.IsSpace(e.text[e.curPos-1]) {
				e.curPos--
			}
			for e.curPos > 0 && !unicode.IsSpace(e.text[e.curPos-1]) {
				e.curPos--
			}
		} else {
			if e.curPos > 0 { e.curPos-- }
		}
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_RIGHT:
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
		if ctrl {
			for e.curPos < len(e.text) && !unicode.IsSpace(e.text[e.curPos]) {
				e.curPos++
			}
			for e.curPos < len(e.text) && unicode.IsSpace(e.text[e.curPos]) {
				e.curPos++
			}
		} else {
			if e.curPos < len(e.text) { e.curPos++ }
		}
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_HOME:
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
		e.curPos = 0
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_END:
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
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

	// Text input
	if event.Char != 0 && (unicode.IsGraphic(event.Char) || event.Char == ' ') {
		// Do not process text input if Ctrl or Alt are pressed.
		// We return false here to let other handlers (like global hotkeys)
		// or the navigation switch above deal with it.
		if (event.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0 {
			return false
		}

		DebugLog("    Edit: Typng char %d", event.Char)
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

	DebugLog("    Edit: Key NOT handled")
	return false
}

func (e *Edit) beginSelection() {
	if e.selStart == -1 {
		e.selAnchor = e.curPos
		e.selStart = e.curPos
		e.selEnd = e.curPos
	}
}

func (e *Edit) endSelection() {
	if e.selAnchor != -1 {
		// Selection is always from the anchor to the current position
		if e.curPos < e.selAnchor {
			e.selStart = e.curPos
			e.selEnd = e.selAnchor
		} else {
			e.selStart = e.selAnchor
			e.selEnd = e.curPos
		}

		if e.selStart == e.selEnd {
			e.selStart = -1
			e.selAnchor = -1
		}
	}
}

func (e *Edit) DeleteBlock() {
	if e.selStart != -1 {
		e.text = append(e.text[:e.selStart], e.text[e.selEnd:]...)
		e.curPos = e.selStart
		e.selStart = -1
		e.selAnchor = -1
	}
}

func (e *Edit) copySelection() {
	if e.selStart == -1 {
		return
	}
	SetClipboard(string(e.text[e.selStart:e.selEnd]))
}
