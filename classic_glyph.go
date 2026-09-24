package vtui

import "math"

// classicRect is a filled rectangle in cell coordinates.  Keeping the
// geometry independent of the drawing API lets the raster and gogpu
// backends consume the same classic box-drawing table.
type classicRect struct {
	x, y, w, h float64
}

func classicGlyphRects(char rune, w, h, thick float64) ([]classicRect, bool) {
	if w <= 0 || h <= 0 {
		return nil, false
	}
	if thick < 1 {
		thick = 1
	}

	// Heavy box-drawing characters have the same topology as their light
	// counterparts; only the ink width changes.
	base := char
	if heavy, ok := map[rune]rune{
		'━': '─', '┃': '│', '┏': '┌', '┓': '┐', '┗': '└', '┛': '┘',
		'┣': '├', '┫': '┤', '┳': '┬', '┻': '┴', '╋': '┼',
	}[char]; ok {
		base = heavy
		thick *= 2
	}

	mx, my := math.Floor(w / 2), math.Floor(h / 2)
	ofs := math.Floor(math.Min(w, h) / 4)
	if ofs < 1 {
		ofs = 1
	}

	var rects []classicRect
	hline := func(x1, x2, y, t float64) {
		if x2 >= x1 && t > 0 {
			rects = append(rects, classicRect{x: x1, y: y, w: x2 - x1 + 1, h: t})
		}
	}
	vline := func(x, y1, y2, t float64) {
		if y2 >= y1 && t > 0 {
			rects = append(rects, classicRect{x: x, y: y1, w: t, h: y2 - y1 + 1})
		}
	}

	switch base {
	case '─':
		hline(0, w-1, my, thick)
	case '│':
		vline(mx, 0, h-1, thick)
	case '┌':
		hline(mx, w-1, my, thick)
		vline(mx, my, h-1, thick)
	case '┐':
		hline(0, mx, my, thick)
		vline(mx, my, h-1, thick)
	case '└':
		hline(mx, w-1, my, thick)
		vline(mx, 0, my, thick)
	case '┘':
		hline(0, mx, my, thick)
		vline(mx, 0, my, thick)
	case '├':
		hline(mx, w-1, my, thick)
		vline(mx, 0, h-1, thick)
	case '┤':
		hline(0, mx, my, thick)
		vline(mx, 0, h-1, thick)
	case '┬':
		hline(0, w-1, my, thick)
		vline(mx, my, h-1, thick)
	case '┴':
		hline(0, w-1, my, thick)
		vline(mx, 0, my, thick)
	case '┼':
		hline(0, w-1, my, thick)
		vline(mx, 0, h-1, thick)
	case '═':
		hline(0, w-1, my-ofs, thick)
		hline(0, w-1, my+ofs, thick)
	case '║':
		vline(mx-ofs, 0, h-1, thick)
		vline(mx+ofs, 0, h-1, thick)
	case '╔':
		hline(mx+ofs, w-1, my-ofs, thick)
		hline(mx-ofs, w-1, my+ofs, thick)
		vline(mx-ofs, my+ofs, h-1, thick)
		vline(mx+ofs, my-ofs, h-1, thick)
	case '╗':
		hline(0, mx-ofs, my-ofs, thick)
		hline(0, mx+ofs, my+ofs, thick)
		vline(mx+ofs, my+ofs, h-1, thick)
		vline(mx-ofs, my-ofs, h-1, thick)
	case '╚':
		hline(mx-ofs, w-1, my-ofs, thick)
		hline(mx+ofs, w-1, my+ofs, thick)
		vline(mx-ofs, 0, my-ofs, thick)
		vline(mx+ofs, 0, my+ofs, thick)
	case '╝':
		hline(0, mx+ofs, my-ofs, thick)
		hline(0, mx-ofs, my+ofs, thick)
		vline(mx+ofs, 0, my-ofs, thick)
		vline(mx-ofs, 0, my+ofs, thick)
	case '╠':
		hline(mx-ofs, w-1, my-ofs, thick)
		hline(mx+ofs, w-1, my+ofs, thick)
		vline(mx-ofs, 0, h-1, thick)
		vline(mx+ofs, 0, h-1, thick)
	case '╣':
		hline(0, mx+ofs, my-ofs, thick)
		hline(0, mx-ofs, my+ofs, thick)
		vline(mx+ofs, 0, h-1, thick)
		vline(mx-ofs, 0, h-1, thick)
	case '╩':
		hline(0, w-1, my+ofs, thick)
		hline(0, mx-ofs, my-ofs, thick)
		hline(mx+ofs, w-1, my-ofs, thick)
		vline(mx-ofs, 0, my-ofs, thick)
		vline(mx+ofs, 0, my-ofs, thick)
	case '╦':
		hline(0, w-1, my-ofs, thick)
		hline(0, mx-ofs, my+ofs, thick)
		hline(mx+ofs, w-1, my+ofs, thick)
		vline(mx-ofs, my+ofs, h-1, thick)
		vline(mx+ofs, my+ofs, h-1, thick)
	case '╟':
		hline(mx+ofs, w-1, my, thick)
		vline(mx-ofs, 0, h-1, thick)
		vline(mx+ofs, 0, h-1, thick)
	case '╢':
		hline(0, mx-ofs, my, thick)
		vline(mx-ofs, 0, h-1, thick)
		vline(mx+ofs, 0, h-1, thick)
	case '╬':
		hline(0, w-1, my-ofs, thick)
		hline(0, w-1, my+ofs, thick)
		vline(mx-ofs, 0, h-1, thick)
		vline(mx+ofs, 0, h-1, thick)
	default:
		return nil, false
	}
	return rects, true
}
