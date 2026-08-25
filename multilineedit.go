package vtui

import (
	"strings"
	"unicode"

	"github.com/unxed/vtinput"
)

// MultiLineEdit is a rectangular text field with multiple visible lines.
// It's the natural companion to Edit for dialog fields whose content
// spans several lines (a stack of commands, a description paragraph,
// etc.). Model: lines are stored as [][]rune, one row per string; the
// cursor is (curRow, curCol) in logical rune coordinates. Rendering and
// navigation use grapheme clusters, and BidiFull maps the logical cursor to
// the visual order shown on screen. Rendering scrolls horizontally per
// active row and vertically over the whole buffer.
//
// Not in scope for the initial cut:
//   - overtype / undo / redo
//   - word wrap (long lines scroll horizontally on the current row only)
//
// Ctrl+V / Shift+Ins paste (splitting on \n) IS supported so users can
// bring in multiline chunks from the system clipboard.
type MultiLineEdit struct {
	ScreenObject
	lines        [][]rune
	curRow       int
	curCol       int
	leftPos      int // horizontal scroll for the current row
	topPos       int // vertical scroll for the whole buffer
	pasting      bool
	selActive    bool
	selStartRow  int
	selStartCol  int
	pasteBuffer  []rune
	OnTextChange func(string)
	// ColorTextIdx allows callers to override the default palette entry
	// (e.g. dim inactive field). Falls back to ColDialogEdit.
	ColorTextIdx int
}

// NewMultiLineEdit builds a new multiline edit control anchored at (x, y)
// with the given visible dimensions (in cells). The default text is split
// by "\n"; an empty string yields a single empty row.
func NewMultiLineEdit(x, y, width, height int, defaultText string) *MultiLineEdit {
	m := &MultiLineEdit{
		ColorTextIdx: ColDialogEdit,
	}
	m.canFocus = true
	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}
	m.SetPosition(x, y, x+width-1, y+height-1)
	m.SetText(defaultText)
	return m
}

// GetText returns the buffer content joined with "\n".
func (m *MultiLineEdit) GetText() string {
	parts := make([]string, len(m.lines))
	for i, line := range m.lines {
		parts[i] = string(line)
	}
	return strings.Join(parts, "\n")
}

// SetText replaces the entire buffer with the given text, splitting on
// "\n". An empty text becomes a single empty row so the cursor always
// has a valid position.
func (m *MultiLineEdit) SetText(text string) {
	parts := strings.Split(text, "\n")
	m.lines = make([][]rune, len(parts))
	for i, p := range parts {
		m.lines[i] = []rune(p)
	}
	if len(m.lines) == 0 {
		m.lines = [][]rune{nil}
	}
	m.curRow = 0
	m.curCol = 0
	m.leftPos = 0
	m.topPos = 0
	m.selActive = false
	m.notifyChange()
}

// GetLines returns a copy of the buffer as a slice of strings.
func (m *MultiLineEdit) GetLines() []string {
	out := make([]string, len(m.lines))
	for i, line := range m.lines {
		out[i] = string(line)
	}
	return out
}

// SetLines replaces the buffer with the given rows.
func (m *MultiLineEdit) SetLines(lines []string) {
	if len(lines) == 0 {
		m.lines = [][]rune{nil}
	} else {
		m.lines = make([][]rune, len(lines))
		for i, s := range lines {
			m.lines[i] = []rune(s)
		}
	}
	m.curRow = 0
	m.curCol = 0
	m.leftPos = 0
	m.topPos = 0
	m.selActive = false
	m.notifyChange()
}

// GetData / SetData let dialogs treat the widget as a DataControl.
func (m *MultiLineEdit) GetData() any { return m.GetText() }
func (m *MultiLineEdit) SetData(v any) {
	if s, ok := v.(string); ok {
		m.SetText(s)
	}
}

// WantsChars — dialog input routing sends printable characters here
// only when this returns true.
func (m *MultiLineEdit) WantsChars() bool { return true }

// notifyChange fires OnTextChange (used by callers to enable/disable
// buttons, etc.). Also mirrors ScreenObject.NotifyChange for owners.
func (m *MultiLineEdit) notifyChange() {
	if m.OnTextChange != nil {
		m.OnTextChange(m.GetText())
	}
	m.NotifyChange()
}

