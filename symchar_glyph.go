package vtui

import "math"

// symGlyphRects returns the active GlyphStyle's geometric shape for a
// checkbox/radio SymGlyph token, in the same cell-relative classicRect
// vocabulary classic_glyph.go uses for box-drawing characters. w and h cover
// the whole glyph -- all 3 cells a checkbox/radio token occupies (symchar.go),
// not one cell -- so the shape can be centred on the glyph's middle cell the
// way "[x]"/"( )" centres its indicator between two bracket cells.
func symGlyphRects(sym SymGlyph, w, h, thick float64) ([]classicRect, bool) {
	return symGlyphRectsForStyle(CurrentGlyphStyle(), sym, w, h, thick)
}

// symGlyphRectsForStyle is symGlyphRects with an explicit style, exactly as
// glyphRectsForStyle is to classicGlyphRects: it lets tests and callers pin
// a style rather than read the process-wide CurrentGlyphStyle().
//
// GlyphStyleClassic always returns (nil, false): checkbox/radio glyphs were
// never geometric shapes under classic (see the design comment on
// SymCharFlag, symchar.go), and this function must keep that true, or every
// graphics backend's classic-style output would stop being pixel-identical
// to the plain "[x]"/"( )" text it replaces.
//
// GlyphStyleRounded draws:
//   - SymCheckboxOff: a small rounded-square outline.
//   - SymCheckboxOn: the same square, filled solid.
//   - SymCheckboxMixed: the outline plus a short horizontal dash -- the
//     conventional "indeterminate" mark -- so it reads as distinct from
//     both Off (empty) and On (fully filled).
//   - SymRadioOff: a circle outline (the same rounded-square primitive with
//     its corner radius widened to half the box's side, which collapses the
//     4 straight edges to nothing and leaves only the 4 corner arcs).
//   - SymRadioOn: the same circle outline plus a smaller filled circle
//     ("dot") centred inside it.
//
// Any other SymGlyph (today, only the button-ear tokens) returns (nil,
// false): geometric button ears are a separate, later slice of f4#285.
func symGlyphRectsForStyle(style GlyphStyle, sym SymGlyph, w, h, thick float64) ([]classicRect, bool) {
	if style != GlyphStyleRounded || w <= 0 || h <= 0 {
		return nil, false
	}
	if thick < 1 {
		thick = 1
	}

	cx, cy := w/2, h/2
	side := math.Floor(h * 0.7)
	if side < 4 {
		side = 4
	}
	x0, y0 := math.Floor(cx-side/2), math.Floor(cy-side/2)
	x1, y1 := x0+side, y0+side

	switch sym {
	case SymCheckboxOff:
		return symRoundedBoxOutlineRects(x0, y0, x1, y1, checkboxCornerRadius(side), thick), true
	case SymCheckboxOn:
		return symRoundedBoxFillRects(x0, y0, x1, y1, checkboxCornerRadius(side)), true
	case SymCheckboxMixed:
		rects := symRoundedBoxOutlineRects(x0, y0, x1, y1, checkboxCornerRadius(side), thick)
		dashH := math.Max(thick, math.Floor(side/6))
		dashW := math.Floor(side * 0.5)
		dashX := math.Floor(cx - dashW/2)
		dashY := math.Floor(cy - dashH/2)
		rects = append(rects, classicRect{x: dashX, y: dashY, w: dashW, h: dashH})
		return rects, true
	case SymRadioOff:
		return symRoundedBoxOutlineRects(x0, y0, x1, y1, math.Floor(side/2), thick), true
	case SymRadioOn:
		rects := symRoundedBoxOutlineRects(x0, y0, x1, y1, math.Floor(side/2), thick)
		dotSide := math.Floor(side * 0.45)
		if dotSide < 2 {
			dotSide = 2
		}
		dx0, dy0 := math.Floor(cx-dotSide/2), math.Floor(cy-dotSide/2)
		rects = append(rects, symRoundedBoxFillRects(dx0, dy0, dx0+dotSide, dy0+dotSide, math.Floor(dotSide/2))...)
		return rects, true
	default:
		return nil, false
	}
}

