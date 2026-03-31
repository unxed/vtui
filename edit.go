package vtui

import (
	"unicode"

	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
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
	clearFlag         bool // If true, first input will clear the text
	PasswordMode      bool // Mask text with '*'
	ShowHistoryButton bool // Show a clickable [v] button
	History           []string
	ActionCommand     int
	ColorTextIdx      int
	Validator         Validator
	ColorUnchangedIdx int
	ColorSelectedIdx  int
}

func NewEdit(x, y, width int, defaultText string) *Edit {
	e := &Edit{
		text:              []rune(defaultText),
		selStart:          -1,
		selAnchor:         -1,
		clearFlag:         false,
		ColorTextIdx:      ColDialogEdit,
		ColorUnchangedIdx: ColDialogEditUnchanged,
		ColorSelectedIdx:  ColDialogEditSelected,
	}
	e.canFocus = true
	e.curPos = len(e.text)
	e.SetPosition(x, y, x+width-1, y)
	return e
}


func (e *Edit) Show(scr *ScreenBuf) {
	e.ScreenObject.Show(scr)

	// Ensure cursor is visible before display
	visibleWidth := e.X2 - e.X1 + 1
	if visibleWidth < 1 { visibleWidth = 1 } // Safety: handle zero-width windows

	if e.curPos < e.leftPos {
		e.leftPos = e.curPos
	}
	// Safety: leftPos must not exceed curPos. StringWidth >= visibleWidth
	// triggers scrolling to the right.
	for e.leftPos < e.curPos && runewidth.StringWidth(string(e.text[e.leftPos:e.curPos])) >= visibleWidth {
		e.leftPos++
	}

	e.DisplayObject(scr)

	if e.IsFocused() {
		scr.SetCursorVisible(true)
		headText := string(e.text[e.leftPos:e.curPos])
		vOffset := runewidth.StringWidth(headText)
		scr.SetCursorPos(e.X1+vOffset, e.Y1)
	}
}

func (e *Edit) DisplayObject(scr *ScreenBuf) {
	if !e.IsVisible() { return }
	fullWidth := e.X2 - e.X1 + 1
	visibleWidth := fullWidth

	if e.ShowHistoryButton {
		visibleWidth--
	}

	// Pre-fill the entire line with background to avoid artifacts
	defaultAttr := Palette[e.ColorTextIdx]
	if e.clearFlag {
		defaultAttr = Palette[e.ColorUnchangedIdx]
	}
	scr.FillRect(e.X1, e.Y1, e.X2, e.Y1, ' ', defaultAttr)

	currX := 0
	for i := e.leftPos; i < len(e.text); i++ {
		r := e.text[i]
		if e.PasswordMode {
			r = '*'
		}
		w := runewidth.RuneWidth(r)

		// Stop if next character doesn't fit visually
		if currX + w > visibleWidth {
			break
		}

		attr := defaultAttr
		if e.selStart != -1 && i >= e.selStart && i < e.selEnd {
			attr = Palette[e.ColorSelectedIdx]
		}
		if e.IsDisabled() {
			attr = DimColor(attr)
		}

		// Write rune (handles WideCharFiller automatically)
		cells := StringToCharInfo(string(r), attr)
		scr.Write(e.X1 + currX, e.Y1, cells)
		currX += w
	}
	// Draw history button if needed
	if e.ShowHistoryButton {
		btnAttr := Palette[ColDialogText]
		if e.focused {
			btnAttr = Palette[ColDialogSelectedButton]
		}
		scr.Write(e.X2, e.Y1, StringToCharInfo("↓", btnAttr))
	}
}

// GetText returns the current content of the edit control as a string.
func (e *Edit) GetText() string {
	return string(e.text)
}
// SetText replaces the content of the edit control.
func (e *Edit) SetText(text string) {
	e.text = []rune(text)
	e.curPos = len(e.text)
	e.selStart = -1
	e.selAnchor = -1
}
func (e *Edit) GetData() any {
	return e.GetText()
}