func (m *MultiLineEdit) viewHeight() int {
	h := m.Y2 - m.Y1 + 1
	if h < 1 {
		return 1
	}
	return h
}

func (m *MultiLineEdit) viewWidth() int {
	w := m.X2 - m.X1 + 1
	if w < 1 {
		return 1
	}
	return w
}

type multiLineCluster struct {
	text       string
	width      int
	logicalPos int
	logicalEnd int
}

func logicalClusters(runes []rune) []multiLineCluster {
	text := string(runes)
	clusters := make([]multiLineCluster, 0, len(runes))
	forEachTerminalCluster(text, func(cluster string, width, _, runeIndex int) {
		clusters = append(clusters, multiLineCluster{
			text:       cluster,
			width:      width,
			logicalPos: runeIndex,
			logicalEnd: runeIndex + len([]rune(cluster)),
		})
	})
	return clusters
}

// visualClusters returns the display order. BidiDisplay deliberately keeps
// the old logical cursor semantics; BidiFull is the mode that makes editing
// follow the visual order too.
func visualClusters(runes []rune) []multiLineCluster {
	logical := logicalClusters(runes)
	if DefaultBidiMode != BidiFull || !HasRTL(string(runes)) {
		return logical
	}

	byLogical := make(map[int]multiLineCluster, len(logical))
	for _, cluster := range logical {
		byLogical[cluster.logicalPos] = cluster
	}
	visualText, logicalPositions := VisualStringWithRuneMap(string(runes))
	visual := make([]multiLineCluster, 0, len(logical))
	index := 0
	forEachTerminalCluster(visualText, func(cluster string, width, _, _ int) {
		if index >= len(logicalPositions) {
			return
		}
		logicalPos := logicalPositions[index]
		original, ok := byLogical[logicalPos]
		if !ok {
			index++
			return
		}
		original.text = cluster
		original.width = width
		visual = append(visual, original)
		index++
	})
	return visual
}

func (m *MultiLineEdit) currentVisualPos() int {
	line := m.lines[m.curRow]
	if DefaultBidiMode == BidiFull && HasRTL(string(line)) {
		caret := BuildCaretMap(string(line))
		if m.curCol >= 0 && m.curCol < len(caret.LogicalToVisual) {
			return caret.LogicalToVisual[m.curCol]
		}
	}
	clusters := visualClusters(line)
	for i, cluster := range clusters {
		if cluster.logicalPos >= m.curCol {
			return i
		}
	}
	return len(clusters)
}

func (m *MultiLineEdit) visualPosToLogical(visualPos int) int {
	line := m.lines[m.curRow]
	if DefaultBidiMode == BidiFull && HasRTL(string(line)) {
		caret := BuildCaretMap(string(line))
		if visualPos >= 0 && visualPos < len(caret.VisualToLogical) {
			return caret.VisualToLogical[visualPos]
		}
	}
	clusters := visualClusters(line)
	if visualPos <= 0 || len(clusters) == 0 {
		return 0
	}
	if visualPos >= len(clusters) {
		return len(line)
	}
	return clusters[visualPos].logicalPos
}

func (m *MultiLineEdit) visualColumn() int {
	clusters := visualClusters(m.lines[m.curRow])
	pos := m.currentVisualPos()
	if pos > len(clusters) {
		pos = len(clusters)
	}
	width := 0
	for _, cluster := range clusters[:pos] {
		width += cluster.width
	}
	return width
}

// ensureVisible scrolls topPos / leftPos so the cursor is on screen.
// Called after every mutation or navigation.
func (m *MultiLineEdit) ensureVisible() {
	if m.curRow < 0 {
		m.curRow = 0
	}
	if m.curRow >= len(m.lines) {
		m.curRow = len(m.lines) - 1
	}
	line := m.lines[m.curRow]
	if m.curCol < 0 {
		m.curCol = 0
	}
	if m.curCol > len(line) {
		m.curCol = len(line)
	}

	// Vertical scroll: keep curRow inside [topPos, topPos+viewHeight-1].
	vh := m.viewHeight()
	if m.curRow < m.topPos {
		m.topPos = m.curRow
	} else if m.curRow >= m.topPos+vh {
		m.topPos = m.curRow - vh + 1
	}
	if m.topPos < 0 {
		m.topPos = 0
	}

	// Horizontal scroll for the CURRENT row only. leftPos is a visual
	// grapheme-cluster index, not a logical rune index.
	vw := m.viewWidth()
	clusters := visualClusters(line)
	if m.leftPos > len(clusters) {
		m.leftPos = len(clusters)
	}
	visualPos := m.currentVisualPos()
	if visualPos < m.leftPos {
		m.leftPos = visualPos
	}
	width := 0
	for _, cluster := range clusters[m.leftPos:visualPos] {
		width += cluster.width
	}
	for m.leftPos < visualPos && width >= vw {
		width -= clusters[m.leftPos].width
		m.leftPos++
	}
}

