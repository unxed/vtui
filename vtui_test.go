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
	scr := NewSilentScreenBuf()
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
	scr := NewSilentScreenBuf()
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
		scr := NewSilentScreenBuf()
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
		scr := NewSilentScreenBuf()
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
		scr := NewSilentScreenBuf()
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
func TestFrame_CloseButtonRendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 10)
	// Frame from X=5 to X=25. Close button [×] should be at 25-4=21, 22, 23.
	frame := NewBorderedFrame(5, 5, 25, 10, DoubleBox, "")
	frame.ShowClose = true
	frame.DisplayObject(scr)

	colBox := Palette[ColDialogBox]
	checkCell(t, scr, 21, 5, uint64(UIStrings.CloseBrackets[0]), colBox)
	checkCell(t, scr, 22, 5, uint64(UIStrings.CloseSymbol), colBox)
	checkCell(t, scr, 23, 5, uint64(UIStrings.CloseBrackets[1]), colBox)
}

func TestFrame_IsBorderClick(t *testing.T) {
	f := NewBorderedFrame(10, 10, 20, 20, SingleBox, "Test")

	tests := []struct {
		x, y int
		want bool
	}{
		{10, 10, true},  // Top-left corner
		{20, 20, true},  // Bottom-right corner
		{15, 10, true},  // Top edge
		{10, 15, true},  // Left edge
		{15, 15, false}, // Center (not the border)
		{5, 5, false},   // Outside
		{25, 25, false}, // Outside
	}

	for _, tt := range tests {
		if got := f.IsBorderClick(tt.x, tt.y); got != tt.want {
			t.Errorf("IsBorderClick(%d, %d) = %v; want %v", tt.x, tt.y, got, tt.want)
		}
	}
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

	scr := NewSilentScreenBuf()
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
func TestMenuBar_ProcessMouse(t *testing.T) {
	mb := NewMenuBar([]string{"&Left", "&Right"})
	mb.SetPosition(0, 0, 80, 0)

	// Click on "Right" (Item 1).
	// Item 0 is "  Left  " -> 8 chars. Starts at 2. Ends at 9.
	// Item 1 starts at 10.
	handled := mb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX: 11, MouseY: 0,
	})

	if !handled {
		t.Error("MenuBar should handle valid mouse click")
	}
	if !mb.Active || mb.SelectPos != 1 {
		t.Error("MenuBar should become active and select correct item")
	}
}
func TestMenuBar_MouseEdgeCases(t *testing.T) {
	mb := NewMenuBar([]string{"A"})
	mb.SetPosition(0, 0, 10, 0)

	// Click outside on Y axis (Y=1)
	handled := mb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: 0, MouseY: 1,
	})
	if handled { t.Error("MenuBar should ignore out of bounds Y clicks") }

	// Click outside on X axis (X=20)
	handled = mb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed, MouseX: 20, MouseY: 0,
	})
	if handled { t.Error("MenuBar should ignore out of bounds X clicks") }
}

func TestVMenu_Callbacks(t *testing.T) {
	// With the new command system, VMenu emits CmMenuLeft, CmMenuRight, CmMenuClose.
	m := NewVMenu("Test")
	m.AddItem(MenuItem{Text: "Item"})

	leftTriggered := false
	rightTriggered := false

	oldFm := FrameManager
	localFm := &frameManager{}
	localFm.Init(NewSilentScreenBuf())
	FrameManager = localFm
	defer func() { FrameManager = oldFm }()

	mb := NewMenuBar([]string{"Left", "Right"})
	mb.Active = true
	mb.SelectPos = 1 // Start at Right
	m.SetOwner(mb)
	mb.SetSubMenu(m)
	FrameManager.MenuBar = mb
	FrameManager.Push(m)

	// We can check if MenuBar responds to the emitted commands
	// by examining mb.SelectPos
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	// After LEFT, MenuBar should have selected Item 0 (Left)
	if mb.SelectPos == 0 {
		leftTriggered = true
	}

	if !leftTriggered {
		t.Error("CmMenuLeft was not properly processed by MenuBar")
	}

	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if mb.SelectPos == 1 {
		rightTriggered = true
	}
	if !rightTriggered {
		t.Error("CmMenuRight was not properly processed by MenuBar")
	}

	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if mb.Active {
		t.Error("CmMenuClose was not properly processed by MenuBar on Escape")
	}
}

