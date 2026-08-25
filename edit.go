package vtui

import (
	"unicode"

	"github.com/unxed/vtinput"
)

type Edit struct {
	ScreenObject
	text               []rune
	curPos             int // Logical position in the runes string
	leftPos            int // Visual offset (scrolling)
	selStart           int // -1 if no selection
	selEnd             int
	selAnchor          int // Position where selection started
	overtype           bool
	clearFlag          bool // If true, first input will clear the text
	pasting            bool
	pasteBuffer        []rune
	PasswordMode       bool // Mask text with '*'
	HideCursor         bool // If true, suppress blinking cursor even when focused
	ShowHistoryButton  bool // Show a clickable [v] button
	History            []string
	HistoryPos         int
	HistoryLimit       int
	DeduplicateHistory bool
	Command            int
	OnAction           func()
	ColorTextIdx       int
	Validator          Validator
	ColorUnchangedIdx  int
	ColorSelectedIdx   int
	HistoryID          string
	// NoAutoComplete opts this field out of the completion menu, the
	// equivalent of Far's DIF_NOAUTOCOMPLETE. Far uses it for the editor's
	// go-to-line prompt, where a drop-down over a few digits is only in the
	// way.
	NoAutoComplete bool
	OnTextChange   func(string)
	// PathHintsEnabled lets the autocomplete menu ask PathHintProvider for
	// file path suggestions in addition to history matches.
	PathHintsEnabled  bool
	mouseSelecting    bool
	mouseSelectAnchor int
}

// HistoryProvider is an interface for external history persistence (e.g. from f4).
type HistoryProvider interface {
	LoadHistory(id string) []string
	SaveHistory(id string, history []string)
}

var GlobalHistoryProvider HistoryProvider

// AutoCompleteEnabled gates the completion menu that opens while typing.
// It mirrors Opt.Dialogs.AutoComplete in Far: a subtractive switch only. A
// field still has to qualify on its own -- history entries, or path hints
// with a provider installed, matching DIF_HISTORY and DIF_EDITPATH -- and
// turning this on can never bring the menu to a field that does not.
var AutoCompleteEnabled = true

// autoCompletes reports whether typing in this field may open the menu.
func (e *Edit) autoCompletes() bool {
	if !AutoCompleteEnabled || e.NoAutoComplete || e.IsDisabled() {
		return false
	}
	// A drop-down listing what was typed into a masked field would put the
	// secret on screen in clear text.
	if e.PasswordMode {
		return false
	}
	return len(e.History) > 0 || (e.PathHintsEnabled && PathHintProvider != nil)
}

// maybeOpenAutoComplete opens the completion menu after typed was inserted,
// when the field qualifies and anything matches. A field with history opens
// on any character, as in Far; a path-only field keeps its old behaviour of
// waiting for a separator, so that typing a long path does not rebuild the
// menu on every keystroke.
func (e *Edit) maybeOpenAutoComplete(typed rune) {
	if FrameManager == nil || !e.autoCompletes() {
		return
	}
	if len(e.History) == 0 && typed != '/' && typed != '\\' {
		return
	}
	if _, isAc := FrameManager.GetTopFrame().(*AutoCompleteMenu); isAc {
		return
	}
	if ac := NewAutoCompleteMenu(e); ac.HasMatches() {
		FrameManager.Push(ac)
	}
}

