package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestEdit_PasswordMode(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)

	e := NewEdit(0, 0, 10, "abc")
	e.PasswordMode = true
	e.Show(scr)

	// Check that buffer contains '*' instead of 'a'
	// Attributes must match ColDialogEdit
	checkCell(t, scr, 0, 0, '*', Palette[ColDialogEdit])
	checkCell(t, scr, 1, 0, '*', Palette[ColDialogEdit])
	checkCell(t, scr, 2, 0, '*', Palette[ColDialogEdit])
}
func TestNewPasswordEdit(t *testing.T) {
	e := NewPasswordEdit(0, 0, 10, "secret")
	if !e.PasswordMode {
		t.Error("NewPasswordEdit constructor failed to enable PasswordMode")
	}
	if e.GetText() != "secret" {
		t.Errorf("NewPasswordEdit failed to store initial text. Expected 'secret', got %q", e.GetText())
	}
}

func TestEdit_IgnoreLockKeys(t *testing.T) {
	e := NewEdit(0, 0, 10, "")

	// Simulate entering 'x' with NumLock and CapsLock enabled
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            'x',
		ControlKeyState: vtinput.NumLockOn | vtinput.CapsLockOn,
	})

	if e.GetText() != "x" {
		t.Errorf("Expected 'x', got %q. Lock keys probably blocked the input.", e.GetText())
	}
}

func TestVMenu_ScrollbarMouseClick(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 10)
	m := NewVMenu("Title")
	// Add 20 items so menu scrolls
	for i := 0; i < 20; i++ {
		m.AddItem(MenuItem{Text: "Item"})
	}
	m.SetPosition(0, 0, 10, 6) // Height 7, data 5 (Y1+1..Y2-1)
	m.Show(scr)

	// Initial state: SelectPos 0

	// 1. Click down arrow (X = X2 = 10, Y = Y2-1 = 5)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 5, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 1 {
		t.Errorf("VMenu down arrow click failed, pos %d", m.TopPos)
	}

	// 2. Click up arrow (Y = Y1+1 = 1)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 1, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 0 {
		t.Errorf("VMenu up arrow click failed, pos %d", m.TopPos)
	}

	// 3. Page Down click (Y = 4)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 4, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 5 { // 0 + height (5) = 5
		t.Errorf("VMenu PageDown click failed, pos %d", m.TopPos)
	}

	// 4. Page Up click (Y = 2)
	m.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 2, ButtonState: vtinput.FromLeft1stButtonPressed,
	})
	if m.TopPos != 0 { // 5 - height (5) = 0
		t.Errorf("VMenu PageUp click failed, pos %d", m.TopPos)
	}
}
func TestVMenu_Hotkeys(t *testing.T) {
	m := NewVMenu("Menu")
	m.AddItem(MenuItem{Text: "Open &File"})
	m.AddItem(MenuItem{Text: "&Save"})
	m.AddItem(MenuItem{Text: "E&xit"})

	// 1. Press 's' (second item hotkey)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 's'})

	if m.SelectPos != 1 {
		t.Errorf("Expected selectPos 1 for 'Save', got %d", m.SelectPos)
	}
	if !m.IsDone() || m.exitCode != 1 {
		t.Error("Menu should be finished with exitCode 1")
	}
}

