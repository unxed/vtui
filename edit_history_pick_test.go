package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// f4 #1331: picking an entry from a dialog field's history dropdown only puts
// the entry into the field, as far2l's Dialog::SelectFromEditHistory does. The
// pick used to queue an Enter behind it, which the dialog took for its default
// button: F7 created the folder the moment a name was picked from history.

func historyPickDialog(t *testing.T) (*frameManager, *Window, *Edit, *int) {
	t.Helper()
	fm := gestureManager(t)
	dlg := NewDialog(0, 1, 60, 20, "Create Folder")
	edit := NewEdit(5, 3, 30, "")
	edit.History = []string{"bbb", "aaa"}
	edit.ShowHistoryButton = true
	pressed := 0
	ok := NewButton(5, 6, "&Ok")
	ok.IsDefault = true
	ok.OnClick = func() { pressed++ }
	dlg.AddItem(edit)
	dlg.AddItem(ok)
	dlg.AddItem(NewButton(15, 6, "&Cancel"))
	fm.Push(dlg)
	dlg.SetFocusedItem(edit)
	return fm, dlg, edit, &pressed
}

// feedHistoryPick dispatches one event the way the event loop does, then runs
// whatever the handlers injected behind it, so a queued Enter would reach the
// dialog here just as it does in the application.
func feedHistoryPick(fm *frameManager, ev *vtinput.InputEvent) {
	fm.dispatchEvent(ev, false)
	fm.cleanupDoneFrames()
	for {
		fm.injectedMu.Lock()
		if len(fm.injectedEvents) == 0 {
			fm.injectedMu.Unlock()
			return
		}
		next := fm.injectedEvents[0]
		fm.injectedEvents = fm.injectedEvents[1:]
		fm.injectedMu.Unlock()
		fm.dispatchEvent(next, true)
		fm.cleanupDoneFrames()
	}
}

func historyPickKey(vk uint16, down bool, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	ev := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: down, VirtualKeyCode: vk, ControlKeyState: mods, RepeatCount: 1}
	if vk == vtinput.VK_RETURN {
		ev.Char = '\r'
	}
	return ev
}

func assertHistoryPickKeptDialog(t *testing.T, fm *frameManager, dlg *Window, edit *Edit, pressed int) {
	t.Helper()
	if got := edit.GetText(); got != "aaa" {
		t.Fatalf("field text after the pick = %q, want %q", got, "aaa")
	}
	if pressed != 0 {
		t.Fatalf("default button pressed %d time(s) by a history pick, want 0", pressed)
	}
	if top := fm.GetTopFrame(); top != dlg {
		t.Fatalf("top frame after the pick = %T, want the dialog", top)
	}
}

func TestEditHistoryPickWithKeyboardKeepsDialogOpen(t *testing.T) {
	fm, dlg, edit, pressed := historyPickDialog(t)

	// Ctrl+Down opens the list, as a console delivers it: key-down and
	// key-up records, modifiers included.
	feedHistoryPick(fm, historyPickKey(vtinput.VK_DOWN, true, vtinput.LeftCtrlPressed))
	if _, ok := fm.GetTopFrame().(*VMenu); !ok {
		t.Fatalf("Ctrl+Down did not open the history list, top frame is %T", fm.GetTopFrame())
	}
	feedHistoryPick(fm, historyPickKey(vtinput.VK_DOWN, false, vtinput.LeftCtrlPressed))
	feedHistoryPick(fm, historyPickKey(vtinput.VK_DOWN, true, 0))
	feedHistoryPick(fm, historyPickKey(vtinput.VK_DOWN, false, 0))
	feedHistoryPick(fm, historyPickKey(vtinput.VK_RETURN, true, 0))
	feedHistoryPick(fm, historyPickKey(vtinput.VK_RETURN, false, 0))

	assertHistoryPickKeptDialog(t, fm, dlg, edit, *pressed)
}

func TestEditHistoryPickWithMouseKeepsDialogOpen(t *testing.T) {
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
			fm, dlg, edit, pressed := historyPickDialog(t)

			// A click on the field's arrow opens the list.
			feedHistoryPick(fm, gestureEvent(edit.X2, edit.Y1, true, vtinput.FromLeft1stButtonPressed, false))
			feedHistoryPick(fm, gestureEvent(edit.X2, edit.Y1, rel.down, rel.buttons, false))
			menu, ok := fm.GetTopFrame().(*VMenu)
			if !ok {
				t.Fatalf("a click on the history arrow did not open the list, top frame is %T", fm.GetTopFrame())
			}

			// A click on the second entry, "aaa", picks it.
			x, y := menu.X1+2, menu.Y1+2
			feedHistoryPick(fm, gestureEvent(x, y, true, vtinput.FromLeft1stButtonPressed, false))
			feedHistoryPick(fm, gestureEvent(x, y, rel.down, rel.buttons, false))

			assertHistoryPickKeptDialog(t, fm, dlg, edit, *pressed)
		})
	}
}
