package vtui

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

// key builds a simple keydown InputEvent with the given VK / Char / mods.
func mleKey(vk uint16, ch rune, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vk,
		Char:            ch,
		ControlKeyState: mods,
	}
}

func mleType(m *MultiLineEdit, s string) {
	for _, r := range s {
		m.ProcessKey(mleKey(0, r, 0))
	}
}

func TestMultiLineEdit_SetGetText(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "a\nb\nc")
	if got := m.GetText(); got != "a\nb\nc" {
		t.Errorf("GetText = %q, want %q", got, "a\nb\nc")
	}
	if got := m.LineCount(); got != 3 {
		t.Errorf("LineCount = %d, want 3", got)
	}
}

func TestMultiLineEdit_SetTextEmptyGivesOneEmptyRow(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "")
	if got := m.LineCount(); got != 1 {
		t.Errorf("LineCount = %d, want 1 (empty text still needs a row for the cursor)", got)
	}
	row, col := m.CursorPos()
	if row != 0 || col != 0 {
		t.Errorf("CursorPos = (%d,%d), want (0,0)", row, col)
	}
}

func TestMultiLineEdit_TypeInsertsAtCursor(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "")
	mleType(m, "hello")
	if got := m.GetText(); got != "hello" {
		t.Errorf("after typing: %q, want %q", got, "hello")
	}
	row, col := m.CursorPos()
	if row != 0 || col != 5 {
		t.Errorf("CursorPos = (%d,%d), want (0,5)", row, col)
	}
}

func TestMultiLineEdit_EnterSplitsLine(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "abcdef")
	// Move cursor to col 3.
	m.SetCursorPos(0, 3)
	m.ProcessKey(mleKey(vtinput.VK_RETURN, '\r', 0))
	if got := m.GetText(); got != "abc\ndef" {
		t.Errorf("after Enter: %q, want %q", got, "abc\ndef")
	}
	row, col := m.CursorPos()
	if row != 1 || col != 0 {
		t.Errorf("CursorPos after split = (%d,%d), want (1,0)", row, col)
	}
}

func TestMultiLineEdit_BackspaceMergesWithPrevRow(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "abc\ndef")
	m.SetCursorPos(1, 0)
	m.ProcessKey(mleKey(vtinput.VK_BACK, 0, 0))
	if got := m.GetText(); got != "abcdef" {
		t.Errorf("after BS at col=0: %q, want %q", got, "abcdef")
	}
	row, col := m.CursorPos()
	if row != 0 || col != 3 {
		t.Errorf("CursorPos = (%d,%d), want (0,3)", row, col)
	}
}

func TestMultiLineEdit_DeleteJoinsNextRow(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "abc\ndef")
	m.SetCursorPos(0, 3) // end of first line
	m.ProcessKey(mleKey(vtinput.VK_DELETE, 0, 0))
	if got := m.GetText(); got != "abcdef" {
		t.Errorf("after Del at eol: %q, want %q", got, "abcdef")
	}
}

func TestMultiLineEdit_ArrowsWrapAcrossLines(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "ab\ncd")
	m.SetCursorPos(0, 2) // end of first line
	m.ProcessKey(mleKey(vtinput.VK_RIGHT, 0, 0))
	if row, col := m.CursorPos(); row != 1 || col != 0 {
		t.Errorf("Right at eol → (%d,%d), want (1,0)", row, col)
	}
	m.ProcessKey(mleKey(vtinput.VK_LEFT, 0, 0))
	if row, col := m.CursorPos(); row != 0 || col != 2 {
		t.Errorf("Left at bol → (%d,%d), want (0,2)", row, col)
	}
}

func TestMultiLineEdit_HomeEnd(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "abcdef\n1234")
	m.SetCursorPos(1, 2)
	m.ProcessKey(mleKey(vtinput.VK_HOME, 0, 0))
	if row, col := m.CursorPos(); row != 1 || col != 0 {
		t.Errorf("Home → (%d,%d), want (1,0)", row, col)
	}
	m.ProcessKey(mleKey(vtinput.VK_END, 0, 0))
	if row, col := m.CursorPos(); row != 1 || col != 4 {
		t.Errorf("End → (%d,%d), want (1,4)", row, col)
	}
	// Ctrl+Home / Ctrl+End jump to buffer start / end.
	m.ProcessKey(mleKey(vtinput.VK_HOME, 0, vtinput.LeftCtrlPressed))
	if row, col := m.CursorPos(); row != 0 || col != 0 {
		t.Errorf("Ctrl+Home → (%d,%d), want (0,0)", row, col)
	}
	m.ProcessKey(mleKey(vtinput.VK_END, 0, vtinput.LeftCtrlPressed))
	if row, col := m.CursorPos(); row != 1 || col != 4 {
		t.Errorf("Ctrl+End → (%d,%d), want (1,4)", row, col)
	}
}

