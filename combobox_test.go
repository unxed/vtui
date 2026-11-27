package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestComboBox_Selection(t *testing.T) {
	items := []string{"One", "Two", "Three"}
	cb := NewComboBox(0, 0, 20, items)

	// Initially text is empty
	if cb.Edit.GetText() != "" {
		t.Errorf("Expected empty text, got %q", cb.Edit.GetText())
	}

	// Simulate selecting the second item ("Two") in menu
	if cb.Menu.OnAction != nil {
		cb.Menu.OnAction(1)
	}

	if cb.Edit.GetText() != "Two" {
		t.Errorf("Expected 'Two', got %q", cb.Edit.GetText())
	}
}

func TestComboBox_DropdownOnly(t *testing.T) {
	cb := NewComboBox(0, 0, 20, []string{"A", "B"})
	cb.DropdownOnly = true

	// Attempting to enter text 'X'
	cb.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    'X',
	})

	if cb.Edit.GetText() == "X" {
		t.Error("DropdownOnly ComboBox should not allow manual text entry")
	}
}
func TestComboBox_DropdownOnly_Enter(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())

	cb := NewComboBox(0, 0, 20, []string{"A", "B"})
	cb.DropdownOnly = true

	// Press Enter in DropdownOnly mode should open the menu
	cb.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	top := fm.GetTopFrame()
	if top == nil || top.GetType() != TypeMenu {
		t.Error("Enter should open dropdown menu when DropdownOnly is true")
	}
}

func TestComboBox_MouseClickAnywhereOpens(t *testing.T) {
	for _, dropdownOnly := range []bool{false, true} {
		SetDefaultPalette()
		FrameManager.Init(NewSilentScreenBuf())

		cb := NewComboBox(10, 3, 20, []string{"A", "B"})
		cb.DropdownOnly = dropdownOnly

		// The click is in the edit portion, not on the one-cell arrow.
		handled := cb.ProcessMouse(&vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			KeyDown:     true,
			ButtonState: vtinput.FromLeft1stButtonPressed,
			MouseX:      12,
			MouseY:      3,
		})
		if !handled {
			t.Fatalf("clicking the ComboBox edit portion should be handled (DropdownOnly=%v)", dropdownOnly)
		}
		cb.ProcessMouse(&vtinput.InputEvent{
			Type:        vtinput.MouseEventType,
			MouseX:      12,
			MouseY:      3,
			KeyDown:     false,
			ButtonState: 0,
		})
		if top := FrameManager.GetTopFrame(); top == nil || top.GetType() != TypeMenu {
			t.Fatalf("clicking the ComboBox edit portion should open its menu (DropdownOnly=%v)", dropdownOnly)
		}
	}
}

func TestComboBox_OpenFlip(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 10) // Small height
	FrameManager.Init(scr)

	cb := NewComboBox(0, 8, 20, []string{"Item 1", "Item 2"})

	// ComboBox is at Y=8. Default open is downwards (Y=9).
	// But screen height is 10, so Y=9 is the last line.
	// With 2 items + border, menu height is 4.
	// It MUST flip upwards to fit.
	cb.Open()

	top := FrameManager.GetTopFrame()
	if top == nil || top.GetType() != TypeMenu {
		t.Fatal("Menu not opened")
	}

	_, y1, _, _ := top.GetPosition()
	// ComboBox is at Y=8. Upward flip with height 4 should start at 8-4 = 4.
	if y1 >= 8 {
		t.Errorf("ComboBox menu did not flip upwards. Y1=%d, ComboBoxY=%d", y1, cb.Y1)
	}
}

func TestComboBox_DisabledState(t *testing.T) {
	FrameManager.Init(NewSilentScreenBuf())
	cb := NewComboBox(0, 0, 20, []string{"A", "B"})

	// 1. Initially enabled
	if cb.IsDisabled() || cb.Edit.IsDisabled() {
		t.Error("ComboBox and its Edit should be enabled by default")
	}

	// 2. Disable ComboBox
	cb.SetDisabled(true)

	if !cb.IsDisabled() {
		t.Error("ComboBox failed to set disabled flag")
	}
	if !cb.Edit.IsDisabled() {
		t.Error("SetDisabled failed to propagate to underlying Edit control")
	}

	// 3. Try to open menu while disabled
	cb.Open()
	if FrameManager.GetTopFrameType() == TypeMenu {
		t.Error("Disabled ComboBox should not allow opening its menu")
	}
}

func TestComboBox_WantsChars(t *testing.T) {
	cb := NewComboBox(0, 0, 20, []string{"A"})

	// Normal mode: should want chars (pass to edit)
	if !cb.WantsChars() {
		t.Error("Standard ComboBox should want chars for editing")
	}

	// DropdownOnly: should NOT want chars
	cb.DropdownOnly = true
	if cb.WantsChars() {
		t.Error("DropdownOnly ComboBox should not want chars")
	}
}

