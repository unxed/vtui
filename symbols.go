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
	// Special connectors for VMenu separators
	'╟', '╢', // U+255F, U+2562 (Double Vertical, Single Horizontal)
}

// Indices for common box drawing symbols
const (
	bsV             = 0 // │ or ║
	bsH             = 1 // ─ or ═
	bsTL            = 2 // ┌ or ╔
	bsTR            = 3 // ┐ or ╗
	bsBL            = 4 // └ or ╚
	bsBR            = 5 // ┘ or ╝
	bsHCrossLeft    = 6 // ├ or ╠ (for basic single/double box horizontal cross)
	bsHCrossRight   = 7 // ┤ or ╣
	bsVCrossTop     = 8 // ┬ or ╦
	bsVCrossBottom  = 9 // ┴ or ╩
	bsCross         = 10 // ┼ or ╬

	// Specific VMenu separator symbols (indices after standard box symbols)
	bsVMenuHCrossLeft  = 22 // ╟
	bsVMenuHCrossRight = 23 // ╢
)

// getBoxSymbols returns a slice of symbols for the specified frame type.
func getBoxSymbols(boxType int) []rune {
	if boxType == DoubleBox {
		return boxSymbols[11:22] // Double box symbols are from index 11 to 21
	}
	return boxSymbols[0:11] // Single box symbols are from index 0 to 10
}