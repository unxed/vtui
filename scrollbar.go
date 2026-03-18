package vtui

// Символы для скроллбара, аналогичные Oem2Unicode из far2l
const (
	ScrollUpArrow    = '▲' // 0x25B2
	ScrollDownArrow  = '▼' // 0x25BC
	ScrollBlockLight = '░' // 0x2591 (BS_X_B0)
	ScrollBlockDark  = '▓' // 0x2593 (BS_X_B2)
)

// MathRound выполняет математическое округление x / y
func MathRound(x, y uint64) uint64 {
	if y == 0 {
		return 0
	}
	return (x + (y / 2)) / y
}

// Max возвращает максимальное из двух чисел
func Max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// Min возвращает минимальное из двух чисел
func Min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// CalcScrollBar вычисляет позицию и размер ползунка скроллбара.
// Возвращает caretPos (смещение от верхней стрелки, от 0) и caretLength (размер ползунка).
func CalcScrollBar(length, topItem, itemsCount int) (caretPos, caretLength int) {
	if length <= 2 || itemsCount <= 0 || length >= itemsCount {
		return 0, 0
	}

	trackLen := uint64(length - 2)
	total := uint64(itemsCount)
	viewHeight := uint64(length)
	top := uint64(topItem)

	// Вычисляем размер ползунка (пропорционально видимой области)
	cLen := MathRound(trackLen*viewHeight, total)
	if cLen < 1 {
		cLen = 1
	}
	if cLen >= trackLen {
		cLen = trackLen - 1
	}

	// Вычисляем максимальные значения для скролла контента и самого ползунка
	maxTop := total - viewHeight
	if top > maxTop {
		top = maxTop
	}

	maxCaret := trackLen - cLen
	cPos := uint64(0)
	if maxTop > 0 {
		// Точная пропорция гарантирует касание нижнего края в самом конце списка
		cPos = MathRound(top*maxCaret, maxTop)
	}

	return int(cPos), int(cLen)
}

// DrawScrollBar отрисовывает вертикальный скроллбар.
// x, y - координаты верхнего символа (стрелки вверх).
// length - полная длина скроллбара (включая 2 стрелки).
// topItem - индекс первого видимого элемента.
// itemsCount - общее количество элементов в списке.
// attr - цветовой атрибут для отрисовки.
func DrawScrollBar(scr *ScreenBuf, x, y, length int, topItem, itemsCount int, attr uint64) bool {
	caretPos, caretLength := CalcScrollBar(length, topItem, itemsCount)
	if caretLength == 0 {
		return false // Скроллбар не нужен
	}

	trackLen := length - 2

	// 1. Верхняя стрелка
	scr.Write(x, y, []CharInfo{{Char: uint64(ScrollUpArrow), Attributes: attr}})

	// 2. Трек
	for i := 0; i < trackLen; i++ {
		char := ScrollBlockLight
		if i >= caretPos && i < caretPos+caretLength {
			char = ScrollBlockDark
		}
		scr.Write(x, y+1+i, []CharInfo{{Char: uint64(char), Attributes: attr}})
	}

	// 3. Нижняя стрелка
	scr.Write(x, y+length-1, []CharInfo{{Char: uint64(ScrollDownArrow), Attributes: attr}})

	return true
}