func TestComboBox_ArrowUsesEditBackground(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(30, 3)

	cb := NewComboBox(1, 1, 20, []string{"One", "Two"})
	cb.Show(scr)

	arrowAttr := scr.GetCell(cb.X2, cb.Y1).Attributes
	editAttr := scr.GetCell(cb.X2-1, cb.Y1).Attributes
	if !sameBackground(arrowAttr, editAttr) {
		t.Fatalf("arrow background %#x does not match edit background %#x", arrowAttr, editAttr)
	}
}

func TestComboBox_DropdownOnlyFocusedArrowUsesSelectedBackground(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(30, 3)

	cb := NewComboBox(1, 1, 20, []string{"One", "Two"})
	cb.DropdownOnly = true
	cb.Edit.SetText("One")
	cb.SetFocus(true)
	cb.Show(scr)

	arrowAttr := scr.GetCell(cb.X2, cb.Y1).Attributes
	selectedTextAttr := scr.GetCell(cb.X1, cb.Y1).Attributes
	if !sameBackground(arrowAttr, selectedTextAttr) {
		t.Fatalf("arrow background %#x does not match selected edit background %#x", arrowAttr, selectedTextAttr)
	}
}

func TestComboBox_RoundedHoverTintsUnfocusedEditAndArrow(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	SetDefaultPalette()
	cb := NewComboBox(1, 1, 20, []string{"One", "Two"})
	draw := func() *ScreenBuf {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(30, 3)
		cb.Show(scr)
		return scr
	}

	base := draw()
	cb.SetMouseHovered(true)
	if !cb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseEventFlags: vtinput.MouseMoved,
		MouseX: int16(cb.X2), MouseY: int16(cb.Y1), // #nosec G115 -- test fixture is a 20-column ComboBox inside the small screen buffer
	}) {
		t.Fatal("arrow hover change was not reported for redraw")
	}
	hovered := draw()
	for _, x := range []int{cb.X1, cb.X2} {
		before := base.GetCell(x, cb.Y1).Attributes
		after := hovered.GetCell(x, cb.Y1).Attributes
		if sameBackground(before, after) {
			t.Errorf("cell %d background did not change on hover", x)
		}
		if got := contrastRatioRGB(attrColorRGB(after, false), attrColorRGB(after, true)); got < roundedButtonMinContrast {
			t.Errorf("cell %d hover contrast = %.2f, want >= %.1f", x, got, roundedButtonMinContrast)
		}
	}
}

func TestComboBox_RoundedFocusedHoverOnlyTintsArrow(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleRounded)
	SetDefaultPalette()
	cb := NewComboBox(1, 1, 20, []string{"One", "Two"})
	cb.SetFocus(true)
	draw := func() *ScreenBuf {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(30, 3)
		cb.Show(scr)
		return scr
	}

	base := draw()
	cb.SetMouseHovered(true)
	cb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, MouseEventFlags: vtinput.MouseMoved,
		MouseX: int16(cb.X2), MouseY: int16(cb.Y1), // #nosec G115 -- test fixture is a 20-column ComboBox inside the small screen buffer
	})
	hovered := draw()
	if !sameBackground(base.GetCell(cb.X1, cb.Y1).Attributes, hovered.GetCell(cb.X1, cb.Y1).Attributes) {
		t.Fatal("focused edit portion changed background on hover")
	}
	if sameBackground(base.GetCell(cb.X2, cb.Y1).Attributes, hovered.GetCell(cb.X2, cb.Y1).Attributes) {
		t.Fatal("focused dropdown arrow did not receive its independent hover tint")
	}
}

func TestComboBox_EditableClickMovesCursor(t *testing.T) {
	cb := NewComboBox(5, 2, 20, []string{"One", "Two"})
	cb.Edit.SetText("abcdef")

	handled := cb.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      int16(cb.X1 + 2),
		MouseY:      int16(cb.Y1),
	})

	if !handled {
		t.Fatal("editable ComboBox did not handle text click")
	}
	if cb.Edit.curPos != 2 {
		t.Fatalf("cursor position = %d, want 2", cb.Edit.curPos)
	}
	if cb.Edit.selStart != -1 {
		t.Fatalf("click left selection active at %d", cb.Edit.selStart)
	}
}

