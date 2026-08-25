package vtui

import (
	"strconv"
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
	if col == 0 {
		return m.col1
	}
	return m.col2
}

func (m mockMultiColSelectableRow) IsColSelected(col int) bool {
	if col >= 0 && col < 2 {
		return m.selected[col]
	}
	return false
}

type mockColorableRow struct {
	col1 string
	attr uint64
}

func (m mockColorableRow) GetCellText(col int) string {
	return m.col1
}

func (m mockColorableRow) GetCellAttr(col int, defaultAttr uint64) uint64 {
	if col == 0 {
		return m.attr
	}
	return defaultAttr
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

func TestTable_CellSelection_EmptyClick(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 5)

	cols := []TableColumn{
		{Title: "C1", Width: 5, Alignment: AlignLeft},
		{Title: "C2", Width: 5, Alignment: AlignLeft},
	}
	tbl := NewTable(0, 0, 11, 3, cols)
	tbl.CellSelection = true

	// Set 1 row
	row1 := mockMultiColSelectableRow{"L1", "R1", [2]bool{false, false}}
	tbl.SetRows([]TableRow{row1})

	// Click on empty row (row 2), col 1 (X=6)
	// Y=0 is header, Y=1 is row 0, Y=2 is row 1 (empty)
	handled := tbl.ProcessMouse(&vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      6,
		MouseY:      2,
	})

	if handled {
		t.Error("Click on empty space should not be handled")
	}
	if tbl.SelectCol != 0 {
		t.Errorf("SelectCol should have been reverted to 0, got %d", tbl.SelectCol)
	}
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
func TestTable_CellColorableRow(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)

	cols := []TableColumn{{Title: "C1", Width: 10, Alignment: AlignLeft}}
	tbl := NewTable(0, 0, 10, 3, cols)

	customAttr := uint64(999)
	row1 := mockColorableRow{"Colored", customAttr}
	row2 := mockRow{"Normal", ""}

	tbl.SetRows([]TableRow{row1, row2})
	tbl.SetFocus(false)
	tbl.SelectPos = -1 // Disable cursor interference
	tbl.Show(scr)

	// First row should have custom color
	checkCell(t, scr, 0, 1, 'C', customAttr)

	// Second row should have default color
	checkCell(t, scr, 0, 2, 'N', Palette[ColTableText])
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
	tbl.SelectPos = 5

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

func TestTable_AlwaysShowCursor(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 9, 3, cols)
	tbl.SetRows([]TableRow{mockRow{"R1", ""}})

	tbl.SetFocus(false)

	// Без флага курсор исчезает (использует обычный текст)
	tbl.Show(scr)
	checkCell(t, scr, 0, 1, 'R', Palette[ColTableText])

	// С флагом курсор остается (использует цвет выделенного текста)
	tbl.AlwaysShowCursor = true
	tbl.Show(scr)
	checkCell(t, scr, 0, 1, 'R', Palette[ColTableSelectedText])
}
func TestTable_FormatCell(t *testing.T) {
	tbl := &Table{}

	// 1. Left Align
	res := tbl.formatCell("ABC", 5, AlignLeft)
	if res != "ABC  " {
		t.Errorf("Left align failed: %q", res)
	}

	// 2. Right Align
	res = tbl.formatCell("ABC", 5, AlignRight)
	if res != "  ABC" {
		t.Errorf("Right align failed: %q", res)
	}

	// 3. Center Align
	res = tbl.formatCell("A", 3, AlignCenter)
	if res != " A " {
		t.Errorf("Center align (odd) failed: %q", res)
	}
	res = tbl.formatCell("A", 4, AlignCenter)
	if res != " A  " {
		t.Errorf("Center align (even) failed: %q", res)
	}

	// 4. Truncation
	res = tbl.formatCell("LongString", 4, AlignLeft)
	if res != "Long" {
		t.Errorf("Truncation failed: %q", res)
	}
}