func TestVMenu_F10_ClosesMenu(t *testing.T) {
	m := NewVMenu("Test")
	handled := m.ProcessKey(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F10,
	})

	if !m.IsDone() || m.exitCode != -1 {
		t.Error("F10 should close the VMenu with exitCode -1")
	}
	
	// In the test, FrameManager is empty, so GetTopFrame() != m. 
	// The key should NOT be swallowed.
	if handled {
		t.Error("VMenu widget should NOT swallow F10 (it should bubble up)")
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
	e.ClearSelection()
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
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)
	e := NewEdit(0, 0, 10, "abc")
	e.ClearSelection()
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
	e.ClearSelection()
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

	e.ClearSelection()
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
	scr := NewSilentScreenBuf()
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
	m.AddItem(MenuItem{Text: "One"})
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "Two"})
	m.SetPosition(0, 0, 10, 5)

	// Initial selection at 0
	if m.SelectPos != 0 {
		t.Errorf("Initial selection: expected 0, got %d", m.SelectPos)
	}

	// 1. Down -> should skip separator (index 1) and go to "Two" (index 2)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if m.SelectPos != 2 {
		t.Errorf("Down: expected index 2, got %d", m.SelectPos)
	}

	// 2. Up -> should skip separator and go back to 0
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if m.SelectPos != 0 {
		t.Errorf("Up: expected index 0, got %d", m.SelectPos)
	}

	// 3. End -> should go to last item (2)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})
	if m.SelectPos != 2 {
		t.Errorf("End: expected index 2, got %d", m.SelectPos)
	}
}

func TestVMenu_Rendering(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(15, 10)
	SetDefaultPalette()
	m := NewVMenu("Title")
	m.AddItem(MenuItem{Text: "Item1"})
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "Item2"})
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
func TestVMenu_TitleCleaning(t *testing.T) {
	// Constructor should automatically strip ampersands from the title
	m := NewVMenu("&Options")
	if m.title != "Options" {
		t.Errorf("VMenu title not cleaned: expected 'Options', got %q", m.title)
	}
}

func TestMenuBar_KeyInterception(t *testing.T) {
	mb := NewMenuBar([]string{"&Files", "&Edit"})
	mb.Active = false

	// 1. Naked hotkey 'F' should NOT be handled when inactive
	if mb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'f'}) {
		t.Error("MenuBar should NOT handle naked hotkeys when inactive")
	}

	// 2. Alt+F should be handled always
	if !mb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'f', ControlKeyState: vtinput.LeftAltPressed}) {
		t.Error("MenuBar should handle Alt+Hotkey even when inactive")
	}

	// 3. Naked hotkey 'E' SHOULD be handled when active
	mb.Active = true
	if !mb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'e'}) {
		t.Error("MenuBar should handle naked hotkeys when active")
	}
}

func TestMenuBar_SubMenuLifecycle(t *testing.T) {
	// This test ensures MenuBar correctly manages its activeSubMenu reference
	mb := NewMenuBar([]string{"&Left", "&Right"})
	mb.Active = true

	mockMenu := NewVMenu("Sub")
	mb.SetSubMenu(mockMenu)

	if mb.activeSubMenu != mockMenu {
		t.Error("SetSubMenu failed to register the menu")
	}

	// Simulating Left key - should trigger closeSub internally
	// (We can't easily check FrameManager.Pop here without refactoring,
	// but we check that the bar continues handling the key)
	handled := mb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if !handled {
		t.Error("MenuBar failed to handle navigation key with active submenu")
	}
}
func TestMenuBar_GetItemX_Ampersands(t *testing.T) {
	// Submenus must align with visible text, ignoring '&'
	mb := NewMenuBar([]string{"&File", "E&xit"})
	mb.X1 = 0

	// Item 0: "  File  " (start at 2, length 8)
	// Item 1: should start at 2 + 8 = 10
	x1 := mb.GetItemX(1)
	if x1 != 10 {
		t.Errorf("GetItemX failed to ignore ampersands: got %d, want 10", x1)
	}
}