func TestEdit_History(t *testing.T) {
	e := NewEdit(0, 0, 10, "")
	e.History = []string{"First", "Second"}

	// Simulate Alt+Down
	handled := e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_DOWN,
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !handled {
		t.Error("Alt+Down should be handled when History is present")
	}
}
func TestEdit_HistorySelection(t *testing.T) {
	e := NewEdit(0, 0, 10, "")
	e.History = []string{"Previous Command"}

	// We can't easily test the full Push/Pop cycle of FrameManager here,
	// but we can test the Edit's SetText which is called by the history menu.
	e.SetText(e.History[0])

	if e.GetText() != "Previous Command" {
		t.Errorf("SetText failed: expected 'Previous Command', got %q", e.GetText())
	}

	if e.curPos != 16 {
		t.Errorf("Cursor position should be at the end of the new text, got %d", e.curPos)
	}
}
func TestEdit_HistoryTrigger(t *testing.T) {
	e := NewEdit(0, 0, 10, "")

	// 1. Alt+Down without history -> should return false
	if e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN, ControlKeyState: vtinput.LeftAltPressed}) {
		t.Error("Edit should NOT handle Alt+Down when history is empty")
	}

	// 2. Alt+Down with history -> should return true
	e.History = []string{"cmd"}
	if !e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN, ControlKeyState: vtinput.LeftAltPressed}) {
		t.Error("Edit should handle Alt+Down when history is available")
	}
}
func TestEdit_HistoryManagement(t *testing.T) {
	e := NewEdit(0, 0, 10, "")

	// 1. Add unique items
	e.AddHistory("one")
	e.AddHistory("two")

	if e.History[0] != "two" || e.History[1] != "one" {
		t.Errorf("AddHistory order error: %v", e.History)
	}

	// 2. Add duplicate - should move to top
	e.AddHistory("one")
	if len(e.History) != 2 {
		t.Errorf("History should not have duplicates, size: %d", len(e.History))
	}
	if e.History[0] != "one" {
		t.Error("Duplicate item should be moved to the top")
	}

	// 3. Size limit
	for i := 0; i < 50; i++ {
		e.AddHistory(string(rune('A' + i)))
	}
	if len(e.History) > 32 {
		t.Errorf("History limit exceeded: %d", len(e.History))
	}
}

func TestEdit_HistoryButtonClick(t *testing.T) {
	e := NewEdit(0, 0, 10, "")
	e.ShowHistoryButton = true
	e.History = []string{"item"}

	// Click on the button at (9, 0)
	handled := e.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 9, MouseY: 0,
	})

	if !handled {
		t.Error("Edit should handle click on history button")
	}
}
func TestEdit_OnAction(t *testing.T) {
	e := NewEdit(0, 0, 10, "test")
	called := false
	e.OnAction = func() { called = true }

	// Simulate Enter
	handled := e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType,
		KeyDown: true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	if !handled || !called {
		t.Error("OnAction callback was not triggered on Enter")
	}
}

func TestEdit_SelectAllAndClear(t *testing.T) {
	e := NewEdit(0, 0, 20, "initial path")
	
	// 1. Trigger SelectAll
	e.SelectAll()
	if e.selStart != 0 || e.selEnd != 12 {
		t.Errorf("SelectAll failed: range [%d:%d]", e.selStart, e.selEnd)
	}
	if !e.clearFlag {
		t.Error("SelectAll should set clearFlag")
	}

	// 2. Typing a character should replace everything
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'C',
	})
	
	if e.GetText() != "C" {
		t.Errorf("Typing after SelectAll failed: expected 'C', got %q", e.GetText())
	}
	if e.selStart != -1 {
		t.Error("Selection should be cleared after typing")
	}
}
func TestEdit_HistoryNavigationShortcuts(t *testing.T) {
	e := NewEdit(0, 0, 10, "")
	e.AddHistory("cmd1")

	// 1. Ctrl+E -> HistoryUp
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_E, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if e.GetText() != "cmd1" {
		t.Errorf("Ctrl+E failed: expected 'cmd1', got %q", e.GetText())
	}

	// 2. Ctrl+X -> HistoryDown (clears since we only have 1 item)
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_X, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if e.GetText() != "" {
		t.Errorf("Ctrl+X failed to cycle down: expected empty string, got %q", e.GetText())
	}
}

