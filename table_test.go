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

func TestTable_Rendering(t *testing.T) {
	SetDefaultPalette() // Must initialize colors before rendering

	scr := NewScreenBuf()
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