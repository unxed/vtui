package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestDialog_RadioButtonIntegration(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Radio Test")
	rb1 := NewRadioButton(1, 1, "R1")
	rb2 := NewRadioButton(1, 2, "R2")

	rb1.Selected = true
	d.AddItem(rb1)
	d.AddItem(rb2)

	if !rb1.Selected || rb2.Selected {
		t.Fatal("Initial selection state invalid")
	}

	// Move to rb2 (Tab)
	d.rootGroup.changeFocus(1)

	// Press Space. Dialog should intercept this and update the group.
	d.rootGroup.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_SPACE,
	})

	if rb1.Selected {
		t.Error("rb1 should be deselected via ProcessKey(Space)")
	}
	if !rb2.Selected {
		t.Error("rb2 should be selected")
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
	btn.SetOnClick(func() { clicked = true })
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

	if d.rootGroup.focusIdx != 2 || !edit.IsFocused() {
		t.Errorf("Focus chain failed. Expected focus on edit (index 2), got index %d", d.rootGroup.focusIdx)
	}
}
func TestDialog_Center(t *testing.T) {
	d := NewDialog(0, 0, 9, 9, "Test") // 10x10 dialog

	// Center it within a 100x100 screen area
	// x1 = (100 - 10) / 2 = 45
	d.Center(100, 100)

	if d.X1 != 45 || d.Y1 != 45 {
		t.Errorf("Dialog.Center failed. Expected (45, 45), got (%d, %d)", d.X1, d.Y1)
	}
	if d.X2 != 54 || d.Y2 != 54 {
		t.Errorf("Dialog.Center warped size. Expected X2, Y2 (54, 54), got (%d, %d)", d.X2, d.Y2)
	}
}

func TestDialog_CloseButton(t *testing.T) {
	// X1:10, Y1:10, X2:30, Y2:20
	d := NewDialog(10, 10, 30, 20, "Close Test")
	d.ShowClose = true

	// Simulate click on the [×] button.
	// X2 is 30. Button is at 30-4=26, 30-3=27, 30-2=28.
	// Y1 is 10.
	d.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		MouseX:      27,
		MouseY:      10,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})

	if !d.IsDone() || d.ExitCode != -1 {
		t.Errorf("Close button click failed. Done: %v, ExitCode: %d", d.IsDone(), d.ExitCode)
	}
}
func TestDialog_GrowModeManualResize(t *testing.T) {
	// 20x10 Dialog
	d := NewDialog(0, 0, 19, 9, "Grow Test")

	// Edit that should stretch with the window (GrowHiX)
	edit := NewEdit(2, 2, 10, "")
	edit.SetGrowMode(GrowHiX)
	d.AddItem(edit)

	// Button that should move to keep its relative position from bottom-right (GrowLoX | GrowHiX | GrowLoY | GrowHiY)
	btn := NewButton(12, 7, "Ok")
	btn.SetGrowMode(GrowLoX | GrowHiX | GrowLoY | GrowHiY)
	d.AddItem(btn)

	// Simulate manual resize to 40x20 (+20 width, +10 height)
	d.ChangeSize(40, 20)

	// Check Edit: x1 remains 2.
	// Original x2 was 2 + 10 - 1 = 11.
	// After width delta +20, x2 should be 11 + 20 = 31.
	ex1, _, ex2, _ := edit.GetPosition()
	if ex1 != 2 || ex2 != 31 {
		t.Errorf("Edit GrowHiX failed: x1=%d, x2=%d", ex1, ex2)
	}

	// Check Button: x1 should be 12 + 20 = 32, y1 should be 7 + 10 = 17
	bx1, by1, _, _ := btn.GetPosition()
	if bx1 != 32 || by1 != 17 {
		t.Errorf("Button anchored resize failed: x1=%d, y1=%d", bx1, by1)
	}
}
func TestDialog_MinBoundsEnforcement(t *testing.T) {
	// Create small dialog 10x5
	d := NewDialog(0, 0, 9, 4, "Small")
	if d.MinW != 10 || d.MinH != 5 {
		t.Errorf("Initial MinBounds error: %dx%d", d.MinW, d.MinH)
	}

	// Add button that extends to X=15, Y=7
	btn := NewButton(10, 7, "Extender")
	d.AddItem(btn)

	// MinW should have increased (btn ends at 10 + len("[ Extender ]") - 1)
	// "[ Extender ]" is 12 chars. 10 + 12 - 1 = 21. ReqW = 21 - 0 + 1 = 22.
	if d.MinW < 22 {
		t.Errorf("MinW should have expanded, got %d", d.MinW)
	}
	if d.MinH < 8 {
		t.Errorf("MinH should have expanded, got %d", d.MinH)
	}

	// Try to force-shrink dialog below MinW via ChangeSize
	d.ChangeSize(5, 5)

	curW := d.X2 - d.X1 + 1
	if curW != d.MinW {
		t.Errorf("Dialog allowed shrinking below MinW! curW=%d, minW=%d", curW, d.MinW)
	}
}

func TestDialog_GetFocusedItem(t *testing.T) {
	d := NewDialog(0, 0, 20, 10, "Focus Test")
	b1 := NewButton(1, 1, "Btn1")
	b2 := NewButton(1, 2, "Btn2")
	d.AddItem(b1)
	d.AddItem(b2)

	// By default, the first focusable item should be focused
	focused := d.GetFocusedItem()
	if focused != b1 {
		t.Errorf("GetFocusedItem() expected b1, got %v", focused)
	}

	// Change focus
	d.rootGroup.changeFocus(1)
	focused = d.GetFocusedItem()
	if focused != b2 {
		t.Errorf("GetFocusedItem() expected b2 after focus change, got %v", focused)
	}
}

