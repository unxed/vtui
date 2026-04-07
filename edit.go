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
	pasting           bool
	pasteBuffer       []rune
	PasswordMode      bool // Mask text with '*'
	ShowHistoryButton bool // Show a clickable [v] button
	History            []string
	HistoryPos         int
	HistoryLimit       int
	DeduplicateHistory bool
	Command            int
	OnAction           func()
	ColorTextIdx      int
	Validator         Validator
	ColorUnchangedIdx int
	ColorSelectedIdx  int
	HistoryID         string
}

// HistoryProvider is an interface for external history persistence (e.g. from f4).
type HistoryProvider interface {
	LoadHistory(id string) []string
	SaveHistory(id string, history []string)
}

var GlobalHistoryProvider HistoryProvider

func NewEdit(x, y, width int, defaultText string) *Edit {
	e := &Edit{
		text:               []rune(defaultText),
		HistoryPos:         -1,
		selStart:           -1,
		selAnchor:          -1,
		clearFlag:          false,
		ColorTextIdx:       ColDialogEdit,
		ColorUnchangedIdx:  ColDialogEditUnchanged,
		ColorSelectedIdx:   ColDialogEditSelected,
		HistoryLimit:       32,
		DeduplicateHistory: true,
	}
	e.canFocus = true
	e.curPos = len(e.text)
	e.SetPosition(x, y, x+width-1, y)
	return e
}
// NewPasswordEdit creates an Edit control that masks input with asterisks.
func NewPasswordEdit(x, y, width int, defaultText string) *Edit {
	e := NewEdit(x, y, width, defaultText)
	e.PasswordMode = true
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
	// Safety: leftPos must not exceed curPos.
	width := 0
	for i := e.leftPos; i < e.curPos; i++ {
		r := e.text[i]
		if e.PasswordMode {
			width += 1
		} else {
			width += runewidth.RuneWidth(r)
		}
	}
	for e.leftPos < e.curPos && width >= visibleWidth {
		r := e.text[e.leftPos]
		if e.PasswordMode {
			width -= 1
		} else {
			width -= runewidth.RuneWidth(r)
		}
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
// SelectAll selects the entire text and sets the clear flag,
// so the next character typed will replace the content.
func (e *Edit) SelectAll() {
	if len(e.text) > 0 {
		e.selStart = 0
		e.selEnd = len(e.text)
		e.selAnchor = 0
		e.curPos = len(e.text)
		e.clearFlag = true
	}
}
func (e *Edit) GetData() any {
	return e.GetText()
}

func (e *Edit) SetData(val any) {
	if s, ok := val.(string); ok {
		e.SetText(s)
	}
}
func (e *Edit) WantsChars() bool {
	return true
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
	if event.Type == vtinput.PasteEventType {
		if event.PasteStart {
			e.pasting = true
			e.pasteBuffer = nil
		} else {
			e.pasting = false
			if len(e.pasteBuffer) > 0 {
				if e.selStart != -1 {
					e.DeleteBlock()
				}
				newText := make([]rune, 0, len(e.text)+len(e.pasteBuffer))
				newText = append(newText, e.text[:e.curPos]...)
				newText = append(newText, e.pasteBuffer...)
				newText = append(newText, e.text[e.curPos:]...)
				e.text = newText
				e.curPos += len(e.pasteBuffer)
				e.pasteBuffer = nil
			}
		}
		return true
	}

	if e.pasting {
		if event.Type == vtinput.KeyEventType && event.KeyDown {
			if event.Char != 0 {
				if event.Char == '\r' || event.Char == '\n' {
					e.pasteBuffer = append(e.pasteBuffer, ' ')
				} else {
					e.pasteBuffer = append(e.pasteBuffer, event.Char)
				}
			}
		}
		return true
	}

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
		case vtinput.VK_A:
			e.SelectAll()
			return true
		case vtinput.VK_E:
			e.HistoryUp()
			return true
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
			e.HistoryDown()
			return true
		case vtinput.VK_V:
			if text := GetClipboard(); text != "" {
				e.InsertString(text)
			}
			return true
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
		case vtinput.VK_INSERT:
			if text := GetClipboard(); text != "" {
				e.InsertString(text)
			}
			return true
		}
	}

	switch event.VirtualKeyCode {

	case vtinput.VK_RETURN:
		return e.FireAction(e.OnAction, nil)

	case vtinput.VK_LEFT:
		if e.curPos == 0 && !shift && !ctrl { return false } // Escape focus to previous
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
		if ctrl {
			for e.curPos > 0 && unicode.IsSpace(e.text[e.curPos-1]) { e.curPos-- }
			for e.curPos > 0 && !unicode.IsSpace(e.text[e.curPos-1]) { e.curPos-- }
		} else {
			if e.curPos > 0 { e.curPos-- }
		}
		if shift { e.endSelection() }
		e.clearFlag = false
		return true

	case vtinput.VK_RIGHT:
		if e.curPos == len(e.text) && !shift && !ctrl { return false } // Escape focus to next
		if shift { e.beginSelection() } else { e.selStart = -1; e.selAnchor = -1 }
		if ctrl {
			for e.curPos < len(e.text) && !unicode.IsSpace(e.text[e.curPos]) { e.curPos++ }
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
			e.SetText("")
			e.clearFlag = false
		}

		if e.selStart != -1 {
			e.DeleteBlock()
		}

		if e.overtype && e.curPos < len(e.text) {
			e.text[e.curPos] = event.Char
		} else {
			var testChar = event.Char
			// Auto-uppercase support for specific mask markers
			if e.Validator != nil {
				if mv, ok := e.Validator.(*MaskValidator); ok && e.curPos < len(mv.Mask) {
					m := []rune(mv.Mask)[e.curPos]
					if m == '&' || m == '!' {
						testChar = unicode.ToUpper(testChar)
					}
				}
			}

			newText := make([]rune, 0, len(e.text)+1)
			newText = append(newText, e.text[:e.curPos]...)
			newText = append(newText, testChar)
			newText = append(newText, e.text[e.curPos:]...)

			if e.Validator != nil && !e.Validator.IsValidInput(string(newText)) {
				return true // Swallow invalid input
			}
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
		// Bounds check to prevent panics from stale selection state
		if e.selStart < 0 { e.selStart = 0 }
		if e.selEnd > len(e.text) { e.selEnd = len(e.text) }
		if e.selStart > e.selEnd { e.selStart, e.selEnd = e.selEnd, e.selStart }

		e.text = append(e.text[:e.selStart], e.text[e.selEnd:]...)
		e.curPos = e.selStart
		e.selStart = -1
		e.selEnd = -1
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
	if e.HistoryID != "" && GlobalHistoryProvider != nil {
		e.History = GlobalHistoryProvider.LoadHistory(e.HistoryID)
	}
	if len(e.History) == 0 {
		return
	}
	menu := NewVMenu(Msg("vtui.History"))
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
	menu.OnAction = func(idx int) {
		e.SetText(e.History[idx])
		e.SetFocus(true)
		e.clearFlag = false
	}

	FrameManager.Push(menu)
}

// AddHistory adds a string to the beginning of the history, removing duplicates.
func (e *Edit) AddHistory(text string) {
	if text == "" {
		return
	}

	if e.DeduplicateHistory {
		newHist := make([]string, 0, len(e.History)+1)
		newHist = append(newHist, text)
		for _, h := range e.History {
			if h != text {
				newHist = append(newHist, h)
			}
		}
		e.History = newHist
	} else {
		// Shell-like behavior: only prevent consecutive duplicates
		if len(e.History) > 0 && e.History[0] == text {
			return
		}
		e.History = append([]string{text}, e.History...)
	}

	limit := e.HistoryLimit
	if limit <= 0 {
		limit = 32 // Fallback to a sane default
	}
	if len(e.History) > limit {
		e.History = e.History[:limit]
	}
	if e.HistoryID != "" && GlobalHistoryProvider != nil {
		GlobalHistoryProvider.SaveHistory(e.HistoryID, e.History)
	}
}
func (e *Edit) HistoryUp() {
	if len(e.History) == 0 { return }
	if e.HistoryPos < len(e.History)-1 {
		e.HistoryPos++
		e.SetText(e.History[e.HistoryPos])
	}
}

func (e *Edit) HistoryDown() {
	if e.HistoryPos > 0 {
		e.HistoryPos--
		e.SetText(e.History[e.HistoryPos])
	} else if e.HistoryPos == 0 {
		e.HistoryPos = -1
		e.SetText("")
	}
}

func (e *Edit) ProcessMouse(ev *vtinput.InputEvent) bool {
	if e.IsDisabled() { return false }
	if ev.KeyDown {
		if ev.ButtonState == vtinput.FromLeft1stButtonPressed {
			if e.ShowHistoryButton && int(ev.MouseX) == e.X2 && int(ev.MouseY) == e.Y1 {
				e.OpenHistory()
				return true
			}
		}
		// Middle-click to paste (standard TUI/Unix behavior)
		if ev.ButtonState == vtinput.FromLeft2ndButtonPressed {
			// This is a placeholder; real implementation would need to request
			// clipboard text asynchronously or via a bridge.
			return true
		}
	}
	return false
}