func TestMultiLineEdit_MultiLinePasteViaInsertString(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "PRE\nSUF")
	m.SetCursorPos(0, 3) // end of "PRE"
	m.insertString("X\nY\nZ")
	want := "PREX\nY\nZ\nSUF"
	if got := m.GetText(); got != want {
		t.Errorf("after paste: %q, want %q", got, want)
	}
	if row, col := m.CursorPos(); row != 2 || col != 1 {
		t.Errorf("CursorPos = (%d,%d), want (2,1) — end of last pasted fragment", row, col)
	}
}

func TestMultiLineEdit_SetTextResetsCursor(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "old")
	m.SetCursorPos(0, 3)
	m.SetText("brand\nnew")
	if row, col := m.CursorPos(); row != 0 || col != 0 {
		t.Errorf("cursor after SetText = (%d,%d), want (0,0)", row, col)
	}
}

func TestMultiLineEdit_GetSetLines(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "")
	m.SetLines([]string{"one", "two", "three"})
	if got := m.GetLines(); len(got) != 3 || got[0] != "one" || got[2] != "three" {
		t.Errorf("GetLines = %v", got)
	}
	if got := m.GetText(); got != "one\ntwo\nthree" {
		t.Errorf("GetText after SetLines = %q", got)
	}
}

func TestMultiLineEdit_BracketedPasteAccumulates(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "")
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true})
	for _, r := range "a\nb" {
		m.ProcessKey(mleKey(0, r, 0))
	}
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: false})
	if got := m.GetText(); got != "a\nb" {
		t.Errorf("after bracketed paste: %q, want %q", got, "a\nb")
	}
}

func TestMultiLineEdit_MouseClickMovesCursor(t *testing.T) {
	m := NewMultiLineEdit(2, 3, 20, 5, "hello\nworld")
	// Click at physical (5, 4) → x=3, y=1 → row 1, col 3.
	m.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      5,
		MouseY:      4,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if row, col := m.CursorPos(); row != 1 || col != 3 {
		t.Errorf("cursor after click = (%d,%d), want (1,3)", row, col)
	}
}

// TestMultiLineEdit_ScrollHorizontalOnLongLine — long lines should scroll
// under the cursor, not clip it invisibly.
func TestMultiLineEdit_ScrollHorizontalOnLongLine(t *testing.T) {
	line := strings.Repeat("x", 50)
	m := NewMultiLineEdit(0, 0, 10, 3, line)
	m.SetCursorPos(0, 40)
	// After ensureVisible (via Show()) the leftPos should have moved so
	// the cursor at col 40 is inside the 10-wide viewport.
	m.ensureVisible()
	if m.leftPos >= 40 {
		t.Errorf("leftPos = %d, want < 40", m.leftPos)
	}
	if _, col := m.CursorPos(); col-m.leftPos >= 10 {
		t.Errorf("cursor visible range: col=%d, leftPos=%d, viewWidth=10", col, m.leftPos)
	}
}

// TestMultiLineEdit_OnTextChange fires on every mutation.
func TestMultiLineEdit_OnTextChange(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "")
	count := 0
	m.OnTextChange = func(string) { count++ }
	mleType(m, "hi")
	m.ProcessKey(mleKey(vtinput.VK_RETURN, '\r', 0))
	mleType(m, "there")
	if count == 0 {
		t.Fatal("OnTextChange never fired")
	}
	if got := m.GetText(); got != "hi\nthere" {
		t.Errorf("GetText = %q", got)
	}
}

func TestMultiLineEdit_WantsChars(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "")
	if !m.WantsChars() {
		t.Error("MultiLineEdit must accept printable chars")
	}
}

// TestMultiLineEdit_CtrlEnterLeavesEvent — Ctrl+Enter should NOT be
// swallowed, so the enclosing dialog can bind it to a default button.
func TestMultiLineEdit_CtrlEnterLeavesEvent(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 5, "line1")
	consumed := m.ProcessKey(mleKey(vtinput.VK_RETURN, '\r', vtinput.LeftCtrlPressed))
	if consumed {
		t.Error("Ctrl+Enter should not be consumed by MultiLineEdit — dialog needs it")
	}
	if got := m.GetText(); got != "line1" {
		t.Errorf("buffer unexpectedly changed: %q", got)
	}
}

