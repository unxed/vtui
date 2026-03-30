package vtui

import (
	"testing"
	"time"
	"github.com/unxed/vtinput"
)

func TestBaseWindow_ShadowFlag(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Title")
	if !bw.HasShadow() {
		t.Error("BaseWindow (Dialogs/Windows) should have shadows enabled by default")
	}
}

func TestBaseWindow_HandleCommand(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Command Test")

	// Add an element to test bubbling down
	btn := NewButton(1, 1, "Btn")
	//clicked := false
	//btn.OnClick = func() { clicked = true }
	bw.AddItem(btn)
	bw.focusIdx = 0

	// 1. Test custom command (should bubble to UI Element, but button ignores raw commands by default)
	handled := bw.HandleCommand(999, nil)
	if handled {
		t.Error("Unrecognized command should not be handled")
	}

	// 2. Test built-in Window command (CmClose)
	if bw.IsDone() {
		t.Fatal("Window should not be done initially")
	}

	bw.HandleCommand(CmClose, nil)

	if !bw.IsDone() {
		t.Error("CmClose command should close the BaseWindow")
	}
}
func TestBaseWindow_ChangeFocus_NoFocusableItems(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "No Focus Test")
	// Add only non-focusable item
	bw.AddItem(NewText(1, 1, "Static Text", 0))

	// Before the fix, this call would cause an infinite loop (deadlock)
	done := make(chan bool, 1)
	go func() {
		bw.changeFocus(1)
		done <- true
	}()

	select {
	case <-done:
		// Success: function returned
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Deadlock detected in changeFocus when no focusable items exist")
	}

	if bw.focusIdx != -1 {
		t.Errorf("Expected focusIdx to be -1, got %d", bw.focusIdx)
	}
}

func TestBaseWindow_ChangeFocus_SingleFocusableItem(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Single Focus Test")
	btn := NewButton(1, 1, "OK")
	bw.AddItem(btn)
	bw.AddItem(NewText(1, 2, "Static", 0))

	// Initial focus should be on the button (idx 0)
	if bw.focusIdx != 0 {
		t.Fatalf("Initial focusIdx expected 0, got %d", bw.focusIdx)
	}

	// Tab should cycle back to the same button
	bw.changeFocus(1)
	if bw.focusIdx != 0 {
		t.Errorf("Focus should have stayed on the only focusable item, got %d", bw.focusIdx)
	}
	if !btn.IsFocused() {
		t.Error("Button should remain focused")
	}
}
func TestDialog_ArrowKeyNavigationFallback(t *testing.T) {
	d := NewDialog(0, 0, 20, 10, "Arrow Navigation")
	b1 := NewButton(1, 1, "Btn1")
	b2 := NewButton(1, 2, "Btn2")
	d.AddItem(b1)
	d.AddItem(b2)

	if d.focusIdx != 0 {
		t.Fatal("Initial focus should be 0")
	}

	// 1. VK_DOWN on a Button. Button.ProcessKey returns false for VK_DOWN.
	// The Dialog should intercept and move focus.
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	if d.focusIdx != 1 {
		t.Errorf("VK_DOWN did not move focus to next element, got index %d", d.focusIdx)
	}

	// 2. VK_UP on Button. Should move focus backwards.
	d.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})

	if d.focusIdx != 0 {
		t.Errorf("VK_UP did not move focus to prev element, got index %d", d.focusIdx)
	}
}

func TestBaseWindow_DataMapping_EdgeCases(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Edge Case Test")
	edit := NewEdit(0, 0, 5, "")
	edit.SetId("field1")
	bw.AddItem(edit)

	// 1. SetData с некорректными типами (не должно паниковать)
	bw.SetData(nil)
	bw.SetData("not a struct")
	bw.SetData(42)

	// 2. GetData в не-указатель или не-структуру
	var target string
	bw.GetData(target) // Не указатель
	bw.GetData(&target) // Указатель не на структуру

	// 3. Несовпадение типов данных
	type WrongTypeStruct struct {
		Field1 int `vtui:"field1"` // В UI это Edit (string), а тут int
	}
	wrong := WrongTypeStruct{Field1: 123}
	bw.SetData(wrong) // Должно просто проигнорировать, так как int не string
	if edit.GetText() != "" {
		t.Error("SetData should ignore value when types are incompatible")
	}

	edit.SetText("NotANumber")
	var result WrongTypeStruct
	bw.GetData(&result) // Не должно упасть, просто не запишет значение
	if result.Field1 != 0 {
		t.Error("GetData should not set field when types are incompatible")
	}

	// 4. Отсутствие ID
	type MissingIdStruct struct {
		UnknownField string `vtui:"no_such_id"`
	}
	missing := MissingIdStruct{UnknownField: "val"}
	bw.SetData(missing) // Ничего не должно произойти
}

type broadcastMockElement struct {
	ScreenObject
	handled bool
}

func (m *broadcastMockElement) HandleBroadcast(cmd int, args any) bool {
	if cmd == 42 {
		m.handled = true
		return true
	}
	return false
}