func TestMenuBar_AltRejection(t *testing.T) {
	mb := NewMenuBar([]string{"&Files"})
	mb.Active = false

	// Alt+Z does not match '&Files'. ProcessKey MUST return false.
	event := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'z',
		ControlKeyState: vtinput.LeftAltPressed,
	}

	if mb.ProcessKey(event) {
		t.Error("MenuBar consumed Alt-key that didn't match any item")
	}
}
func TestIntegration_HotkeyPriority(t *testing.T) {
	// Replicating the priority logic from the demo app's EventFilter
	SetDefaultPalette()

	// 1. Setup: Menu with '&Files', Dialog with '&Ok' button
	mb := NewMenuBar([]string{"&Files"})
	mb.Active = false

	dlg := NewDialog(0, 0, 10, 10, "Test")
	btnOk := NewButton(1, 1, "&Ok")
	dlg.AddItem(btnOk)

	// Priority Filter Logic helper
	process := func(e *vtinput.InputEvent) string {
		// If menu active -> priority
		if mb.Active {
			if mb.ProcessKey(e) { return "menu" }
		}
		// If inactive -> Dialog first
		if dlg.ProcessKey(e) { return "dialog" }
		// Fallback to menu activation
		if !mb.Active && (e.ControlKeyState & vtinput.LeftAltPressed) != 0 {
			if mb.ProcessKey(e) { return "menu_activated" }
		}
		return "none"
	}

	// SCENARIO 1: Press Alt+O (matches Dialog's &Ok)
	// Menu is inactive. Dialog should win.
	evAltO := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'o',
		ControlKeyState: vtinput.LeftAltPressed,
	}
	res := process(evAltO)
	if res != "dialog" || mb.Active {
		t.Errorf("Alt+O should go to Dialog. Got %s, Menu Active: %v", res, mb.Active)
	}

	// SCENARIO 2: Press Alt+F (matches Menu's &Files)
	// Dialog doesn't have 'f'. Menu should activate.
	evAltF := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, Char: 'f',
		ControlKeyState: vtinput.LeftAltPressed,
	}
	res = process(evAltF)
	if res != "menu_activated" || !mb.Active {
		t.Errorf("Alt+F should activate Menu. Got %s, Menu Active: %v", res, mb.Active)
	}

	// SCENARIO 3: Menu is now Active. Press Alt+O again.
	// Menu is active, so it takes precedence in the filter.
	// Note: MenuBar.ProcessKey only returns true for Alt+Letter if it matches an item.
	// Since Alt+O doesn't match 'Files', it should fall through to the Dialog.
	res = process(evAltO)
	if res != "dialog" {
		t.Errorf("Active Menu should pass non-matching Alt-keys to Dialog. Got %s", res)
	}
}