func TestMultiLineEdit_CombiningClustersMoveTogetherAndBackspacePeelsMarks(t *testing.T) {
	m := NewMultiLineEdit(0, 0, 20, 2, "e\u0301x")
	m.SetCursorPos(0, 3)

	m.ProcessKey(mleKey(vtinput.VK_LEFT, 0, 0))
	if _, col := m.CursorPos(); col != 2 {
		t.Fatalf("left moved into a grapheme cluster: col=%d, want 2", col)
	}
	m.ProcessKey(mleKey(vtinput.VK_BACK, 0, 0))
	if got := m.GetText(); got != "ex" {
		t.Fatalf("backspace did not peel the combining mark: %q, want %q", got, "ex")
	}
}

func TestMultiLineEdit_BidiFullUsesVisualCaretOrder(t *testing.T) {
	oldMode := DefaultBidiMode
	DefaultBidiMode = BidiFull
	defer func() { DefaultBidiMode = oldMode }()

	m := NewMultiLineEdit(0, 0, 20, 2, "שלום")
	m.SetCursorPos(0, 4)
	m.ProcessKey(mleKey(vtinput.VK_RIGHT, 0, 0))
	if _, col := m.CursorPos(); col != 3 {
		t.Fatalf("visual right used logical order: col=%d, want 3", col)
	}
	m.ProcessKey(mleKey(vtinput.VK_LEFT, 0, 0))
	if _, col := m.CursorPos(); col != 4 {
		t.Fatalf("visual left did not return to the original caret: col=%d, want 4", col)
	}
}

// TestMultiLineEdit_CtrlArrowsMoveByWord covers the keys a one line Edit has
// always had. MultiLineEdit returned false for Ctrl+Left and Ctrl+Right, so
// the cursor stayed put and the key escaped the field entirely -- in f4 that
// means the panel split moves while the user is editing an SQL statement.
func TestMultiLineEdit_CtrlArrowsMoveByWord(t *testing.T) {
	const ctrl = vtinput.LeftCtrlPressed

	m := NewMultiLineEdit(0, 0, 40, 5, "select id from t\nwhere id = 1")
	m.SetCursorPos(0, 0)

	m.ProcessKey(mleKey(vtinput.VK_RIGHT, 0, ctrl))
	if row, col := m.CursorPos(); row != 0 || col != 7 {
		t.Fatalf("Ctrl+Right → (%d,%d), want (0,7)", row, col)
	}
	m.ProcessKey(mleKey(vtinput.VK_LEFT, 0, ctrl))
	if row, col := m.CursorPos(); row != 0 || col != 0 {
		t.Fatalf("Ctrl+Left → (%d,%d), want (0,0)", row, col)
	}

	// At the end of a row the cursor continues onto the next one, the way the
	// plain arrows already do.
	m.SetCursorPos(0, 16)
	m.ProcessKey(mleKey(vtinput.VK_RIGHT, 0, ctrl))
	if row, col := m.CursorPos(); row != 1 || col != 0 {
		t.Fatalf("Ctrl+Right at end of row → (%d,%d), want (1,0)", row, col)
	}
	m.ProcessKey(mleKey(vtinput.VK_LEFT, 0, ctrl))
	if row, col := m.CursorPos(); row != 0 || col != 16 {
		t.Fatalf("Ctrl+Left at start of row → (%d,%d), want (0,16)", row, col)
	}
}

func TestMultiLineEdit_CtrlShiftArrowsSelectByWord(t *testing.T) {
	const ctrlShift = vtinput.LeftCtrlPressed | vtinput.ShiftPressed

	m := NewMultiLineEdit(0, 0, 40, 5, "select id from t")
	m.SetCursorPos(0, 0)
	m.ProcessKey(mleKey(vtinput.VK_RIGHT, 0, ctrlShift))
	if got := m.CopySelection(); got != "select " {
		t.Fatalf("Ctrl+Shift+Right selected %q, want %q", got, "select ")
	}

	m.SetCursorPos(0, 16)
	m.ProcessKey(mleKey(vtinput.VK_LEFT, 0, ctrlShift))
	if got := m.CopySelection(); got != "t" {
		t.Fatalf("Ctrl+Shift+Left selected %q, want %q", got, "t")
	}
}
