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

	scr := NewScreenBuf()
	scr.AllocBuf(10, 5)
	tbl.Show(scr)

	// Без заголовка первая строка данных должна быть в Y=0
	checkCell(t, scr, 0, 0, 'R', Palette[ColTableText])
}
func TestTable_OptionalScrollBar(t *testing.T) {
	cols := []TableColumn{{Title: "Col", Width: 10}}
	rows := make([]TableRow, 20)
	for i := range rows {
		rows[i] = mockRow{"a", "b"}
	}

	t.Run("ScrollBar Off (Default)", func(t *testing.T) {
		scr := NewScreenBuf()
		scr.AllocBuf(12, 5)
		tbl := NewTable(0, 0, 11, 5, cols)
		tbl.SetRows(rows)
		tbl.Show(scr)

		// X=10 (last column) should be part of the table content, not a scrollbar
		checkCell(t, scr, 10, 2, ' ', Palette[ColTableText]) // Check a data row
	})

	t.Run("ScrollBar On", func(t *testing.T) {
		scr := NewScreenBuf()
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
	// "Ф" - одна руна, но два байта в UTF-8
	clean, hk, pos := ParseAmpersandString("Сохранить &файл")
	if clean != "Сохранить файл" {
		t.Errorf("Clean string mismatch: got %q", clean)
	}
	if hk != 'ф' {
		t.Errorf("Hotkey mismatch: got %c", hk)
	}
	if pos != 10 { // "Сохранить " (10 рун)
		t.Errorf("Hotkey pos mismatch: got %d", pos)
	}
}