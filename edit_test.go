package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestEdit_PasswordMode(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)

	e := NewEdit(0, 0, 10, "abc")
	e.PasswordMode = true
	e.Show(scr)

	// Check that buffer contains '*' instead of 'a'
	// Attributes must match ColDialogEdit
	checkCell(t, scr, 0, 0, '*', Palette[ColDialogEdit])
	checkCell(t, scr, 1, 0, '*', Palette[ColDialogEdit])
	checkCell(t, scr, 2, 0, '*', Palette[ColDialogEdit])
}

func TestEdit_IgnoreLockKeys(t *testing.T) {
	e := NewEdit(0, 0, 10, "")

	// Simulate entering 'x' with NumLock and CapsLock enabled
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            'x',
		ControlKeyState: vtinput.NumLockOn | vtinput.CapsLockOn,
	})

	if e.GetText() != "x" {
		t.Errorf("Expected 'x', got %q. Lock keys probably blocked the input.", e.GetText())
	}
}

func TestVMenu_ScrollbarMouseClick(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 10)
	m := NewVMenu("Title")
	// Add 20 items so menu scrolls
	for i := 0; i < 20; i++ {
		m.AddItem(MenuItem{Text: "Item"})
	}
	m.SetPosition(0, 0, 10, 6) // Height 7, data 5 (Y1+1..Y2-1)
	m.Show(scr)

	// Initial state: SelectPos 0

	// 1. Click down arrow (X = X2 = 10, Y = Y2-1 = 5)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 5, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 1 {
		t.Errorf("VMenu down arrow click failed, pos %d", m.TopPos)
	}

	// 2. Click up arrow (Y = Y1+1 = 1)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 1, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 0 {
		t.Errorf("VMenu up arrow click failed, pos %d", m.TopPos)
	}

	// 3. Page Down click (Y = 4)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 4, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 5 { // 0 + height (5) = 5
		t.Errorf("VMenu PageDown click failed, pos %d", m.TopPos)
	}

	// 4. Page Up click (Y = 2)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 2, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 0 { // 5 - height (5) = 0
		t.Errorf("VMenu PageUp click failed, pos %d", m.TopPos)
	}
}
func TestVMenu_Hotkeys(t *testing.T) {
	m := NewVMenu("Menu")
	m.AddItem(MenuItem{Text: "Open &File"})
	m.AddItem(MenuItem{Text: "&Save"})
	m.AddItem(MenuItem{Text: "E&xit"})

	// 1. Press 's' (second item hotkey)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 's'})

	if m.SelectPos != 1 {
		t.Errorf("Expected selectPos 1 for 'Save', got %d", m.SelectPos)
	}
	if !m.IsDone() || m.exitCode != 1 {
		t.Error("Menu should be finished with exitCode 1")
	}
}

func TestEdit_History(t *testing.T) {
	e := NewEdit(0, 0, 10, "")
	e.History = []string{"First", "Second"}

	// Simulate Alt+Down
	handled := e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_DOWN,
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !handled {
		t.Error("Alt+Down should be handled when History is present")
	}
}
func TestEdit_HistorySelection(t *testing.T) {
	e := NewEdit(0, 0, 10, "")
	e.History = []string{"Previous Command"}

	// We can't easily test the full Push/Pop cycle of FrameManager here,
	// but we can test the Edit's SetText which is called by the history menu.
	e.SetText(e.History[0])

	if e.GetText() != "Previous Command" {
		t.Errorf("SetText failed: expected 'Previous Command', got %q", e.GetText())
	}

	if e.curPos != 16 {
		t.Errorf("Cursor position should be at the end of the new text, got %d", e.curPos)
	}
}
func TestEdit_HistoryTrigger(t *testing.T) {
	e := NewEdit(0, 0, 10, "")

	// 1. Alt+Down without history -> should return false
	if e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN, ControlKeyState: vtinput.LeftAltPressed}) {
		t.Error("Edit should NOT handle Alt+Down when history is empty")
	}

	// 2. Alt+Down with history -> should return true
	e.History = []string{"cmd"}
	if !e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN, ControlKeyState: vtinput.LeftAltPressed}) {
		t.Error("Edit should handle Alt+Down when history is available")
	}
}
func TestEdit_HistoryManagement(t *testing.T) {
	e := NewEdit(0, 0, 10, "")

	// 1. Add unique items
	e.AddHistory("one")
	e.AddHistory("two")

	if e.History[0] != "two" || e.History[1] != "one" {
		t.Errorf("AddHistory order error: %v", e.History)
	}

	// 2. Add duplicate - should move to top
	e.AddHistory("one")
	if len(e.History) != 2 {
		t.Errorf("History should not have duplicates, size: %d", len(e.History))
	}
	if e.History[0] != "one" {
		t.Error("Duplicate item should be moved to the top")
	}

	// 3. Size limit
	for i := 0; i < 50; i++ {
		e.AddHistory(string(rune('A' + i)))
	}
	if len(e.History) > 32 {
		t.Errorf("History limit exceeded: %d", len(e.History))
	}
}

func TestEdit_HistoryButtonClick(t *testing.T) {
	e := NewEdit(0, 0, 10, "")
	e.ShowHistoryButton = true
	e.History = []string{"item"}

	// Click on the button at (9, 0)
	handled := e.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 9, MouseY: 0,
	})

	if !handled {
		t.Error("Edit should handle click on history button")
	}
}
func TestEdit_OnAction(t *testing.T) {
	e := NewEdit(0, 0, 10, "test")
	called := false
	e.OnAction = func() { called = true }

	// Simulate Enter
	handled := e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType,
		KeyDown: true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	if !handled || !called {
		t.Error("OnAction callback was not triggered on Enter")
	}
}

func TestEdit_SelectAllAndClear(t *testing.T) {
	e := NewEdit(0, 0, 20, "initial path")
	
	// 1. Trigger SelectAll
	e.SelectAll()
	if e.selStart != 0 || e.selEnd != 12 {
		t.Errorf("SelectAll failed: range [%d:%d]", e.selStart, e.selEnd)
	}
	if !e.clearFlag {
		t.Error("SelectAll should set clearFlag")
	}

	// 2. Typing a character should replace everything
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'C',
	})
	
	if e.GetText() != "C" {
		t.Errorf("Typing after SelectAll failed: expected 'C', got %q", e.GetText())
	}
	if e.selStart != -1 {
		t.Error("Selection should be cleared after typing")
	}
}

func TestEdit_SelectAllAndNavigate(t *testing.T) {
	e := NewEdit(0, 0, 20, "some text")
	e.SelectAll()

	// 3. Navigating (Left Arrow) should clear selection but NOT the text
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT,
	})

	if e.GetText() != "some text" {
		t.Error("Navigation should not clear text when SelectAll is active")
	}
	if e.selStart != -1 || e.clearFlag {
		t.Error("Navigation should clear selection and clearFlag")
	}
}
