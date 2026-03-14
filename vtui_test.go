package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// checkCell is a helper to verify the character and attributes at a specific coordinate in the ScreenBuf.
func checkCell(t *testing.T, scr *ScreenBuf, x, y int, expectedChar rune, expectedAttr uint64) {
	t.Helper()
	if x < 0 || y < 0 || x >= scr.width || y >= scr.height {
		t.Errorf("checkCell coordinates (%d, %d) are out of bounds for screen size (%d, %d)", x, y, scr.width, scr.height)
		return
	}
	cell := scr.buf[y*scr.width+x]
	if rune(cell.Char) != expectedChar {
		t.Errorf("at (%d, %d): expected char '%c' (U+%04X), got '%c' (U+%04X)", x, y, expectedChar, expectedChar, rune(cell.Char), cell.Char)
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
	checkCell(t, scr, 5, 5, fillChar, attr)
	checkCell(t, scr, 15, 5, fillChar, attr)
	checkCell(t, scr, 5, 8, fillChar, attr)
	checkCell(t, scr, 15, 8, fillChar, attr)
	// Check center
	checkCell(t, scr, 10, 6, fillChar, attr)
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

	// Обязательно инициализируем дефолтную палитру
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