func (e *Edit) SetData(val any) {
	if s, ok := val.(string); ok {
		e.SetText(s)
	}
}
func (e *Edit) Valid(cmd int) bool {
	if e.Validator != nil && (cmd == CmOK || cmd == CmDefault) {
		if !e.Validator.Validate(e.GetText()) {
			// Find the parent frame to show the error message on
			var top Frame
			if FrameManager != nil {
				top = FrameManager.GetTopFrame()
			}
			e.Validator.Error(top)
			return false
		}
	}
	return true
}
// InsertString inserts text at the current cursor position.
func (e *Edit) InsertString(text string) {
	if e.selStart != -1 {
		e.DeleteBlock()
	}
	runes := []rune(text)
	newText := make([]rune, 0, len(e.text)+len(runes))
	newText = append(newText, e.text[:e.curPos]...)
	newText = append(newText, runes...)
	newText = append(newText, e.text[e.curPos:]...)
	e.text = newText
	e.curPos += len(runes)
}

func (e *Edit) ProcessKey(event *vtinput.InputEvent) bool {
	if !event.KeyDown { return false }
	if e.IsDisabled() { return false }

	// Navigation with selection reset or set
	DebugLog("  Edit.ProcessKey: VK=%X Char=%d", event.VirtualKeyCode, event.Char)
	shift := (event.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (event.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (event.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

	if alt && event.VirtualKeyCode == vtinput.VK_DOWN && len(e.History) > 0 {
		e.OpenHistory()
		return true
	}

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

	case vtinput.VK_RETURN:
		if e.ActionCommand != 0 {
			e.HandleCommand(e.ActionCommand, nil)
			return true
		}
		return false

	case vtinput.VK_LEFT:
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
		if ctrl {
			// Skip leading spaces
			for e.curPos > 0 && unicode.IsSpace(e.text[e.curPos-1]) { e.curPos-- }
			// Skip word
			for e.curPos > 0 && !unicode.IsSpace(e.text[e.curPos-1]) { e.curPos-- }
		} else {
			if e.curPos > 0 { e.curPos-- }
		}
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_RIGHT:
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
		if ctrl {
			// Skip current word
			for e.curPos < len(e.text) && !unicode.IsSpace(e.text[e.curPos]) { e.curPos++ }
			// Skip following spaces
			for e.curPos < len(e.text) && unicode.IsSpace(e.text[e.curPos]) { e.curPos++ }
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
		// When checking modifiers, ignore Lock keys (Num, Caps, Scroll),
		// because they should not block text input.
		mods := event.ControlKeyState & ^uint32(vtinput.NumLockOn|vtinput.CapsLockOn|vtinput.ScrollLockOn|vtinput.EnhancedKey)
		if (mods & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0 {
			return false
		}

		DebugLog("    Edit: Typing char %d", event.Char)
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
func (e *Edit) OpenHistory() {
	if len(e.History) == 0 {
		return
	}
	menu := NewVMenu("History")
	for _, h := range e.History {
		menu.AddItem(MenuItem{Text: h})
	}

	h := len(e.History) + 2
	if h > 10 { h = 10 }

	// Calculate width: at least the width of the input field, but max 50
	w := e.X2 - e.X1 + 1
	if w < 20 { w = 20 }
	if w > 50 { w = 50 }

	// Positioning logic
	scrH := 25
	if FrameManager.scr != nil { scrH = FrameManager.scr.height }

	y := e.Y1 + 1
	if y + h > scrH {
		y = e.Y1 - h // Flip upwards if no space below
	}

	menu.SetPosition(e.X1, y, e.X1+w-1, y+h-1)

	menu.SetOwner(e)
	menu.SelectCommand = e.AddCallback(func(args any) {
		if idx, ok := args.(int); ok {
			e.SetText(e.History[idx])
			e.SetFocus(true)
			e.clearFlag = false
		}
	})

	FrameManager.Push(menu)
}

// AddHistory adds a string to the beginning of the history, removing duplicates.
func (e *Edit) AddHistory(text string) {
	if text == "" { return }
	newHist := make([]string, 0, len(e.History)+1)
	newHist = append(newHist, text)
	for _, h := range e.History {
		if h != text {
			newHist = append(newHist, h)
		}
	}
	if len(newHist) > 32 {
		newHist = newHist[:32]
	}
	e.History = newHist
}

func (e *Edit) ProcessMouse(ev *vtinput.InputEvent) bool {
	if e.IsDisabled() { return false }
	if ev.ButtonState == vtinput.FromLeft1stButtonPressed && ev.KeyDown {
		if e.ShowHistoryButton && int(ev.MouseX) == e.X2 && int(ev.MouseY) == e.Y1 {
			e.OpenHistory()
			return true
		}
	}
	return false
}