func TestEdit_SelectAllAndNavigate(t *testing.T) {
	e := NewEdit(0, 0, 20, "some text")
	e.SelectAll()

	// 3. Navigating (Left Arrow) should clear selection but NOT the text
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT,
	})

	if e.GetText() != "some text" {
		t.Error("Navigation should not clear text when SelectAll is active")
	}
	if e.selStart != -1 || e.clearFlag {
		t.Error("Navigation should clear selection and clearFlag")
	}
}
func TestEdit_FullCoverage_FarHotkeys(t *testing.T) {
	SetDefaultPalette()
	e := NewEdit(0, 0, 20, "Far Navigation")

	// 1. Ctrl+A -> Select All
	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if e.selStart != 0 || e.selEnd != len(e.text) || !e.clearFlag {
		t.Error("Edit Ctrl+A failed to select all or set clearFlag")
	}

	// 2. Any key after Ctrl+A should replace text
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'X'})
	if e.GetText() != "X" {
		t.Errorf("Replacement after Ctrl+A failed, got %q", e.GetText())
	}
}
func TestEdit_FarHotkeys_SelectAll(t *testing.T) {
	SetDefaultPalette()
	e := NewEdit(0, 0, 20, "Select All Me")

	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if e.selStart != 0 || e.selEnd != len(e.text) {
		t.Errorf("Edit Ctrl+A failed: range [%d:%d]", e.selStart, e.selEnd)
	}
	if !e.clearFlag {
		t.Error("Ctrl+A in Edit should set clearFlag")
	}
}
func TestEdit_CtrlA_SelectAll(t *testing.T) {
	SetDefaultPalette()
	e := NewEdit(0, 0, 20, "Select All Me")

	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_A, ControlKeyState: vtinput.LeftCtrlPressed,
	})

	if e.selStart != 0 || e.selEnd != len(e.text) {
		t.Errorf("Edit Ctrl+A failed: range [%d:%d]", e.selStart, e.selEnd)
	}
	if !e.clearFlag {
		t.Error("Ctrl+A in Edit should set clearFlag")
	}
}

func TestEdit_InsertString_Selection(t *testing.T) {
	e := NewEdit(0, 0, 20, "original")
	// Select "rigin"
	e.selStart = 1
	e.selEnd = 6
	e.curPos = 6

	// Programmatic insertion (e.g. from f4 Ctrl+Enter)
	e.InsertString("NEW")

	if e.GetText() != "oNEWal" {
		t.Errorf("InsertString failed to overwrite selection. Got %q", e.GetText())
	}
	if e.selStart != -1 {
		t.Error("Selection should be cleared after InsertString")
	}
}
func TestEdit_Validation_RealTime(t *testing.T) {
	// Tests that IsValidInput correctly blocks keystrokes in Edit control
	e := NewEdit(0, 0, 10, "")

	// 1. Numeric filter
	e.Validator = &FilterValidator{ValidChars: "0123456789"}

	// Try typing '1' - accepted
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '1'})
	if e.GetText() != "1" { t.Errorf("Expected '1', got %q", e.GetText()) }

	// Try typing 'A' - rejected
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'A'})
	if e.GetText() != "1" { t.Errorf("Keystroke 'A' should have been blocked by FilterValidator, text is %q", e.GetText()) }

	// 2. Mask validation
	e.SetText("")
	e.Validator = &MaskValidator{Mask: "##-##"} // 2 digits, dash, 2 digits

	// Type '1' - ok
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '1'})
	// Type 'X' - rejected
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'X'})

	if e.GetText() != "1" { t.Errorf("Mask mismatch char should be blocked. Got %q", e.GetText()) }

	// Type another valid digit to reach the separator naturally
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '2'})
	if e.GetText() != "12" { t.Errorf("Expected '12', got %q", e.GetText()) }

	// Type '-' at correct position (index 2)
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '-'})
	if e.GetText() != "12-" { t.Errorf("Valid mask character should be accepted. Got %q", e.GetText()) }
}

func TestEdit_Validation_FinalTrigger(t *testing.T) {
	// Tests that Valid(CmOK) triggers error UI
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())

	dlg := NewDialog(0, 0, 20, 10, "Test")
	edit := NewEdit(2, 2, 10, "abc")
	edit.Validator = &IntRangeValidator{Min: 1, Max: 10}
	dlg.AddItem(edit)
	fm.Push(dlg)

	// Validate with CmOK (like clicking OK button)
	res := edit.Valid(CmOK)

	if res {
		t.Error("Final validation should return false for invalid data ('abc' is not int)")
	}

	// Check if message box appeared
	if fm.GetTopFrameType() != TypeDialog || fm.GetTopFrame() == dlg {
		t.Error("Validator.Error() should have pushed a message box")
	}
}