func NewEdit(x, y, width int, defaultText string) *Edit {
	e := &Edit{
		text:               []rune(defaultText),
		HistoryPos:         -1,
		selStart:           -1,
		selAnchor:          -1,
		clearFlag:          len(defaultText) > 0,
		ColorTextIdx:       ColDialogEdit,
		ColorUnchangedIdx:  ColDialogEditUnchanged,
		ColorSelectedIdx:   ColDialogEditSelected,
		HistoryLimit:       32,
		DeduplicateHistory: true,
	}
	e.canFocus = true
	e.curPos = len(e.text)
	e.SetPosition(x, y, x+width-1, y)
	if len(e.text) > 0 {
		e.SelectAll()
	}
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

	visibleWidth := e.X2 - e.X1 + 1
	if e.ShowHistoryButton {
		visibleWidth--
	}
	if visibleWidth < 1 {
		visibleWidth = 1
	}

	if DefaultBidiMode == BidiFull {
		cmap := e.caretMap()
		vPos := cmap.LogicalToVisual[e.curPos]
		if vPos < e.leftPos {
			e.leftPos = vPos
		}
		vis, _ := VisualStringWithRuneMap(string(e.text))
		width := 0
		vIdx := 0
		forEachTerminalCluster(vis, func(_ string, w, _, _ int) {
			if vIdx >= e.leftPos && vIdx < vPos {
				width += w
			}
			vIdx++
		})
		for e.leftPos < vPos && width >= visibleWidth {
			vIdx2 := 0
			forEachTerminalCluster(vis, func(_ string, w, _, _ int) {
				if vIdx2 == e.leftPos {
					width -= w
				}
				vIdx2++
			})
			e.leftPos++
		}
	} else {
		if e.curPos < e.leftPos {
			e.leftPos = e.curPos
		}
		width := 0
		for i := e.leftPos; i < e.curPos; i++ {
			r := e.text[i]
			if e.PasswordMode {
				width += 1
			} else {
				width += ClusterWidth(string(r))
			}
		}
		for e.leftPos < e.curPos && width >= visibleWidth {
			r := e.text[e.leftPos]
			if e.PasswordMode {
				width -= 1
			} else {
				width -= ClusterWidth(string(r))
			}
			e.leftPos++
		}
	}

	e.DisplayObject(scr)

	if e.IsFocused() && !e.HideCursor {
		scr.SetCursorVisible(true)
		if e.overtype {
			scr.SetCursorShape(CursorShapeBlock)
		} else {
			scr.SetCursorShape(CursorShapeUnderline)
		}
		vOffset := 0
		if DefaultBidiMode == BidiFull {
			cmap := e.caretMap()
			vPos := cmap.LogicalToVisual[e.curPos]
			vis, _ := VisualStringWithRuneMap(string(e.text))
			vIdx := 0
			forEachTerminalCluster(vis, func(_ string, w, _, _ int) {
				if vIdx >= vPos {
					return
				}
				if vIdx >= e.leftPos {
					vOffset += w
				}
				vIdx++
			})
		} else {
			headText := string(e.text[e.leftPos:e.curPos])
			vOffset = StringWidth(headText)
		}
		scr.SetCursorPos(e.X1+vOffset, e.Y1)
	}
}

func (e *Edit) caretMap() CaretMap {
	return BuildCaretMap(string(e.text))
}

// prevClusterBoundary and nextClusterBoundary must walk the *terminal*
// clusters, the same units DisplayObject paints and columnToLogicalPos
// resolves. ForEachClusterAt stops at the raw UAX #29 boundaries, which split
// an Indic virama from the consonant that follows it; the caret would then
// land inside a shaped glyph and Backspace/Delete would eat a fraction of a
// cell. See TEXTSEG.md and unxed/f4#546.
func (e *Edit) prevClusterBoundary(pos int) int {
	if pos <= 0 {
		return 0
	}
	s := string(e.text)
	lastBoundary := 0
	forEachTerminalCluster(s, func(cluster string, w, offset, runeIndex int) {
		if runeIndex < pos {
			lastBoundary = runeIndex
		}
	})
	return lastBoundary
}

func (e *Edit) nextClusterBoundary(pos int) int {
	s := string(e.text)
	nextBoundary := len(e.text)
	found := false
	forEachTerminalCluster(s, func(cluster string, w, offset, runeIndex int) {
		if !found && runeIndex > pos {
			nextBoundary = runeIndex
			found = true
		}
	})
	return nextBoundary
}