func TestElements_HelpTopic(t *testing.T) {
	btn := NewButton(0, 0, "HelpMe")
	btn.SetHelp("ButtonHelp")
	if btn.GetHelp() != "ButtonHelp" {
		t.Errorf("Button HelpTopic failed: %s", btn.GetHelp())
	}

	cb := NewCheckbox(0, 0, "Check", false)
	cb.SetHelp("CheckHelp")
	if cb.GetHelp() != "CheckHelp" {
		t.Errorf("Checkbox HelpTopic failed: %s", cb.GetHelp())
	}
}

func TestDialog_ShadowAndFocusColors(t *testing.T) {
	fm := &frameManager{}
	scr := NewScreenBuf()
	scr.AllocBuf(20, 20)
	fm.Init(scr)
	SetDefaultPalette()

	desktop := NewDesktop()
	fm.Push(desktop)

	// Inactive dialog
	d1 := NewDialog(2, 2, 7, 7, "D1")
	// Active dialog
	d2 := NewDialog(10, 10, 15, 15, "D2")

	fm.Push(d1)
	fm.Push(d2) // d2 is now on top

	// Simulate focus states
	d1.SetFocus(false)
	d2.SetFocus(true)

	// Simulate FrameManager render loop
	for i, frame := range fm.frames {
		// Draw shadow BEFORE showing the frame itself
		if i > 0 { // Skip desktop for shadow
			x1, y1, x2, y2 := frame.GetPosition()
			if x1 > 0 || y1 > 0 || x2 < fm.scr.width-1 || y2 < fm.scr.height-1 {
				fm.scr.ApplyShadow(x1+2, y2+1, x2+2, y2+1) // Bottom
				fm.scr.ApplyShadow(x2+1, y1+1, x2+2, y2)   // Right
			}
		}
		frame.Show(fm.scr)
	}

	// 1. Test inactive dialog color (d1)
	checkCell(t, scr, 4, 2, 'D', Palette[ColDialogBoxTitle])

	// 2. Test active dialog color (d2)
	checkCell(t, scr, 12, 10, 'D', Palette[ColDialogHighlightBoxTitle])

	// 3. Test shadow rendering of d2
	baseAttr := Palette[ColDesktopBackground]
	shadowedFg := GetRGBFore(baseAttr) / 2
	shadowedBg := GetRGBBack(baseAttr) / 2
	expectedShadowAttr := SetRGBBoth(baseAttr, shadowedFg, shadowedBg)

	// Check a cell inside the shadow of d2. It should be over the desktop background.
	// d2 is (10,10)-(15,15). Shadow bottom is at Y=16, X=12..17.
	checkCell(t, scr, 13, 16, ' ', expectedShadowAttr)

	// Make sure the area far from the shadow is clean
	checkCell(t, scr, 19, 16, ' ', baseAttr)

	// Check a cell of d1's shadow that is *not* covered by d2 or its shadow.
	checkCell(t, scr, 8, 8, ' ', expectedShadowAttr)
}

func TestBaseWindow_FocusVisualFeedback(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(40, 20)

	// Окно 5,5 -> 20,10. Ширина = 16.
	// Текст " Active " (8 симв). Отступ = (16-8)/2 = 4.
	// Старт текста: 5 + 4 = 9. Буква 'A' на позиции 10.
	win := NewWindow(5, 5, 20, 10, "Active")

	// 1. Тестируем активное состояние
	win.SetFocus(true)
	win.Show(scr)
	// Заголовок должен быть ярким (ColDialogHighlightBoxTitle)
	checkCell(t, scr, 10, 5, 'A', Palette[ColDialogHighlightBoxTitle])

	// 2. Тестируем потерю фокуса
	win.ProcessKey(&vtinput.InputEvent{Type: vtinput.FocusEventType, SetFocus: false})
	win.Show(scr)
	// Заголовок должен стать обычным (ColDialogBoxTitle)
	checkCell(t, scr, 10, 5, 'A', Palette[ColDialogBoxTitle])
}

func TestDialog_EnterTriggersFirstButton(t *testing.T) {
	d := NewDialog(0, 0, 40, 10, "Enter Test")
	
	okClicked := false
	cancelClicked := false
	
	// Add an edit field (to simulate focus being somewhere else)
	edit := NewEdit(1, 1, 20, "text")
	d.AddItem(edit)
	
	btnOk := NewButton(1, 3, "&Ok")
	btnOk.SetOnClick(func() { okClicked = true })
	d.AddItem(btnOk)

	btnCancel := NewButton(10, 3, "&Cancel")
	btnCancel.SetOnClick(func() { cancelClicked = true })
	d.AddItem(btnCancel)
	
	// Ensure focus is on the edit field
	d.rootGroup.focusIdx = 0
	edit.SetFocus(true)
	btnOk.SetFocus(false)

	// 1. Press Enter. Since Edit doesn't have OnAction, BaseWindow should find the button.
	d.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	if !okClicked {
		t.Error("Enter key should have triggered the OK button")
	}
	if cancelClicked {
		t.Error("Enter key triggered the wrong button (Cancel)")
	}
}
