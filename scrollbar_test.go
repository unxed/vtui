package vtui

import (
	"testing"
)

func TestScrollBarMath(t *testing.T) {
	// Проверяем логику позиционирования ползунка
	// Длина скроллбара 12 (трек 10). Элементов 100.
	length := 12
	total := 100

	// Начало списка (top = 0)
	caretPos, caretLen := CalcScrollBar(length, 0, total)
	if caretPos != 0 {
		t.Errorf("Expected caret at 0, got %d", caretPos)
	}
	if caretLen != 1 {
		t.Errorf("Expected caret length 1, got %d", caretLen)
	}

	// Середина списка (top = 44)
	// maxTop = 100 - 12 = 88. 44 is exactly half.
	// maxCaret = 10 - 1 = 9. Half is 4.5 -> 5
	caretPos, _ = CalcScrollBar(length, 44, total)
	if caretPos != 5 {
		t.Errorf("Expected caret at 5, got %d", caretPos)
	}

	// Конец списка (top = 88)
	caretPos, _ = CalcScrollBar(length, 88, total)
	if caretPos != 9 {
		t.Errorf("Expected caret at 9, got %d", caretPos)
	}
}

func TestDrawScrollBar(t *testing.T) {
	SetDefaultPalette()
	scr := NewScreenBuf()
	scr.AllocBuf(10, 10)

	attr := Palette[ColTableBox]

	// Рисуем скроллбар по координатам X=5, Y=2, длина 6. Элементов 20, мы на 0.
	drawn := DrawScrollBar(scr, 5, 2, 6, 0, 20, attr)

	if !drawn {
		t.Fatal("Scrollbar should have been drawn")
	}

	// Y=2 -> Верхняя стрелка
	checkCell(t, scr, 5, 2, ScrollUpArrow, attr)

	// Y=3 -> Ползунок (темный блок), т.к. мы в начале
	checkCell(t, scr, 5, 3, ScrollBlockDark, attr)

	// Y=4..6 -> Трек (светлый блок)
	checkCell(t, scr, 5, 4, ScrollBlockLight, attr)

	// Y=7 -> Нижняя стрелка
	checkCell(t, scr, 5, 7, ScrollDownArrow, attr)
}

func TestDrawScrollBar_EdgeCases(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(10, 10)
	attr := uint64(1)

	// 1. Длина меньше 3 (нельзя нарисовать)
	if DrawScrollBar(scr, 0, 0, 2, 0, 10, attr) {
		t.Error("Should not draw scrollbar if length <= 2")
	}

	// 2. Нет элементов
	if DrawScrollBar(scr, 0, 0, 5, 0, 0, attr) {
		t.Error("Should not draw scrollbar if 0 items")
	}

	// 3. Элементов меньше длины (не нужен)
	if DrawScrollBar(scr, 0, 0, 10, 0, 5, attr) {
		t.Error("Should not draw scrollbar if length >= items")
	}

	// 4. Отрисовка ползунка в самом конце
	// Трек = 4 (длина 6-2). Всего элементов 100.
	drawn := DrawScrollBar(scr, 0, 0, 6, 95, 100, attr)
	if !drawn {
		t.Fatal("Scrollbar should have been drawn")
	}
	// Верх - Y=0 (Стрелка)
	// Y=1, Y=2, Y=3 - Светлый трек
	// Y=4 - Темный трек (самый низ ползунка)
	checkCell(t, scr, 0, 4, ScrollBlockDark, attr)
}

func TestMathRound_DivByZero(t *testing.T) {
	res := MathRound(10, 0)
	if res != 0 {
		t.Errorf("MathRound with div 0 expected 0, got %d", res)
	}
}