package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// mockRow implementation for tests
type mockRow struct {
	col1 string
	col2 string
}

func (m mockRow) GetCellText(col int) string {
	if col == 0 {
		return m.col1
	}
	return m.col2
}

type mockSelectableRow struct {
	col1     string
	selected bool
}

func (m mockSelectableRow) GetCellText(col int) string {
	return m.col1
}

func (m mockSelectableRow) IsSelected() bool {
	return m.selected
}
type mockMultiColSelectableRow struct {
	col1     string
	col2     string
	selected [2]bool
}

func (m mockMultiColSelectableRow) GetCellText(col int) string {
	if col == 0 { return m.col1 }
	return m.col2
}

func (m mockMultiColSelectableRow) IsColSelected(col int) bool {
	if col >= 0 && col < 2 { return m.selected[col] }
	return false
}

func TestTable_SelectableRowRendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(15, 5)

	cols := []TableColumn{{Title: "C1", Width: 10, Alignment: AlignLeft}}
	tbl := NewTable(0, 0, 10, 3, cols)
	tbl.ColorItemSelectTextIdx = ColDialogHighlightText
	tbl.ColorItemSelectCursorIdx = ColDialogHighlightSelectedButton

	row1 := mockSelectableRow{"Unsel", false}
	row2 := mockSelectableRow{"Sel", true}
	tbl.SetRows([]TableRow{row1, row2})

	tbl.SetFocus(true)
	tbl.SelectPos = 0
	tbl.Show(scr)

	// row1 (unselected, cursor) -> ColTableSelectedText
	checkCell(t, scr, 0, 1, 'U', Palette[ColTableSelectedText])

	// row2 (selected, no cursor) -> ColorItemSelectTextIdx
	checkCell(t, scr, 0, 2, 'S', Palette[ColDialogHighlightText])

	// Move cursor to row2
	tbl.SelectPos = 1
	tbl.Show(scr)

	// row2 (selected, cursor) -> ColorItemSelectCursorIdx
	checkCell(t, scr, 0, 2, 'S', Palette[ColDialogHighlightSelectedButton])
}
func TestTable_CellSelection(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 5)

	cols := []TableColumn{
		{Title: "C1", Width: 5, Alignment: AlignLeft},
		{Title: "C2", Width: 5, Alignment: AlignLeft},
	}
	tbl := NewTable(0, 0, 11, 3, cols)
	tbl.CellSelection = true
	tbl.ColorItemSelectTextIdx = ColDialogHighlightText
	tbl.ColorItemSelectCursorIdx = ColDialogHighlightSelectedButton

	row1 := mockMultiColSelectableRow{"L1", "R1", [2]bool{false, true}}
	row2 := mockMultiColSelectableRow{"L2", "R2", [2]bool{true, false}}
	tbl.SetRows([]TableRow{row1, row2})

	tbl.SetFocus(true)
	tbl.SelectPos = 0
	tbl.SelectCol = 0
	tbl.Show(scr)

	// row1 col1 (unselected, cursor) -> ColTableSelectedText
	checkCell(t, scr, 0, 1, 'L', Palette[ColTableSelectedText])
	// row1 col2 (selected, no cursor) -> ColorItemSelectTextIdx
	checkCell(t, scr, 6, 1, 'R', Palette[ColDialogHighlightText])

	// Navigate right
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})

	if tbl.SelectCol != 1 || tbl.SelectPos != 0 {
		t.Errorf("Right navigation failed: pos=%d, col=%d", tbl.SelectPos, tbl.SelectCol)
	}

	tbl.Show(scr)
	// row1 col2 (selected, cursor) -> ColorItemSelectCursorIdx
	checkCell(t, scr, 6, 1, 'R', Palette[ColDialogHighlightSelectedButton])

	// Navigate right across row boundary
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})

	if tbl.SelectCol != 0 || tbl.SelectPos != 1 {
		t.Errorf("Right wrapping navigation failed: pos=%d, col=%d", tbl.SelectPos, tbl.SelectCol)
	}

	// Navigate left across row boundary
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})

	if tbl.SelectCol != 1 || tbl.SelectPos != 0 {
		t.Errorf("Left wrapping navigation failed: pos=%d, col=%d", tbl.SelectPos, tbl.SelectCol)
	}
}
func TestTable_EmptyGeometry(t *testing.T) {
	// Ensure table can be sized before data is provided
	tbl := NewTable(0, 0, 10, 10, []TableColumn{{Title: "Test", Width: 5}})

	tbl.SetPosition(5, 5, 25, 15)

	x1, y1, x2, y2 := tbl.GetPosition()
	if x1 != 5 || y1 != 5 || x2 != 25 || y2 != 15 {
		t.Errorf("Table failed to update bounds when empty: got (%d,%d)-(%d,%d)", x1, y1, x2, y2)
	}
}

