package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestAutoComplete_SelectPos(t *testing.T) {
	SetDefaultPalette()
	edit := NewEdit(0, 0, 20, "l")
	edit.History = []string{"ls -la", "ls"}
	ac := NewAutoCompleteMenu(edit)
	if ac.SelectPos() != 0 {
		t.Errorf("Expected initial SelectPos 0, got %d", ac.SelectPos())
	}
}
func TestAutoComplete_IsBusyInheritance(t *testing.T) {
	SetDefaultPalette()
	fm := NewFrameManager()
	fm.Init(NewSilentScreenBuf())
	FrameManager = fm

	busyUnder := &busyFrame{Busy: true}
	fm.Push(busyUnder)

	edit := NewEdit(0, 0, 20, "l")
	ac := NewAutoCompleteMenu(edit)
	fm.Push(ac)

	if !ac.IsBusy() {
		t.Error("AutoCompleteMenu should inherit busy state when underlying frame is busy")
	}

	busyUnder.Busy = false
	if ac.IsBusy() {
		t.Error("AutoCompleteMenu should not be busy when underlying frame is not busy")
	}
}

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

func TestAutoComplete_MatchRanking(t *testing.T) {
	SetDefaultPalette()
	edit := NewEdit(0, 0, 20, "git")
	edit.History = []string{
		"got",        // fuzzy: 1 error (k = 1)
		"a git x",    // substring at 2
		"git status", // prefix
		"git",        // exact, must win despite being last in history
	}

	ac := NewAutoCompleteMenu(edit)

	want := []string{"git", "git status", "a git x", "got"}
	if len(ac.Matches) != len(want) {
		t.Fatalf("expected %d matches, got %d: %v", len(want), len(ac.Matches), ac.Matches)
	}
	for i := range want {
		if ac.Matches[i] != want[i] {
			t.Errorf("match %d: expected %q, got %q (all: %v)", i, want[i], ac.Matches[i], ac.Matches)
		}
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

func TestEdit_WordUnderCursor(t *testing.T) {
	SetDefaultPalette()
	e := NewEdit(0, 0, 40, `copy C:\Program Files\app "D:\My Docs"`)
	e.curPos = len([]rune(`copy C:\Program Files\app`)) - 2 // inside the second token

	from, to, word := e.WordUnderCursor()
	if word != `Files\app` && word != `C:\Program` {
		// sanity fallback, real assertion below
		t.Logf("word=%q from=%d to=%d", word, from, to)
	}
	// cursor sits in the token after the space inside the path
	if word != `Files\app` {
		t.Errorf("Expected token %q, got %q (from=%d to=%d)", `Files\app`, word, from, to)
	}

	// Cursor right after a separator inside a quoted path
	e2 := NewEdit(0, 0, 40, `"D:\My Docs\`)
	e2.curPos = len([]rune(e2.GetText()))
	_, _, word2 := e2.WordUnderCursor()
	if word2 != `"D:\My` && word2 != `Docs\` {
		t.Logf("word2=%q", word2)
	}
	if word2 != `Docs\` {
		t.Errorf("Expected quoted token tail %q, got %q", `Docs\`, word2)
	}

	// Empty text
	e3 := NewEdit(0, 0, 40, "")
	from3, to3, word3 := e3.WordUnderCursor()
	if word3 != "" || from3 != 0 || to3 != 0 {
		t.Errorf("Empty edit: got (%d,%d,%q)", from3, to3, word3)
	}
}

func TestAutoComplete_PathHintMerge(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	old := PathHintProvider
	defer func() { PathHintProvider = old }()
	PathHintProvider = func(edit *Edit, word string, from, to int) []AutoCompleteItem {
		return []AutoCompleteItem{
			{Text: `C:\tools\`, MatchStart: 3, MatchEnd: 7, ReplaceFrom: from, ReplaceTo: to},
			{Text: `C:\temp\`, MatchStart: 3, MatchEnd: 6, ReplaceFrom: from, ReplaceTo: to},
		}
	}

	edit := NewEdit(0, 10, 40, `C:\t`)
	edit.History = []string{`C:\tools old`, `cd home`}
	edit.PathHintsEnabled = true

	ac := NewAutoCompleteMenu(edit)
	FrameManager.Push(ac)
	defer ac.Close()

	// paths first, separator, then history prefix matches
	if len(ac.items) != 4 {
		t.Fatalf("Expected 4 items (2 paths + separator + 1 history), got %d", len(ac.items))
	}
	if !ac.items[2].Separator {
		t.Error("Item 2 should be the separator")
	}
	if ac.items[3].Text != `C:\tools old` {
		t.Errorf("History item after separator: got %q", ac.items[3].Text)
	}
	if ac.lb.IsSelectable(2) {
		t.Error("Separator must not be selectable")
	}

	// Down from 1 must skip the separator and land on the history item
	ac.lb.SetSelectPos(1)
	ac.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if ac.lb.SelectPos != 3 {
		t.Errorf("Down should skip the separator: SelectPos=%d, want 3", ac.lb.SelectPos)
	}
}

func TestAutoComplete_PathSpanReplaceAndDrillDown(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	old := PathHintProvider
	defer func() { PathHintProvider = old }()

	var lastWord string
	PathHintProvider = func(edit *Edit, word string, from, to int) []AutoCompleteItem {
		lastWord = word
		if word == `C:\t` {
			return []AutoCompleteItem{{Text: `C:\tools\`, MatchStart: -1, MatchEnd: -1, ReplaceFrom: from, ReplaceTo: to}}
		}
		if word == `C:\tools\` {
			return []AutoCompleteItem{{Text: `C:\tools\app.exe`, MatchStart: -1, MatchEnd: -1, ReplaceFrom: from, ReplaceTo: to}}
		}
		return nil
	}

	edit := NewEdit(0, 10, 40, `run C:\t --flag`)
	edit.curPos = 6 // right after the typed separator... place cursor inside token
	edit.curPos = len([]rune(`run C:\t`))
	edit.PathHintsEnabled = true

	ac := NewAutoCompleteMenu(edit)
	FrameManager.Push(ac)

	// Tab-accept the directory: replaces only the token, keeps the tail
	ac.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if edit.GetText() != `run C:\tools\ --flag` {
		t.Fatalf("Span replace failed: %q", edit.GetText())
	}
	if ac.IsDone() {
		t.Fatal("Menu should stay open for directory drill-down")
	}
	if lastWord != `C:\tools\` {
		t.Errorf("Provider should be re-queried with the new token, got %q", lastWord)
	}

	// Accept the file: menu closes, no Enter injected
	fm := FrameManager
	fm.injectedEvents = nil
	ac.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if edit.GetText() != `run C:\tools\app.exe --flag` {
		t.Fatalf("File accept failed: %q", edit.GetText())
	}
	if !ac.IsDone() {
		t.Error("Menu should close after accepting a file")
	}
	if len(fm.injectedEvents) != 0 {
		t.Error("Path item accept must not inject Enter")
	}
}

func TestAutoComplete_NearCursorPosition(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 40)
	FrameManager.Init(scr)

	edit := NewEdit(20, 30, 40, "git")
	edit.History = []string{"git status", "git pull"}
	ac := NewAutoCompleteMenu(edit)
	defer ac.Close()

	ax1, ay1, ax2, ay2 := ac.GetPosition()
	// Below the edit, at the cursor column (end of "git" -> X1+3)
	if ay1 != edit.Y1+1 {
		t.Errorf("Menu should open below the edit: y1=%d, want %d", ay1, edit.Y1+1)
	}
	if ax1 != edit.X1+3 {
		t.Errorf("Menu should open at the cursor column: x1=%d, want %d", ax1, edit.X1+3)
	}
	// Height: 2 items + frame, capped at 5 visible rows
	if ay2-ay1+1 != 4 {
		t.Errorf("Menu height: got %d, want 4", ay2-ay1+1)
	}
	if ax2-ax1+1 < 24 {
		t.Errorf("Menu width below minimum: %d", ax2-ax1+1)
	}

	// Near the bottom edge: flip above the edit
	edit2 := NewEdit(0, 38, 40, "git")
	edit2.History = []string{"git status", "git pull", "git push", "git log", "git diff", "git tag", "git add"}
	ac2 := NewAutoCompleteMenu(edit2)
	defer ac2.Close()
	_, ay21, _, _ := ac2.GetPosition()
	if ay21 >= edit2.Y1 {
		t.Errorf("Menu should flip above the edit near the bottom edge: y1=%d, edit y=%d", ay21, edit2.Y1)
	}
}

func TestAutoComplete_DisplayAndColumns(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 40)
	FrameManager.Init(scr)

	old := PathHintProvider
	defer func() { PathHintProvider = old }()
	PathHintProvider = func(edit *Edit, word string, from, to int) []AutoCompleteItem {
		return []AutoCompleteItem{
			{
				Text:    `C:\tools\app.exe`,
				Display: "app.exe",
				Cells:   []string{"app.exe", "12 KB"},
				// span relative to the displayed string
				MatchStart: 0, MatchEnd: 2,
				ReplaceFrom: from, ReplaceTo: to,
			},
		}
	}

	edit := NewEdit(5, 10, 40, `C:\ap`)
	edit.PathHintsEnabled = true
	ac := NewAutoCompleteMenu(edit)
	defer ac.Close()

	// Multi-column layout was picked up: main column flexible + one fixed
	if len(ac.lb.Columns) != 2 {
		t.Fatalf("Expected 2 columns, got %d", len(ac.lb.Columns))
	}
	if ac.lb.Columns[0].Width != 0 {
		t.Errorf("Main column should be flexible (Width 0), got %d", ac.lb.Columns[0].Width)
	}
	if ac.lb.Columns[1].Width != 5 { // "12 KB"
		t.Errorf("Second column width: got %d, want 5", ac.lb.Columns[1].Width)
	}
	if !ac.lb.ShowSeparators {
		t.Error("Multi-column list should show separators")
	}

	// Display text is rendered, needle span refers to it
	row := acRow{ac: ac, idx: 0}
	if got := row.GetCellText(0); got != "app.exe" {
		t.Errorf("Cell 0 should show Display: %q", got)
	}
	if got := row.GetCellText(1); got != "12 KB" {
		t.Errorf("Cell 1: %q", got)
	}

	// Accept still inserts the full Text
	ac.accept(0, false)
	if edit.GetText() != `C:\tools\app.exe` {
		t.Errorf("Accept should insert Text, not Display: %q", edit.GetText())
	}
}

func TestAutoComplete_AnchoredToTriggerSeparator(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 40)
	FrameManager.Init(scr)

	old := PathHintProvider
	defer func() { PathHintProvider = old }()
	PathHintProvider = func(edit *Edit, word string, from, to int) []AutoCompleteItem {
		return []AutoCompleteItem{{Text: word + "x", MatchStart: -1, MatchEnd: -1, ReplaceFrom: from, ReplaceTo: to}}
	}

	edit := NewEdit(10, 10, 40, `run C:\to`)
	edit.curPos = len([]rune(`run C:\to`))
	edit.PathHintsEnabled = true
	ac := NewAutoCompleteMenu(edit)
	defer ac.Close()

	ax1, _, _, _ := ac.GetPosition()
	// Trigger separator is at rune 6 (0-based) in "run C:\to"; edit starts at X1=10
	wantX := edit.X1 + 6
	if ax1 != wantX {
		t.Errorf("Menu should anchor at the trigger separator: x1=%d, want %d", ax1, wantX)
	}

	// Typing more moves the cursor but not the menu
	edit.SetText(`run C:\tools`)
	edit.curPos = len([]rune(`run C:\tools`))
	ac.UpdateMatches()
	ax1b, _, _, _ := ac.GetPosition()
	if ax1b != wantX {
		t.Errorf("Menu drifted while typing: x1=%d, want %d", ax1b, wantX)
	}
}

func TestAutoComplete_MaxVisibleAndPerCategory(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(100, 40)
	FrameManager.Init(scr)

	old := PathHintProvider
	defer func() { PathHintProvider = old }()
	defer SetAutoCompleteMaxVisible(5)
	defer SetAutoCompletePerCategory(false)

	PathHintProvider = func(edit *Edit, word string, from, to int) []AutoCompleteItem {
		return []AutoCompleteItem{
			{Text: "p1", MatchStart: -1, MatchEnd: -1},
			{Text: "p2", MatchStart: -1, MatchEnd: -1},
			{Text: "p3", MatchStart: -1, MatchEnd: -1},
			{Text: "p4", MatchStart: -1, MatchEnd: -1},
		}
	}

	newMenu := func() *AutoCompleteMenu {
		edit := NewEdit(0, 10, 40, "x")
		edit.History = []string{"x1", "x2", "x3"}
		edit.PathHintsEnabled = true
		return NewAutoCompleteMenu(edit)
	}

	// Total mode: one shared window of N rows, no truncation
	SetAutoCompleteMaxVisible(2)
	ac := newMenu()
	if len(ac.items) != 8 { // 4 paths + separator + 3 history
		t.Errorf("Total mode must not truncate: got %d items", len(ac.items))
	}
	_, ay1, _, ay2 := ac.GetPosition()
	if ay2-ay1+1 != 4 { // 2 visible + frame
		t.Errorf("Total mode height: got %d, want 4", ay2-ay1+1)
	}
	ac.Close()

	// Per-category mode: each group truncated to N, all rows visible
	SetAutoCompletePerCategory(true)
	ac = newMenu()
	defer ac.Close()
	// 2 paths + separator + 2 history = 5 items
	if len(ac.items) != 5 {
		t.Fatalf("Per-category truncation: got %d items, want 5", len(ac.items))
	}
	if ac.items[0].Text != "p1" || ac.items[1].Text != "p2" || !ac.items[2].Separator ||
		ac.items[3].Text != "x1" || ac.items[4].Text != "x2" {
		t.Errorf("Wrong per-category composition: %v", ac.Matches)
	}
	_, ay1, _, ay2 = ac.GetPosition()
	if ay2-ay1+1 != 7 { // all 5 items + frame
		t.Errorf("Per-category height: got %d, want 7", ay2-ay1+1)
	}
}
