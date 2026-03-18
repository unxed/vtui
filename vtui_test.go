package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// checkCell is a helper to verify the character and attributes at a specific coordinate in the ScreenBuf.
func checkCell(t *testing.T, scr *ScreenBuf, x, y int, expectedChar uint64, expectedAttr uint64) {
	t.Helper()
	if x < 0 || y < 0 || x >= scr.width || y >= scr.height {
		t.Errorf("checkCell coordinates (%d, %d) are out of bounds for screen size (%d, %d)", x, y, scr.width, scr.height)
		return
	}
	cell := scr.buf[y*scr.width+x]
	if cell.Char != expectedChar {
		if expectedChar == WideCharFiller || cell.Char == WideCharFiller {
			t.Errorf("at (%d, %d): expected char %X, got %X", x, y, expectedChar, cell.Char)
		} else {
			t.Errorf("at (%d, %d): expected char '%c' (U+%04X), got '%c' (U+%04X)", x, y, rune(expectedChar), expectedChar, rune(cell.Char), cell.Char)
		}
	}
	if cell.Attributes != expectedAttr {
		t.Errorf("at (%d, %d): expected attr %X, got %X", x, y, expectedAttr, cell.Attributes)
	}
}

func TestScreenBuf_Write(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(10, 5)
	attr := uint64(123)

	text := StringToCharInfo("hello", attr)
	scr.Write(2, 2, text)

	// Check written text
	checkCell(t, scr, 2, 2, 'h', attr)
	checkCell(t, scr, 6, 2, 'o', attr)
	// Check surrounding cells are untouched (still zero)
	checkCell(t, scr, 1, 2, 0, 0)
	checkCell(t, scr, 7, 2, 0, 0)

	// Test clipping
	longText := StringToCharInfo("1234567890", attr)
	scr.Write(5, 3, longText)
	checkCell(t, scr, 5, 3, '1', attr)
	checkCell(t, scr, 9, 3, '5', attr)
	if scr.buf[3*scr.width+9].Char != uint64('5') { // ensure we didn't write past the boundary
		t.Error("write operation did not clip correctly")
	}

	// Test writing outside of bounds (should be a no-op)
	scr.Write(-10, -10, text) // Should not panic or write anything
	scr.Write(20, 20, text)   // Should not panic or write anything
}

func TestScreenBuf_FillRect(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(20, 10)
	attr := uint64(456)
	fillChar := 'X'

	scr.FillRect(5, 5, 15, 8, fillChar, attr)

	// Check corners
	checkCell(t, scr, 5, 5, uint64(fillChar), attr)
	checkCell(t, scr, 15, 5, uint64(fillChar), attr)
	checkCell(t, scr, 5, 8, uint64(fillChar), attr)
	checkCell(t, scr, 15, 8, uint64(fillChar), attr)
	// Check center
	checkCell(t, scr, 10, 6, uint64(fillChar), attr)
	// Check outside
	checkCell(t, scr, 4, 5, 0, 0)
	checkCell(t, scr, 16, 8, 0, 0)
	checkCell(t, scr, 5, 4, 0, 0)
	checkCell(t, scr, 15, 9, 0, 0)
}