// Show paints the widget onto scr.
func (m *MultiLineEdit) Show(scr *ScreenBuf) {
	m.ScreenObject.Show(scr)
	m.ensureVisible()
	m.DisplayObject(scr)
	if m.IsFocused() {
		scr.SetCursorVisible(true)
		scr.SetCursorShape(CursorShapeUnderline)
		off := 0
		clusters := visualClusters(m.lines[m.curRow])
		visualPos := m.currentVisualPos()
		for _, cluster := range clusters[m.leftPos:visualPos] {
			off += cluster.width
		}
		scr.SetCursorPos(m.X1+off, m.Y1+m.curRow-m.topPos)
	}
}

// DisplayObject fills the widget rectangle with background attribute
// and renders each visible row starting at leftPos.
func (m *MultiLineEdit) DisplayObject(scr *ScreenBuf) {
	if !m.IsVisible() {
		return
	}
	attr := m.GetStateAttr(m.ColorTextIdx, m.ColorTextIdx)
	if m.IsDisabled() {
		attr = DimColor(attr)
	}
	scr.FillRect(m.X1, m.Y1, m.X2, m.Y2, ' ', attr)

	selAttr := Palette[ColDialogEditSelected]
	vw := m.viewWidth()
	vh := m.viewHeight()
	for row := 0; row < vh; row++ {
		idx := m.topPos + row
		if idx >= len(m.lines) {
			break
		}
		clusters := visualClusters(m.lines[idx])
		start := m.leftPos
		if start > len(clusters) {
			continue
		}
		x := 0
		for i := start; i < len(clusters); i++ {
			cluster := clusters[i]
			w := cluster.width
			if x+w > vw {
				break
			}
			cAttr := attr
			if m.isSelectedRange(idx, cluster.logicalPos, cluster.logicalEnd) {
				cAttr = selAttr
			}
			scr.Write(m.X1+x, m.Y1+row, AppendCluster(nil, cluster.text, w, cAttr))
			x += w
		}
	}
}

