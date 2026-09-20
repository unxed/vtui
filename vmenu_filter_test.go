package vtui

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

func filterKey(vk uint16, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: mods}
}

func filterChar(r rune) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r}
}

const ctrlAlt = vtinput.LeftCtrlPressed | vtinput.LeftAltPressed

func typeFilter(m *VMenu, text string) {
	for _, r := range text {
		m.ProcessKey(filterChar(r))
	}
}

func newFilterMenu(texts ...string) *VMenu {
	m := NewVMenu("Pick")
	for _, s := range texts {
		if s == "-" {
			m.AddSeparator()
		} else {
			m.AddItem(MenuItem{Text: s})
		}
	}
	m.SetPosition(0, 0, 30, 9) // ViewHeight 8
	m.ClearDone()
	return m
}

// menuRowTexts reads the item texts the menu painted, top to bottom.
func menuRowTexts(t *testing.T, m *VMenu) []string {
	t.Helper()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 12)
	m.Show(scr)
	var rows []string
	for y := m.Y1 + 1; y < m.Y2; y++ {
		var sb strings.Builder
		for x := m.X1 + 1; x < m.X2; x++ {
			sb.WriteRune(rune(scr.GetCell(x, y).Char))
		}
		if text := strings.TrimSpace(sb.String()); text != "" {
			rows = append(rows, text)
		}
	}
	return rows
}

func TestVMenuFilter_CtrlAltFHidesRowsAndKeepsItemIndices(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("alpha", "beta", "gamma", "delta", "epsilon")

	confirmed := -1
	m.OnAction = func(i int) { confirmed = i }

	// Letters do nothing until the filter is on.
	m.ProcessKey(filterChar('l'))
	if m.FilterText() != "" {
		t.Fatalf("a letter started the filter without Ctrl+Alt+F: %q", m.FilterText())
	}

	if !m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt)) {
		t.Fatal("Ctrl+Alt+F was not taken")
	}
	typeFilter(m, "LTA")
	if m.FilterText() != "LTA" {
		t.Fatalf("filter text = %q, want LTA", m.FilterText())
	}
	if got := menuRowTexts(t, m); strings.Join(got, "|") != "delta" {
		t.Fatalf("shown rows = %v, want only delta", got)
	}
	// The selection was on alpha, which is hidden now: it moves to the
	// first shown item and names it by its item index.
	if m.SelectPos != 3 {
		t.Fatalf("SelectPos = %d, want 3 (delta)", m.SelectPos)
	}

	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	// "l" matches alpha, delta and epsilon.
	if got := menuRowTexts(t, m); strings.Join(got, "|") != "alpha|delta|epsilon" {
		t.Fatalf("shown rows = %v, want alpha|delta|epsilon", got)
	}
	m.ProcessKey(filterKey(vtinput.VK_DOWN, 0))
	if m.SelectPos != 4 {
		t.Fatalf("Down should skip hidden rows to epsilon (4), got %d", m.SelectPos)
	}
	m.ProcessKey(filterKey(vtinput.VK_DOWN, 0))
	if m.SelectPos != 0 {
		t.Fatalf("Down on the last shown row should wrap to alpha (0), got %d", m.SelectPos)
	}
	m.ProcessKey(filterKey(vtinput.VK_END, 0))
	if m.SelectPos != 4 {
		t.Fatalf("End should land on the last shown row (4), got %d", m.SelectPos)
	}

	m.ProcessKey(filterKey(vtinput.VK_RETURN, 0))
	if confirmed != 4 || !m.IsDone() || m.exitCode != 4 {
		t.Fatalf("Enter must confirm the item index: OnAction %d, done %v, exit %d", confirmed, m.IsDone(), m.exitCode)
	}
	if m.FilterText() != "" || m.filtering() {
		t.Fatal("closing the menu must drop the filter")
	}
}

func TestVMenuFilter_EmptyStringShowsEverythingAndCtrlAltFTurnsItOff(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("one", "two", "three")

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	if got := menuRowTexts(t, m); len(got) != 3 {
		t.Fatalf("an empty filter should show all rows, got %v", got)
	}
	typeFilter(m, "tw")
	if got := menuRowTexts(t, m); strings.Join(got, "|") != "two" {
		t.Fatalf("shown rows = %v, want two", got)
	}
	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	if m.FilterText() != "" {
		t.Fatalf("second Ctrl+Alt+F should turn the filter off, text %q", m.FilterText())
	}
	if got := menuRowTexts(t, m); len(got) != 3 {
		t.Fatalf("turning the filter off should show all rows, got %v", got)
	}
	if m.SelectPos != 1 {
		t.Fatalf("the selection should stay on two (1), got %d", m.SelectPos)
	}
}