func (e *Edit) DisplayObject(scr *ScreenBuf) {
	if !e.IsVisible() {
		return
	}
	fullWidth := e.X2 - e.X1 + 1
	visibleWidth := fullWidth

	if e.ShowHistoryButton {
		visibleWidth--
	}

	defaultAttr := e.GetStateAttr(e.ColorTextIdx, e.ColorTextIdx)
	scr.FillRect(e.X1, e.Y1, e.X2, e.Y1, ' ', defaultAttr)

	type logicalCluster struct {
		text    string
		runeIdx int
		width   int
		attr    uint64
	}

	var logicalClusters []logicalCluster
	sText := string(e.text)
	forEachTerminalCluster(sText, func(clText string, width, _, runeIdx int) {

		attr := defaultAttr
		if e.selStart != -1 && runeIdx >= e.selStart && runeIdx < e.selEnd {
			selectedIdx := e.ColorSelectedIdx
			if e.clearFlag {
				selectedIdx = e.ColorUnchangedIdx
			}
			attr = e.GetStateAttr(selectedIdx, selectedIdx)
		}
		if e.IsDisabled() {
			attr = DimColor(attr)
		}

		logicalClusters = append(logicalClusters, logicalCluster{
			text:    clText,
			runeIdx: runeIdx,
			width:   width,
			attr:    attr,
		})
	})

	visualClusters := make([]logicalCluster, 0, len(logicalClusters))
	byRuneIndex := make(map[int]logicalCluster, len(logicalClusters))
	for _, c := range logicalClusters {
		byRuneIndex[c.runeIdx] = c
	}
	// The cells are painted in the order the line is read on screen, which
	// ForEachVisualCluster resolves with the full bidi algorithm; the
	// per-cluster attributes computed above travel with their clusters.
	ForEachVisualCluster(string(e.text), func(text string, _, _, runeIdx int) {
		if c, ok := byRuneIndex[runeIdx]; ok {
			c.text = text
			visualClusters = append(visualClusters, c)
		}
	})
	if len(visualClusters) != len(logicalClusters) {
		visualClusters = logicalClusters
	}

	currX := 0
	for i, c := range visualClusters {
		if i < e.leftPos {
			continue
		}
		w := c.width
		if currX+w > visibleWidth {
			break
		}

		if e.PasswordMode {
			scr.FillRect(e.X1+currX, e.Y1, e.X1+currX+w-1, e.Y1, '*', c.attr)
		} else {
			cells := AppendCluster(nil, c.text, w, c.attr)
			scr.Write(e.X1+currX, e.Y1, cells)
		}
		currX += w
	}

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
	e.leftPos = 0
	e.selStart = -1
	e.selAnchor = -1
	e.NotifyChange()
}

func (e *Edit) NotifyChange() {
	e.ScreenObject.NotifyChange()
	if FrameManager != nil && e.ID() != "" {
		FrameManager.emitEventSink(UIEvent{
			Kind:  "changed",
			SrcID: e.ID(),
			Value: PropValString(e.GetText()),
		})
	}
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
		e.SelectAll()
	}
}
func (e *Edit) WantsChars() bool {
	return true
}
func (e *Edit) SizeSpecH() SizeSpec {
	if e.sizeSpecH != nil {
		return *e.sizeSpecH
	}
	hint := 20
	textW := StringWidth(e.cleanText)
	if textW > hint {
		hint = textW
	}
	return SizeSpec{
		Hint:    hint,
		Min:     3,
		Policy:  PolicyExpanding,
		Stretch: 1,
	}
}