// ProcessKey handles navigation, insertion, deletion and paste. Returns
// true when the event was consumed so the surrounding dialog knows not
// to reroute it.
func (m *MultiLineEdit) ProcessKey(event *vtinput.InputEvent) bool {
	if event.Type == vtinput.PasteEventType {
		if event.PasteStart {
			m.pasting = true
			m.pasteBuffer = m.pasteBuffer[:0]
		} else {
			m.pasting = false
			if len(m.pasteBuffer) > 0 {
				m.insertString(string(m.pasteBuffer))
			}
			m.pasteBuffer = nil
		}
		return true
	}
	if m.pasting {
		if event.Type == vtinput.KeyEventType && event.KeyDown && event.Char != 0 {
			m.pasteBuffer = append(m.pasteBuffer, event.Char)
		}
		return true
	}
	if !event.KeyDown {
		return false
	}

	ctrl := (event.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0
	alt := (event.ControlKeyState & (vtinput.LeftAltPressed | vtinput.RightAltPressed)) != 0
	shift := (event.ControlKeyState & vtinput.ShiftPressed) != 0

	// Clipboard: Ctrl+V and Shift+Ins paste (may span multiple lines).
	if ctrl && !alt && !shift && event.VirtualKeyCode == vtinput.VK_V {
		if text := GetClipboard(); text != "" {
			m.insertString(text)
		}
		return true
	}
	if shift && !ctrl && !alt && event.VirtualKeyCode == vtinput.VK_INSERT {
		if text := GetClipboard(); text != "" {
			m.insertString(text)
		}
		return true
	}

	if ctrl && !alt && !shift && event.VirtualKeyCode == vtinput.VK_A {
		m.SelectAll()
		return true
	}
	if ctrl && !alt && !shift && (event.VirtualKeyCode == vtinput.VK_C || event.VirtualKeyCode == vtinput.VK_INSERT) {
		if text := m.CopySelection(); text != "" {
			SetClipboard(text)
		}
		return true
	}

	switch event.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if alt {
			return false
		}
		m.handleNav(shift)
		if ctrl {
			m.wordLeft(shift)
			m.ensureVisible()
			return true
		}
		visualPos := m.currentVisualPos()
		if DefaultBidiMode == BidiFull && HasRTL(string(m.lines[m.curRow])) && visualPos > 0 {
			m.curCol = m.visualPosToLogical(visualPos - 1)
		} else if m.curCol > 0 {
			m.curCol = previousClusterBoundary(m.lines[m.curRow], m.curCol)
		} else if m.curRow > 0 {
			m.curRow--
			m.curCol = len(m.lines[m.curRow])
		}
		m.ensureVisible()
		return true
	case vtinput.VK_RIGHT:
		if alt {
			return false
		}
		m.handleNav(shift)
		if ctrl {
			m.wordRight(shift)
			m.ensureVisible()
			return true
		}
		line := m.lines[m.curRow]
		clusters := visualClusters(line)
		visualPos := m.currentVisualPos()
		if DefaultBidiMode == BidiFull && HasRTL(string(line)) && visualPos < len(clusters) {
			m.curCol = m.visualPosToLogical(visualPos + 1)
		} else if m.curCol < len(line) {
			m.curCol = nextClusterBoundary(line, m.curCol)
		} else if m.curRow+1 < len(m.lines) {
			m.curRow++
			m.curCol = 0
		}
		m.ensureVisible()
		return true
	case vtinput.VK_UP:
		if ctrl || alt {
			return false
		}
		m.handleNav(shift)
		if m.curRow > 0 {
			m.curRow--
			m.ensureVisible()
		}
		return true
	case vtinput.VK_DOWN:
		if ctrl || alt {
			return false
		}
		m.handleNav(shift)
		if m.curRow+1 < len(m.lines) {
			m.curRow++
			m.ensureVisible()
		}
		return true
	case vtinput.VK_HOME:
		if alt {
			return false
		}
		m.handleNav(shift)
		if ctrl {
			m.curRow = 0
		}
		m.curCol = 0
		m.ensureVisible()
		return true
	case vtinput.VK_END:
		if alt {
			return false
		}
		m.handleNav(shift)
		if ctrl {
			m.curRow = len(m.lines) - 1
		}
		m.curCol = len(m.lines[m.curRow])
		m.ensureVisible()
		return true
	case vtinput.VK_PRIOR:
		if ctrl || alt {
			return false
		}
		m.handleNav(shift)
		step := m.viewHeight()
		m.curRow -= step
		m.topPos -= step
		m.ensureVisible()
		return true
	case vtinput.VK_NEXT:
		if ctrl || alt {
			return false
		}
		m.handleNav(shift)
		step := m.viewHeight()
		m.curRow += step
		m.topPos += step
		m.ensureVisible()
		return true
	case vtinput.VK_RETURN:
		if ctrl || alt || shift {
			// Ctrl+Enter / Shift+Enter / Alt+Enter are for the surrounding
			// dialog (e.g. default button, insert-something menu). Let it
			// through.
			return false
		}
		m.splitLine()
		return true
	case vtinput.VK_BACK:
		if ctrl || alt || shift {
			return false
		}
		m.backspace()
		return true
	case vtinput.VK_DELETE:
		if ctrl || alt || shift {
			return false
		}
		m.deleteForward()
		return true
	}

	// Printable character insertion — only when no Ctrl/Alt is held so we
	// don't swallow dialog shortcuts (Alt+letter hotkeys, etc.).
	if event.Char != 0 && !ctrl && !alt && unicode.IsPrint(event.Char) {
		m.insertRune(event.Char)
		return true
	}
	return false
}

