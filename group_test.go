package vtui

import (
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestGroup_FocusCycle(t *testing.T) {
	g := NewGroup(0, 0, 20, 10)
	g.WrapFocus = true
	b1 := NewButton(1, 1, "B1")
	txt := NewText(1, 2, "Static", 0) // Not focusable
	e1 := NewEdit(1, 3, 10, "E1")
	b2 := NewButton(1, 4, "B2")

	g.AddItem(b1)
	g.AddItem(txt)
	g.AddItem(e1)
	g.AddItem(b2)

	if g.focusIdx != 0 {
		t.Errorf("Initial focus: expected 0, got %d", g.focusIdx)
	}

	// 1. Tab -> skips txt (1), goes to e1 (index 2)
	g.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if g.focusIdx != 2 {
		t.Errorf("Tab 1: expected index 2 (e1), got %d", g.focusIdx)
	}
	if !e1.IsFocused() || b1.IsFocused() {
		t.Error("Focus state not updated correctly after Tab 1")
	}

	// 2. Tab -> goes to b2 (index 3)
	g.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if g.focusIdx != 3 {
		t.Errorf("Tab 2: expected index 3 (b2), got %d", g.focusIdx)
	}

	// 3. Tab -> cycles back to b1 (index 0)
	g.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if g.focusIdx != 0 {
		t.Errorf("Tab 3 (cycle): expected index 0 (b1), got %d", g.focusIdx)
	}
}

func TestGroup_Nested(t *testing.T) {
	// Window -> GroupBox -> Button
	dlg := NewDialog(0, 0, 40, 20, "Nested Test")
	outerEdit := NewEdit(1, 1, 10, "Outer")
	dlg.AddItem(outerEdit)

	gb := NewGroupBox(1, 3, 38, 10, "Inner Group")
	innerBtn1 := NewButton(gb.X1+1, gb.Y1+1, "&Inner1")
	innerBtn2 := NewButton(gb.X1+1, gb.Y1+2, "I&nner2")
	gb.AddItem(innerBtn1)
	gb.AddItem(innerBtn2)
	dlg.AddItem(gb)

	outerBtn := NewButton(1, 12, "Outer")
	dlg.AddItem(outerBtn)

	// Focus initially on outerEdit (index 0)
	if dlg.rootGroup.focusIdx != 0 {
		t.Fatalf("Initial focus should be on outerEdit, got %d", dlg.rootGroup.focusIdx)
	}

	// 1. Tab into the GroupBox. It's the next focusable element.
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if dlg.rootGroup.focusIdx != 1 { // GroupBox is item 1
		t.Fatalf("Tab 1: Focus should be on GroupBox. Got %d", dlg.rootGroup.focusIdx)
	}
	if !gb.IsFocused() {
		t.Fatal("GroupBox should be focused")
	}
	if gb.focusIdx != 0 || !innerBtn1.IsFocused() {
		t.Fatal("GroupBox did not pass focus to its first child (innerBtn1)")
	}

	// 2. Tab inside the GroupBox.
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if dlg.rootGroup.focusIdx != 1 { // Focus remains on GroupBox
		t.Fatal("Tab 2: Focus should remain on GroupBox.")
	}
	if gb.focusIdx != 1 || !innerBtn2.IsFocused() {
		t.Fatal("Tab 2: Focus did not move to innerBtn2.")
	}

	// 3. Tab out of the GroupBox.
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if dlg.rootGroup.focusIdx != 2 { // Focus moves to outerBtn (index 2)
		t.Fatalf("Tab 3: Focus should move to outerBtn. Got %d", dlg.rootGroup.focusIdx)
	}
	if !outerBtn.IsFocused() || gb.IsFocused() {
		t.Fatal("Tab 3: Focus state is incorrect.")
	}

	// 4. Test hotkey (Alt+I for innerBtn1)
	dlg.rootGroup.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'i', ControlKeyState: vtinput.LeftAltPressed})
	if dlg.rootGroup.focusIdx != 1 || gb.focusIdx != 0 || !innerBtn1.IsFocused() {
		t.Fatal("Hotkey did not correctly focus nested button.")
	}
}

func TestGroup_NoFocusableItems(t *testing.T) {
	g := NewGroup(0, 0, 10, 10)
	g.AddItem(NewText(1, 1, "Static Text", 0))

	done := make(chan bool, 1)
	go func() {
		g.changeFocus(1)
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Deadlock in changeFocus with no focusable items")
	}

	if g.focusIdx != -1 {
		t.Errorf("Expected focusIdx to be -1, got %d", g.focusIdx)
	}
}

func TestGroup_SetFocusedItem(t *testing.T) {
	g := NewGroup(0, 0, 20, 10)
	b1 := NewButton(0, 0, "B1")
	b2 := NewButton(0, 1, "B2")
	g.AddItem(b1)
	g.AddItem(b2)

	// Initial focus is on b1
	if g.GetFocusedItem() != b1 { t.Fatal("Initial focus fail") }

	// Set focus to b2
	g.SetFocusedItem(b2)
	if g.GetFocusedItem() != b2 {
		t.Error("SetFocusedItem failed to move focus to b2")
	}
	if !b2.IsFocused() || b1.IsFocused() {
		t.Error("Focus flags not updated after SetFocusedItem")
	}
}
func TestGroup_ActivateHotkey_FocusLinkRecursion(t *testing.T) {
	// Tests that hotkey on a Label focuses the linked element even if nested
	dlg := NewDialog(0, 0, 40, 10, "Hotkey Test")

	group := NewGroupBox(1, 1, 38, 8, "Inner")
	edit := NewEdit(2, 4, 10, "")
	label := NewLabel(2, 2, "&Target:", edit)

	group.AddItem(label)
	group.AddItem(edit)
	dlg.AddItem(group)

	// Focus initially elsewhere
	dummy := NewButton(1, 1, "Dummy")
	dlg.AddItem(dummy)
	dlg.SetFocusedItem(dummy)

	// Activate hotkey 't' (from &Target)
	dlg.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 't',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !edit.IsFocused() {
		t.Error("Hotkey failed to focus linked element through nested group")
	}
	if !group.IsFocused() {
		t.Error("Parent group of the focused element should also be marked as focused")
	}
}
