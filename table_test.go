package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// mockRow реализация для тестов
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

	// Вниз
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if tbl.SelectPos != 1 {
		t.Errorf("Expected SelectPos 1, got %d", tbl.SelectPos)
	}

	// End
	tbl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})
	if tbl.SelectPos != 2 {
		t.Errorf("Expected SelectPos 2, got %d", tbl.SelectPos)
	}

	// Вверх
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
	SetDefaultPalette() // Обязательно инициализируем цвета перед рендерингом

	scr := NewScreenBuf()
	scr.AllocBuf(15, 5)

	cols := []TableColumn{
		{Title: "C1", Width: 4, Alignment: AlignLeft},
		{Title: "C2", Width: 4, Alignment: AlignRight},
	}
	// Ширина таблицы = 4 (кол1) + 1 (разделитель) + 4 (кол2) = 9
	tbl := NewTable(0, 0, 9, 3, cols)
	tbl.SetRows([]TableRow{mockRow{"A", "B"}})

	// Focus table to trigger ColPanelCursor instead of ColPanelText
	tbl.SetFocus(true)
	tbl.Show(scr)

	// Проверяем шапку (заголовок первой колонки)
	checkCell(t, scr, 0, 0, 'C', Palette[ColPanelColumnTitle])
	checkCell(t, scr, 1, 0, '1', Palette[ColPanelColumnTitle])

	// Проверяем разделитель в шапке
	checkCell(t, scr, 4, 0, boxSymbols[bsV], Palette[ColPanelBox])

	// Проверяем первую строку данных
	// Колонка 1 (Left aligned): "A   "
	checkCell(t, scr, 0, 1, 'A', Palette[ColPanelCursor]) // Выделено по умолчанию
	checkCell(t, scr, 1, 1, ' ', Palette[ColPanelCursor]) // Padding

	// Разделитель в данных
	checkCell(t, scr, 4, 1, boxSymbols[bsV], Palette[ColPanelBox])

	// Колонка 2 (Right aligned): "   B"
	checkCell(t, scr, 5, 1, ' ', Palette[ColPanelCursor]) // Padding
	checkCell(t, scr, 8, 1, 'B', Palette[ColPanelCursor])
}