func TestTable_Navigation(t *testing.T) {
	cols := []TableColumn{
		{Title: "C1", Width: 5},
		{Title: "C2", Width: 5},
	}
	tbl := NewTable(0, 0, 15, 5, cols)

	rows := []TableRow{
		mockRow{"1", "A"},
		mockRow{"2", "B"},
		mockRow{"3", "C"},
	}
	tbl.SetRows(rows)

	if tbl.SelectPos != 0 {
		t.Errorf("Expected SelectPos 0, got %d", tbl.SelectPos)
	}

	// Down
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if tbl.SelectPos != 1 {
		t.Errorf("Expected SelectPos 1, got %d", tbl.SelectPos)
	}

	// End
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})
	if tbl.SelectPos != 2 {
		t.Errorf("Expected SelectPos 2, got %d", tbl.SelectPos)
	}

	// Up
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
	if tbl.SelectPos != 1 {
		t.Errorf("Expected SelectPos 1, got %d", tbl.SelectPos)
	}

	// Home
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	if tbl.SelectPos != 0 {
		t.Errorf("Expected SelectPos 0, got %d", tbl.SelectPos)
	}
}
func TestTable_BoundaryNavigation(t *testing.T) {
	cols := []TableColumn{{Title: "C", Width: 5}}
	tbl := NewTable(0, 0, 10, 5, cols)
	tbl.SetRows([]TableRow{mockRow{"1", ""}, mockRow{"2", ""}})

	// 1. Up at top -> false
	tbl.SetSelectPos(0)
	if tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) {
		t.Error("Table Up at row 0 should return false")
	}

	// 2. Down at bottom -> false
	tbl.SetSelectPos(1)
	if tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}) {
		t.Error("Table Down at last row should return false")
	}
}
func TestTable_PageNavigation(t *testing.T) {
	cols := []TableColumn{{Title: "Col", Width: 10}}
	tbl := NewTable(0, 0, 10, 5, cols) // Height 5, Header 1 -> Data Height 4

	rows := make([]TableRow, 20)
	for i := range rows {
		rows[i] = mockRow{"a", "b"}
	}
	tbl.SetRows(rows)

	// 1. PgDn from top
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if tbl.SelectPos != 4 {
		t.Errorf("PgDn failed: expected index 4, got %d", tbl.SelectPos)
	}

	// 2. PgDn again
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if tbl.SelectPos != 8 {
		t.Errorf("PgDn(2) failed: expected index 8, got %d", tbl.SelectPos)
	}

	// 3. PgUp
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
	if tbl.SelectPos != 4 {
		t.Errorf("PgUp failed: expected index 4, got %d", tbl.SelectPos)
	}

	// 4. Boundary check - PgUp at top
	tbl.SelectPos = 2
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
	if tbl.SelectPos != 0 {
		t.Errorf("PgUp boundary failed: expected 0, got %d", tbl.SelectPos)
	}

	// 5. Boundary check - PgDn at bottom
	tbl.SelectPos = 18
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if tbl.SelectPos != 19 {
		t.Errorf("PgDn boundary failed: expected 19, got %d", tbl.SelectPos)
	}
}