func (e *Edit) SizeSpecV() SizeSpec {
	if e.sizeSpecV != nil {
		return *e.sizeSpecV
	}
	return SizeSpec{
		Hint:    1,
		Min:     1,
		Policy:  PolicyFixed,
		Stretch: 1,
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
	if e.clearFlag {
		e.SetText("")
		e.ClearSelection()
	} else if e.selStart != -1 {
		e.DeleteBlock()
	}
	runes := []rune(text)
	newText := make([]rune, 0, len(e.text)+len(runes))
	newText = append(newText, e.text[:e.curPos]...)
	newText = append(newText, runes...)
	newText = append(newText, e.text[e.curPos:]...)
	e.text = newText
	e.curPos += len(runes)
	if e.OnTextChange != nil {
		e.OnTextChange(string(e.text))
	}
	e.NotifyChange()
}

func (e *Edit) ProcessKey(event *vtinput.InputEvent) bool {
	if event.Type == vtinput.KeyEventType && event.KeyDown {
		DebugLog("EDIT_DEBUG: ProcessKey entry. Char:%d, VK:0x%X, Mods:0x%X", event.Char, event.VirtualKeyCode, event.ControlKeyState)
	}
	if event.Type == vtinput.PasteEventType {
		if event.PasteStart {
			e.pasting = true
			e.pasteBuffer = nil
		} else {
			e.pasting = false
			if len(e.pasteBuffer) > 0 {
				var newText []rune
				var newCurPos int

				if e.clearFlag {
					newText = make([]rune, len(e.pasteBuffer))
					copy(newText, e.pasteBuffer)
					newCurPos = len(e.pasteBuffer)
					e.leftPos = 0
				} else if e.selStart != -1 {
					start, end := e.selStart, e.selEnd
					if start > end {
						start, end = end, start
					}
					newText = make([]rune, 0, len(e.text)-(end-start)+len(e.pasteBuffer))
					newText = append(newText, e.text[:start]...)
					newText = append(newText, e.pasteBuffer...)
					newText = append(newText, e.text[end:]...)
					newCurPos = start + len(e.pasteBuffer)
					if e.leftPos > start {
						e.leftPos = start
					}
				} else {
					newText = make([]rune, 0, len(e.text)+len(e.pasteBuffer))
					newText = append(newText, e.text[:e.curPos]...)
					newText = append(newText, e.pasteBuffer...)
					newText = append(newText, e.text[e.curPos:]...)
					newCurPos = e.curPos + len(e.pasteBuffer)
				}

				if e.Validator != nil && !e.Validator.IsValidInput(string(newText)) {
					e.pasteBuffer = nil
					return true
				}

				e.text = newText
				e.curPos = newCurPos
				e.ClearSelection()
				e.pasteBuffer = nil

				if e.OnTextChange != nil {
					e.OnTextChange(string(e.text))
				}
				e.NotifyChange()
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

	if !event.KeyDown {
		return false
	}
	if e.IsDisabled() {
		return false
	}

	// Navigation with selection reset or set
	DebugLog("  Edit.ProcessKey: VK=%s Char=%d", vtinput.VKString(event.VirtualKeyCode), event.Char)
	shift := (event.ControlKeyState & vtinput.ShiftPressed) != 0
	ctrl := (event.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (event.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0

	if ctrl && event.VirtualKeyCode == vtinput.VK_DOWN && len(e.History) > 0 {
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
		// zoin-bot: A full edit selection starts at the right edge. When
		// Ctrl+Shift+Left reaches the beginning of an undivided value, keep
		// that full selection so the next character replaces the value.
		fullSelectionAtEnd := e.selStart == 0 && e.selEnd == len(e.text) && e.curPos == len(e.text)
		isAtStart := false
		var cmap CaretMap
		// Keep Ctrl+Left/Right on the logical word-navigation path; see the
		// corresponding branch above.
		if DefaultBidiMode == BidiFull && !ctrl {
			cmap = e.caretMap()
			isAtStart = cmap.LogicalToVisual[e.curPos] == 0
		} else {
			isAtStart = e.curPos == 0
		}

		if isAtStart && !shift && !ctrl {
			return false // Escape focus to previous
		}

		if shift {
			e.beginSelection()
		} else {
			e.selStart = -1
			e.selAnchor = -1
		}

		// Plain arrows follow visual caret order in full bidi mode. Word
		// navigation is a logical-text operation, so Ctrl+Left/Right must
		// stay on the branch below even when full bidi input is enabled.
		if DefaultBidiMode == BidiFull && !ctrl {
			vPos := cmap.LogicalToVisual[e.curPos]
			if vPos > 0 {
				vPos--
				e.curPos = cmap.VisualToLogical[vPos]
			}
		} else {
			if ctrl {
				if e.curPos > 0 {
					e.curPos = e.prevClusterBoundary(e.curPos)
					if shift {
						e.endSelection()
					}
					for e.curPos > 0 {
						prev, curr := e.text[e.curPos-1], e.text[e.curPos]
						if stopBeforeRuneLeft(prev, curr, shift) {
							break
						}
						e.curPos = e.prevClusterBoundary(e.curPos)
						if shift {
							e.endSelection()
						}
					}
				}
			} else {
				if e.curPos > 0 {
					e.curPos = e.prevClusterBoundary(e.curPos)
				}
			}
		}
		if shift {
			e.endSelection()
			if ctrl && fullSelectionAtEnd && e.curPos == 0 {
				e.selStart = 0
				e.selEnd = len(e.text)
				e.selAnchor = 0
			}
		}
		e.clearFlag = false
		return true

	case vtinput.VK_RIGHT:
		isAtEnd := false
		var cmap CaretMap
		if DefaultBidiMode == BidiFull && !ctrl {
			cmap = e.caretMap()
			isAtEnd = cmap.LogicalToVisual[e.curPos] == len(cmap.VisualToLogical)-1
		} else {
			isAtEnd = e.curPos == len(e.text)
		}

		if isAtEnd && !shift && !ctrl {
			// Feature: if everything is selected and we are at the end,
			// just clear selection and stay in this field instead of losing focus.
			if e.selStart == 0 && e.selEnd == len(e.text) {
				e.selStart = -1
				e.selAnchor = -1
				e.clearFlag = false
				return true
			}
			return false // Escape focus to next
		}

		if shift {
			e.beginSelection()
		} else {
			e.selStart = -1
			e.selAnchor = -1
		}

		if DefaultBidiMode == BidiFull && !ctrl {
			vPos := cmap.LogicalToVisual[e.curPos]
			N := len(cmap.VisualToLogical) - 1
			if vPos < N {
				vPos++
				e.curPos = cmap.VisualToLogical[vPos]
			}
		} else {
			if ctrl {
				if e.curPos < len(e.text) {
					e.curPos = e.nextClusterBoundary(e.curPos)
					if shift {
						e.endSelection()
					}
					for e.curPos < len(e.text) {
						prev, curr := e.text[e.curPos-1], e.text[e.curPos]
						if stopBeforeRuneRight(prev, curr, shift) {
							break
						}
						e.curPos = e.nextClusterBoundary(e.curPos)
						if shift {
							e.endSelection()
						}
					}
				}
			} else {
				if e.curPos < len(e.text) {
					e.curPos = e.nextClusterBoundary(e.curPos)
				}
			}
		}
		if shift {
			e.endSelection()
		}
		e.clearFlag = false
		return true

	case vtinput.VK_HOME:
		if shift {
			e.beginSelection()
		} else {
			e.selStart = -1
			e.selAnchor = -1
		}
		if DefaultBidiMode == BidiFull {
			cmap := e.caretMap()
			e.curPos = cmap.VisualToLogical[0]
		} else {
			e.curPos = 0
		}
		if shift {
			e.endSelection()
		}
		e.clearFlag = false
		return true

	case vtinput.VK_END:
		if shift {
			e.beginSelection()
		} else {
			e.selStart = -1
			e.selAnchor = -1
		}
		if DefaultBidiMode == BidiFull {
			cmap := e.caretMap()
			N := len(cmap.VisualToLogical) - 1
			e.curPos = cmap.VisualToLogical[N]
		} else {
			e.curPos = len(e.text)
		}
		if shift {
			e.endSelection()
		}
		e.clearFlag = false
		return true

	case vtinput.VK_BACK:
		if e.clearFlag {
			e.SetText("")
			e.ClearSelection()
		} else if e.selStart != -1 {
			e.DeleteBlock()
		} else if DefaultBidiMode == BidiFull {
			cmap := e.caretMap()
			vPos := cmap.LogicalToVisual[e.curPos]
			if vPos > 0 {
				r1 := cmap.VisualToLogical[vPos-1]
				r2 := cmap.VisualToLogical[vPos]
				start, end := r1, r2
				if start > end {
					start, end = end, start
				}
				start = backspaceStart(e.text, start, end)
				e.text = append(e.text[:start], e.text[end:]...)
				e.curPos = start
			}
		} else if e.curPos > 0 {
			prevBoundary := backspaceStart(e.text, e.prevClusterBoundary(e.curPos), e.curPos)
			e.text = append(e.text[:prevBoundary], e.text[e.curPos:]...)
			e.curPos = prevBoundary
		}
		e.clearFlag = false
		if e.OnTextChange != nil {
			e.OnTextChange(string(e.text))
		}
		e.NotifyChange()
		return true

	case vtinput.VK_DELETE:
		if e.clearFlag {
			e.SetText("")
			e.ClearSelection()
		} else if e.selStart != -1 {
			e.DeleteBlock()
		} else if DefaultBidiMode == BidiFull {
			cmap := e.caretMap()
			vPos := cmap.LogicalToVisual[e.curPos]
			N := len(cmap.VisualToLogical) - 1
			if vPos < N {
				r1 := cmap.VisualToLogical[vPos]
				r2 := cmap.VisualToLogical[vPos+1]
				start, end := r1, r2
				if start > end {
					start, end = end, start
				}
				e.text = append(e.text[:start], e.text[end:]...)
				e.curPos = cmap.VisualToLogical[vPos]
			}
		} else if e.curPos < len(e.text) {
			nextBoundary := e.nextClusterBoundary(e.curPos)
			e.text = append(e.text[:e.curPos], e.text[nextBoundary:]...)
		}
		e.clearFlag = false
		if e.OnTextChange != nil {
			e.OnTextChange(string(e.text))
		}
		e.NotifyChange()
		return true

	case vtinput.VK_INSERT:
		// Toggle overtype mode only if no modifiers are pressed
		if !shift && !ctrl && !alt {
			e.overtype = !e.overtype
		}
		return true
	}

	// Text input
	if event.Char != 0 && (unicode.IsGraphic(event.Char) || event.Char == ' ') {
		DebugLog("EDIT_TRACE: Candidate for text input: '%c' (%d), VK: 0x%X, Mods: 0x%X", event.Char, event.Char, event.VirtualKeyCode, event.ControlKeyState)
		// When checking modifiers, ignore Lock keys (Num, Caps, Scroll),
		// because they should not block text input.
		mods := event.ControlKeyState & ^vtinput.ControlKeyState(vtinput.NumLockOn|vtinput.CapsLockOn|vtinput.ScrollLockOn|vtinput.EnhancedKey)
		if (mods & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0 {
			return false
		}

		DebugLog("    Edit: Typing char %d", event.Char)

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

		var newText []rune
		var newCurPos int

		if e.clearFlag {
			newText = []rune{testChar}
			newCurPos = 1
			e.leftPos = 0
		} else if e.selStart != -1 {
			start, end := e.selStart, e.selEnd
			if start > end {
				start, end = end, start
			}
			newText = make([]rune, 0, len(e.text)-(end-start)+1)
			newText = append(newText, e.text[:start]...)
			newText = append(newText, testChar)
			newText = append(newText, e.text[end:]...)
			newCurPos = start + 1
			if e.leftPos > start {
				e.leftPos = start
			}
		} else if e.overtype && e.curPos < len(e.text) {
			newText = make([]rune, len(e.text))
			copy(newText, e.text)
			newText[e.curPos] = testChar
			newCurPos = e.curPos + 1
		} else {
			newText = make([]rune, 0, len(e.text)+1)
			newText = append(newText, e.text[:e.curPos]...)
			newText = append(newText, testChar)
			newText = append(newText, e.text[e.curPos:]...)
			newCurPos = e.curPos + 1
		}

		if e.Validator != nil && !e.Validator.IsValidInput(string(newText)) {
			return true // Swallow invalid input
		}

		e.text = newText
		e.curPos = newCurPos
		e.ClearSelection()

		DebugLog("EDIT_DEBUG: Character ACCEPTED. New text len: %d", len(e.text))
		if e.OnTextChange != nil {
			e.OnTextChange(string(e.text))
		}
		e.NotifyChange()
		e.maybeOpenAutoComplete(testChar)
		return true
	}

	DebugLog("EDIT_DEBUG: Key REJECTED or not handled as text.")
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

// ClearSelection removes any active text selection and resets the clear flag.
func (e *Edit) ClearSelection() {
	e.selStart = -1
	e.selEnd = -1
	e.selAnchor = -1
	e.clearFlag = false
}

func (e *Edit) DeleteBlock() {
	if e.selStart != -1 {
		// Bounds check to prevent panics from stale selection state
		if e.selStart < 0 {
			e.selStart = 0
		}
		if e.selEnd > len(e.text) {
			e.selEnd = len(e.text)
		}
		if e.selStart > e.selEnd {
			e.selStart, e.selEnd = e.selEnd, e.selStart
		}

		e.text = append(e.text[:e.selStart], e.text[e.selEnd:]...)
		e.curPos = e.selStart
		e.ClearSelection()
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
	menu.BoxType = SingleBox
	for _, h := range e.History {
		menu.AddItem(MenuItem{Text: h})
	}

	h := len(e.History) + 2
	if h > 10 {
		h = 10
	}

	// Calculate width: at least the width of the input field, but max 50
	w := e.X2 - e.X1 + 1
	if w < 20 {
		w = 20
	}
	if w > 50 {
		w = 50
	}

	// Positioning logic
	scrH := 25
	if FrameManager.scr != nil {
		scrH = FrameManager.scr.height
	}

	y := e.Y1 + 1
	if y+h > scrH {
		y = e.Y1 - h // Flip upwards if no space below
	}

	menu.SetPosition(e.X1, y, e.X1+w-1, y+h-1)

	menu.SetOwner(e)
	menu.OnAction = func(idx int) {
		e.SetText(e.History[idx])
		e.SetFocus(true)
		e.clearFlag = false
		e.HistoryPos = -1
		// Auto-execute on mouse selection (matches Far behavior)
		if FrameManager != nil {
			FrameManager.InjectEvents([]*vtinput.InputEvent{
				{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN},
			})
		}
	}

	menu.OnKeyDown = func(ev *vtinput.InputEvent) bool {
		// Handle deleting items from history
		if ev.VirtualKeyCode == vtinput.VK_DELETE || ev.VirtualKeyCode == vtinput.VK_BACK {
			if len(menu.Items) == 0 {
				return true
			}
			idx := menu.SelectPos
			e.History = append(e.History[:idx], e.History[idx+1:]...)
			if e.HistoryID != "" && GlobalHistoryProvider != nil {
				GlobalHistoryProvider.SaveHistory(e.HistoryID, e.History)
			}
			menu.Items = append(menu.Items[:idx], menu.Items[idx+1:]...)
			menu.ItemCount = len(menu.Items)

			if menu.SelectPos >= menu.ItemCount && menu.ItemCount > 0 {
				menu.SetSelectPos(menu.ItemCount - 1)
			} else if menu.ItemCount > 0 {
				menu.SetSelectPos(menu.SelectPos) // Refresh view
			}

			if menu.ItemCount == 0 {
				menu.Close()
			}
			FrameManager.Redraw()
			return true
		}

		// Handle Enter (Execute) vs Shift+Enter (Insert only)
		if ev.VirtualKeyCode == vtinput.VK_RETURN {
			if len(menu.Items) == 0 {
				return true
			}
			shift := (ev.ControlKeyState & vtinput.ShiftPressed) != 0
			idx := menu.SelectPos
			e.SetText(e.History[idx])
			e.SetFocus(true)
			e.clearFlag = false
			menu.Close()

			if !shift {
				// Inject a real Enter event so the parent frame handles execution
				FrameManager.InjectEvents([]*vtinput.InputEvent{ev})
			}
			return true
		}
		return false
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
	if len(e.History) == 0 {
		return
	}
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
	if e.IsDisabled() {
		return false
	}
	if e.mouseSelecting {
		if ev.ButtonState == 0 {
			e.mouseSelecting = false
			return true
		}
		if ev.ButtonState&vtinput.FromLeft1stButtonPressed != 0 {
			e.curPos = e.cursorPositionAtX(int(ev.MouseX))
			e.selAnchor = e.mouseSelectAnchor
			if e.curPos < e.selAnchor {
				e.selStart, e.selEnd = e.curPos, e.selAnchor
			} else {
				e.selStart, e.selEnd = e.selAnchor, e.curPos
			}
			if e.selStart == e.selEnd {
				e.ClearSelection()
			}
			return true
		}
	}
	if ev.KeyDown {
		if ev.ButtonState == vtinput.FromLeft1stButtonPressed {
			if e.ShowHistoryButton && int(ev.MouseX) == e.X2 && int(ev.MouseY) == e.Y1 {
				e.OpenHistory()
				return true
			}
			if e.HitTest(int(ev.MouseX), int(ev.MouseY)) {
				e.curPos = e.cursorPositionAtX(int(ev.MouseX))
				if ev.MouseEventFlags&TripleClick != 0 {
					e.SelectAll()
					return true
				}
				if ev.MouseEventFlags&vtinput.DoubleClick != 0 {
					e.selectWordAtCursor()
					return true
				}
				e.ClearSelection()
				e.clearFlag = false
				e.mouseSelecting = true
				e.mouseSelectAnchor = e.curPos
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

func (e *Edit) selectWordAtCursor() {
	if e.curPos < 0 || e.curPos >= len(e.text) {
		e.ClearSelection()
		return
	}

	category := getCharCategory(e.text[e.curPos])
	start, end := e.curPos, e.curPos+1
	for start > 0 && getCharCategory(e.text[start-1]) == category {
		start--
	}
	for end < len(e.text) && getCharCategory(e.text[end]) == category {
		end++
	}
	e.selStart, e.selEnd = start, end
	e.selAnchor = start
	e.curPos = end
	e.clearFlag = false
}

// WordUnderCursor returns the whitespace-bounded token around the cursor as
// a rune span [from, to) plus its text. The boundary rule is word_nav's
// character classification: only the space class (space/tab) terminates the
// token, so path separators and other divider characters stay inside it.
func (e *Edit) WordUnderCursor() (from, to int, text string) {
	cur := e.curPos
	if cur < 0 {
		cur = 0
	}
	if cur > len(e.text) {
		cur = len(e.text)
	}
	from = cur
	for from > 0 && getCharCategory(e.text[from-1]) != catSpace {
		from--
	}
	to = cur
	for to < len(e.text) && getCharCategory(e.text[to]) != catSpace {
		to++
	}
	return from, to, string(e.text[from:to])
}

func (e *Edit) cursorPositionAtX(x int) int {
	if x < e.X1 {
		return 0
	}
	if x == e.X1 {
		return e.leftPos
	}
	visibleRight := e.X2
	if e.ShowHistoryButton {
		visibleRight--
	}
	if x > visibleRight {
		return len(e.text)
	}

	column := x - e.X1
	currX := 0
	result := len(e.text)
	found := false

	if DefaultBidiMode == BidiFull {
		cmap := e.caretMap()
		vis, _ := VisualStringWithRuneMap(string(e.text))
		vIdx := 0
		forEachTerminalCluster(vis, func(_ string, w, _, _ int) {
			if found || vIdx < e.leftPos {
				vIdx++
				return
			}
			if currX+w > column {
				result = cmap.VisualToLogical[vIdx]
				found = true
				return
			}
			currX += w
			vIdx++
		})
		if !found {
			result = cmap.VisualToLogical[vIdx]
		}
	} else {
		forEachTerminalCluster(string(e.text), func(cluster string, w, _, runeIndex int) {
			if found || runeIndex < e.leftPos {
				return
			}
			if currX+w > column {
				result = runeIndex
				found = true
				return
			}
			currX += w
		})
	}
	return result
}
