package vtui

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

// A HistoryMatches hook decides the history rows itself: its rows are shown
// verbatim and the built-in fuzzy match, which would offer every entry that
// merely contains the text, is not consulted.
func TestAutoComplete_HistoryMatchesHookReplacesFuzzyMatch(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	edit := NewEdit(0, 0, 40, "./")
	edit.History = []string{"cd ../x", "./build.sh", "git add ./y"}
	var asked []string
	edit.HistoryMatches = func(text string) []AutoCompleteItem {
		asked = append(asked, text)
		var items []AutoCompleteItem
		for _, h := range edit.History {
			if strings.HasPrefix(h, text) {
				items = append(items, AutoCompleteItem{Text: h, MatchStart: 0, MatchEnd: len(text) - 1})
			}
		}
		return items
	}

	ac := NewAutoCompleteMenu(edit)
	if len(asked) == 0 || asked[len(asked)-1] != "./" {
		t.Fatalf("hook was not asked for the typed text: %v", asked)
	}
	if len(ac.Matches) != 1 || ac.Matches[0] != "./build.sh" {
		t.Fatalf("Matches = %v, want only the hook's row", ac.Matches)
	}
}

func TestAutoComplete_HistorySearchKeepsTypedTextAsBlankFirstRow(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	edit := NewEdit(0, 0, 40, "git")
	edit.History = []string{"git status", "git commit"}
	edit.HistorySearchKeepTypedText = true

	ac := NewAutoCompleteMenu(edit)
	if len(ac.items) != 3 {
		t.Fatalf("items = %#v, want typed-text row and two history matches", ac.items)
	}
	if got := ac.items[0]; got.Text != "git" || !got.DisplayBlank || (acRow{ac: ac, idx: 0}).GetCellText(0) != "" {
		t.Fatalf("first item = %#v, want blank display that restores typed input", got)
	}
	ac.accept(0, false)
	if got := edit.GetText(); got != "git" {
		t.Fatalf("accepting blank row changed input to %q, want original typed text", got)
	}
}

func TestAutoComplete_ArrowSelectBlankHistoryRowKeepsTypedText(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())
	fm.injectedEvents = nil

	edit := NewEdit(0, 0, 40, "git")
	edit.History = []string{"git status", "git commit"}
	edit.HistorySearchKeepTypedText = true

	ac := NewAutoCompleteMenu(edit)
	fm.Push(ac)
	ac.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if ac.SelectPos() != 0 {
		t.Fatalf("first Down selected row %d, want blank typed-text row 0", ac.SelectPos())
	}
	ac.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if got := edit.GetText(); got != "git" {
		t.Fatalf("accepting the blank row changed input to %q, want the typed text", got)
	}
	if len(fm.injectedEvents) != 1 || fm.injectedEvents[0].VirtualKeyCode != vtinput.VK_RETURN {
		t.Fatalf("accepting typed text should pass Enter to the underlying frame, got %v", fm.injectedEvents)
	}
}

func TestAutoComplete_ArrowSelectHistoryMatch(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())
	fm.injectedEvents = nil

	edit := NewEdit(0, 0, 40, "git")
	edit.History = []string{"git status", "git commit"}
	edit.HistorySearchKeepTypedText = true

	ac := NewAutoCompleteMenu(edit)
	fm.Push(ac)
	down := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}
	ac.ProcessKey(down) // select the blank row that preserves the typed text
	ac.ProcessKey(down) // move to the first filtered history match
	if got := ac.SelectPos(); got != 1 {
		t.Fatalf("second Down selected row %d, want first history match at row 1", got)
	}
	ac.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if got := edit.GetText(); got != "git status" {
		t.Fatalf("accepting selected history match set text to %q, want %q", got, "git status")
	}
}

func TestAutoComplete_HistorySearchPreviewsOnlyExplicitSelection(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())

	edit := NewEdit(0, 0, 40, "git")
	edit.History = []string{"git status", "git commit"}
	edit.HistorySearchKeepTypedText = true
	ac := NewAutoCompleteMenu(edit)
	fm.Push(ac)
	if got := edit.GetText(); got != "git" {
		t.Fatalf("opening history changed typed text to %q", got)
	}

	down := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}
	ac.ProcessKey(down) // blank first row: preserve the typed query
	if got := edit.GetText(); got != "git" {
		t.Fatalf("selecting the blank row changed text to %q", got)
	}
	ac.ProcessKey(down) // first history match
	if got := edit.GetText(); got != "git status" {
		t.Fatalf("selecting a match previewed %q, want %q", got, "git status")
	}
	up := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}
	ac.ProcessKey(up)
	if got := edit.GetText(); got != "git" {
		t.Fatalf("returning to the blank row restored %q, want typed query", got)
	}
}

func TestAutoComplete_HistorySearchPreviewFollowsWheelAndHover(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())

	edit := NewEdit(0, 0, 40, "git")
	edit.History = []string{"git status", "git commit", "git log", "git diff", "git show", "git add"}
	edit.HistorySearchKeepTypedText = true
	ac := NewAutoCompleteMenu(edit)
	ac.SetPosition(2, 2, 40, 10)
	fm.Push(ac)
	ac.choose(0) // establish the blank typed-text row

	wheel := gestureEvent(ac.lb.X1+1, ac.lb.Y1, true, 0, false)
	wheel.WheelDirection = -1
	ac.ProcessMouse(wheel)
	if got := edit.GetText(); got != "git status" {
		t.Fatalf("wheel selection previewed %q, want first history match", got)
	}

	// Hover over the next visible row; it is another explicit choice.
	ac.ProcessMouse(gestureEvent(ac.lb.X1+1, ac.lb.Y1+2, false, 0, true))
	if got := edit.GetText(); got != "git commit" {
		t.Fatalf("hover previewed %q, want next history match", got)
	}
}