func TestTable_ResolvedWidths_Flexible(t *testing.T) {
	SetDefaultPalette()

	// Width 20, no scrollbar: content width is 20.
	// Fixed col (5) + 2 separators = 7, leaving 13 for two flex columns: 7 + 6.
	cols := []TableColumn{
		{Title: "Fixed", Width: 5},
		{Title: "Flex1", Width: 0},
		{Title: "Flex2", Width: 0},
	}
	tbl := NewTable(0, 0, 20, 3, cols)

	widths := tbl.resolvedWidths()
	want := []int{5, 7, 6}
	if len(widths) != len(want) {
		t.Fatalf("expected %d widths, got %d", len(want), len(widths))
	}
	for i := range want {
		if widths[i] != want[i] {
			t.Errorf("col %d: expected width %d, got %d", i, want[i], widths[i])
		}
	}

	// Columns must fill the whole content width (widths + separators).
	total := 0
	for _, w := range widths {
		total += w
	}
	total += len(widths) - 1
	if total != tbl.GetContentWidth() {
		t.Errorf("columns do not fill content width: %d != %d", total, tbl.GetContentWidth())
	}
}

func TestTable_ResolvedWidths_Resize(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{
		{Title: "Fixed", Width: 4},
		{Title: "Flex", Width: 0},
	}
	tbl := NewTable(0, 0, 10, 3, cols)

	// content=10: flex = 10 - 4 - 1 = 5
	if w := tbl.resolvedWidths()[1]; w != 5 {
		t.Errorf("before resize: expected flex width 5, got %d", w)
	}

	// Grow the widget: flexible column must follow.
	tbl.SetPosition(0, 0, 19, 2)
	if w := tbl.resolvedWidths()[1]; w != 15 {
		t.Errorf("after resize: expected flex width 15, got %d", w)
	}
}

func TestTable_ResolvedWidths_AllFixed(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{
		{Title: "A", Width: 5},
		{Title: "B", Width: 6},
	}
	tbl := NewTable(0, 0, 30, 3, cols)

	widths := tbl.resolvedWidths()
	if widths[0] != 5 || widths[1] != 6 {
		t.Errorf("fixed widths must be preserved, got %v", widths)
	}
}

func TestTable_ResolvedWidths_TooNarrow(t *testing.T) {
	SetDefaultPalette()

	// No room left after the fixed column: each flex column gets at least its
	// minimum width (here: the title width, 5 for both "Flex1" and "Flex2").
	cols := []TableColumn{
		{Title: "Fixed", Width: 10},
		{Title: "Flex1", Width: 0},
		{Title: "Flex2", Width: 0},
	}
	tbl := NewTable(0, 0, 10, 3, cols)

	widths := tbl.resolvedWidths()
	if widths[1] != 5 || widths[2] != 5 {
		t.Errorf("flex columns must be clamped to their minimum width, got %v", widths)
	}
}

func TestTable_ResolvedWidths_MinWidth(t *testing.T) {
	SetDefaultPalette()

	// Explicit MinWidth wins over the title width: "AB" is 2 cells wide,
	// but the column must not shrink below 8.
	cols := []TableColumn{
		{Title: "AB", Width: 0, MinWidth: 8},
	}
	tbl := NewTable(0, 0, 5, 3, cols)

	if w := tbl.resolvedWidths()[0]; w != 8 {
		t.Errorf("expected MinWidth 8 to be respected, got %d", w)
	}
}

func TestTable_ResolvedWidths_TitleAsMinWidth(t *testing.T) {
	SetDefaultPalette()

	// Without MinWidth the title width is the minimum: "LongTitle" (9 cells)
	// must not be cut off even though the table is only 5 wide.
	cols := []TableColumn{
		{Title: "LongTitle", Width: 0},
	}
	tbl := NewTable(0, 0, 5, 3, cols)

	if w := tbl.resolvedWidths()[0]; w != 9 {
		t.Errorf("expected title width 9 as minimum, got %d", w)
	}

	// The remaining space is distributed on top of the minimums, not evenly:
	// two flex columns with different title lengths get different widths.
	cols = []TableColumn{
		{Title: "AA", Width: 0},     // min 2
		{Title: "BBBBBB", Width: 0}, // min 6
	}
	tbl = NewTable(0, 0, 21, 3, cols)
	// content=21, separator=1: avail=20, mins 2+6=8, extra 12 -> +6 each.
	widths := tbl.resolvedWidths()
	if widths[0] != 8 || widths[1] != 12 {
		t.Errorf("expected widths [8 12], got %v", widths)
	}
}

