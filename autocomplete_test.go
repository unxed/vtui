package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestAutoComplete_Matching(t *testing.T) {
	SetDefaultPalette()
	edit := NewEdit(0, 0, 20, "l")
	edit.History = []string{"ls -la", "ls", "cd /tmp", "git status"}

	ac := NewAutoCompleteMenu(edit)

	// Should match "ls -la" and "ls"
	if len(ac.Matches) != 2 {
		t.Errorf("Expected 2 matches, got %d: %v", len(ac.Matches), ac.Matches)
	}

	if ac.Matches[0] != "ls -la" || ac.Matches[1] != "ls" {
		t.Errorf("Wrong matches order or content: %v", ac.Matches)
	}
}

func TestAutoComplete_TabCompletion(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	edit := NewEdit(0, 10, 20, "g")
	edit.History = []string{"git commit", "git push"}

	ac := NewAutoCompleteMenu(edit)
	FrameManager.Push(ac)

	// Navigate to second item
	ac.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN,
	})

	// Press Tab
	ac.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB,
	})

	if edit.GetText() != "git push" {
		t.Errorf("Tab completion failed: expected 'git push', got %q", edit.GetText())
	}

	if !ac.IsDone() {
		t.Error("AutoComplete menu should close after Tab completion")
	}
}

func TestAutoComplete_ShiftDelete(t *testing.T) {
	SetDefaultPalette()
	GlobalHistoryProvider = &mockHistoryProvider{storage: make(map[string][]string)}

	edit := NewEdit(0, 0, 20, "rm")
	edit.History = []string{"rm -rf /", "rm test.txt"}
	edit.HistoryID = "test"

	ac := NewAutoCompleteMenu(edit)

	// Remove first item via Shift+Del
	ac.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_DELETE, ControlKeyState: vtinput.ShiftPressed,
	})

	if len(edit.History) != 1 || edit.History[0] != "rm test.txt" {
		t.Errorf("History item was not removed. Current history: %v", edit.History)
	}

	if len(ac.Matches) != 1 || ac.Matches[0] != "rm test.txt" {
		t.Errorf("Matches not updated after deletion. Got: %v", ac.Matches)
	}
}

func TestAutoComplete_EmptyOnDelete(t *testing.T) {
	SetDefaultPalette()
	edit := NewEdit(0, 0, 20, "a")
	edit.History = []string{"apple"}
	ac := NewAutoCompleteMenu(edit)

	// Simulate Backspace
	edit.curPos = 1
	ac.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK,
	})

	if edit.GetText() != "" {
		t.Fatalf("Edit should be empty, got %q", edit.GetText())
	}

	if !ac.IsDone() {
		t.Error("AutoComplete menu should close when text becomes empty")
	}
}