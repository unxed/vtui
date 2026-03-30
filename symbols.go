package vtui

// BoxType defines the frame style.
const (
	NoBox = iota
	SingleBox
	DoubleBox
)

// boxSymbols contains symbols for drawing frames.
var boxSymbols = []rune{
	// Single Box (0-10)
	'│', '─', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼',
	// Double Box (11-21)
	'║', '═', '╔', '╗', '╚', '╝', '╠', '╣', '╦', '╩', '╬',
	// Special connectors for VMenu (22-23): Double Vertical + Single Horizontal
	'╟', '╢',
	// Button brackets (24-25)
	'[', ']',
	// Separator connectors for Single Box (26-27)
	'╟', '╢',
}

const (
	bsV = 0 // Vertical line
	bsH = 1 // Horizontal line
	bsTL = 2 // Top-Left
	bsTR = 3 // Top-Right
	bsBL = 4 // Bottom-Left
	bsBR = 5 // Bottom-Right
)

// getBoxSymbols returns a slice of symbols for the specified frame type.
func getBoxSymbols(boxType int) []rune {
	if boxType == DoubleBox {
		return boxSymbols[11:]
	}
	return boxSymbols[:11]
}