func TestFrame_Rendering(t *testing.T) {
	SetDefaultPalette()
	borderColor := Palette[ColDialogBox]
	titleColor := Palette[ColDialogBoxTitle]

	t.Run("SingleBox", func(t *testing.T) {
		scr := NewScreenBuf()
		scr.AllocBuf(20, 10)
		frame := NewBorderedFrame(1, 1, 18, 8, SingleBox, "")
		frame.DisplayObject(scr)

		checkCell(t, scr, 1, 1, '┌', borderColor)
		checkCell(t, scr, 18, 1, '┐', borderColor)
		checkCell(t, scr, 1, 8, '└', borderColor)
		checkCell(t, scr, 18, 8, '┘', borderColor)
		checkCell(t, scr, 10, 1, '─', borderColor)
		checkCell(t, scr, 1, 5, '│', borderColor)
	})

	t.Run("DoubleBox", func(t *testing.T) {
		scr := NewScreenBuf()
		scr.AllocBuf(20, 10)
		frame := NewBorderedFrame(1, 1, 18, 8, DoubleBox, "")
		frame.DisplayObject(scr)

		checkCell(t, scr, 1, 1, '╔', borderColor)
		checkCell(t, scr, 18, 1, '╗', borderColor)
		checkCell(t, scr, 1, 8, '╚', borderColor)
		checkCell(t, scr, 18, 8, '╝', borderColor)
		checkCell(t, scr, 10, 1, '═', borderColor)
		checkCell(t, scr, 1, 5, '║', borderColor)
	})

	t.Run("TitledFrame", func(t *testing.T) {
		title := "Title"
		scr := NewScreenBuf()
		scr.AllocBuf(30, 10)
		// Frame width is 22 (from 4 to 25)
		frame := NewBorderedFrame(4, 2, 25, 8, DoubleBox, title)
		frame.DisplayObject(scr)

		// Title is " Title ", length 6. Start pos = 4 + (22 - 6)/2 - 1 = 4 + 8 - 1 = 11
		checkCell(t, scr, 11, 2, ' ', titleColor)
		checkCell(t, scr, 12, 2, 'T', titleColor)
		checkCell(t, scr, 13, 2, 'i', titleColor)
		checkCell(t, scr, 14, 2, 't', titleColor)
		checkCell(t, scr, 15, 2, 'l', titleColor)
		checkCell(t, scr, 16, 2, 'e', titleColor)
		checkCell(t, scr, 17, 2, ' ', titleColor)

		// Check that frame border is present outside title
		checkCell(t, scr, 10, 2, '═', borderColor)
		checkCell(t, scr, 18, 2, '═', borderColor)

		// Check left corner
		checkCell(t, scr, 4, 2, '╔', borderColor)
	})
}

func TestScreenObject_HelpInheritance(t *testing.T) {
	parent := &ScreenObject{}
	child := &ScreenObject{owner: parent}

	// 1. No help by default
	if child.GetHelp() != "" {
		t.Errorf("Expected empty help, got '%s'", child.GetHelp())
	}

	// 2. Set help for parent — child should inherit it
	parent.SetHelp("ParentTopic")
	if child.GetHelp() != "ParentTopic" {
		t.Errorf("Child should inherit parent help. Expected 'ParentTopic', got '%s'", child.GetHelp())
	}

	// 3. Set own help for child — it should override parent's
	child.SetHelp("ChildTopic")
	if child.GetHelp() != "ChildTopic" {
		t.Errorf("Child should override parent help. Expected 'ChildTopic', got '%s'", child.GetHelp())
	}

	// 4. Check that parent's help remained unchanged
	if parent.GetHelp() != "ParentTopic" {
		t.Errorf("Parent help should remain unchanged. Expected 'ParentTopic', got '%s'", parent.GetHelp())
	}
}
func TestKeyBar_Modifiers(t *testing.T) {
	kb := NewKeyBar()
	kb.Normal[0] = "Normal1"
	kb.Shift[0] = "Shift1"
	kb.Alt[0] = "Alt1"
	kb.SetVisible(true)

	scr := NewScreenBuf()
	scr.AllocBuf(40, 1)
	kb.SetPosition(0, 0, 39, 0)
	SetDefaultPalette()

	// 1. Test Normal state
	kb.SetModifiers(false, false, false)
	kb.Show(scr)
	// Cell 0 is number '1', Cell 1 start of text 'N'
	checkCell(t, scr, 1, 0, 'N', Palette[ColKeyBarText])

	// 2. Test Shift state
	kb.SetModifiers(true, false, false)
	kb.Show(scr)
	checkCell(t, scr, 1, 0, 'S', Palette[ColKeyBarText])

	// 3. Test Alt state
	kb.SetModifiers(false, false, true)
	kb.Show(scr)
	checkCell(t, scr, 1, 0, 'A', Palette[ColKeyBarText])
}
func TestMenuBar_Geometry(t *testing.T) {
	mb := NewMenuBar([]string{"Left", "Files"})
	mb.SetPosition(0, 0, 80, 0)

	// "  Left  " -> 8 chars. Start is X1+2 = 2.
	// Item 0 starts at 2.
	// Item 1 should start after Item 0.
	x0 := mb.GetItemX(0)
	x1 := mb.GetItemX(1)

	if x0 != 2 {
		t.Errorf("Expected first item at X=2, got %d", x0)
	}

	expectedX1 := 2 + 8 // 2 + width of "  Left  "
	if x1 != expectedX1 {
		t.Errorf("Expected second item at X=%d, got %d", expectedX1, x1)
	}
}

