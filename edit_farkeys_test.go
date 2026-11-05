package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func farKeysEdit(text string, farKeys bool) *Edit {
	e := NewEdit(0, 0, 20, text)
	e.ClearSelection()
	e.FarKeys = farKeys
	return e
}

func pressKey(e *Edit, vk uint16, mods vtinput.ControlKeyState) bool {
	return e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vk,
		ControlKeyState: mods,
	})
}

func TestEditFarKeys_CtrlBackspaceDeletesWordLeft(t *testing.T) {
	e := farKeysEdit("abc def", true)
	if !pressKey(e, vtinput.VK_BACK, vtinput.LeftCtrlPressed) {
		t.Fatal("Ctrl+Backspace was not handled")
	}
	if got := e.GetText(); got != "abc " {
		t.Errorf("text = %q, want %q", got, "abc ")
	}
	if e.CursorPos() != 4 {
		t.Errorf("cursor = %d, want 4", e.CursorPos())
	}
}

func TestEditFarKeys_CtrlDeleteDeletesWordRight(t *testing.T) {
	e := farKeysEdit("abc def", true)
	e.SetCursorPos(4)
	if !pressKey(e, vtinput.VK_DELETE, vtinput.RightCtrlPressed) {
		t.Fatal("Ctrl+Delete was not handled")
	}
	if got := e.GetText(); got != "abc " {
		t.Errorf("text = %q, want %q", got, "abc ")
	}
	if e.CursorPos() != 4 {
		t.Errorf("cursor = %d, want 4", e.CursorPos())
	}
}

func TestEditFarKeys_CtrlKDeletesToEnd(t *testing.T) {
	e := farKeysEdit("abc def", true)
	pressKey(e, vtinput.VK_HOME, 0)
	if !pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed) {
		t.Fatal("Ctrl+K was not handled")
	}
	if got := e.GetText(); got != "" {
		t.Errorf("text = %q, want empty", got)
	}

	e = farKeysEdit("abc def", true)
	e.SetCursorPos(3)
	pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "abc" {
		t.Errorf("text = %q, want %q", got, "abc")
	}
}

func TestEditFarKeys_WordIsTheDoubleClickRun(t *testing.T) {
	// Dividers and words are separate runs, as in the double-click selection.
	e := farKeysEdit("cd /usr/bin", true)
	pressKey(e, vtinput.VK_BACK, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "cd /usr/" {
		t.Errorf("text = %q, want %q", got, "cd /usr/")
	}
	pressKey(e, vtinput.VK_BACK, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "cd /usr" {
		t.Errorf("text = %q, want %q", got, "cd /usr")
	}
	// A run of spaces counts as one word too.
	e = farKeysEdit("abc   ", true)
	pressKey(e, vtinput.VK_BACK, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "abc" {
		t.Errorf("text = %q, want %q", got, "abc")
	}
}

func TestEditFarKeys_EdgesDoNothing(t *testing.T) {
	e := farKeysEdit("abc", true)
	e.SetCursorPos(0)
	changes := 0
	e.OnTextChange = func(string) { changes++ }
	pressKey(e, vtinput.VK_BACK, vtinput.LeftCtrlPressed)
	e.SetCursorPos(3)
	pressKey(e, vtinput.VK_DELETE, vtinput.LeftCtrlPressed)
	pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "abc" {
		t.Errorf("text = %q, want unchanged", got)
	}
	if changes != 0 {
		t.Errorf("OnTextChange fired %d times for keys that removed nothing", changes)
	}
	if e.CursorPos() != 3 {
		t.Errorf("cursor = %d, want 3", e.CursorPos())
	}
}

func TestEditFarKeys_SelectionIsDeleted(t *testing.T) {
	e := farKeysEdit("abc def", true)
	pressKey(e, vtinput.VK_HOME, 0)
	pressKey(e, vtinput.VK_RIGHT, vtinput.ShiftPressed)
	pressKey(e, vtinput.VK_RIGHT, vtinput.ShiftPressed)
	pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "c def" {
		t.Errorf("text = %q, want %q", got, "c def")
	}
}

func TestEditFarKeys_ReplaceAllFlagIsDeleted(t *testing.T) {
	e := NewEdit(0, 0, 20, "abc def") // untouched initial text is replaced by input
	e.FarKeys = true
	pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "" {
		t.Errorf("text = %q, want empty", got)
	}
}

func TestEditFarKeys_OffByDefault(t *testing.T) {
	// Dialog fields keep their behaviour: Ctrl+Backspace removes one rune and
	// Ctrl+K is not consumed.
	e := farKeysEdit("abc def", false)
	if pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed) {
		t.Error("Ctrl+K handled without FarKeys")
	}
	if got := e.GetText(); got != "abc def" {
		t.Errorf("text = %q after Ctrl+K, want unchanged", got)
	}
	pressKey(e, vtinput.VK_BACK, vtinput.LeftCtrlPressed)
	if got := e.GetText(); got != "abc de" {
		t.Errorf("text = %q after Ctrl+Backspace, want %q", got, "abc de")
	}
}

func TestEditFarKeys_OtherModifiersAreNotTaken(t *testing.T) {
	e := farKeysEdit("abc def", true)
	if pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed) {
		t.Error("Ctrl+Alt+K handled")
	}
	pressKey(e, vtinput.VK_K, vtinput.LeftCtrlPressed|vtinput.ShiftPressed)
	if got := e.GetText(); got != "abc def" {
		t.Errorf("text = %q, want unchanged", got)
	}
}

func TestEditCursorPos(t *testing.T) {
	e := farKeysEdit("abc", false)
	if e.CursorPos() != 3 {
		t.Errorf("cursor after construction = %d, want 3", e.CursorPos())
	}
	e.SetCursorPos(1)
	if e.CursorPos() != 1 {
		t.Errorf("cursor = %d, want 1", e.CursorPos())
	}
	e.SetCursorPos(99)
	if e.CursorPos() != 3 {
		t.Errorf("cursor = %d, want clamped to 3", e.CursorPos())
	}
	e.SetCursorPos(-5)
	if e.CursorPos() != 0 {
		t.Errorf("cursor = %d, want clamped to 0", e.CursorPos())
	}

	// SetCursorPos drops the pending replace-all, so typing inserts.
	e = NewEdit(0, 0, 20, "abc")
	e.SetCursorPos(1)
	e.InsertString("X")
	if got := e.GetText(); got != "aXbc" {
		t.Errorf("text = %q, want %q", got, "aXbc")
	}
}