func TestVMenu_SeparatorNavigation(t *testing.T) {
	m := NewVMenu("Test")
	m.AddItem(MenuItem{Text: "One"})
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "Two"})

	// 1. Initial pos is 0
	if m.SelectPos != 0 { t.Fatal("Start pos should be 0") }

	// 2. Down from 0 should land on 2 (skipping separator at 1)
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if m.SelectPos != 2 {
		t.Errorf("Separator not skipped during Down: got pos %d, want 2", m.SelectPos)
	}

	// 3. Up from 2 should land on 0
	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if m.SelectPos != 0 {
		t.Errorf("Separator not skipped during Up: got pos %d, want 0", m.SelectPos)
	}
}
func TestMenuBar_SubMenuActivation(t *testing.T) {
	mb := NewMenuBar(nil)

	commandFired := false

	mb.Items = []MenuBarItem{
		{Label: "File", SubItems: []MenuItem{{Text: "Open"}, {Text: "Save", OnClick: func() { commandFired = true }}}},
		{Label: "Edit", SubItems: []MenuItem{{Text: "Undo"}}},
	}

	// Setup a local FrameManager to catch the pushed VMenu
	oldFm := FrameManager
	localFm := &frameManager{}
	localFm.Init(NewSilentScreenBuf())
	FrameManager = localFm
	defer func() { FrameManager = oldFm }()

	// 1. Activate SubMenu 0
	mb.ActivateSubMenu(0)
	if FrameManager.GetTopFrameType() != TypeMenu {
		t.Fatal("ActivateSubMenu did not push a VMenu to FrameManager")
	}

	subMenu := FrameManager.frames[len(FrameManager.frames)-1].(*VMenu)

	// 2. Select the second item ("Save") in the submenu
	subMenu.SetSelectPos(1)
	subMenu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if !commandFired {
		t.Error("SubMenu selection did not trigger MenuBar item command")
	}
	if mb.Active {
		t.Error("MenuBar should deactivate after a command is executed")
	}
}

func TestMenuBar_OrphanedSubMenu(t *testing.T) {
	// Setup a local FrameManager to simulate the environment
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	// 1. Create a menubar with one item that has a submenu
	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{{Label: "&File", SubItems: []MenuItem{{Text: "Open"}}}}
	fm.MenuBar = mb

	// 2. Activate the menubar and open the submenu
	mb.Active = true
	mb.ActivateSubMenu(0)

	// At this point, the stack should be [Desktop, VMenu]
	if len(fm.frames) != 2 || fm.GetTopFrameType() != TypeMenu {
		t.Fatal("Submenu was not pushed to FrameManager")
	}

	// 3. Simulate losing focus: something else happens, making the menubar inactive
	mb.Active = false

	// 4. Simulate the pre-render cleanup phase. This is where the fix should trigger.
	fm.cleanupOrphanedMenus()

	// 5. Assert: The VMenu should have been popped from the stack.
	if len(fm.frames) != 1 || fm.GetTopFrameType() == TypeMenu {
		t.Errorf("cleanupOrphanedMenus failed to close orphaned submenu. Frame count: %d", len(fm.frames))
	}
}