// Implement remaining UIElement methods with stubs
func (m *broadcastMockElement) GetPosition() (int, int, int, int) { return 0,0,0,0 }
func (m *broadcastMockElement) SetPosition(x1, y1, x2, y2 int) {}
func (m *broadcastMockElement) GetGrowMode() GrowMode { return 0 }
func (m *broadcastMockElement) Show(scr *ScreenBuf) {}
func (m *broadcastMockElement) Hide(scr *ScreenBuf) {}
func (m *broadcastMockElement) GetHotkey() rune { return 0 }
func (m *broadcastMockElement) GetHelp() string { return "" }
func (m *broadcastMockElement) ProcessKey(e *vtinput.InputEvent) bool { return false }
func (m *broadcastMockElement) ProcessMouse(e *vtinput.InputEvent) bool { return false }
func (m *broadcastMockElement) HandleCommand(cmd int, args any) bool { return false }

func TestBaseWindow_HandleBroadcast_Propagation(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Test")
	el1 := &broadcastMockElement{}
	el2 := &broadcastMockElement{}
	bw.AddItem(el1)
	bw.AddItem(el2)

	res := bw.HandleBroadcast(42, nil)

	if !res {
		t.Error("BaseWindow should return true if items handled the broadcast")
	}
	if !el1.handled || !el2.handled {
		t.Error("Broadcast was not propagated to all items")
	}
}
func TestBaseWindow_Validation(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewScreenBuf())
	defer fm.Shutdown()

	// Use NewDialog because BaseWindow doesn't implement GetType()
	bw := NewDialog(0, 0, 40, 10, "Validation Test")
	edit := NewEdit(1, 1, 10, "50")
	// Only allow 1-10
	edit.Validator = &IntRangeValidator{Min: 1, Max: 10}
	bw.AddItem(edit)
	fm.Push(bw)

	// 1. Test failure
	// Try to "click OK" (Command CmOK)
	handled := bw.HandleCommand(CmOK, nil)

	if !handled { t.Error("Command should be consumed") }
	if bw.IsDone() { t.Error("Window should not close with invalid input") }

	// 2. Test success
	edit.SetText("5")
	handled = bw.HandleCommand(CmOK, nil)

	if !handled { t.Error("Command should be handled") }
	if !bw.IsDone() { t.Error("Window should close with valid input") }
	if bw.ExitCode != CmOK { t.Errorf("Wrong exit code: %d", bw.ExitCode) }
}

func TestRegexValidator(t *testing.T) {
	v := &RegexValidator{Pattern: "^[a-z]+$"}

	if !v.Validate("abc") { t.Error("Regex should match simple lowercase") }
	if v.Validate("123") { t.Error("Regex should not match numbers") }
	if v.Validate("ABC") { t.Error("Regex should be case sensitive") }
}
func TestIntRangeValidator_EdgeCases(t *testing.T) {
	v := &IntRangeValidator{Min: 0, Max: 100}

	// 1. Non-numeric input
	if v.Validate("abc") { t.Error("IntRange should not validate non-numeric strings") }
	if v.Validate("") { t.Error("IntRange should not validate empty strings") }

	// 2. Out of bounds
	if v.Validate("-1") { t.Error("Below min") }
	if v.Validate("101") { t.Error("Above max") }
}

func TestBaseWindow_Validation_Recursive(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewScreenBuf())
	defer fm.Shutdown()

	dlg := NewDialog(0, 0, 40, 10, "Multi Validation")
	e1 := NewEdit(1, 1, 10, "valid")
	e1.Validator = &RegexValidator{Pattern: "^valid$"}
	e2 := NewEdit(1, 2, 10, "invalid")
	e2.Validator = &RegexValidator{Pattern: "^valid$"}

	dlg.AddItem(e1)
	dlg.AddItem(e2)
	fm.Push(dlg)

	// Should be invalid because e2 is invalid
	if dlg.Valid(CmOK) {
		t.Error("Dialog should be invalid if ANY item is invalid")
	}
}
func TestBaseWindow_DisabledFocus(t *testing.T) {
	bw := NewBaseWindow(0, 0, 20, 10, "Test")
	btn1 := NewButton(1, 1, "Btn1")
	btn2 := NewButton(1, 2, "Btn2")
	btn3 := NewButton(1, 3, "Btn3")

	btn2.SetDisabled(true)

	bw.AddItem(btn1)
	bw.AddItem(btn2)
	bw.AddItem(btn3)

	// Initial focus should be btn1
	if bw.focusIdx != 0 {
		t.Fatalf("Expected focus on 0, got %d", bw.focusIdx)
	}

	// Tab should skip btn2 and go to btn3
	bw.changeFocus(1)
	if bw.focusIdx != 2 {
		t.Errorf("Expected focus to skip disabled btn2 and land on 2, got %d", bw.focusIdx)
	}

	// Shift+Tab should skip btn2 backwards and go to btn1
	bw.changeFocus(-1)
	if bw.focusIdx != 0 {
		t.Errorf("Expected focus to skip disabled btn2 and land on 0, got %d", bw.focusIdx)
	}
}

