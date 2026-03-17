package vtui

import "testing"

func TestVText_Rendering(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(5, 10)

	color := uint64(777)
	vt := NewVText(1, 1, "ABC", color)
	vt.Show(scr)

	// Проверяем вертикаль:
	// (1, 1) -> 'A'
	// (1, 2) -> 'B'
	// (1, 3) -> 'C'
	checkCell(t, scr, 1, 1, 'A', color)
	checkCell(t, scr, 1, 2, 'B', color)
	checkCell(t, scr, 1, 3, 'C', color)

	// Соседние ячейки должны быть пустыми
	checkCell(t, scr, 0, 1, 0, 0)
	checkCell(t, scr, 2, 1, 0, 0)
}