func TestVMenu_GetItemCount(t *testing.T) {
	m := NewVMenu("Test")
	if m.GetItemCount() != 0 {
		t.Errorf("Expected 0 items, got %d", m.GetItemCount())
	}

	m.AddItem(MenuItem{Text: "One"})
	m.AddSeparator()
	m.AddItem(MenuItem{Text: "Two"})

	if m.GetItemCount() != 3 {
		t.Errorf("Expected 3 items (including separator), got %d", m.GetItemCount())
	}
}
func TestMenuBar_SubMenuCycling(t *testing.T) {
	// Setup FrameManager and MenuBar with two submenus
	oldFm := FrameManager
	localFm := &frameManager{}
	localFm.Init(NewSilentScreenBuf())
	FrameManager = localFm
	defer func() { FrameManager = oldFm }()

	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{Label: "Menu0", SubItems: []MenuItem{{Text: "Item0"}}},
		{Label: "Menu1", SubItems: []MenuItem{{Text: "Item1"}}},
	}
	mb.Active = true
	localFm.MenuBar = mb

	// 1. Activate first submenu
	mb.ActivateSubMenu(0)
	if localFm.GetTopFrameType() != TypeMenu {
		t.Fatal("Menu0 not pushed")
	}

	// 2. Simulate Right Arrow inside the VMenu
	// VMenu OnRight should call mb.ActivateSubMenu(1)
	currentSub := localFm.frames[len(localFm.frames)-1].(*VMenu)
	currentSub.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})

	// 3. Verify: old menu popped, new menu pushed
	if len(localFm.frames) != 1 {
		t.Errorf("Expected exactly 1 frame (the new submenu), got %d", len(localFm.frames))
	}
	newSub := localFm.frames[0].(*VMenu)
	if newSub.title != "Menu1" {
		t.Errorf("Expected Menu1 to be active, got %q", newSub.title)
	}
}
func TestMenuBar_SubMenuCleanup_Deep(t *testing.T) {
	// Setup FrameManager and MenuBar
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	fm.Push(NewDesktop())

	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{{Label: "&File", SubItems: []MenuItem{{Text: "Open"}}}}
	fm.MenuBar = mb

	// 1. Activate Menu and SubMenu
	mb.Active = true
	mb.ActivateSubMenu(0)
	if fm.GetTopFrameType() != TypeMenu {
		t.Fatal("Submenu not pushed")
	}
	subMenu := fm.frames[len(fm.frames)-1]

	// 2. Simulate a dialog popping up ON TOP of the submenu
	// (e.g. background task finished)
	dlg := NewDialog(10, 10, 30, 15, "Notification")
	fm.Push(dlg)

	if len(fm.frames) != 3 { // [Desktop, VMenu, Dialog]
		t.Errorf("Expected 3 frames, got %d", len(fm.frames))
	}

	// 3. Something makes the MenuBar inactive (e.g. focus shifted to dialog)
	mb.Active = false

	// 4. Trigger render cycle cleanup
	fm.cleanupOrphanedMenus()

	// 5. Assert: Submenu should be removed even though it was NOT the top frame
	for _, f := range fm.frames {
		if f == subMenu {
			t.Error("Submenu still exists in FrameManager after MenuBar became inactive")
		}
	}
	if fm.GetTopFrameType() != TypeDialog {
		t.Error("The top-most dialog was accidentally removed during cleanup")
	}
	if len(fm.frames) != 2 { // Should be [Desktop, Dialog]
		t.Errorf("Expected 2 frames after cleanup, got %d", len(fm.frames))
	}
}

func TestVMenu_NavigationCallbacks(t *testing.T) {
	// With the new architecture, VMenu doesn't have OnLeft/OnRight.
	// It sends global CmMenuLeft/CmMenuRight commands.
	// This test is now obsolete in its current form, but let's check
	// that Esc closes the menu properly as that is still a "navigation" case.
	m := NewVMenu("Test")
	m.AddItem(MenuItem{Text: "Item"})

	m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})

	if !m.IsDone() {
		t.Error("VMenu should be closed on Escape")
	}
}

func TestVMenu_ShortcutRendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(30, 10)

	m := NewVMenu("Shortcuts")
	// Item: "Copy", Shortcut: "F5". Total width: 15 (0..14)
	m.AddItem(MenuItem{Text: "Copy", Shortcut: "F5"})
	m.SetPosition(0, 0, 14, 3) // Interior width: 13. Padding: 13 - 5 (text) - 3 (shortcut) = 5
	m.Show(scr)

	// Interior string: " Copy" (5) + "     " (5) + "F5 " (3) = 13 chars
	// Written to X=1..13. 
	// 'F' is at index 10 of the string -> X = 11
	// '5' is at index 11 of the string -> X = 12
	// Attributes: item is selected by default, so ColMenuSelectedText.
	attr := Palette[ColMenuSelectedText]
	checkCell(t, scr, 11, 1, 'F', attr)
	checkCell(t, scr, 12, 1, '5', attr)
}

func TestMenuBar_DynamicSubMenuWidth(t *testing.T) {
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	oldFm := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFm }()

	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{
			Label: "Test",
			SubItems: []MenuItem{
				{Text: "Short"},
				{Text: "This is a very long label", Shortcut: "Alt+F10"},
			},
		},
	}

	mb.ActivateSubMenu(0)
	
	if fm.GetTopFrameType() != TypeMenu {
		t.Fatal("Submenu not activated")
	}

	menu := fm.frames[len(fm.frames)-1].(*VMenu)
	x1, _, x2, _ := menu.GetPosition()
	width := x2 - x1 + 1

	// " This is a very long label" (26) + "Alt+F10 " (8) + 4 padding = 38
	// The logic should result in a width significantly larger than default 24
	if width < 30 {
		t.Errorf("Submenu width was not calculated dynamically. Expected >30, got %d", width)
	}
}