func TestEdit_WordJumps_FarSpec(t *testing.T) {
	// Спецификация Far2l для vtui.Edit (использует индексы рун)
	// word...///next   ...spaces 🍏.apple
	// 0123456789012345678901234567890123
	//           10        20        30
	e := NewEdit(0, 0, 100, "word...///next   ...spaces 🍏.apple")
	e.curPos = 0

	// 1. Ctrl+Right: [W] -> [D] (остановка на первой точке)
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 4 { t.Errorf("Stop W->D fail: expected 4, got %d", e.curPos) }

	// 2. Ctrl+Right: [D1] -> [D2] (смена разделителя с точки на слэш)
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 7 { t.Errorf("Stop D1->D2 fail: expected 7, got %d", e.curPos) }

	// 3. Ctrl+Right: [D] -> [W] (прыжок к началу 'next')
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 10 { t.Errorf("Stop D->W fail: expected 10, got %d", e.curPos) }

	// 4. Ctrl+Right: [S] -> [D] (прыжок через пробелы к блоку точек)
	e.curPos = 14 // сразу после 'next'
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 17 { t.Errorf("Stop S->D fail: expected 17, got %d", e.curPos) }

	// 5. Ctrl+Left: [S] -> [W] (прыжок назад к 'spaces')
	e.curPos = 26 // пробел после 'spaces'
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 20 { t.Errorf("Stop S->W fail: expected 20, got %d", e.curPos) }

	// 6. Ctrl+Left: [D] -> [W] (прыжок назад от начала "apple" к Эмодзи)
	// apple начинается на 29. Перед ним точка на 28. Перед ней яблоко на 27.
	e.curPos = 29
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 27 { t.Errorf("Stop D->W fail (Unicode): expected 27, got %d", e.curPos) }
}
func TestEdit_WordJumps_Left_FarSpec(t *testing.T) {
	// Проверяем специфичные правила для движения ВЛЕВО
	// "word   ///...next"
	//  01234567890123456
	e := NewEdit(0, 0, 100, "word   ///...next")

	// 1. Проверка [D] -> [W]: прыжок от конца 'next' к началу 'next'
	e.curPos = 17
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.LeftCtrlPressed})
	// prev='.', curr='n' ([D]->[W]). Остановка на 'n' (13)
	if e.curPos != 13 { t.Errorf("Stop D->W fail: expected 13, got %d", e.curPos) }

	// 2. Проверка игнорирования [D1] -> [D2] и остановки на [S] -> [D]
	// Стартуем с 'n' (13), прыгаем влево.
	// Должен проскочить '.' и '/', так как смена разделителей при движении влево игнорируется.
	// Должен остановиться на первом слэше (7), так как слева от него пробел ([S]->[D]).
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 7 { t.Errorf("Stop S->D fail (and ignore D1->D2): expected 7, got %d", e.curPos) }

	// 3. Проверка [S] -> [W]: от начала разделителей (7) к началу слова 'word'
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.LeftCtrlPressed})
	if e.curPos != 0 { t.Errorf("Stop S->W fail: expected 0, got %d", e.curPos) }
}

func TestEdit_WordSelection_FarSpec(t *testing.T) {
	e := NewEdit(0, 0, 100, "select this word")
	e.SelectAll() // curPos=16, selAnchor=0, selStart=0, selEnd=16, clearFlag=true

	e.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_LEFT,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	})

	if e.clearFlag { t.Error("Shift+Ctrl navigation must reset clearFlag") }

	// Якорь в 0, курсор прыгнул к началу "word" (индекс руны 12).
	// Выделение охватывает [0:12].
	if e.selStart != 0 || e.selEnd != 12 {
		t.Errorf("Selection fail: expected [0:12], got [%d:%d]", e.selStart, e.selEnd)
	}
}
func TestEdit_WordJumps_DifferentDividers(t *testing.T) {
	// Прыжок вправо должен останавливаться при смене одного знака препинания на другой (D1 -> D2)
	e := NewEdit(0, 0, 20, "...///")
	e.curPos = 0

	// Ctrl+Right
	// Первый шаг (обязательный) -> индекс 1.
	// Цикл:
	// Индекс 2: Prev='.', Curr='.' (D->D, одинаковые) -> продолжаем.
	// Индекс 3: Prev='.', Curr='/' (D1->D2, разные) -> STOP.
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.LeftCtrlPressed})

	if e.curPos != 3 {
		t.Errorf("Expected stop on first slash (index 3), got %d", e.curPos)
	}
}
