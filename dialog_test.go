package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestDialog_FocusCycle(t *testing.T) {
	d := NewDialog(0, 0, 20, 10, "Test")
	b1 := NewButton(1, 1, "B1")
	txt := NewText(1, 2, "Static", 0) // Text is not focusable
	e1 := NewEdit(1, 3, 10, "E1")
	b2 := NewButton(1, 4, "B2")

	d.AddItem(b1)
	d.AddItem(txt)
	d.AddItem(e1)
	d.AddItem(b2)

	// Initial focus should be on the first focusable element (index 0 - b1)
	if d.focusIdx != 0 {
		t.Errorf("Initial focus: expected 0, got %d", d.focusIdx)
	}
	if !b1.IsFocused() {
		t.Error("b1 should be focused initially")
	}

	// 1. Tab -> skips txt (1), goes to e1 (index 2)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if d.focusIdx != 2 {
		t.Errorf("Tab 1: expected index 2 (e1), got %d", d.focusIdx)
	}
	if !e1.IsFocused() || b1.IsFocused() {
		t.Error("Focus state not updated correctly after Tab 1")
	}

	// 2. Tab -> goes to b2 (index 3)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if d.focusIdx != 3 {
		t.Errorf("Tab 2: expected index 3 (b2), got %d", d.focusIdx)
	}

	// 3. Tab -> cycles back to b1 (index 0)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if d.focusIdx != 0 {
		t.Errorf("Tab 3 (cycle): expected index 0 (b1), got %d", d.focusIdx)
	}

	// 4. Shift+Tab -> cycles backward to b2 (index 3)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed})
	if d.focusIdx != 3 {
		t.Errorf("Shift+Tab 1 (cycle back): expected index 3 (b2), got %d", d.focusIdx)
	}

	// 5. Shift+Tab -> goes back to e1 (index 2)
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed})
	if d.focusIdx != 2 {
		t.Errorf("Shift+Tab 2: expected index 2 (e1), got %d", d.focusIdx)
	}
}

func TestDialog_RadioButtonIntegration(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Radio Test")
	rb1 := NewRadioButton(1, 1, "R1")
	rb2 := NewRadioButton(1, 2, "R2")
	rb1.Selected = true
	d.AddItem(rb1)
	d.AddItem(rb2)

	// Move to rb2 (Tab)
	d.changeFocus(1)

	// Press Space. Dialog should intercept this and update the group.
	d.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_SPACE,
	})

	if rb1.Selected {
		t.Error("rb1 should be deselected via ProcessKey(Space)")
	}
	if !rb2.Selected {
		t.Error("rb2 should be selected via ProcessKey(Space)")
	}
}
func TestDialog_Shadow(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(20, 20)

	// 5x5 Dialog
	d := NewDialog(2, 2, 7, 7, "Title")
	d.Show(scr)

	// Checking shadow cell.
	// Bottom-right corner of dialog is (7, 7).
	// Shadow should be at (8, 8) and (9, 8).
	shAttr := Palette[ColShadow]
	checkCell(t, scr, 9, 8, ' ', shAttr)
	checkCell(t, scr, 8, 3, ' ', shAttr) // Vertical shadow part
}
func TestDialog_MouseFocus(t *testing.T) {
	d := NewDialog(0, 0, 20, 10, "Test")
	b1 := NewButton(1, 1, "B1")
	b2 := NewButton(1, 2, "B2")
	d.AddItem(b1)
	d.AddItem(b2)

	// Initially, b1 (index 0) has focus
	if d.focusIdx != 0 {
		t.Fatalf("Initial focus should be on b1 (0), got %d", d.focusIdx)
	}

	// Simulate a click on b2 at (1, 2)
	d.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      1,
		MouseY:      2,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})

	// Focus should now be on b2 (index 1)
	if d.focusIdx != 1 {
		t.Errorf("Mouse click failed to change focus: expected 1, got %d", d.focusIdx)
	}
	if !b2.IsFocused() {
		t.Error("b2 should be focused after click")
	}
	if b1.IsFocused() {
		t.Error("b1 should lose focus after click on b2")
	}
}

func TestDialog_HotkeyColor(t *testing.T) {
	d := NewDialog(0, 0, 20, 5, "Test")
	btn := NewButton(1, 1, "&Test")
	d.AddItem(btn)

	scr := NewScreenBuf()
	scr.AllocBuf(22, 7)
	SetDefaultPalette()

	// 1. Unfocused state
	btn.SetFocus(false)
	d.Show(scr)
	// Hotkey 'T' should have highlight color
	// Text is "[ &Test ]", clean is "[ Test ]". Hotkey pos is 2.
	checkCell(t, scr, 1+2, 1, 'T', Palette[ColDialogHighlightButton])
	
	// 2. Focused state
	btn.SetFocus(true)
	d.Show(scr)
	// Hotkey 'T' should have SELECTED highlight color
	checkCell(t, scr, 1+2, 1, 'T', Palette[ColDialogHighlightSelectedButton])
}