// checkboxCornerRadius picks a subtle rounding for a checkbox's square --
// enough to read as "rounded" without approaching the fully round radius
// (side/2) a radio button uses to look like a circle instead.
func checkboxCornerRadius(side float64) float64 {
	r := math.Floor(side / 4)
	if r < 1 {
		r = 1
	}
	return r
}

// symRoundedBoxOutlineRects strokes a rounded-rect outline within
// [x0,y0]-[x1,y1]: 4 straight edges shortened by radius at both ends, plus 4
// corner arcs traced the same way roundedCornerRects (classic_glyph.go)
// traces a rounded box-drawing corner -- a run of thick x thick stamps
// stepped along the quarter circle at that radius. When radius is half the
// box's side, the straight edges vanish (their shortened length is 0) and
// only the 4 arcs remain, which is the same box-drawing "approximate a
// curve with pixel-stepped stamps" trick applied all the way round -- the
// approximated circle a radio button's outline uses.
//
// This is deliberately independent of roundedCornerRects/glyphRectsForStyle:
// the two never share code, so nothing here can change box-drawing geometry.
func symRoundedBoxOutlineRects(x0, y0, x1, y1, radius, thick float64) []classicRect {
	var rects []classicRect
	hline := func(xa, xb, y float64) {
		if xb >= xa {
			rects = append(rects, classicRect{x: xa, y: y, w: xb - xa + 1, h: thick})
		}
	}
	vline := func(x, ya, yb float64) {
		if yb >= ya {
			rects = append(rects, classicRect{x: x, y: ya, w: thick, h: yb - ya + 1})
		}
	}
	arc := func(cx, cy, sx, sy float64) {
		steps := int(math.Max(1, radius*2))
		for i := 0; i <= steps; i++ {
			t := float64(i) / float64(steps) * math.Pi / 2
			x := cx + sx*radius*math.Cos(t)
			y := cy + sy*radius*math.Sin(t)
			rects = append(rects, classicRect{x: math.Floor(x), y: math.Floor(y), w: thick, h: thick})
		}
	}

	hline(x0+radius, x1-radius, y0)
	hline(x0+radius, x1-radius, y1-thick+1)
	vline(x0, y0+radius, y1-radius)
	vline(x1-thick+1, y0+radius, y1-radius)
	arc(x0+radius, y0+radius, -1, -1)
	arc(x1-radius, y0+radius, 1, -1)
	arc(x0+radius, y1-radius, -1, 1)
	arc(x1-radius, y1-radius, 1, 1)
	return rects
}

// symRoundedBoxFillRects fills a rounded rect within [x0,y0]-[x1,y1] with
// one horizontal classicRect per row, its x-extent narrowed near the top and
// bottom rows by the corner radius via the circle equation. This is the
// filled counterpart of symRoundedBoxOutlineRects -- a checked checkbox, or
// a selected radio button's inner dot -- and, exactly like that function,
// collapses to an approximated filled circle when radius is half the box's
// side.
func symRoundedBoxFillRects(x0, y0, x1, y1, radius float64) []classicRect {
	var rects []classicRect
	for y := y0; y <= y1; y++ {
		dx := 0.0
		if d := (y0 + radius) - y; d > 0 {
			dx = radius - math.Sqrt(math.Max(0, radius*radius-d*d))
		} else if d := y - (y1 - radius); d > 0 {
			dx = radius - math.Sqrt(math.Max(0, radius*radius-d*d))
		}
		xa, xb := math.Floor(x0+dx), math.Floor(x1-dx)
		if xb >= xa {
			rects = append(rects, classicRect{x: xa, y: y, w: xb - xa + 1, h: 1})
		}
	}
	return rects
}