func TestTable_FlexibleColumnRendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 3)

	// A single right-aligned flexible column must stretch to the full width:
	// its header title ends at the right edge of the table.
	cols := []TableColumn{{Title: "H", Width: 0, Alignment: AlignRight}}
	tbl := NewTable(0, 0, 10, 3, cols)
	tbl.SetRows([]TableRow{mockRow{col1: "X"}})
	tbl.Show(scr)

	checkCell(t, scr, 9, 0, 'H', Palette[ColTableColumnTitle])
	checkCell(t, scr, 9, 1, 'X', Palette[ColTableText])
}

func TestTable_SortDefaultOff(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(11, 4)

	cols := []TableColumn{
		{Title: "C1", Width: 5},
		{Title: "C2", Width: 5},
	}
	tbl := NewTable(0, 0, 11, 4, cols)
	tbl.SetRows([]TableRow{mockRow{"B", "2"}, mockRow{"A", "10"}})

	if tbl.SortColumn != -1 {
		t.Errorf("sorting must be off by default, SortColumn = %d", tbl.SortColumn)
	}
	if tbl.RowAt(0) != 0 || tbl.RowAt(1) != 1 {
		t.Error("row order must be identity by default")
	}

	// Header shows plain titles, no sort arrows.
	tbl.Show(scr)
	checkCell(t, scr, 4, 0, ' ', Palette[ColTableColumnTitle])
	checkCell(t, scr, 10, 0, ' ', Palette[ColTableColumnTitle])
}

func TestTable_SetSort(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(11, 5)

	cols := []TableColumn{
		{Title: "C1", Width: 5},
		{Title: "C2", Width: 5},
	}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.SetRows([]TableRow{mockRow{"B", "2"}, mockRow{"C", "1"}, mockRow{"A", "10"}})

	// Ascending by col 0: A, B, C.
	tbl.SetSort(0, true)
	if tbl.RowAt(0) != 2 || tbl.RowAt(1) != 0 || tbl.RowAt(2) != 1 {
		t.Errorf("ascending sort failed: %d %d %d", tbl.RowAt(0), tbl.RowAt(1), tbl.RowAt(2))
	}
	tbl.Show(scr)
	checkCell(t, scr, 0, 1, 'A', Palette[ColTableText])
	checkCell(t, scr, 0, 2, 'B', Palette[ColTableText])
	checkCell(t, scr, 0, 3, 'C', Palette[ColTableText])

	// The sorted column header shows the ascending arrow at its right edge.
	checkCell(t, scr, 4, 0, '↑', Palette[ColTableColumnTitle])

	// Descending: C, B, A.
	tbl.SetSort(0, false)
	tbl.Show(scr)
	checkCell(t, scr, 0, 1, 'C', Palette[ColTableText])
	checkCell(t, scr, 4, 0, '↓', Palette[ColTableColumnTitle])

	// The Rows slice itself must not be reordered.
	if tbl.Rows[0].(mockRow).col1 != "B" {
		t.Error("Rows slice must keep its original order")
	}

	// ClearSort restores the original order.
	tbl.ClearSort()
	if tbl.RowAt(0) != 0 || tbl.RowAt(1) != 1 || tbl.RowAt(2) != 2 {
		t.Error("ClearSort must restore the original row order")
	}
}

func TestTable_SortCompareCustom(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 5}}
	tbl := NewTable(0, 0, 6, 5, cols)
	// Default string comparison would order "10" before "2"; a custom
	// comparator fixes numeric ordering.
	tbl.SortCompare = func(a, b TableRow, col int) int {
		an, _ := strconv.Atoi(a.GetCellText(col))
		bn, _ := strconv.Atoi(b.GetCellText(col))
		return an - bn
	}
	tbl.SetRows([]TableRow{mockRow{"10", ""}, mockRow{"2", ""}, mockRow{"1", ""}})
	tbl.SetSort(0, true)

	if tbl.RowAt(0) != 2 || tbl.RowAt(1) != 1 || tbl.RowAt(2) != 0 {
		t.Errorf("custom comparator failed: %d %d %d", tbl.RowAt(0), tbl.RowAt(1), tbl.RowAt(2))
	}
}