// ProcessMouse repositions the cursor on left-click inside the widget.
// Wheel-scroll adjusts topPos.
func (m *MultiLineEdit) ProcessMouse(event *vtinput.InputEvent) bool {
	if event.Type != vtinput.MouseEventType {
		return false
	}
	if event.WheelDirection != 0 {
		if event.WheelDirection > 0 && m.topPos > 0 {
			m.topPos -= WheelLinesPerNotch()
			if m.topPos < 0 {
				m.topPos = 0
			}
		} else if event.WheelDirection < 0 && m.topPos+1 <= len(m.lines)-1 {
			m.topPos += WheelLinesPerNotch()
			if maxTop := len(m.lines) - 1; m.topPos > maxTop {
				m.topPos = maxTop
			}
		}
		return true
	}
	if event.ButtonState == vtinput.FromLeft1stButtonPressed && event.KeyDown {
		x := int(event.MouseX) - m.X1
		y := int(event.MouseY) - m.Y1
		if x < 0 || y < 0 || x >= m.viewWidth() || y >= m.viewHeight() {
			return false
		}
		row := m.topPos + y
		if row >= len(m.lines) {
			row = len(m.lines) - 1
		}
		m.curRow = row
		// Walk the row measuring widths until we reach x.
		line := m.lines[m.curRow]
		clusters := visualClusters(line)
		visualPos := m.leftPos
		acc := 0
		for visualPos < len(clusters) && acc+clusters[visualPos].width <= x {
			acc += clusters[visualPos].width
			visualPos++
		}
		m.curCol = m.visualPosToLogical(visualPos)
		m.ensureVisible()
		return true
	}
	return false
}

// --- mutations ---

// insertRune inserts r at the cursor and advances curCol.
func (m *MultiLineEdit) insertRune(r rune) {
	if m.selActive {
		m.DeleteSelection()
	}
	line := m.lines[m.curRow]
	newLine := make([]rune, 0, len(line)+1)
	newLine = append(newLine, line[:m.curCol]...)
	newLine = append(newLine, r)
	newLine = append(newLine, line[m.curCol:]...)
	m.lines[m.curRow] = newLine
	m.curCol++
	m.ensureVisible()
	m.notifyChange()
}

// insertString inserts text at the cursor, splitting on "\n".
func (m *MultiLineEdit) insertString(text string) {
	if text == "" {
		return
	}
	if m.selActive {
		m.DeleteSelection()
	}
	// Normalize CRLF and LF.
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	parts := strings.Split(text, "\n")

	line := m.lines[m.curRow]
	head := append([]rune{}, line[:m.curCol]...)
	tail := append([]rune{}, line[m.curCol:]...)

	// First fragment glues to the current row's head.
	first := []rune(parts[0])
	head = append(head, first...)

	if len(parts) == 1 {
		// Single-line paste — put tail back on the same row.
		m.lines[m.curRow] = append(head, tail...)
		m.curCol = len(head)
		m.ensureVisible()
		m.notifyChange()
		return
	}

	// Last fragment gets the original tail appended.
	last := append([]rune(parts[len(parts)-1]), tail...)
	// Middle fragments are their own rows.
	middle := make([][]rune, 0, len(parts)-1)
	for i := 1; i < len(parts)-1; i++ {
		middle = append(middle, []rune(parts[i]))
	}

	newLines := make([][]rune, 0, len(m.lines)+len(parts)-1)
	newLines = append(newLines, m.lines[:m.curRow]...)
	newLines = append(newLines, head)
	newLines = append(newLines, middle...)
	newLines = append(newLines, last)
	newLines = append(newLines, m.lines[m.curRow+1:]...)
	m.lines = newLines
	m.curRow = m.curRow + len(parts) - 1
	m.curCol = len([]rune(parts[len(parts)-1]))
	m.ensureVisible()
	m.notifyChange()
}

// splitLine breaks the current row at the cursor. Enter without modifiers.
func (m *MultiLineEdit) splitLine() {
	if m.selActive {
		m.DeleteSelection()
	}
	line := m.lines[m.curRow]
	head := append([]rune{}, line[:m.curCol]...)
	tail := append([]rune{}, line[m.curCol:]...)
	newLines := make([][]rune, 0, len(m.lines)+1)
	newLines = append(newLines, m.lines[:m.curRow]...)
	newLines = append(newLines, head)
	newLines = append(newLines, tail)
	newLines = append(newLines, m.lines[m.curRow+1:]...)
	m.lines = newLines
	m.curRow++
	m.curCol = 0
	m.ensureVisible()
	m.notifyChange()
}