func TestVMenu_ActionOrdering_Integration(t *testing.T) {
	// Integration test for the "disappearing menu action" bug.
	// Scenario: VMenu is a child of MenuBar.
	// Action: FireAction (must bubble to PanelsFrame) then OnAction (must deactivate MenuBar).
	
	panelsFrame := &mockOwner{}
	mb := NewMenuBar([]string{"&File"})
	mb.SetOwner(panelsFrame)
	mb.Active = true
	
	vm := NewVMenu("Sub")
	vm.SetOwner(mb)
	vm.AddItem(MenuItem{Text: "Test", Command: 123})
	vm.SetSelectPos(0)
	
	// MenuBar's activation logic usually sets this up:
	vm.OnAction = func(idx int) { mb.Active = false }
	
	// Simulate Enter
	vm.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, 
		KeyDown: true, 
		VirtualKeyCode: vtinput.VK_RETURN,
	})
	
	if !panelsFrame.commandHandled {
		t.Error("Command did not reach PanelsFrame because MenuBar was deactivated too early")
	}
	if mb.Active {
		t.Error("MenuBar should be inactive after item selection")
	}
}
func TestMenuBar_DisabledItemDimming(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 5)
	FrameManager.Init(scr)

	mb := NewMenuBar(nil)
	mb.Items = []MenuBarItem{
		{
			Label: "&Active",
			SubItems: []MenuItem{{Text: "Cmd", Command: 100}},
		},
		{
			Label: "&Disabled",
			SubItems: []MenuItem{{Text: "Cmd", Command: 101}},
		},
	}
	mb.SetPosition(0, 0, 79, 0)

	// Disable command 101
	FrameManager.DisabledCommands.Disable(101)

	mb.Show(scr)

	// Get X of "&Disabled"
	x1 := mb.GetItemX(1)
	cell := scr.GetCell(x1+2, 0) // Skip internal padding and space

	// Should be dimmed (compared to active item)
	activeX := mb.GetItemX(0)
	activeCell := scr.GetCell(activeX+2, 0)

	if cell.Attributes == activeCell.Attributes {
		t.Error("Disabled MenuBar item should have different attributes (dimmed)")
	}
}

func TestMenuBar_ActiveHotkeys(t *testing.T) {
	SetDefaultPalette()
	fm := FrameManager
	fm.Init(NewSilentScreenBuf())
	
	mb := NewMenuBar([]string{"&Files", "&Options"})
	mb.Active = true
	mb.SelectPos = 0 // On Files

	// When active, pressing 'o' should switch to Options even without Alt
	mb.ProcessKey(&vtinput.InputEvent{
		Type:    vtinput.KeyEventType,
		KeyDown: true,
		Char:    'o',
	})

	if mb.SelectPos != 1 {
		t.Errorf("Expected SelectPos 1 (Options) after hotkey, got %d", mb.SelectPos)
	}
}
func TestMenuBar_AltHotkey_Deep(t *testing.T) {
	mb := NewMenuBar([]string{"&File", "&Edit", "&Search"})
	mb.Active = false

	// Test activation via Alt+S (Search)
	mb.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            's',
		ControlKeyState: vtinput.LeftAltPressed,
	})

	if !mb.Active {
		t.Error("MenuBar should be active after Alt+S")
	}
	if mb.SelectPos != 2 {
		t.Errorf("Expected Search (2) to be selected, got %d", mb.SelectPos)
	}

	// Test activation via Alt+E (Edit)
	mb.Active = false
	mb.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		Char:            'e',
		ControlKeyState: vtinput.LeftAltPressed,
	})
	if mb.SelectPos != 1 {
		t.Errorf("Expected Edit (1) to be selected, got %d", mb.SelectPos)
	}
}