func TestTable_HeaderClickSort(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{
		{Title: "C1", Width: 5},
		{Title: "C2", Width: 5},
	}
	tbl := NewTable(0, 0, 11, 4, cols)
	tbl.Sortable = true
	tbl.SetRows([]TableRow{mockRow{"B", "2"}, mockRow{"A", "1"}})

	click := func(x, y int) bool {
		return tbl.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true,
			ButtonState: vtinput.FromLeft1stButtonPressed,
			MouseX:      int16(x), MouseY: int16(y),
		})
	}

	// Click on the second column header (x in 6..10, y = 0): sort ascending.
	if !click(8, 0) {
		t.Fatal("header click must be handled")
	}
	if tbl.SortColumn != 1 || !tbl.SortAscending {
		t.Errorf("expected sort by col 1 ascending, got col=%d asc=%v", tbl.SortColumn, tbl.SortAscending)
	}

	// Second click on the same header toggles the direction.
	click(8, 0)
	if tbl.SortColumn != 1 || tbl.SortAscending {
		t.Errorf("expected descending toggle, got col=%d asc=%v", tbl.SortColumn, tbl.SortAscending)
	}

	// Click on another column starts ascending sort by it.
	click(2, 0)
	if tbl.SortColumn != 0 || !tbl.SortAscending {
		t.Errorf("expected sort by col 0 ascending, got col=%d asc=%v", tbl.SortColumn, tbl.SortAscending)
	}

	// Click on the separator (x = 5) is consumed but does not change the sort.
	if !click(5, 0) {
		t.Fatal("separator click must be consumed")
	}
	if tbl.SortColumn != 0 || !tbl.SortAscending {
		t.Error("separator click must not change the sort")
	}
}

func TestTable_HeaderClickSortDisabled(t *testing.T) {
	SetDefaultPalette()

	// f4-style manual sorting: the app rewrites titles itself and must not
	// be affected by the built-in header-click sorting (Sortable = false).
	cols := []TableColumn{{Title: "Name ↑", Width: 8}}
	tbl := NewTable(0, 0, 9, 4, cols)
	tbl.SetRows([]TableRow{mockRow{"B", ""}, mockRow{"A", ""}})

	handled := tbl.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      2, MouseY: 0,
	})
	if tbl.SortColumn != -1 {
		t.Error("header click must not sort when Sortable is false")
	}
	if tbl.Columns[0].Title != "Name ↑" {
		t.Error("column title must not be modified by the table")
	}
	if tbl.RowAt(0) != 0 {
		t.Error("row order must not change when Sortable is false")
	}
	_ = handled
}

// typeSearch feeds printable characters into the table as key events.
func typeSearch(tbl *Table, s string) {
	for _, r := range s {
		tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r})
	}
}

func TestTable_QuickSearchFilter(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{
		mockRow{"zzneedle", ""},
		mockRow{"needle zz", ""},
		mockRow{"x needle", ""},
		mockRow{"unrelated", ""},
	})

	typeSearch(tbl, "needle")

	if tbl.ItemCount != 3 {
		t.Fatalf("expected 3 matching rows, got %d", tbl.ItemCount)
	}
	// Ranked by (score, position): all exact, positions 2, 0, 2.
	// "needle zz" (pos 0) first; ties keep the original order.
	got := []string{
		tbl.Rows[tbl.RowAt(0)].(mockRow).col1,
		tbl.Rows[tbl.RowAt(1)].(mockRow).col1,
		tbl.Rows[tbl.RowAt(2)].(mockRow).col1,
	}
	want := []string{"needle zz", "zzneedle", "x needle"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}

func TestTable_QuickSearchFuzzyRanking(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{
		mockRow{"xxabd", ""}, // 1 error from "abc"
		mockRow{"xxabc", ""}, // exact
		mockRow{"xyz", ""},   // too far, must be dropped
	})

	typeSearch(tbl, "abc") // k = 1

	if tbl.ItemCount != 2 {
		t.Fatalf("expected 2 rows, got %d", tbl.ItemCount)
	}
	if got := tbl.Rows[tbl.RowAt(0)].(mockRow).col1; got != "xxabc" {
		t.Errorf("exact match must rank first, got %q", got)
	}
	if got := tbl.Rows[tbl.RowAt(1)].(mockRow).col1; got != "xxabd" {
		t.Errorf("fuzzy match must rank second, got %q", got)
	}
}

