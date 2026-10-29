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
