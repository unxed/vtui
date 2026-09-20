package vtui

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/unxed/vtinput"
)

// newFrozenMenu is a 30-item menu with a view of ten rows whose first three
// items are frozen at the top (f4 #1233: pinned folders stay in sight).
func newFrozenMenu() *VMenu {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("Frozen")
	for i := 0; i < 30; i++ {
		m.AddItem(MenuItem{Text: fmt.Sprintf("item %d", i)})
	}
	m.DisableFilter = true
	m.SetPosition(0, 0, 30, 11) // ViewHeight 10
	m.ClearDone()
	m.FrozenTop = 3
	m.SetSelectPos(0)
	return m
}

func rowsShown(m *VMenu) []int {
	rows := make([]int, m.ViewHeight)
	for i := range rows {
		rows[i] = m.ItemAtRow(i)
	}
	return rows
}

func TestFrozenTopStaysWhileTheRestScrolls(t *testing.T) {
	m := newFrozenMenu()
	m.SetSelectPos(29)

	want := []int{0, 1, 2, 23, 24, 25, 26, 27, 28, 29}
	if got := rowsShown(m); !reflect.DeepEqual(got, want) {
		t.Fatalf("items on the rows after jumping to the end = %v, want %v", got, want)
	}

	// Walking up scrolls the lower part only.
	for i := 0; i < 7; i++ {
		m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	}
	want = []int{0, 1, 2, 22, 23, 24, 25, 26, 27, 28}
	if got := rowsShown(m); m.SelectPos != 22 || !reflect.DeepEqual(got, want) {
		t.Fatalf("after 7 x Up: select %d, items on the rows %v, want select 22, %v", m.SelectPos, got, want)
	}
}

func TestFrozenTopSelectionInsideTheFrozenRowsDoesNotScroll(t *testing.T) {
	m := newFrozenMenu()
	m.SetSelectPos(29)
	top := m.TopPos

	m.SetSelectPos(1)
	if m.TopPos != top {
		t.Errorf("selecting a frozen row moved the view: TopPos %d -> %d", top, m.TopPos)
	}
	// Coming back out of the frozen rows must bring the selection into view.
	m.SetSelectPos(4)
	if got := rowsShown(m); got[3] != 4 {
		t.Errorf("selected item 4 is not on the first scrolling row: %v", got)
	}
}

func TestFrozenTopPagesByTheScrollingRows(t *testing.T) {
	m := newFrozenMenu()
	m.SetSelectPos(3)
	m.PageBy(1)
	// Seven rows scroll, so one page down moves seven items.
	if m.SelectPos != 10 {
		t.Errorf("PgDn from 3 selected %d, want 10", m.SelectPos)
	}
	if got := rowsShown(m); got[0] != 0 || got[3] != m.TopPos {
		t.Errorf("rows after PgDn = %v", got)
	}
}

func TestFrozenTopClickHitsWhatIsDrawn(t *testing.T) {
	m := newFrozenMenu()
	m.SetSelectPos(29)

	first := m.Y1 + m.MarginTop
	for row, want := range rowsShown(m) {
		if got := m.GetClickIndex(first + row); got != want {
			t.Errorf("click on row %d hit item %d, want %d", row, got, want)
		}
	}
}

func TestFrozenTopIsPaintedAtTheTop(t *testing.T) {
	m := newFrozenMenu()
	m.SetSelectPos(29)

	want := []string{"item 0", "item 1", "item 2", "item 23", "item 24", "item 25", "item 26", "item 27", "item 28", "item 29"}
	if got := menuRowTexts(t, m); !reflect.DeepEqual(got, want) {
		t.Errorf("rows painted = %v, want %v", got, want)
	}
}

func TestFrozenTopScrollBarCountsOnlyTheScrollingPart(t *testing.T) {
	m := newFrozenMenu()
	m.SetSelectPos(29)

	// Scrolled to the very end: the bar has to be at its maximum, not short
	// of it by the frozen rows.
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 12)
	m.DrawScrollBar(scr)
	if m.ScrollBar.Value != m.ScrollBar.Max {
		t.Errorf("scrollbar at %d of %d at the end of the list", m.ScrollBar.Value, m.ScrollBar.Max)
	}
}

func TestFrozenTopIgnoredWhenItLeavesNoRoom(t *testing.T) {
	m := newFrozenMenu()
	m.FrozenTop = 8 // more than half of the ten rows
	m.SetSelectPos(29)

	if got := rowsShown(m); got[0] != 20 {
		t.Errorf("a frozen area taller than half the view still froze rows: %v", got)
	}
}

func TestFrozenTopZeroChangesNothing(t *testing.T) {
	m := newFrozenMenu()
	m.FrozenTop = 0
	m.SetSelectPos(29)

	want := []int{20, 21, 22, 23, 24, 25, 26, 27, 28, 29}
	if got := rowsShown(m); !reflect.DeepEqual(got, want) {
		t.Errorf("an ordinary menu shows %v, want %v", got, want)
	}
}