// backspace deletes the char before the cursor, or merges with the
// previous row when the cursor is at column zero.
func (m *MultiLineEdit) backspace() {
	if m.selActive {
		m.DeleteSelection()
		return
	}
	line := m.lines[m.curRow]
	visualPos := m.currentVisualPos()
	clusters := visualClusters(line)
	if DefaultBidiMode == BidiFull && HasRTL(string(line)) && visualPos > 0 {
		cluster := clusters[visualPos-1]
		start := backspaceStart(line, cluster.logicalPos, cluster.logicalEnd)
		m.removeRange(start, cluster.logicalEnd)
		m.curCol = start
		m.ensureVisible()
		m.notifyChange()
		return
	}
	if m.curCol > 0 {
		start := backspaceStart(line, previousClusterBoundary(line, m.curCol), m.curCol)
		m.lines[m.curRow] = append(line[:start], line[m.curCol:]...)
		m.curCol = start
		m.ensureVisible()
		m.notifyChange()
		return
	}
	if m.curRow == 0 {
		return
	}
	prev := m.lines[m.curRow-1]
	newCol := len(prev)
	m.lines[m.curRow-1] = append(prev, m.lines[m.curRow]...)
	m.lines = append(m.lines[:m.curRow], m.lines[m.curRow+1:]...)
	m.curRow--
	m.curCol = newCol
	m.ensureVisible()
	m.notifyChange()
}

// deleteForward deletes the char at the cursor, or joins with the next
// row when the cursor is at end-of-line.
func (m *MultiLineEdit) deleteForward() {
	if m.selActive {
		m.DeleteSelection()
		return
	}
	line := m.lines[m.curRow]
	visualPos := m.currentVisualPos()
	clusters := visualClusters(line)
	if DefaultBidiMode == BidiFull && HasRTL(string(line)) && visualPos < len(clusters) {
		cluster := clusters[visualPos]
		m.removeRange(cluster.logicalPos, cluster.logicalEnd)
		m.notifyChange()
		return
	}
	if m.curCol < len(line) {
		end := nextClusterBoundary(line, m.curCol)
		m.lines[m.curRow] = append(line[:m.curCol], line[end:]...)
		m.notifyChange()
		return
	}
	if m.curRow+1 >= len(m.lines) {
		return
	}
	next := m.lines[m.curRow+1]
	m.lines[m.curRow] = append(line, next...)
	m.lines = append(m.lines[:m.curRow+1], m.lines[m.curRow+2:]...)
	m.notifyChange()
}

// CursorPos exposes (row, col) for tests and dialogs that want to
// restore a saved caret. Zero-indexed.
func (m *MultiLineEdit) CursorPos() (int, int) { return m.curRow, m.curCol }

// SetCursorPos moves the cursor to the given (row, col), clamping to
// valid range. Scrolls the viewport so the cursor is visible.
func (m *MultiLineEdit) SetCursorPos(row, col int) {
	m.curRow = row
	m.curCol = col
	// A caret move that is not a Shift+navigation ends the selection. Keeping
	// selActive here left the old anchor paired with the new caret, so the
	// widget painted -- and CopySelection returned -- a run the user never
	// selected. handleNav does the same on every unshifted arrow key.
	m.selActive = false
	m.ensureVisible()
}

// LineCount returns the number of rows in the buffer.
func (m *MultiLineEdit) LineCount() int { return len(m.lines) }

// wordLeft moves the cursor to the start of the word to its left, wrapping to
// the end of the row above when there is nothing left on this one.
//
// The stop rules are the ones Edit uses -- and far2l's edit.cpp before it --
// so Ctrl+Left means the same thing in a one line field and in this one, and
// Ctrl+Shift+Left selects the same run of text either way.
func (m *MultiLineEdit) wordLeft(selecting bool) {
	if m.curCol == 0 {
		if m.curRow == 0 {
			return
		}
		m.curRow--
		m.curCol = len(m.lines[m.curRow])
		return
	}
	line := m.lines[m.curRow]
	m.curCol = previousClusterBoundary(line, m.curCol)
	for m.curCol > 0 {
		if stopBeforeRuneLeft(line[m.curCol-1], line[m.curCol], selecting) {
			break
		}
		m.curCol = previousClusterBoundary(line, m.curCol)
	}
}