func TestVMenuFilter_TitleShowsFilterAndLock(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("one", "two")

	if m.displayTitle() != "Pick" {
		t.Fatalf("title without a filter = %q", m.displayTitle())
	}
	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	if m.displayTitle() != "Pick []" {
		t.Fatalf("title with an empty filter = %q, want %q", m.displayTitle(), "Pick []")
	}
	typeFilter(m, "o")
	if m.displayTitle() != "Pick [o]" {
		t.Fatalf("title = %q, want %q", m.displayTitle(), "Pick [o]")
	}
	m.ProcessKey(filterKey(vtinput.VK_L, ctrlAlt))
	if m.displayTitle() != "Pick <o>" {
		t.Fatalf("locked title = %q, want %q", m.displayTitle(), "Pick <o>")
	}
}

func TestVMenuFilter_LockedFilterHandsLettersToShownHotkeys(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("Hotkeys")
	m.AddItem(MenuItem{Text: "&Copy"})
	m.AddItem(MenuItem{Text: "&Rename"})
	m.AddItem(MenuItem{Text: "&Delete"})
	m.SetPosition(0, 0, 30, 6)
	m.ClearDone()

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "e")
	m.ProcessKey(filterKey(vtinput.VK_L, ctrlAlt))

	// Copy is hidden by the filter, so its hotkey must not fire.
	m.ProcessKey(filterChar('c'))
	if m.IsDone() {
		t.Fatal("the hotkey of a hidden item fired")
	}
	if m.FilterText() != "e" {
		t.Fatalf("a locked filter took a letter: %q", m.FilterText())
	}
	m.ProcessKey(filterChar('d'))
	if !m.IsDone() || m.exitCode != 2 {
		t.Fatalf("the hotkey of a shown item should fire: done %v, exit %d", m.IsDone(), m.exitCode)
	}
}

func TestVMenuFilter_SeparatorsOnlyBetweenShownGroups(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("apple", "apricot", "-", "banana", "-", "avocado")

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "a")
	rows := m.visibleRows()
	// "a" is in every fruit, so both separators still divide something.
	if len(rows) != 6 {
		t.Fatalf("rows = %v, want all six", rows)
	}
	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	typeFilter(m, "av")
	if rows := m.visibleRows(); len(rows) != 1 || rows[0] != 5 {
		t.Fatalf("rows = %v, want only avocado without a leading separator", rows)
	}
	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	typeFilter(m, "o")
	// apricot | avocado — banana's group is gone, one separator stays.
	if rows := m.visibleRows(); len(rows) != 3 || rows[0] != 1 || rows[1] != 4 || rows[2] != 5 {
		t.Fatalf("rows = %v, want [1 4 5]", rows)
	}
	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	typeFilter(m, "an")
	if rows := m.visibleRows(); len(rows) != 1 || rows[0] != 3 {
		t.Fatalf("rows = %v, want only banana", rows)
	}
}

func TestVMenuFilter_NoMatchWithholdsKeysThatWouldActOnAHiddenItem(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("one", "two")

	hookCalls := 0
	m.OnKeyDown = func(e *vtinput.InputEvent) bool {
		hookCalls++
		return false
	}
	confirmed := -1
	m.OnAction = func(i int) { confirmed = i }

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "zz")
	if m.FilterText() != "z" {
		t.Fatalf("far2l stops taking characters once nothing matches; text %q", m.FilterText())
	}
	if hookCalls != 0 {
		t.Fatalf("filter keys reached OnKeyDown %d times", hookCalls)
	}

	m.ProcessKey(filterKey(vtinput.VK_RETURN, 0))
	m.ProcessKey(filterKey(vtinput.VK_DELETE, vtinput.ShiftPressed))
	if hookCalls != 0 || confirmed != -1 || m.IsDone() {
		t.Fatalf("keys acted on a hidden item: hook %d, OnAction %d, done %v", hookCalls, confirmed, m.IsDone())
	}
	if m.GetClickIndex(1) != -1 {
		t.Fatal("a click with nothing shown must not name an item")
	}

	m.ProcessKey(filterKey(vtinput.VK_ESCAPE, 0))
	if !m.IsDone() {
		t.Fatal("Esc must still close the menu")
	}
}

