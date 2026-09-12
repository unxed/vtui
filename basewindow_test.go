package vtui

import (
	"github.com/unxed/vtinput"
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
func TestBaseWindow_PgUpFocus(t *testing.T) {
	bw := NewBaseWindow(0, 0, 40, 10, "PgUp Test")
	edit1 := NewEdit(1, 1, 20, "one")
	edit2 := NewEdit(1, 2, 20, "two")

	bw.AddItem(edit1)
	bw.AddItem(edit2)

	// Set initial focus to edit2 (index 1) manually before opening
	bw.rootGroup.setFocus(1)

	// Simulate window getting focus (opening)
	bw.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.FocusEventType, SetFocus: true,
	})

	if bw.initialFocusItem != edit2 {
		t.Fatalf("Expected initialFocusItem to be edit2, got %v", bw.initialFocusItem)
	}

	// Change focus to edit1
	bw.rootGroup.setFocus(0)
	if !edit1.IsFocused() {
		t.Fatal("Setup failed: edit1 not focused")
	}

	// Press PgUp (VK_PRIOR)
	bw.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR,
	})

	if !edit2.IsFocused() {
		t.Error("PgUp failed to restore focus to the initial focus item (edit2)")
	}
	if edit1.IsFocused() {
		t.Error("Focus remained on edit1 after PgUp")
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

func TestBaseWindow_GetTitle(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Test Title")
	if bw.GetTitle() != "Test Title" {
		t.Errorf("Expected 'Test Title', got %q", bw.GetTitle())
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
func TestBaseWindow_PgDnFocus(t *testing.T) {
	bw := NewBaseWindow(0, 0, 40, 10, "PgDn Test")
	edit := NewEdit(1, 1, 20, "")
	btnOk := NewButton(1, 5, "&Ok")
	btnOk.IsDefault = true

	bw.AddItem(edit)
	bw.AddItem(btnOk)

	// Ensure initial focus is on edit
	bw.rootGroup.setFocus(0)
	if !edit.IsFocused() {
		t.Fatal("Setup failed: edit not focused")
	}

	// Press PgDn
	bw.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT,
	})

	if !btnOk.IsFocused() {
		t.Error("PgDn failed to move focus to the default button")
	}
	if edit.IsFocused() {
		t.Error("Focus remained on the edit field after PgDn")
	}
}
func TestBaseWindow_GetPaletteIndex_WarningMapping(t *testing.T) {
	bw := NewBaseWindow(0, 0, 10, 10, "Warn")
	bw.IsWarning = true

	cases := []struct {
		base int
		want int
	}{
		{ColDialogText, ColWarnText},
		{ColDialogBox, ColWarnBox},
		{ColDialogButton, ColWarnButton},
		{ColDialogSelectedButton, ColWarnSelectedButton},
		{ColDialogHighlightButton, ColWarnHighlightButton},
		{ColDialogHighlightSelectedButton, ColWarnHighlightSelectedButton},
		{ColDialogEdit, ColWarnEdit},
		{ColDialogComboText, ColWarnEdit},
		{ColDialogComboSelectedText, ColWarnEdit},
		{ColDialogComboHighlight, ColWarnHighlightText},
		{ColDialogComboBox, ColWarnBox},
	}
	for _, tc := range cases {
		if got := bw.GetPaletteIndex(tc.base); got != tc.want {
			t.Errorf("GetPaletteIndex(%d) = %d, want %d", tc.base, got, tc.want)
		}
	}
}

// TestBaseWindow_ToggleZoomKeepsWorkspaceTabRow covers f4 issue #1144. A zoomed
// window used to start at row 0, which drawWorkspaceTabs fills unconditionally
// at the end of every Redraw: the window's top border and title vanished
// whenever the workspace tab strip was visible.
func TestBaseWindow_ToggleZoomKeepsWorkspaceTabRow(t *testing.T) {
	oldScreens := FrameManager.Screens
	oldMode := FrameManager.WorkspaceTabMode
	t.Cleanup(func() {
		FrameManager.Screens = oldScreens
		FrameManager.WorkspaceTabMode = oldMode
	})

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)
	FrameManager.WorkspaceTabMode = WorkspaceTabsAlways
	if got := FrameManager.WorkspaceTopInset(); got != 1 {
		t.Fatalf("workspace top inset = %d, want 1", got)
	}

	w := NewWindow(10, 5, 49, 18, " Zoom ")
	w.ToggleZoom()
	if x1, y1, x2, y2 := w.GetPosition(); x1 != 0 || y1 != 1 || x2 != 79 || y2 != 23 {
		t.Fatalf("zoomed bounds = (%d,%d)-(%d,%d), want (0,1)-(79,23)", x1, y1, x2, y2)
	}

	w.ToggleZoom()
	if x1, y1, x2, y2 := w.GetPosition(); x1 != 10 || y1 != 5 || x2 != 49 || y2 != 18 {
		t.Fatalf("restored bounds = (%d,%d)-(%d,%d), want (10,5)-(49,18)", x1, y1, x2, y2)
	}

	FrameManager.WorkspaceTabMode = WorkspaceTabsNever
	if got := FrameManager.WorkspaceTopInset(); got != 0 {
		t.Fatalf("inset without a tab strip = %d, want 0", got)
	}
	w.ToggleZoom()
	if x1, y1, x2, y2 := w.GetPosition(); x1 != 0 || y1 != 0 || x2 != 79 || y2 != 23 {
		t.Fatalf("zoomed bounds without a tab strip = (%d,%d)-(%d,%d), want (0,0)-(79,23)", x1, y1, x2, y2)
	}
}