func TestVMenu_Callbacks(t *testing.T) {
	m := NewVMenu("Test")
	m.AddItem("Item")

	leftTriggered := false
	rightTriggered := false
	closeTriggered := false

	m.OnLeft = func() { leftTriggered = true }
	m.OnRight = func() { rightTriggered = true }
	m.OnClose = func() { closeTriggered = true }

	// 1. Test Left Arrow
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if !leftTriggered {
		t.Error("OnLeft callback was not triggered")
	}
	if !m.IsDone() {
		t.Error("Menu should be closed after switching via arrow")
	}

	// 2. Test Right Arrow
	m.ClearDone()
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if !rightTriggered {
		t.Error("OnRight callback was not triggered")
	}

	// 3. Test Escape (Close)
	m.ClearDone()
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if !closeTriggered {
		t.Error("OnClose callback was not triggered on Escape")
	}
}

func TestEdit_Navigation(t *testing.T) {
	e := NewEdit(0, 0, 20, "hello world")
	e.curPos = 0 // Override default (which is at the end)

	// 1. Simple Right
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if e.curPos != 1 {
		t.Errorf("Right: expected curPos 1, got %d", e.curPos)
	}

	// 2. End
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})
	if e.curPos != 11 {
		t.Errorf("End: expected curPos 11, got %d", e.curPos)
	}

	// 3. Simple Left
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if e.curPos != 10 {
		t.Errorf("Left: expected curPos 10, got %d", e.curPos)
	}

	// 4. Home
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	if e.curPos != 0 {
		t.Errorf("Home: expected curPos 0, got %d", e.curPos)
	}

	// 5. Ctrl+Right (Word wise)
	// "hello world" -> should jump to start of "world" (index 6)
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RIGHT,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if e.curPos != 6 {
		t.Errorf("Ctrl+Right: expected curPos 6, got %d", e.curPos)
	}

	// 6. Ctrl+Left (Word wise)
	// back to 0
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_LEFT,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if e.curPos != 0 {
		t.Errorf("Ctrl+Left: expected curPos 0, got %d", e.curPos)
	}
}

func TestEdit_Selection(t *testing.T) {
	e := NewEdit(0, 0, 20, "hello world")
	e.curPos = 0

	// 1. Shift + Right (Select 'h')
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_RIGHT,
		ControlKeyState: vtinput.ShiftPressed,
	})
	if e.selStart != 0 || e.selEnd != 1 || e.selAnchor != 0 {
		t.Errorf("Shift+Right: expected range [0:1] anchor 0, got [%d:%d] anchor %d", e.selStart, e.selEnd, e.selAnchor)
	}

	// 2. Shift + End (Select the rest)
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_END,
		ControlKeyState: vtinput.ShiftPressed,
	})
	if e.selStart != 0 || e.selEnd != 11 {
		t.Errorf("Shift+End: expected range [0:11], got [%d:%d]", e.selStart, e.selEnd)
	}

	// 3. Move without Shift should clear selection
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if e.selStart != -1 {
		t.Error("Move without Shift did not clear selection")
	}

	// 4. Backward selection
	e.curPos = 5
	e.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_LEFT,
		ControlKeyState: vtinput.ShiftPressed,
	})
	// Anchor was 5, curPos moved to 4. Range should be [4:5]
	if e.selStart != 4 || e.selEnd != 5 || e.selAnchor != 5 {
		t.Errorf("Backward Shift+Left: expected range [4:5] anchor 5, got [%d:%d] anchor %d", e.selStart, e.selEnd, e.selAnchor)
	}
}

func TestEdit_Editing(t *testing.T) {
	e := NewEdit(0, 0, 20, "test")
	e.curPos = 4

	// 1. Char input
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: '!'})
	if string(e.text) != "test!" {
		t.Errorf("Input: expected 'test!', got '%s'", string(e.text))
	}

	// 2. Backspace
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK})
	if string(e.text) != "test" {
		t.Errorf("Backspace: expected 'test', got '%s'", string(e.text))
	}

	// 3. Delete with selection
	e.curPos = 0
	// Select "te"
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.ShiftPressed})
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.ShiftPressed})
	// Press Delete
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE})
	if string(e.text) != "st" {
		t.Errorf("Delete selection: expected 'st', got '%s'", string(e.text))
	}
	if e.selStart != -1 {
		t.Error("Selection was not cleared after Delete")
	}
}

func TestEdit_Rendering(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(10, 1)
	e := NewEdit(0, 0, 10, "abc")
	e.curPos = 1
	e.selStart = 1
	e.selEnd = 2 // 'b' is selected

	e.Show(scr)
	e.DisplayObject(scr)

	// Must initialize default palette
	SetDefaultPalette()

	colNorm := Palette[ColDialogEdit]
	colSel := Palette[ColDialogEditSelected]

	// 'a' - normal
	checkCell(t, scr, 0, 0, 'a', colNorm)
	// 'b' - selected
	checkCell(t, scr, 1, 0, 'b', colSel)
	// 'c' - normal
	checkCell(t, scr, 2, 0, 'c', colNorm)
	// padding - normal
	checkCell(t, scr, 5, 0, ' ', colNorm)
}

