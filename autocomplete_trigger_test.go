package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// typeChar feeds a single printable character to the frame on top, which is
// the edit itself until a completion menu opens over it.
func typeChar(t *testing.T, edit *Edit, ch rune) {
	t.Helper()
	ev := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ch}
	if ac, ok := FrameManager.GetTopFrame().(*AutoCompleteMenu); ok {
		ac.ProcessKey(ev)
		return
	}
	edit.ProcessKey(ev)
}

func topAutoComplete() *AutoCompleteMenu {
	ac, _ := FrameManager.GetTopFrame().(*AutoCompleteMenu)
	return ac
}

func newHistoryEdit(history ...string) *Edit {
	edit := NewEdit(0, 10, 30, "")
	edit.History = history
	edit.HistoryID = "Test"
	return edit
}

func TestAutoComplete_OpensOnTypingInHistoryField(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	edit := newHistoryEdit("git status", "git commit", "ls -la")
	typeChar(t, edit, 'g')
	ac := topAutoComplete()
	if ac == nil {
		t.Fatal("typing in a history field did not open the completion menu")
	}
	defer ac.Close()

	if len(ac.Matches) != 2 {
		t.Fatalf("expected the two git entries, got %v", ac.Matches)
	}

	// The menu stays open and narrows as more characters arrive.
	typeChar(t, edit, 'i')
	typeChar(t, edit, 't')
	typeChar(t, edit, ' ')
	typeChar(t, edit, 'c')
	if got := edit.GetText(); got != "git c" {
		t.Fatalf("typing did not reach the edit: text is %q", got)
	}
	// Fuzzy matching keeps "git status" (1 error, k = 1) as a tail result,
	// but the prefix match must rank first.
	if len(ac.Matches) != 2 || ac.Matches[0] != "git commit" || ac.Matches[1] != "git status" {
		t.Errorf("menu did not rank matches while typing: %v", ac.Matches)
	}
}

func TestAutoComplete_ClosesWhenNothingMatches(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	edit := newHistoryEdit("git status")
	typeChar(t, edit, 'g')
	ac := topAutoComplete()
	if ac == nil {
		t.Fatal("menu did not open")
	}
	typeChar(t, edit, 'z')
	if !ac.IsDone() {
		ac.Close()
		t.Errorf("menu stayed open with no matches: %v", ac.Matches)
	}
}

func TestAutoComplete_NotOfferedWithoutHistoryOrPathHints(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	// A plain field qualifies for nothing, so the switch cannot reach it.
	plain := NewEdit(0, 10, 30, "")
	AutoCompleteEnabled = true
	typeChar(t, plain, 'a')
	if ac := topAutoComplete(); ac != nil {
		ac.Close()
		t.Error("a field with neither history nor path hints opened a menu")
	}
	if plain.autoCompletes() {
		t.Error("a plain field reports itself as completing")
	}
}

func TestAutoComplete_GlobalSwitchOnlySubtracts(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	previous := AutoCompleteEnabled
	defer func() { AutoCompleteEnabled = previous }()

	edit := newHistoryEdit("git status")
	AutoCompleteEnabled = false
	typeChar(t, edit, 'g')
	if ac := topAutoComplete(); ac != nil {
		ac.Close()
		t.Fatal("menu opened while the global switch was off")
	}

	AutoCompleteEnabled = true
	typeChar(t, edit, 'i')
	ac := topAutoComplete()
	if ac == nil {
		t.Fatal("menu did not come back when the switch was turned on")
	}
	ac.Close()
}

func TestAutoComplete_NoAutoCompleteOptsFieldOut(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	// Far's go-to-line prompt carries history but sets DIF_NOAUTOCOMPLETE.
	edit := newHistoryEdit("120", "1200")
	edit.NoAutoComplete = true
	typeChar(t, edit, '1')
	if ac := topAutoComplete(); ac != nil {
		ac.Close()
		t.Error("an opted-out field opened a menu")
	}
	if got := edit.GetText(); got != "1" {
		t.Errorf("opting out swallowed the keystroke: text is %q", got)
	}
}

func TestAutoComplete_PasswordFieldNeverCompletes(t *testing.T) {
	SetDefaultPalette()

	edit := NewPasswordEdit(0, 10, 30, "")
	edit.History = []string{"hunter2"}
	if edit.autoCompletes() {
		t.Error("a masked field would have shown its history in clear text")
	}
}

func TestAutoComplete_PathOnlyFieldStillWaitsForSeparator(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	old := PathHintProvider
	defer func() { PathHintProvider = old }()
	PathHintProvider = func(edit *Edit, word string, from, to int) []AutoCompleteItem {
		return []AutoCompleteItem{
			{Text: "/usr/bin/", MatchStart: 0, MatchEnd: 4, ReplaceFrom: from, ReplaceTo: to},
		}
	}

	edit := NewEdit(0, 10, 30, "")
	edit.PathHintsEnabled = true
	// No history here, so ordinary characters must not rebuild the menu on
	// every keystroke of a long path -- only a separator opens it.
	typeChar(t, edit, 'u')
	if ac := topAutoComplete(); ac != nil {
		ac.Close()
		t.Fatal("a plain character opened the menu in a path-only field")
	}
	typeChar(t, edit, '/')
	ac := topAutoComplete()
	if ac == nil {
		t.Fatal("a separator did not open the menu in a path-only field")
	}
	ac.Close()
}