func TestTable_QuickSearchExactOnHit(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "Key", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SearchExactOnHit = true
	tbl.SetRows([]TableRow{
		mockRow{"CtrlUp", ""},
		mockRow{"CtrlA", ""},
		mockRow{"CtrlAltA", ""},
		mockRow{"CtrlPgUp", ""},
	})

	typeSearch(tbl, "ctrla")
	if tbl.ItemCount != 1 {
		t.Fatalf("expected only the exact key match, got %d rows", tbl.ItemCount)
	}
	if got := tbl.Rows[tbl.RowAt(0)].(mockRow).col1; got != "CtrlA" {
		t.Fatalf("exact match = %q, want CtrlA", got)
	}

	tbl.SetSearchText("ctrlx")
	if tbl.ItemCount == 0 {
		t.Fatal("fuzzy matches should remain available when no exact result exists")
	}
}

func TestTable_QuickSearchExactRanksFirst(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{
		mockRow{"файлы", ""}, // exact substring at 0, but longer than the needle
		mockRow{"файл", ""},  // full-cell equality, must win despite the position
	})

	typeSearch(tbl, "файл") // non-ASCII: guards against rune/byte unit mixups

	if tbl.ItemCount != 2 {
		t.Fatalf("expected 2 rows, got %d", tbl.ItemCount)
	}
	if got := tbl.Rows[tbl.RowAt(0)].(mockRow).col1; got != "файл" {
		t.Errorf("full-cell exact match must rank first, got %q", got)
	}
}

func TestTable_QuickSearchExactOnHitNonASCII(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SearchExactOnHit = true
	tbl.SetRows([]TableRow{
		mockRow{"файлы", ""},
		mockRow{"файл", ""},
	})

	typeSearch(tbl, "файл")
	if tbl.ItemCount != 1 {
		t.Fatalf("expected only the exact match, got %d rows", tbl.ItemCount)
	}
	if got := tbl.Rows[tbl.RowAt(0)].(mockRow).col1; got != "файл" {
		t.Fatalf("exact match = %q, want файл", got)
	}
}

func TestTable_QuickSearchExactInAnyColumn(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}, {Title: "C2", Width: 10}}
	tbl := NewTable(0, 0, 21, 5, cols)
	tbl.QuickSearch = true
	tbl.SearchExactOnHit = true
	tbl.SetRows([]TableRow{
		mockRow{"abc", "zzz"},  // exact in column 0, no match in the last column
		mockRow{"xabc", "zzz"}, // substring only
	})

	typeSearch(tbl, "abc")

	if tbl.ItemCount != 1 {
		t.Fatalf("expected only the row with an exact cell, got %d", tbl.ItemCount)
	}
	if got := tbl.Rows[tbl.RowAt(0)].(mockRow).col1; got != "abc" {
		t.Fatalf("kept row = %q, want abc", got)
	}
}

func TestTable_QuickSearchCaseInsensitive(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{mockRow{"ReadMe.txt", ""}})

	typeSearch(tbl, "readme")
	if tbl.ItemCount != 1 {
		t.Error("case-insensitive search must match ReadMe.txt")
	}

	tbl.SearchCaseSensitive = true
	tbl.SetSearchText("README") // 3 case mismatches > k=2, must not fuzzy-match
	if tbl.ItemCount != 0 {
		t.Error("case-sensitive search must not match")
	}
}

func TestTable_QuickSearchAllColumns(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{
		{Title: "C1", Width: 5},
		{Title: "C2", Width: 5},
	}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{
		mockRow{"foo", "bar"},
		mockRow{"baz", "qux"},
	})

	typeSearch(tbl, "qux") // only in the second column
	if tbl.ItemCount != 1 || tbl.Rows[tbl.RowAt(0)].(mockRow).col1 != "baz" {
		t.Error("search must span all columns")
	}
}

