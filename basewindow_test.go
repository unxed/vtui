package vtui

import (
	"testing"
)

func TestBaseWindow_ShadowFlag(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Title")
	if !bw.HasShadow() {
		t.Error("BaseWindow (Dialogs/Windows) should have shadows enabled by default")
	}
}

func TestBaseWindow_HandleCommand(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Command Test")
	// Test built-in Window command (CmClose)
	if bw.IsDone() {
		t.Fatal("Window should not be done initially")
	}
	bw.HandleCommand(CmClose, nil)
	if !bw.IsDone() {
		t.Error("CmClose command should close the BaseWindow")
	}
}

func TestBaseWindow_AddItem(t *testing.T) {
	// Создаем окно 10x5. С учетом рамок, контент занимает 8x3.
	bw := NewBaseWindow(0, 0, 10, 5, "Test MinSize")
	
	// Начальный MinW должен быть равен переданному размеру (11 символов: 0..10)
	if bw.MinW != 11 {
		t.Errorf("Initial MinW is wrong, got %d, want 11", bw.MinW)
	}

	// Добавляем кнопку, которая выходит за границы окна вправо.
	// Кнопка на x=15, её ширина ~10. Конец будет на x=25.
	// Окно должно увеличить MinW, чтобы вместить элемент + рамку.
	btn := NewButton(15, 3, "Wide")
	bw.AddItem(btn)

	// x2 кнопки (15 + len("[ Wide ]") - 1) = 22.
	// MinW окна = x2 кнопки - bw.X1 + 2 (рамка) = 24.
	if bw.MinW < 24 {
		t.Errorf("MinW did not update correctly after adding wide item. Expected >= 24, got %d", bw.MinW)
	}
}

func TestBaseWindow_DataMapping(t *testing.T) {
	type TestData struct {
		Name  string `vtui:"user_name"`
		Admin bool   `vtui:"is_admin"`
	}

	bw := NewBaseWindow(0, 0, 40, 20, "Data Test")

	edit := NewEdit(1, 1, 20, "")
	edit.SetId("user_name")
	bw.AddItem(edit)

	chk := NewCheckbox(1, 2, "Admin", false)
	chk.SetId("is_admin")
	bw.AddItem(chk)

	// 1. Test SetData (делегирование в rootGroup)
	input := TestData{
		Name:  "Explorer",
		Admin: true,
	}
	bw.SetData(input)

	if edit.GetText() != "Explorer" {
		t.Errorf("SetData failed to update Edit: %s", edit.GetText())
	}
	if chk.State != 1 {
		t.Error("SetData failed to update Checkbox")
	}

	// 2. Test GetData
	edit.SetText("NewName")
	var output TestData
	bw.GetData(&output)

	if output.Name != "NewName" {
		t.Errorf("GetData failed to retrieve string: %s", output.Name)
	}
}

func TestBaseWindow_DataMapping_EdgeCases(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Edge Case Test")
	edit := NewEdit(0, 0, 5, "")
	edit.SetId("field1")
	bw.AddItem(edit)

	// 1. SetData with incorrect types (should not panic)
	bw.SetData(nil)
	bw.SetData("not a struct")
	bw.SetData(42)

	// 2. GetData into non-pointer or non-struct
	var target string
	bw.GetData(target)  // Not a pointer
	bw.GetData(&target) // Pointer to non-struct

	// 3. Type mismatch
	type WrongTypeStruct struct {
		Field1 int `vtui:"field1"` // UI is Edit (string), here it's int
	}
	wrong := WrongTypeStruct{Field1: 123}
	bw.SetData(wrong) // Should be ignored as int is not a string
	if edit.GetText() != "" {
		t.Error("SetData should ignore value when types are incompatible")
	}

	edit.SetText("NotANumber")
	var result WrongTypeStruct
	bw.GetData(&result)
	if result.Field1 != 0 {
		t.Error("GetData should not set field when types are incompatible")
	}
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
		t.Error("Broadcast was not propagated to all items through rootGroup")
	}
}

func TestBaseWindow_Validation_CmDefault(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()

	dlg := NewDialog(0, 0, 20, 5, "Enter Test")
	edit := NewEdit(1, 1, 10, "wrong")
	edit.Validator = &RegexValidator{Pattern: "^correct$"}
	dlg.AddItem(edit)
	fm.Push(dlg)

	// CmDefault обычно вызывается при нажатии Enter
	handled := dlg.HandleCommand(CmDefault, nil)

	if !handled {
		t.Error("Command should be consumed")
	}
	if dlg.IsDone() {
		t.Error("CmDefault should be blocked by validation")
	}
}


func TestBaseWindow_NoDownwardCommandRouting(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Recursion Test")

	itemHandledCommand := false
	item := &cmdMockFrame{}
	item.onCmd = func(cmd int, args any) bool {
		itemHandledCommand = true
		return true
	}
	bw.AddItem(item)
	bw.rootGroup.focusIdx = 0

	// Вызываем команду на окне.
	// Оно не должно пытаться передать её вниз сфокусированному элементу (это делает FrameManager),
	// чтобы избежать бесконечной рекурсии, если элемент сам вызывает метод окна.
	bw.HandleCommand(CmOK, nil)

	if itemHandledCommand {
		t.Error("Window passed command down to focused item, risking infinite recursion")
	}
}
