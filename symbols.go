package vtui

// BoxType определяет стиль рамки.
const (
	NoBox = iota
	SingleBox
	DoubleBox
)

// boxSymbols содержит символы для отрисовки рамок.
var boxSymbols = []rune{
	// Single Box (0-10)
	'│', '─', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼',
	// Double Box (11-21)
	'║', '═', '╔', '╗', '╚', '╝', '╠', '╣', '╦', '╩', '╬',
	// Special connectors for VMenu (22-23): Double Vertical + Single Horizontal
	'╟', '╢',
	// Button brackets (24-25)
	'[', ']',
}

const (
	bsV = 0 // Вертикальная линия
	bsH = 1 // Горизонтальная
	bsTL = 2 // Top-Left
	bsTR = 3 // Top-Right
	bsBL = 4 // Bottom-Left
	bsBR = 5 // Bottom-Right
)

// getBoxSymbols возвращает срез символов для указанного типа рамки.
func getBoxSymbols(boxType int) []rune {
	if boxType == DoubleBox {
		return boxSymbols[11:]
	}
	return boxSymbols[:11]
}