func TestAutoComplete_HistorySearchArrowDismissesBeforeOpeningFullHistory(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())

	edit := NewEdit(0, 0, 40, "git")
	edit.History = []string{"git status", "git commit"}
	edit.ShowHistoryButton = true
	edit.HistoryButtonOnly = true
	edit.HistorySearchKeepTypedText = true
	ac := NewAutoCompleteMenu(edit)
	fm.Push(ac)
	ac.choose(1) // preview a match before using the arrow
	if got := edit.GetText(); got != "git status" {
		t.Fatalf("preview text = %q, want %q", got, "git status")
	}

	clickArrow := &vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      int16(edit.X2), MouseY: int16(edit.Y1),
	}
	ac.ProcessMouse(clickArrow)
	if !ac.IsDone() {
		t.Fatal("first arrow click must dismiss filtered matches")
	}
	if got := edit.GetText(); got != "git" {
		t.Fatalf("dismissing filtered matches restored %q, want original query", got)
	}

	// The next click is delivered to the edit and opens its ordinary, full
	// history list rather than reopening the filtered autocomplete menu.
	edit.ProcessMouse(clickArrow)
	if _, ok := fm.GetTopFrame().(*VMenu); !ok {
		t.Fatalf("second arrow click opened %T, want the full history menu", fm.GetTopFrame())
	}
}

// A field whose only history source is the hook still qualifies for the menu.
func TestAutoComplete_HistoryMatchesHookCountsAsSource(t *testing.T) {
	edit := NewEdit(0, 0, 20, "")
	if edit.autoCompletes() {
		t.Fatal("a field with no history and no path hints must not autocomplete")
	}
	edit.HistoryMatches = func(string) []AutoCompleteItem { return nil }
	if !edit.autoCompletes() {
		t.Fatal("a field with a HistoryMatches hook must autocomplete")
	}
}

// PathHintsLast puts the history rows first, as Far's command line does,
// with a single separator between the two groups.
func TestAutoComplete_PathHintsLast(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	old := PathHintProvider
	defer func() { PathHintProvider = old }()
	PathHintProvider = func(edit *Edit, word string, from, to int) []AutoCompleteItem {
		return []AutoCompleteItem{
			{Text: "./build.sh", MatchStart: 0, MatchEnd: 1, ReplaceFrom: from, ReplaceTo: to},
		}
	}

	edit := NewEdit(0, 10, 40, "./")
	edit.PathHintsEnabled = true
	edit.PathHintsLast = true
	edit.HistoryMatches = func(string) []AutoCompleteItem {
		return []AutoCompleteItem{
			{Text: "./run.sh here", MatchStart: 0, MatchEnd: 1},
			{Separator: true, MatchStart: -1, MatchEnd: -1},
			{Text: "./run.sh elsewhere", MatchStart: 0, MatchEnd: 1},
		}
	}

	ac := NewAutoCompleteMenu(edit)
	want := []string{"./run.sh here", "", "./run.sh elsewhere", "", "./build.sh"}
	if strings.Join(ac.Matches, "|") != strings.Join(want, "|") {
		t.Fatalf("Matches = %q, want %q", ac.Matches, want)
	}
	if !ac.items[3].Separator {
		t.Fatal("history and path hints must be split by a separator")
	}

	// Without history rows there is no leading separator.
	edit.HistoryMatches = func(string) []AutoCompleteItem { return nil }
	ac.UpdateMatches()
	if len(ac.items) != 1 || ac.items[0].Text != "./build.sh" {
		t.Fatalf("items = %+v, want the path hint alone", ac.items)
	}
}

// Shift+Del removes the picked row from History, and the menu asks the hook
// again so the row disappears from the list.
func TestAutoComplete_HistoryMatchesHookShiftDelete(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	GlobalHistoryProvider = &mockHistoryProvider{storage: make(map[string][]string)}

	edit := NewEdit(0, 0, 40, "./")
	edit.HistoryID = "test"
	edit.History = []string{"./a.sh", "./b.sh"}
	edit.HistoryMatches = func(text string) []AutoCompleteItem {
		var items []AutoCompleteItem
		for _, h := range edit.History {
			items = append(items, AutoCompleteItem{Text: h, MatchStart: 0, MatchEnd: len(text) - 1})
		}
		return items
	}

	ac := NewAutoCompleteMenu(edit)
	ac.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	ac.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_DELETE, ControlKeyState: vtinput.ShiftPressed,
	})
	if len(edit.History) != 1 || edit.History[0] != "./b.sh" {
		t.Fatalf("History = %v, want the picked row removed", edit.History)
	}
	if len(ac.Matches) != 1 || ac.Matches[0] != "./b.sh" {
		t.Fatalf("Matches = %v, want the hook asked again", ac.Matches)
	}
}