func TestTable_Rendering(t *testing.T) {
	SetDefaultPalette() // Must initialize colors before rendering

	scr := NewSilentScreenBuf()
	scr.AllocBuf(15, 5)

	cols := []TableColumn{
		{Title: "C1", Width: 4, Alignment: AlignLeft},
		{Title: "C2", Width: 4, Alignment: AlignRight},
	}
	// Table width = 4 (col1) + 1 (separator) + 4 (col2) = 9
	tbl := NewTable(0, 0, 9, 3, cols)
	tbl.SetRows([]TableRow{mockRow{"A", "B"}})

	// Focus table to trigger ColTableSelectedText instead of ColTableText
	tbl.SetFocus(true)
	tbl.Show(scr)

	// Check header (first column title)
	checkCell(t, scr, 0, 0, 'C', Palette[ColTableColumnTitle])
	checkCell(t, scr, 1, 0, '1', Palette[ColTableColumnTitle])

	// Check separator in header
	checkCell(t, scr, 4, 0, uint64(boxSymbols[bsV]), Palette[ColTableBox])

	// Check first data row
	// Column 1 (Left aligned): "A   "
	checkCell(t, scr, 0, 1, 'A', Palette[ColTableSelectedText]) // Selected by default
	checkCell(t, scr, 1, 1, ' ', Palette[ColTableSelectedText]) // Padding

	// Separator in data
	checkCell(t, scr, 4, 1, uint64(boxSymbols[bsV]), Palette[ColTableBox])

	// Column 2 (Right aligned): "   B"
	checkCell(t, scr, 5, 1, ' ', Palette[ColTableSelectedText]) // Padding
	checkCell(t, scr, 8, 1, 'B', Palette[ColTableSelectedText])
}

func TestTable_MouseWheel(t *testing.T) {
	cols := []TableColumn{{Title: "Col", Width: 10}}
	tbl := NewTable(0, 0, 10, 5, cols)

	var rows []TableRow
	for i := 0; i < 20; i++ {
		rows = append(rows, mockRow{"A", "B"})
	}
	tbl.SetRows(rows)

	tbl.TopPos = 5

	// 1. Scroll Down
	tbl.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	if tbl.TopPos != 6 {
		t.Errorf("Mouse wheel down failed, TopPos: %d", tbl.TopPos)
	}

	// 2. Scroll Up
	tbl.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1})
	if tbl.TopPos != 5 {
		t.Errorf("Mouse wheel up failed, TopPos: %d", tbl.TopPos)
	}
}
func TestTable_NoHeaderGeometry(t *testing.T) {
	SetDefaultPalette()
	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 10, 5, cols)
	tbl.ShowHeader = false
	tbl.SetRows([]TableRow{mockRow{"R1", "B"}, mockRow{"R2", "B"}})

	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	tbl.Show(scr)

	// Without a header, the first data row should be at Y=0
	checkCell(t, scr, 0, 0, 'R', Palette[ColTableText])
}
func TestTable_OptionalScrollBar(t *testing.T) {
	cols := []TableColumn{{Title: "Col", Width: 10}}
	rows := make([]TableRow, 20)
	for i := range rows {
		rows[i] = mockRow{"a", "b"}
	}

	t.Run("ScrollBar Off (Default)", func(t *testing.T) {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(12, 5)
		tbl := NewTable(0, 0, 11, 5, cols)
		tbl.SetRows(rows)
		tbl.Show(scr)

		// X=10 (last column) should be part of the table content, not a scrollbar
		checkCell(t, scr, 10, 2, ' ', Palette[ColTableText]) // Check a data row
	})

	t.Run("ScrollBar On", func(t *testing.T) {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(12, 5)
		tbl := NewTable(0, 0, 11, 5, cols)
		tbl.SetRows(rows)
		tbl.ShowScrollBar = true
		tbl.Show(scr)

		// X=10 (last column) should be a scrollbar arrow or track
		checkCell(t, scr, 10, 1, ScrollUpArrow, Palette[ColTableBox])
		checkCell(t, scr, 10, 2, ScrollBlockDark, Palette[ColTableBox])
	})

	t.Run("Mouse on ScrollBar without ShowScrollBar", func(t *testing.T) {
		tbl := NewTable(0, 0, 10, 5, cols)
		tbl.SetRows(rows)
		tbl.ShowScrollBar = false
		tbl.TopPos = 5

		tbl.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true, MouseX: 9, MouseY: 4, ButtonState: vtinput.FromLeft1stButtonPressed,
		})

		if tbl.SelectPos == 6 { // A click on the 'down arrow' area
			t.Error("Scrollbar click should be ignored when ShowScrollBar is false")
		}
	})
}

func TestParseAmpersandString_Unicode(t *testing.T) {
	// "Ф" - one rune, but two bytes in UTF-8
	clean, hk, pos := ParseAmpersandString("Сохранить &файл")
	if clean != "Сохранить файл" {
		t.Errorf("Clean string mismatch: got %q", clean)
	}
	if hk != 'ф' {
		t.Errorf("Hotkey mismatch: got %c", hk)
	}
	if pos != 10 { // "Сохранить " (10 runes)
		t.Errorf("Hotkey pos mismatch: got %d", pos)
	}
}