func TestTable_QuickSearchEditing(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{mockRow{"abc", ""}, mockRow{"xyz", ""}})

	key := func(vk uint16) {
		tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk})
	}

	typeSearch(tbl, "ac") // no exact match, but within k=0? "ac" vs "abc": 1 error, k=0 -> no rows
	if tbl.ItemCount != 0 {
		t.Fatalf("expected 0 rows for k=0, got %d", tbl.ItemCount)
	}

	// Move cursor left and insert 'b' in the middle: "ac" -> "abc".
	key(vtinput.VK_LEFT)
	typeSearch(tbl, "b")
	if tbl.SearchText() != "abc" {
		t.Errorf("expected search text %q, got %q", "abc", tbl.SearchText())
	}
	if tbl.ItemCount != 1 {
		t.Errorf("expected 1 row after inserting 'b', got %d", tbl.ItemCount)
	}

	// Backspace removes the char before the cursor: "abc" (cursor at 2) -> "ac".
	key(vtinput.VK_BACK)
	if tbl.SearchText() != "ac" {
		t.Errorf("backspace failed: %q", tbl.SearchText())
	}

	// Delete at cursor 1 of "a|c"... wait: after backspace cursor is at 1: "a|c".
	// Delete removes 'c' -> "a".
	key(vtinput.VK_DELETE)
	if tbl.SearchText() != "a" {
		t.Errorf("delete failed: %q", tbl.SearchText())
	}

	// Esc clears the search and restores all rows.
	key(vtinput.VK_ESCAPE)
	if tbl.SearchText() != "" || tbl.ItemCount != 2 {
		t.Errorf("Esc must clear search: text=%q count=%d", tbl.SearchText(), tbl.ItemCount)
	}

	// Esc on an empty search string is not consumed.
	handled := tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if handled {
		t.Error("Esc with empty search must propagate")
	}
}

func TestTable_QuickSearchCellSelectionConflict(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{
		{Title: "C1", Width: 5},
		{Title: "C2", Width: 5},
	}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.CellSelection = true
	tbl.SetRows([]TableRow{mockRow{"ab", "cd"}})

	typeSearch(tbl, "ab")

	// Plain Left: CellSelection wins, the search cursor stays.
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT})
	if tbl.searchCursor != 2 {
		t.Errorf("plain Left must not move the search cursor under CellSelection, cursor=%d", tbl.searchCursor)
	}

	// Ctrl+Left moves the search cursor.
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.LeftCtrlPressed})
	if tbl.searchCursor != 1 {
		t.Errorf("Ctrl+Left must move the search cursor, cursor=%d", tbl.searchCursor)
	}
}

func TestTable_QuickSearchLineRendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(11, 5)

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{mockRow{"abc", ""}, mockRow{"abd", ""}, mockRow{"xyz", ""}, mockRow{"aaa", ""}})

	typeSearch(tbl, "ab")
	tbl.Show(scr)

	// The search line takes one row from the data area.
	if tbl.ViewHeight != 3 {
		t.Errorf("expected ViewHeight 3 (5 - header - search line), got %d", tbl.ViewHeight)
	}

	// The search line sits at the very top, above the header.
	checkCell(t, scr, 0, 0, '>', Palette[ColTableText])
	checkCell(t, scr, 2, 0, 'a', Palette[ColTableText])
	checkCell(t, scr, 3, 0, 'b', Palette[ColTableText])
}

func TestTable_QuickSearchDisabledByDefault(t *testing.T) {
	SetDefaultPalette()

	// f4 compatibility: without QuickSearch, typing must not filter anything.
	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.SetRows([]TableRow{mockRow{"abc", ""}, mockRow{"xyz", ""}})

	typeSearch(tbl, "abc")
	if tbl.ItemCount != 2 || tbl.SearchText() != "" {
		t.Error("typing must not filter when QuickSearch is off")
	}
	if tbl.ViewHeight != 4 {
		t.Errorf("search line must not reserve space when QuickSearch is off, ViewHeight=%d", tbl.ViewHeight)
	}
}

func TestTable_QuickSearchOverridesColumnSort(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.Sortable = true
	tbl.SetRows([]TableRow{mockRow{"bx", ""}, mockRow{"ax", ""}, mockRow{"cx", ""}})

	// Column sort ascending: ax, bx, cx.
	tbl.SetSort(0, true)
	if tbl.Rows[tbl.RowAt(0)].(mockRow).col1 != "ax" {
		t.Fatal("column sort broken before search")
	}

	// While searching, match ranking wins over the column sort: "x bx" scores
	// lower... actually all contain "x" at pos 1; use a discriminating needle.
	tbl.SetSearchText("bx")
	if tbl.ItemCount != 1 || tbl.Rows[tbl.RowAt(0)].(mockRow).col1 != "bx" {
		t.Error("search filter must apply over the column sort")
	}

	// Clearing the search restores the column sort.
	tbl.ClearSearch()
	if tbl.ItemCount != 3 || tbl.Rows[tbl.RowAt(0)].(mockRow).col1 != "ax" {
		t.Error("ClearSearch must restore the column-sorted order")
	}
}