func TestWidget_DisabledInput(t *testing.T) {
	btn := NewButton(0, 0, "OK")
	clicked := false
	btn.OnClick = func() { clicked = true }
	btn.SetDisabled(true)

	handled := btn.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	if handled || clicked {
		t.Error("Disabled button should not process keys")
	}

	handled = btn.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed})
	if handled || clicked {
		t.Error("Disabled button should not process mouse")
	}
}
func TestAllWidgets_DisabledState(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(80, 25)

	widgets := []UIElement{
		NewButton(0, 0, "Btn"),
		NewCheckbox(0, 0, "Chk", false),
		NewCheckGroup(0, 0, 1, []string{"Chk1"}),
		NewComboBox(0, 0, 10, []string{"A"}),
		NewEdit(0, 0, 10, "Edit"),
		NewListBox(0, 0, 10, 5, []string{"A"}),
		NewRadioButton(0, 0, "Rad"),
		NewRadioGroup(0, 0, 1, []string{"Rad1"}),
		NewText(0, 0, "Txt", Palette[ColDialogText]),
		NewVMenu("Menu"),
		NewTreeView(0, 0, 10, 10, &TreeNode{Text: "Node"}),
		NewMenuBar([]string{"Menu"}),
	}

	keyEv := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}
	mouseEv := &vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: 0, MouseY: 0}

	for _, w := range widgets {
		// Set focus first to ensure SetDisabled(true) removes it
		w.SetFocus(true)
		if !w.IsFocused() {
			t.Errorf("%T failed to set focus initially", w)
		}

		w.SetDisabled(true)

		if w.IsFocused() {
			t.Errorf("%T should not be focused after SetDisabled(true)", w)
		}
		if !w.IsDisabled() {
			t.Errorf("%T IsDisabled() returned false", w)
		}

		if w.ProcessKey(keyEv) {
			t.Errorf("%T processed key while disabled", w)
		}

		if w.ProcessMouse(mouseEv) {
			t.Errorf("%T processed mouse while disabled", w)
		}

		// Render the disabled widget. This exercises the `DimColor` paths in DisplayObject.
		// If anything panics here, the test will naturally fail.
		w.Show(scr)
	}
}

func TestBaseWindow_Validation_CmDefault(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewScreenBuf())
	defer fm.Shutdown()

	dlg := NewDialog(0, 0, 20, 5, "Enter Test")
	edit := NewEdit(1, 1, 10, "wrong")
	edit.Validator = &RegexValidator{Pattern: "^correct$"}
	dlg.AddItem(edit)
	fm.Push(dlg)

	// CmDefault is usually triggered by Enter
	handled := dlg.HandleCommand(CmDefault, nil)

	if !handled { t.Error("CmDefault should be consumed") }
	if dlg.IsDone() { t.Error("CmDefault should be blocked by validation") }
}

func TestValidators_ErrorUI(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewScreenBuf())
	defer fm.Shutdown()

	dlg := NewDialog(0, 0, 20, 5, "Target")
	fm.Push(dlg)

	initialFrames := len(fm.frames)

	// Trigger error on IntRange
	iv := &IntRangeValidator{Min: 1, Max: 10}
	iv.Error(dlg)

	if len(fm.frames) <= initialFrames {
		t.Error("IntRangeValidator.Error() did not push a message box")
	}

	// Trigger error on Regex
	rv := &RegexValidator{ErrorMessage: "Custom Error"}
	rv.Error(dlg)

	if len(fm.frames) <= initialFrames+1 {
		t.Error("RegexValidator.Error() did not push a message box")
	}
}

func TestBaseWindow_NoDownwardCommandRouting(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Recursion Test")
	
	itemHandledCommand := false
	//mock := &broadcastMockElement{}
	// Override HandleCommand for this instance
	// Since we can't easily override methods on instances in Go,
	// we use a specific mock that records calls.
	item := &cmdMockFrame{}
	item.onCmd = func(cmd int, args any) bool {
		itemHandledCommand = true
		return true
	}
	// Items are usually UIElement, cmdMockFrame satisfies this
	bw.AddItem(item)
	bw.focusIdx = 0

	// Trigger a command on the Window.
	// It should NOT try to pass it to the item anymore.
	bw.HandleCommand(CmOK, nil)

	if itemHandledCommand {
		t.Error("Window passed command down to focused item, risking infinite recursion")
	}
}

func TestBaseWindow_ChangeFocus_FromUnfocused(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Initial Focus")
	b1 := NewButton(1, 1, "B1")
	b2 := NewButton(1, 2, "B2")
	bw.AddItem(b1)
	bw.AddItem(b2)

	// Manually un-focus
	bw.focusIdx = -1
	b1.SetFocus(false)
	b2.SetFocus(false)

	// 1. Move forward from nothing -> should land on index 0
	bw.changeFocus(1)
	if bw.focusIdx != 0 {
		t.Errorf("Forward from unfocused failed: expected 0, got %d", bw.focusIdx)
	}

	// 2. Move backward from nothing -> should land on index 1
	bw.focusIdx = -1
	bw.changeFocus(-1)
	if bw.focusIdx != 1 {
		t.Errorf("Backward from unfocused failed: expected 1, got %d", bw.focusIdx)
	}
}