func TestComboBox_EditableDragSelectsTextOutsideControl(t *testing.T) {
	cb := NewComboBox(5, 2, 10, []string{"One", "Two"})
	cb.Edit.SetText("abcdef")

	cb.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      int16(cb.X1 + 1), MouseY: int16(cb.Y1),
	})
	cb.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
		MouseX:          int16(cb.X2 + 5), MouseY: int16(cb.Y1 + 2),
	})

	if cb.Edit.curPos != len(cb.Edit.text) {
		t.Fatalf("cursor position = %d, want %d after dragging right", cb.Edit.curPos, len(cb.Edit.text))
	}
	if cb.Edit.selStart != 1 || cb.Edit.selEnd != len(cb.Edit.text) {
		t.Fatalf("selection = [%d,%d), want [1,%d)", cb.Edit.selStart, cb.Edit.selEnd, len(cb.Edit.text))
	}

	cb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, ButtonState: 0,
		MouseX: int16(cb.X2 + 5), MouseY: int16(cb.Y1 + 2),
	})
	if cb.Edit.mouseSelecting || cb.editMouseCaptured {
		t.Fatal("mouse selection capture was not released")
	}
}

func TestComboBox_EditableDoubleClickSelectsWord(t *testing.T) {
	cb := NewComboBox(2, 1, 20, nil)
	cb.Edit.SetText("one two three")

	cb.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.DoubleClick,
		MouseX:          int16(cb.X1 + 5), MouseY: int16(cb.Y1),
	})

	if cb.Edit.selStart != 4 || cb.Edit.selEnd != 7 {
		t.Fatalf("double-click selection = [%d,%d), want [4,7)", cb.Edit.selStart, cb.Edit.selEnd)
	}
}

func TestComboBox_EditableTripleClickSelectsAll(t *testing.T) {
	cb := NewComboBox(2, 1, 20, nil)
	cb.Edit.SetText("one two three")

	cb.ProcessMouse(&vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		KeyDown:         true,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: TripleClick,
		MouseX:          int16(cb.X1 + 5), MouseY: int16(cb.Y1),
	})

	if cb.Edit.selStart != 0 || cb.Edit.selEnd != len(cb.Edit.text) {
		t.Fatalf("triple-click selection = [%d,%d), want all text", cb.Edit.selStart, cb.Edit.selEnd)
	}
}

func sameBackground(a, b uint64) bool {
	if a&IsBgRGB != b&IsBgRGB {
		return false
	}
	if a&IsBgRGB != 0 {
		return GetRGBBack(a) == GetRGBBack(b) && a&BackgroundIntensity == b&BackgroundIntensity
	}
	return GetIndexBack(a) == GetIndexBack(b) && a&BackgroundIntensity == b&BackgroundIntensity
}

// Issue unxed/f4#320: the highlighted default button is what Enter presses,
// also while a DropdownOnly combo box has the focus (far2l behaviour).
func TestComboBox_DropdownOnlyEnterPressesDialogDefault(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	dlg := NewDialog(0, 0, 40, 10, "Enter")
	cb := NewComboBox(2, 2, 20, []string{"A", "B"})
	cb.DropdownOnly = true
	pressed := false
	ok := NewButton(2, 4, "OK")
	ok.IsDefault = true
	ok.OnClick = func() { pressed = true }
	dlg.AddItem(cb)
	dlg.AddItem(ok)
	dlg.SetFocusedItem(cb)

	dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if !pressed {
		t.Error("Enter on a DropdownOnly combo box did not press the dialog's default button")
	}
	if top := FrameManager.GetTopFrame(); top != nil && top.GetType() == TypeMenu {
		t.Error("Enter opened the dropdown although the dialog has a default button")
	}
}

func TestComboBox_DropdownOnlyEnterInNestedGroupPressesDialogDefault(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	dlg := NewDialog(0, 0, 40, 10, "Enter")
	page := NewGroup(1, 1, 30, 3)
	cb := NewComboBox(2, 2, 20, []string{"A", "B"})
	cb.DropdownOnly = true
	page.AddItem(cb)
	pressed := false
	ok := NewButton(2, 6, "OK")
	ok.IsDefault = true
	ok.OnClick = func() { pressed = true }
	dlg.AddItem(page)
	dlg.AddItem(ok)
	dlg.SetFocusedItem(page)
	page.SetFocusedItem(cb)

	dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if !pressed {
		t.Error("Enter on a DropdownOnly combo box in a nested group did not press the dialog's default button")
	}
}

// Without a default button, Enter must not fall through to the first
// actionable button (TriggerDefaultAction's fallback): it opens the list.
func TestComboBox_DropdownOnlyEnterWithoutDefaultOpensList(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())

	dlg := NewDialog(0, 0, 40, 10, "Enter")
	cb := NewComboBox(2, 2, 20, []string{"A", "B"})
	cb.DropdownOnly = true
	clicked := false
	other := NewButton(2, 4, "Clear")
	other.OnClick = func() { clicked = true }
	dlg.AddItem(cb)
	dlg.AddItem(other)
	dlg.SetFocusedItem(cb)

	dlg.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if clicked {
		t.Error("Enter on a DropdownOnly combo box pressed a non-default button")
	}
	if top := FrameManager.GetTopFrame(); top == nil || top.GetType() != TypeMenu {
		t.Error("Enter should open the dropdown when the dialog has no default button")
	}
}
