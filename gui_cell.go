package vtui

import "math"

// pixelToCell turns a pixel offset in a GUI window into vtinput's int16 cell
// coordinate. It guards a cell size not computed yet and clamps an offset
// far off the window, such as a drag that left it.
func pixelToCell(px, cell int) int16 {
	if cell <= 0 {
		return 0
	}
	v := px / cell
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return int16(v)
}