// wordRight is the rightward counterpart of wordLeft, wrapping to the start of
// the row below at the end of a line.
func (m *MultiLineEdit) wordRight(selecting bool) {
	line := m.lines[m.curRow]
	if m.curCol >= len(line) {
		if m.curRow+1 >= len(m.lines) {
			return
		}
		m.curRow++
		m.curCol = 0
		return
	}
	m.curCol = nextClusterBoundary(line, m.curCol)
	for m.curCol < len(line) {
		if stopBeforeRuneRight(line[m.curCol-1], line[m.curCol], selecting) {
			break
		}
		m.curCol = nextClusterBoundary(line, m.curCol)
	}
}

func (m *MultiLineEdit) handleNav(shift bool) {
	if shift {
		if !m.selActive {
			m.selActive = true
			m.selStartRow, m.selStartCol = m.curRow, m.curCol
		}
	} else {
		m.selActive = false
	}
}

func (m *MultiLineEdit) isSelected(row, col int) bool {
	return m.isSelectedRange(row, col, col+1)
}

func (m *MultiLineEdit) isSelectedRange(row, start, end int) bool {
	if !m.selActive {
		return false
	}
	r1, c1 := m.selStartRow, m.selStartCol
	r2, c2 := m.curRow, m.curCol
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if r1 == r2 && c1 == c2 {
		return false
	}
	if row < r1 || row > r2 {
		return false
	}
	if row == r1 && end <= c1 {
		return false
	}
	if row == r2 && start >= c2 {
		return false
	}
	return true
}

func previousClusterBoundary(line []rune, pos int) int {
	if pos <= 0 {
		return 0
	}
	previous := 0
	forEachTerminalCluster(string(line), func(_ string, _, _ int, runeIndex int) {
		if runeIndex < pos {
			previous = runeIndex
		}
	})
	return previous
}

func nextClusterBoundary(line []rune, pos int) int {
	next := len(line)
	found := false
	forEachTerminalCluster(string(line), func(_ string, _, _ int, runeIndex int) {
		if !found && runeIndex > pos {
			next = runeIndex
			found = true
		}
	})
	return next
}

func (m *MultiLineEdit) removeRange(start, end int) {
	line := m.lines[m.curRow]
	if start < 0 {
		start = 0
	}
	if end > len(line) {
		end = len(line)
	}
	if start >= end {
		return
	}
	m.lines[m.curRow] = append(line[:start], line[end:]...)
}

func (m *MultiLineEdit) SelectAll() {
	m.selActive = true
	m.selStartRow, m.selStartCol = 0, 0
	m.curRow = len(m.lines) - 1
	m.curCol = len(m.lines[m.curRow])
	m.ensureVisible()
	m.notifyChange()
}

func (m *MultiLineEdit) CopySelection() string {
	if !m.selActive {
		return ""
	}
	r1, c1 := m.selStartRow, m.selStartCol
	r2, c2 := m.curRow, m.curCol
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if r1 == r2 && c1 == c2 {
		return ""
	}
	var sb strings.Builder
	for r := r1; r <= r2; r++ {
		line := m.lines[r]
		start := 0
		if r == r1 {
			start = c1
		}
		end := len(line)
		if r == r2 {
			end = c2
		}
		if start < end {
			sb.WriteString(string(line[start:end]))
		}
		if r < r2 {
			sb.WriteRune('\n')
		}
	}
	return sb.String()
}

func (m *MultiLineEdit) DeleteSelection() {
	if !m.selActive {
		return
	}
	r1, c1 := m.selStartRow, m.selStartCol
	r2, c2 := m.curRow, m.curCol
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	if r1 == r2 && c1 == c2 {
		m.selActive = false
		return
	}
	m.selActive = false
	head := m.lines[r1][:c1]
	tail := m.lines[r2][c2:]
	newLines := append([][]rune(nil), m.lines[:r1]...)
	merged := append([]rune(nil), head...)
	merged = append(merged, tail...)
	newLines = append(newLines, merged)
	newLines = append(newLines, m.lines[r2+1:]...)
	m.lines = newLines
	m.curRow, m.curCol = r1, c1
	m.ensureVisible()
	m.notifyChange()
}
