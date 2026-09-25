package vtui

import (
	"math"
	"sync/atomic"
)

// GlyphStyle selects the geometric set used by graphical backends. Classic
// is deliberately the zero value so existing applications keep their layout
// and appearance until they opt into another set.
type GlyphStyle uint32

const (
	GlyphStyleClassic GlyphStyle = iota
	GlyphStyleRounded
)

var activeGlyphStyle atomic.Uint32

// SetGlyphStyle changes the geometric set used by graphical backends. Text
// and terminal backends continue to emit their ordinary box-drawing runes.
func SetGlyphStyle(style GlyphStyle) {
	if style > GlyphStyleRounded {
		style = GlyphStyleClassic
	}
	activeGlyphStyle.Store(uint32(style))
}

// CurrentGlyphStyle reports the set used for subsequent graphical cells.
func CurrentGlyphStyle() GlyphStyle {
	style := GlyphStyle(activeGlyphStyle.Load())
	if style > GlyphStyleRounded {
		return GlyphStyleClassic
	}
	return style
}

// classicRect is a filled rectangle in cell coordinates.  Keeping the
// geometry independent of the drawing API lets the raster and gogpu
// backends consume the same classic box-drawing table.
type classicRect struct {
	x, y, w, h float64
}

func classicGlyphRects(char rune, w, h, thick float64) ([]classicRect, bool) {
	return glyphRectsForStyle(CurrentGlyphStyle(), char, w, h, thick)
}

func glyphRectsForStyle(style GlyphStyle, char rune, w, h, thick float64) ([]classicRect, bool) {
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

	mx, my := math.Floor(w/2), math.Floor(h/2)
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

	if style == GlyphStyleRounded {
		switch base {
		case '┌', '┐', '└', '┘':
			return roundedCornerRects(base, w, h, thick), true
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

func roundedCornerRects(char rune, w, h, thick float64) []classicRect {
	mx, my := math.Floor(w/2), math.Floor(h/2)
	radius := math.Floor(math.Min(w, h) / 4)
	if radius < 1 {
		radius = 1
	}

	var rects []classicRect
	hline := func(x1, x2, y float64) {
		if x2 >= x1 {
			rects = append(rects, classicRect{x: x1, y: y, w: x2 - x1 + 1, h: thick})
		}
	}
	vline := func(x, y1, y2 float64) {
		if y2 >= y1 {
			rects = append(rects, classicRect{x: x, y: y1, w: thick, h: y2 - y1 + 1})
		}
	}

	// Add a small pixel-stepped quarter arc between the shortened horizontal
	// and vertical legs. The four cases are rotations of the same primitive.
	arc := func(cx, cy, sx, sy float64) {
		steps := int(math.Max(1, radius*2))
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps) * math.Pi / 2
			x := cx + sx*radius*math.Cos(t)
			y := cy + sy*radius*math.Sin(t)
			rects = append(rects, classicRect{x: math.Floor(x), y: math.Floor(y), w: thick, h: thick})
		}
	}

	switch char {
	case '┌':
		hline(mx+radius, w-1, my)
		vline(mx, my+radius, h-1)
		arc(mx+radius, my+radius, -1, -1)
	case '┐':
		hline(0, mx-radius, my)
		vline(mx, my+radius, h-1)
		arc(mx-radius, my+radius, 1, -1)
	case '└':
		hline(mx+radius, w-1, my)
		vline(mx, 0, my-radius)
		arc(mx+radius, my-radius, -1, 1)
	case '┘':
		hline(0, mx-radius, my)
		vline(mx, 0, my-radius)
		arc(mx-radius, my-radius, 1, 1)
	}
	return rects
}