func TestTable_QuickSearchHighlight(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(11, 5)

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{mockRow{"xxabcxx", ""}})

	typeSearch(tbl, "abc")
	tbl.Show(scr)

	// Single match: ViewHeight=3, data rows y=2..4, the row at the top (y=2).
	base := Palette[ColTableText]
	hl := Palette[ColMenuHighlight]
	checkCell(t, scr, 0, 2, 'x', base)
	checkCell(t, scr, 1, 2, 'x', base)
	checkCell(t, scr, 2, 2, 'a', hl)
	checkCell(t, scr, 3, 2, 'b', hl)
	checkCell(t, scr, 4, 2, 'c', hl)
	checkCell(t, scr, 5, 2, 'x', base)
}

func TestTable_QuickSearchHighlightFuzzySpan(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(11, 5)

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	// "abxc" matches needle "abc" with one insertion; the whole "abxc" span
	// must be highlighted, not just m=3 runes.
	tbl.SetRows([]TableRow{mockRow{"abxc", ""}})

	typeSearch(tbl, "abc")
	tbl.Show(scr)

	hl := Palette[ColMenuHighlight]
	checkCell(t, scr, 0, 2, 'a', hl)
	checkCell(t, scr, 1, 2, 'b', hl)
	checkCell(t, scr, 2, 2, 'x', hl)
	checkCell(t, scr, 3, 2, 'c', hl)
}

func TestTable_QuickSearchTopDown(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(11, 6)

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 6, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{
		mockRow{"xab", ""}, // pos 1
		mockRow{"abx", ""}, // pos 0 -> best
		mockRow{"zzz", ""}, // no match
	})

	typeSearch(tbl, "ab")
	tbl.Show(scr)

	// ViewHeight = 6 - search line - header = 4; data rows y=2..5.
	// Best match ("abx", displayPos 0) at the top, next one below it.
	base := Palette[ColTableText]
	hl := Palette[ColMenuHighlight]
	checkCell(t, scr, 0, 2, 'a', hl)   // "abx" at the top, needle highlighted
	checkCell(t, scr, 2, 2, 'x', base) // 'x' is outside the match span
	checkCell(t, scr, 0, 3, 'x', base) // "xab" below
	checkCell(t, scr, 0, 4, ' ', base) // empty space at the bottom
	checkCell(t, scr, 0, 5, ' ', base)
}

func TestTable_QuickSearchCursorFollowsFilteredPosition(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(11, 5)

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetFocus(true)
	tbl.SetRows([]TableRow{
		mockRow{"xab", ""}, // underlying row 0, second match
		mockRow{"abx", ""}, // underlying row 1, best match at display position 0
	})

	typeSearch(tbl, "ab")
	tbl.Show(scr)

	// Search results are drawn top-down. The selected display position 0 is
	// the underlying row 1, so the cursor must be on the top "abx" data row.
	checkCell(t, scr, 2, 2, 'x', Palette[ColTableSelectedText])
	checkCell(t, scr, 0, 3, 'x', Palette[ColTableText])
}

func TestTable_QuickSearchKeys(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{mockRow{"xab", ""}, mockRow{"abx", ""}})

	typeSearch(tbl, "ab")
	if tbl.SelectPos != 0 {
		t.Fatalf("cursor must start at the best match, got %d", tbl.SelectPos)
	}

	key := func(vk uint16) bool {
		return tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk})
	}

	// Down moves to the next (worse) match, Up moves back.
	if !key(vtinput.VK_DOWN) || tbl.SelectPos != 1 {
		t.Errorf("Down must move to the next match, SelectPos=%d", tbl.SelectPos)
	}
	if key(vtinput.VK_DOWN) {
		t.Error("Down past the last match must propagate")
	}
	if !key(vtinput.VK_UP) || tbl.SelectPos != 0 {
		t.Errorf("Up must move back towards the best match, SelectPos=%d", tbl.SelectPos)
	}
	if key(vtinput.VK_UP) {
		t.Error("Up above the best match must propagate")
	}
}

