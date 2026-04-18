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

func TestAutoComplete_ReturnLogic(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())
	fm.injectedEvents = nil

	// История: "go run ."
	// Ввод: "g"
	edit := NewEdit(0, 10, 20, "g")
	edit.History = []string{"go run ."}

	ac := NewAutoCompleteMenu(edit)
	fm.Push(ac)

	// Нажимаем Enter СРАЗУ, без навигации стрелками
	ac.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	// 1. Текст должен замениться на подсказку
	if edit.GetText() != "go run ." {
		t.Errorf("Enter should update text even without navigation. Got %q", edit.GetText())
	}

	// 2. Должно быть инжектировано событие Enter для выполнения
	if len(fm.injectedEvents) == 0 || fm.injectedEvents[0].VirtualKeyCode != vtinput.VK_RETURN {
		t.Error("Enter should inject Return event for immediate execution")
	}

	if !ac.IsDone() {
		t.Error("Menu should close on Enter")
	}
}

func TestAutoComplete_ShiftEnter(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())
	fm.injectedEvents = nil

	edit := NewEdit(0, 10, 20, "g")
	edit.History = []string{"go run ."}

	ac := NewAutoCompleteMenu(edit)
	fm.Push(ac)

	// Нажимаем Shift+Enter
	ac.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RETURN,
		ControlKeyState: vtinput.ShiftPressed,
	})

	// 1. Текст должен замениться
	if edit.GetText() != "go run ." {
		t.Errorf("Shift+Enter should update text. Got %q", edit.GetText())
	}

	// 2. Событие инжектироваться НЕ должно (просто подстановка для редактирования)
	if len(fm.injectedEvents) != 0 {
		t.Error("Shift+Enter should NOT inject execution event")
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