func TestVMenuFilter_MouseClickNamesTheShownItem(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("red", "green", "blue", "grey")
	confirmed := -1
	m.OnAction = func(i int) { confirmed = i }

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "gr")
	// Row 2 of the box (y = 2) is the second shown item: grey.
	m.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		MouseX:      5,
		MouseY:      2,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		KeyDown:     true,
	})
	if confirmed != 3 {
		t.Fatalf("click should confirm grey (3), got %d", confirmed)
	}
}

func TestVMenuFilter_PageKeysAndWheelWalkShownRows(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("Long")
	for i := 0; i < 40; i++ {
		text := "skip"
		if i%2 == 0 {
			text = "keep"
		}
		m.AddItem(MenuItem{Text: text})
	}
	m.SetPosition(0, 0, 20, 6) // ViewHeight 5
	m.ClearDone()

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "keep") // 20 shown rows: items 0, 2, ..., 38
	if m.SelectPos != 0 || m.filterTop != 0 {
		t.Fatalf("start: SelectPos %d, filterTop %d", m.SelectPos, m.filterTop)
	}
	m.ProcessKey(filterKey(vtinput.VK_NEXT, 0))
	if m.SelectPos != 10 || m.filterTop != 5 {
		t.Fatalf("PgDn: SelectPos %d (want item 10), filterTop %d (want 5)", m.SelectPos, m.filterTop)
	}
	m.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	if m.SelectPos%2 != 0 || m.SelectPos <= 10 {
		t.Fatalf("wheel down should move to a later shown item, got %d", m.SelectPos)
	}
	m.ProcessKey(filterKey(vtinput.VK_HOME, 0))
	if m.SelectPos != 0 || m.filterTop != 0 {
		t.Fatalf("Home: SelectPos %d, filterTop %d", m.SelectPos, m.filterTop)
	}
	m.ProcessKey(filterKey(vtinput.VK_PRIOR, 0))
	if m.SelectPos != 0 {
		t.Fatalf("PgUp at the top must stay, got %d", m.SelectPos)
	}
}

func TestVMenuFilter_DisabledAndVirtualMenusIgnoreIt(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	m := newFilterMenu("one", "two")
	m.DisableFilter = true
	var seen []rune
	m.OnKeyDown = func(e *vtinput.InputEvent) bool {
		if e.Char != 0 {
			seen = append(seen, e.Char)
			return true
		}
		return false
	}
	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	m.ProcessKey(filterChar('x'))
	if m.FilterText() != "" || string(seen) != "x" {
		t.Fatalf("DisableFilter: filter %q, OnKeyDown saw %q", m.FilterText(), string(seen))
	}

	v := NewVMenu("Virtual")
	v.ItemCount = 50
	v.FilterOnType = true
	v.SetPosition(0, 0, 20, 6)
	v.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	v.ProcessKey(filterChar('a'))
	if v.FilterText() != "" {
		t.Fatalf("a virtual menu has no item text to filter, got %q", v.FilterText())
	}
}

func TestVMenuFilter_ConsumerDeletionKeepsSelectionOnShownItem(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("cat", "dog", "cow", "pig", "cod")
	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "c") // cat, cow, cod
	m.ProcessKey(filterKey(vtinput.VK_DOWN, 0))
	if m.SelectPos != 2 {
		t.Fatalf("SelectPos = %d, want cow (2)", m.SelectPos)
	}
	// What Edit's Shift+Del does: drop the item and reselect its index,
	// which now names the hidden "pig".
	m.Items = append(m.Items[:2], m.Items[3:]...)
	m.ItemCount = len(m.Items)
	m.SetSelectPos(2)
	if m.Items[m.SelectPos].Text != "cod" {
		t.Fatalf("selection landed on %q, want the next shown item cod", m.Items[m.SelectPos].Text)
	}
}

func TestVMenuFilter_SemanticActivateUsesFullListIndex(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := newFilterMenu("red", "green", "blue")
	confirmed := -1
	m.OnAction = func(i int) { confirmed = i }

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "gr") // only green is shown
	m.HandleSemanticAction(map[string]any{"target": SemanticID(m), "action": "menu.activate", "index": 2})
	if confirmed != 2 {
		t.Fatalf("activating index 2 of the full list confirmed %d, want blue (2)", confirmed)
	}
}

