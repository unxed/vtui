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