func TestDialog_HotkeyActivation(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Hotkey Test")

	clicked := false
	btn := NewButton(1, 1, "&Click")
	btn.OnClick = func() { clicked = true }

	e1 := NewEdit(1, 2, 10, "")

	d.AddItem(btn)
	d.AddItem(e1)

	// Move focus to Edit so button is not focused
	d.focusIdx = 1
	btn.SetFocus(false)
	e1.SetFocus(true)

	// 1. Press Alt+C (button hotkey)
	d.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'c',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !clicked {
		t.Error("Button should be clicked via Alt+C")
	}
	if d.focusIdx != 0 || !btn.IsFocused() {
		t.Error("Focus should move to the button after hotkey activation")
	}
}

func TestDialog_LabelFocusLink(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "FocusLink Test")

	edit := NewEdit(1, 2, 10, "")
	label := NewText(1, 1, "&Name:", 0)
	label.FocusLink = edit // Link label to the edit field!

	d.AddItem(label)
	d.AddItem(edit)

	// Initially focus on first element (label), but it is not canFocus,
	// so Dialog will choose edit on addition. Reset manually for test.
	d.focusIdx = -1
	edit.SetFocus(false)

	// 1. Press Alt+N (label hotkey)
	d.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'n',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if d.focusIdx != 1 || !edit.IsFocused() {
		t.Errorf("Focus should move to edit via label hotkey. focusIdx=%d", d.focusIdx)
	}
}
func TestDialog_HotkeyCaseInsensitivity(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Case Test")
	btn := NewButton(1, 1, "&File")
	d.AddItem(btn)

	// Press capital 'F', although it can be any in string.
	// Parser stores 'f', so comparison should succeed.
	handled := d.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'F',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !handled {
		t.Error("Hotkey should be case-insensitive")
	}
}

func TestDialog_DraggingLogic(t *testing.T) {
	// Create 10x10 dialog at (0,0)
	d := NewDialog(0, 0, 9, 9, "Move Me")
	btn := NewButton(2, 2, "OK") // Button inside
	d.AddItem(btn)

	// 1. Press left mouse button on border (0,0)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 0, MouseY: 0,
	})

	if !d.isDragging {
		t.Fatal("Dialog should start dragging after clicking on border")
	}

	// 2. Move mouse to (5,5)
	// Emulate move event (in vtinput usually KeyDown: false with button pressed)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: false,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 5, MouseY: 5,
	})

	// Dialog should shift by +5, +5
	if d.X1 != 5 || d.Y1 != 5 {
		t.Errorf("Dialog did not move correctly. Got (%d,%d), want (5,5)", d.X1, d.Y1)
	}

	// Check that internal button also shifted!
	bx1, by1, _, _ := btn.GetPosition()
	if bx1 != 7 || by1 != 7 { // 2 (orig) + 5 (offset) = 7
		t.Errorf("Child element (button) did not move with dialog. Got (%d,%d), want (7,7)", bx1, by1)
	}

	// 3. Release mouse
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: false,
		ButtonState: 0,
		MouseX: 5, MouseY: 5,
	})

	if d.isDragging {
		t.Error("Dialog should stop dragging after mouse button release")
	}
}

func TestDialog_NoDragWhenClickingElement(t *testing.T) {
	d := NewDialog(0, 0, 10, 10, "No Drag")
	clicked := false
	btn := NewButton(1, 1, "Btn")
	btn.OnClick = func() { clicked = true }
	d.AddItem(btn)

	// Click button at (1, 1)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 1, MouseY: 1,
	})

	if d.isDragging {
		t.Error("Dialog should NOT start dragging when clicking on an interactive element")
	}
	if !clicked {
		t.Error("Button should have handled the click")
	}
}

func TestDialog_DragRelativeConsistency(t *testing.T) {
	// Test for "error accumulation" during multiple moves
	d := NewDialog(0, 0, 10, 10, "Consistency")

	// Start capture at point (0,0)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 0, MouseY: 0,
	})

	// Series of small moves
	for i := 1; i <= 10; i++ {
		d.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: false,
			ButtonState: vtinput.FromLeft1stButtonPressed,
			MouseX: uint16(i), MouseY: uint16(i),
		})
	}

	if d.X1 != 10 || d.Y1 != 10 {
		t.Errorf("Incremental move failed. Expected (10,10), got (%d,%d)", d.X1, d.Y1)
	}
}