// f4 #263: the history dropdown of an input field narrows as you type, and
// the keys that act on an entry act on the one the filtered list shows.
func TestEdit_HistoryMenu_TypingFilters(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())
	mock := &mockHistoryProvider{storage: map[string][]string{
		"dlg": {"make test", "git status", "make build", "ls -la"},
	}}
	GlobalHistoryProvider = mock
	defer func() { GlobalHistoryProvider = nil }()

	e := NewEdit(0, 0, 30, "")
	e.HistoryID = "dlg"
	e.OpenHistory()
	menu, ok := fm.GetTopFrame().(*VMenu)
	if !ok {
		t.Fatal("OpenHistory did not push a VMenu")
	}

	typeFilter(menu, "make")
	if got := menu.visibleRows(); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("shown rows = %v, want [0 2]", got)
	}
	menu.ProcessKey(filterKey(vtinput.VK_DOWN, 0))

	// Shift+Del drops "make build", not whatever sits at row 1 unfiltered.
	menu.ProcessKey(filterKey(vtinput.VK_DELETE, vtinput.ShiftPressed))
	if strings.Join(e.History, "|") != "make test|git status|ls -la" {
		t.Fatalf("Shift+Del removed the wrong entry: %v", e.History)
	}
	if menu.Items[menu.SelectPos].Text != "make test" {
		t.Fatalf("after the delete the cursor is on %q, want the remaining match", menu.Items[menu.SelectPos].Text)
	}

	menu.ProcessKey(filterKey(vtinput.VK_RETURN, vtinput.ShiftPressed))
	if e.GetText() != "make test" {
		t.Fatalf("Shift+Enter inserted %q, want %q", e.GetText(), "make test")
	}
}

// A separator's heading is drawn by the menu on the separator's own row, so it
// goes with the row when the filter hides items and is not left behind on a
// row that holds something else (f4 #263).
func TestVMenuFilter_SeparatorHeadingsFollowTheirRows(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("Pick")
	m.AddItem(MenuItem{Text: "apple"})
	m.AddItem(MenuItem{Separator: true, Text: "Fruit"})
	m.AddItem(MenuItem{Text: "banana"})
	m.AddItem(MenuItem{Separator: true, Text: "Veg"})
	m.AddItem(MenuItem{Text: "carrot"})
	m.SetPosition(0, 0, 30, 9)
	m.ClearDone()

	rowWith := func(rows []string, s string) int {
		for i, r := range rows {
			if strings.Contains(r, s) {
				return i
			}
		}
		return -1
	}

	rows := menuRowTexts(t, m)
	if apple, fruit, banana := rowWith(rows, "apple"), rowWith(rows, "Fruit"), rowWith(rows, "banana"); fruit != apple+1 || banana != fruit+1 {
		t.Fatalf("unfiltered rows %q: the heading is not between apple and banana", rows)
	}

	m.ProcessKey(filterKey(vtinput.VK_F, ctrlAlt))
	typeFilter(m, "n")
	// banana and... only banana holds an "n": the headings divide nothing now.
	rows = menuRowTexts(t, m)
	if rowWith(rows, "banana") != 0 || rowWith(rows, "Fruit") != -1 || rowWith(rows, "Veg") != -1 {
		t.Fatalf("filtered rows %q: headings were left on screen", rows)
	}

	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	typeFilter(m, "r")
	// carrot, banana? "r" is in carrot only; and none between.
	rows = menuRowTexts(t, m)
	if rowWith(rows, "carrot") != 0 || rowWith(rows, "Fruit") != -1 || rowWith(rows, "Veg") != -1 {
		t.Fatalf("filtered rows %q: headings were left on screen", rows)
	}

	m.ProcessKey(filterKey(vtinput.VK_BACK, 0))
	typeFilter(m, "a")
	// apple, banana, carrot all hold an "a": both headings are back, each on its own row.
	rows = menuRowTexts(t, m)
	if apple, fruit, banana, veg, carrot := rowWith(rows, "apple"), rowWith(rows, "Fruit"), rowWith(rows, "banana"), rowWith(rows, "Veg"), rowWith(rows, "carrot"); fruit != apple+1 || banana != fruit+1 || veg != banana+1 || carrot != veg+1 {
		t.Fatalf("rows %q: headings are not on their separators", rows)
	}
}
