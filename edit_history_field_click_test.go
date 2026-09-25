package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// f4 #1392: a plain click anywhere in a ShowHistoryButton field's body opens
// the history drop-down, matching how a DropdownOnly ComboBox already opens
// on a click anywhere in its field, not only on the arrow. A click that turns
// into a drag still selects text instead, exactly as ComboBox's editable
// path already does.

func TestEditHistoryFieldClickOpensHistory(t *testing.T) {
	releases := []struct {
		name    string
		down    bool
		buttons uint32
	}{
		{"console release", true, 0},
		{"SGR release", false, vtinput.FromLeft1stButtonPressed},
	}
	for _, rel := range releases {
		t.Run(rel.name, func(t *testing.T) {
			fm, _, edit, _ := historyPickDialog(t)

			// A click inside the field body, away from the [v] arrow at X2.
			x, y := edit.X1+3, edit.Y1
			if x == edit.X2 {
				t.Fatalf("test field too narrow to click away from the arrow")
			}
			feedHistoryPick(fm, gestureEvent(x, y, true, vtinput.FromLeft1stButtonPressed, false))
			feedHistoryPick(fm, gestureEvent(x, y, rel.down, rel.buttons, false))

			if _, ok := fm.GetTopFrame().(*VMenu); !ok {
				t.Fatalf("a plain click in the field body did not open the history list, top frame is %T", fm.GetTopFrame())
			}
		})
	}
}

func TestEditHistoryFieldDragSelectsTextInsteadOfHistory(t *testing.T) {
	fm, dlg, edit, _ := historyPickDialog(t)
	edit.SetText("hello")

	x, y := edit.X1+1, edit.Y1
	feedHistoryPick(fm, gestureEvent(x, y, true, vtinput.FromLeft1stButtonPressed, false))
	// A move while the button is still down turns the click into a drag.
	feedHistoryPick(fm, gestureEvent(x+3, y, true, vtinput.FromLeft1stButtonPressed, true))
	feedHistoryPick(fm, gestureEvent(x+3, y, false, 0, false))

	if top := fm.GetTopFrame(); top != dlg {
		t.Fatalf("a drag opened the history list instead of selecting text, top frame is %T", top)
	}
	if edit.selStart == -1 || edit.selStart == edit.selEnd {
		t.Fatalf("dragging across the field text did not select anything, selStart=%d selEnd=%d", edit.selStart, edit.selEnd)
	}
}