func TestDialog_DraggingOffscreen(t *testing.T) {
	d := NewDialog(10, 10, 20, 20, "Offscreen")

	// Start capture
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 10, MouseY: 10,
	})

	// Drag mouse into "negative"
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 0, MouseY: 0,
	})

	// Dialog should allow moving to negative coordinates (like in Far)
	if d.X1 != 0 || d.Y1 != 0 {
		t.Errorf("Expected dialog at (0,0), got (%d,%d)", d.X1, d.Y1)
	}
}

func TestDialog_ResizingLogic(t *testing.T) {
	d := NewDialog(0, 0, 9, 9, "Resize Me")
	d.MinW = 5
	d.MinH = 5

	// 1. Click bottom-right corner (9,9)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 9, MouseY: 9,
	})

	if !d.isResizing {
		t.Fatal("Dialog should start resizing when clicking bottom-right corner")
	}

	// 2. Drag to (14, 14) -> size should become 15x15
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 14, MouseY: 14,
	})

	if d.X2 != 14 || d.Y2 != 14 {
		t.Errorf("Dialog did not resize correctly. Got X2=%d, Y2=%d", d.X2, d.Y2)
	}

	// 3. Drag to (2, 2) -> size should hit minimum 5x5 (so X2=4, Y2=4)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 2, MouseY: 2,
	})

	if d.X2 != 4 || d.Y2 != 4 {
		t.Errorf("Dialog minimum size failed. Got X2=%d, Y2=%d", d.X2, d.Y2)
	}

	// 4. Release mouse
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: 0,
		MouseX: 4, MouseY: 4,
	})

	if d.isResizing {
		t.Error("Dialog should stop resizing on mouse release")
	}
}
func TestDialog_ResizingConstraints(t *testing.T) {
	// Verify that MinW and MinH are respected even with large mouse deltas
	d := NewDialog(10, 10, 20, 20, "Constrain")
	d.MinW = 15
	d.MinH = 15

	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 20, MouseY: 20,
	})

	// Drag mouse far to the top-left (e.g., coordinate 0,0)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 0, MouseY: 0,
	})

	width := d.X2 - d.X1 + 1
	height := d.Y2 - d.Y1 + 1

	if width < d.MinW || height < d.MinH {
		t.Errorf("Resizing broke constraints: got %dx%d, want min %dx%d", width, height, d.MinW, d.MinH)
	}
}
func TestDialog_ResizeGrowMode(t *testing.T) {
	// 10x10 Dialog
	d := NewDialog(0, 0, 9, 9, "Grow Test")

	// Button anchored to the bottom-right (GrowLoX | GrowHiX | GrowLoY | GrowHiY moves it)
	btn := NewButton(5, 5, "Fixed")
	btn.SetGrowMode(GrowLoX | GrowHiX | GrowLoY | GrowHiY)
	d.AddItem(btn)

	// Initiate resize via mouse at bottom-right corner
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 9, MouseY: 9,
	})

	// Drag mouse to (19, 19) -> Dialog size becomes 20x20 (Delta +10, +10)
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: false,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 19, MouseY: 19,
	})

	bx1, by1, _, _ := btn.GetPosition()
	// Original (5,5) + Delta (10,10) = (15,15)
	if bx1 != 15 || by1 != 15 {
		t.Errorf("Child GrowMode failed during resize. Expected (15,15), got (%d,%d)", bx1, by1)
	}
}
func TestDialog_RightClickNoDrag(t *testing.T) {
	d := NewDialog(0, 0, 10, 10, "Left Button Only")

	// Attempting capture with RIGHT button
	d.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.RightmostButtonPressed,
		MouseX: 0, MouseY: 0,
	})

	if d.isDragging {
		t.Error("Dialog should NOT start dragging with Right Mouse Button")
	}
}

func TestDialog_FocusLinkChain(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Chain Test")
	edit := NewEdit(1, 3, 10, "")
	label2 := NewLabel(1, 2, "Sub-Label:", edit)
	label1 := NewLabel(1, 1, "&Grand-Label:", label2) // References another label

	d.AddItem(label1)
	d.AddItem(label2)
	d.AddItem(edit)

	// Press Alt+G (first label hotkey)
	d.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'g',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if d.focusIdx != 2 || !edit.IsFocused() {
		t.Errorf("Focus chain failed. Expected focus on edit (index 2), got index %d", d.focusIdx)
	}
}
