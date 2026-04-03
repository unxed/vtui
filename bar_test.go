package vtui

import "testing"

func TestBar_EnforceSingleLine(t *testing.T) {
	bar := &Bar{}
	// Пытаемся задать высоту в 5 строк (Y: 10-14)
	bar.SetPosition(0, 10, 80, 14)

	x1, y1, x2, y2 := bar.GetPosition()

	if y1 != 10 || y2 != 10 {
		t.Errorf("Bar did not enforce single line height. Got Y1:%d, Y2:%d; want Y1:10, Y2:10", y1, y2)
	}
	if x1 != 0 || x2 != 80 {
		t.Errorf("Bar corrupted X coordinates. Got X1:%d, X2:%d", x1, x2)
	}
}

func TestBar_DrawBackground(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)

	bar := &Bar{}
	bar.SetPosition(0, 2, 9, 2)
	bar.SetVisible(true)

	attr := uint64(0xAABBCC)
	bar.DrawBackground(scr, attr)

	// Проверяем, что вся строка 2 заполнена этим аттрибутом
	for x := 0; x < 10; x++ {
		checkCell(t, scr, x, 2, ' ', attr)
	}
}