func TestTable_QuickSearchFocusFollowsBestMatch(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{mockRow{"xab", ""}, mockRow{"abx", ""}, mockRow{"abz", ""}})

	typeSearch(tbl, "ab")
	if tbl.SelectPos != 0 {
		t.Fatalf("cursor must start at the best match, got %d", tbl.SelectPos)
	}

	// Navigate away from the best match...
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if tbl.SelectPos != 1 {
		t.Fatalf("Down must move to the second match, got %d", tbl.SelectPos)
	}

	// ...typing another character must refocus the new best match.
	typeSearch(tbl, "x")
	if tbl.SelectPos != 0 {
		t.Errorf("typing must refocus the best match, got SelectPos=%d", tbl.SelectPos)
	}
	if got := tbl.Rows[tbl.RowAt(0)].(mockRow).col1; got != "abx" {
		t.Errorf("best match for %q = %q, want abx", tbl.SearchText(), got)
	}
}

func TestTable_QuickSearchMouse(t *testing.T) {
	SetDefaultPalette()

	cols := []TableColumn{{Title: "C1", Width: 10}}
	tbl := NewTable(0, 0, 11, 5, cols)
	tbl.QuickSearch = true
	tbl.SetRows([]TableRow{mockRow{"xab", ""}, mockRow{"abx", ""}})
	// DisplayObject normally re-syncs margins every frame; do it explicitly.
	tbl.SetPosition(tbl.X1, tbl.Y1, tbl.X2, tbl.Y2)

	typeSearch(tbl, "ab")

	click := func(y int) {
		tbl.ProcessMouse(&vtinput.InputEvent{
			Type: vtinput.MouseEventType, KeyDown: true,
			ButtonState: vtinput.FromLeft1stButtonPressed,
			MouseX:      2, MouseY: int16(y),
		})
	}

	// Data rows y=2..4 (ViewHeight 3); the best match is at the top (y=2).
	click(2)
	if tbl.SelectPos != 0 {
		t.Errorf("click on the top row must select the best match, got %d", tbl.SelectPos)
	}
	click(3)
	if tbl.SelectPos != 1 {
		t.Errorf("click one row below must select the second match, got %d", tbl.SelectPos)
	}
	// Click on the empty area below the results must not move the cursor.
	click(4)
	if tbl.SelectPos != 1 {
		t.Errorf("click on empty space must be ignored, got %d", tbl.SelectPos)
	}
}

// mockGridCellProvider is a TableCellProvider for a grid layout where two
// display columns each hold an independently selectable item on the same
// row (e.g. a two-column file panel). It implements both
// TableCellSelectProvider (row-only, always wrong here) and
// TableCellColSelectProvider (row+col, correct), to prove the table prefers
// the column-aware one when both are present.
type mockGridCellProvider struct {
	// items[col] is the selection state of the item shown in that column.
	items [2]bool
}

func (m mockGridCellProvider) RowCount() int                    { return 1 }
func (m mockGridCellProvider) GetCellText(row, col int) string  { return "X" }
func (m mockGridCellProvider) IsRowSelected(row int) bool       { return false }
func (m mockGridCellProvider) IsCellSelected(row, col int) bool { return m.items[col] }

func TestTable_ColSelectProviderPreferredOverRowSelectProvider(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 3)

	cols := []TableColumn{
		{Title: "L", Width: 5, Alignment: AlignLeft},
		{Title: "R", Width: 5, Alignment: AlignLeft},
	}
	tbl := NewTable(0, 0, 10, 2, cols)
	tbl.ColorTextIdx = ColTableText
	tbl.ColorItemSelectTextIdx = ColDialogHighlightText

	// Left column's item is selected, right column's is not.
	tbl.SetCellProvider(mockGridCellProvider{items: [2]bool{true, false}})
	tbl.SetRowCount(1)
	tbl.Show(scr)

	checkCell(t, scr, 0, 1, 'X', Palette[ColDialogHighlightText])
	checkCell(t, scr, 6, 1, 'X', Palette[ColTableText])
}