func TestEdit_Overtype(t *testing.T) {
	e := NewEdit(0, 0, 10, "abc")
	e.curPos = 0
	e.overtype = true

	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'X'})
	if string(e.text) != "Xbc" {
		t.Errorf("Overtype: expected 'Xbc', got '%s'", string(e.text))
	}

	e.overtype = false
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'Y'})
	if string(e.text) != "XYbc" {
		t.Errorf("Insert: expected 'XYbc', got '%s'", string(e.text))
	}
}
func TestEdit_Unicode_Selection(t *testing.T) {
	SetDefaultPalette()
	// "A世B" -> A(1), 世(2), B(1). Total 4 cells.
	e := NewEdit(0, 0, 10, "A世B")
	e.curPos = 0

	// Select "A" (Shift + Right)
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.ShiftPressed})
	if e.selStart != 0 || e.selEnd != 1 {
		t.Errorf("Expected selection [0:1], got [%d:%d]", e.selStart, e.selEnd)
	}

	// Select "A世" (Right again)
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT, ControlKeyState: vtinput.ShiftPressed})
	if e.selStart != 0 || e.selEnd != 2 {
		t.Errorf("Expected selection [0:2], got [%d:%d]", e.selStart, e.selEnd)
	}

	// Rendering into 4-cell buffer
	scr := NewScreenBuf()
	scr.AllocBuf(4, 1)
	e.SetPosition(0, 0, 3, 0)
	e.Show(scr)

	// 'A' (cell 0) and '世' (cells 1, 2) should be in Palette[ColDialogEditSelected]
	colSel := Palette[ColDialogEditSelected]
	checkCell(t, scr, 0, 0, 'A', colSel)
	checkCell(t, scr, 1, 0, '世', colSel)
	checkCell(t, scr, 2, 0, WideCharFiller, colSel)

	// 'B' (cell 3) is not selected
	checkCell(t, scr, 3, 0, 'B', Palette[ColDialogEdit])
}

func TestVMenu_Navigation(t *testing.T) {
	m := NewVMenu("Title")
	m.AddItem("One")
	m.AddSeparator()
	m.AddItem("Two")
	m.SetPosition(0, 0, 10, 5)

	// Initial selection at 0
	if m.selectPos != 0 {
		t.Errorf("Initial selection: expected 0, got %d", m.selectPos)
	}

	// 1. Down -> should skip separator (index 1) and go to "Two" (index 2)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if m.selectPos != 2 {
		t.Errorf("Down: expected index 2, got %d", m.selectPos)
	}

	// 2. Up -> should skip separator and go back to 0
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if m.selectPos != 0 {
		t.Errorf("Up: expected index 0, got %d", m.selectPos)
	}

	// 3. End -> should go to last item (2)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})
	if m.selectPos != 2 {
		t.Errorf("End: expected index 2, got %d", m.selectPos)
	}
}

func TestVMenu_Rendering(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(15, 10)
	SetDefaultPalette()
	m := NewVMenu("Title")
	m.AddItem("Item1")
	m.AddSeparator()
	m.AddItem("Item2")
	m.SetPosition(2, 2, 12, 6)
	m.Show(scr)

	// Check frame corners (DoubleBox)
	SetDefaultPalette()

	borderColor := Palette[ColMenuBox]
	checkCell(t, scr, 2, 2, '╔', borderColor)
	checkCell(t, scr, 12, 2, '╗', borderColor)
	checkCell(t, scr, 2, 6, '╚', borderColor)
	checkCell(t, scr, 12, 6, '╝', borderColor)

	// Check first item (selected by default)
	// Pos: X1 + 1 (3), Y1 + 1 (3)
	// Characters in VMenu are padded. "Item1" with checkmark and padding.
	checkCell(t, scr, 4, 3, 'I', Palette[ColMenuSelectedText])

	// Check separator rendering
	// Pos: Y1 + 2 (4)
	// It uses boxSymbols[22] (╟) and boxSymbols[23] (╢)
	checkCell(t, scr, 2, 4, '╟', borderColor)
	checkCell(t, scr, 12, 4, '╢', borderColor)
	checkCell(t, scr, 7, 4, '─', borderColor)
}
