//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

// Glyph drawing tests + table tooling.
//
// The GUI draws the TUI chrome runes itself: fonts cover the box/arrow
// ranges unevenly, and composite paths rasterize to solid boxes on some
// Windows GPU stacks, so every shape is decomposed into single-rect fills.
// Two paths:
//
//   - Runtime converter (gogpu_renderer.go): any rune the loaded font covers
//     is rasterized like DrawString and memoized - pixel-identical to text.
//   - Fallback table (gogpu_glyph_table.go): runes the GUI font lacks are
//     drawn from this committed table, rasterized from the reference font.
//
// TestGenerateGlyphTable verifies the table is current, or regenerates it:
//
//	go test -run TestGenerateGlyphTable -v .                      # verify
//	VTUI_REGEN_GLYPHS=1 go test -run TestGenerateGlyphTable -v .  # regenerate

import (
	"fmt"
	"image"
	"os"
	"strings"
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// refFontPath is the reference font for the fallback table: the font the GUI
// picks first on Windows. Point at another TTF and regenerate to rebase.
const refFontPath = "C:/Windows/Fonts/consola.ttf"

// glyphTableRunes is the symbol set in the fallback table. Adding a rune
// here (plus a case in drawCustomChar's table lookup) gives it the same
// fallback treatment; it must exist in refFontPath.
var glyphTableRunes = []rune{'↑', '↓', '↕', '←', '→', '↔', '▲', '▼'}

// glyphFace returns the reference face and the canonical cell geometry:
// cellW = Advance("A"), cellH = Ascent+Descent, baseline = Ascent.
func glyphFace(t testing.TB) (font.Face, int, int, int) {
	data, err := os.ReadFile(refFontPath)
	if err != nil {
		t.Skipf("%s not available: %v", refFontPath, err)
	}
	ft, err := opentype.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	const size = 18.0
	face, err := opentype.NewFace(ft, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	m := face.Metrics()
	adv, _ := face.GlyphAdvance('A')
	return face, int(adv>>6 + 1), int((m.Ascent+m.Descent)>>6 + 1), int(m.Ascent >> 6)
}

// scanGlyph reads the rune's bitmap straight out of the face's alpha mask
// (font.Face.Glyph) - no cell image, no Drawer. Threshold: coverage > 0x80,
// matching the runtime converter. Coordinates are clipped to the cell.
func scanGlyph(face font.Face, ch rune, w, h, baseline int) ([]glyphRect, error) {
	dr, mask, _, _, ok := face.Glyph(fixed.P(0, baseline), ch)
	if !ok {
		return nil, fmt.Errorf("no glyph for %q", ch)
	}
	a, isA := mask.(*image.Alpha)
	if !isA || dr.Dx() > a.Rect.Dx() || dr.Dy() > a.Rect.Dy() {
		return nil, fmt.Errorf("unexpected mask %T for %q", mask, ch)
	}
	var rects []glyphRect
	for j := 0; j < dr.Dy(); j++ {
		y := dr.Min.Y + j
		if y < 0 || y >= h {
			continue
		}
		row := a.Pix[j*a.Stride : j*a.Stride+dr.Dx()]
		for i := 0; i < dr.Dx(); {
			if row[i] <= 0x80 {
				i++
				continue
			}
			k := i
			for k+1 < dr.Dx() && row[k+1] > 0x80 {
				k++
			}
			x0, x1 := dr.Min.X+i, dr.Min.X+k
			if x0 >= w || x1 < 0 {
				i = k + 1
				continue
			}
			if x0 < 0 {
				x0 = 0
			}
			if x1 >= w {
				x1 = w - 1
			}
			rects = append(rects, glyphRect{y0: y, y1: y, x0: x0, x1: x1})
			i = k + 1
		}
	}
	return rects, nil
}

// scanCellAlpha is an independent reference scan (Drawer into a cell image,
// then per-pixel threshold) used to verify scanGlyph and the table.
func scanCellAlpha(face font.Face, ch rune, w, h, baseline int) []glyphRect {
	img := image.NewAlpha(image.Rect(0, 0, w, h))
	d := font.Drawer{Dst: img, Src: image.White, Face: face, Dot: fixed.P(0, baseline)}
	d.DrawString(string(ch))
	var rects []glyphRect
	for y := 0; y < h; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+w]
		for x := 0; x < w; {
			if row[x] <= 0x80 {
				x++
				continue
			}
			x1 := x
			for x1+1 < w && row[x1+1] > 0x80 {
				x1++
			}
			rects = append(rects, glyphRect{y0: y, y1: y, x0: x, x1: x1})
			x = x1 + 1
		}
	}
	return rects
}

func equalRects(a, b []glyphRect) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGenerateGlyphTable(t *testing.T) {
	face, cellW, cellH, baseline := glyphFace(t)

	type entry struct {
		ch    rune
		rects []glyphRect
	}
	got := make([]entry, 0, len(glyphTableRunes))
	for _, ch := range glyphTableRunes {
		rects, err := scanGlyph(face, ch, cellW, cellH, baseline)
		if err != nil {
			t.Fatal(err)
		}
		rects = mergeGlyphRectsV(rects)
		if len(rects) == 0 {
			t.Fatalf("%c: rasterized to zero rects", ch)
		}
		got = append(got, entry{ch, rects})
	}

	if os.Getenv("VTUI_REGEN_GLYPHS") == "" {
		// Verify the committed table is current, using the independent
		// cell-scan as ground truth (a broken mask scan must not pass by
		// reproducing itself).
		for _, e := range got {
			want := mergeGlyphRectsV(scanCellAlpha(face, e.ch, cellW, cellH, baseline))
			if !equalRects(want, e.rects) {
				t.Errorf("%c: mask scan diverged from the reference raster", e.ch)
			}
			if !equalRects(want, glyphTable[e.ch]) {
				t.Errorf("%c: table out of date - regenerate with VTUI_REGEN_GLYPHS=1", e.ch)
			}
		}
		return
	}

	var out strings.Builder
	fmt.Fprintf(&out, "package vtui\n\n")
	out.WriteString("// Code generated by TestGenerateGlyphTable (gogpu_glyphgen_test.go); DO NOT EDIT.\n")
	fmt.Fprintf(&out, "// Canonical cell %dx%d px, baseline row %d; rasterized from %s at 18px,\n", cellW, cellH, baseline, refFontPath)
	out.WriteString("// hinting full, vertically merged. Fallback for GUI fonts lacking the runes;\n")
	out.WriteString("// runes the font covers are drawn pixel-exact by the runtime converter.\n")
	out.WriteString("// Regenerate: VTUI_REGEN_GLYPHS=1 go test -run TestGenerateGlyphTable -v .\n\n")
	fmt.Fprintf(&out, "type glyphRect struct{ y0, y1, x0, x1 int }\n\n")
	fmt.Fprintf(&out, "const glyphCellW, glyphCellH = %d, %d\n\n", cellW, cellH)
	out.WriteString("var glyphTable = map[rune][]glyphRect{\n")
	for _, e := range got {
		fmt.Fprintf(&out, "\t'\\u%04X': {", e.ch)
		for i, rc := range e.rects {
			if i > 0 {
				out.WriteString(", ")
			}
			fmt.Fprintf(&out, "{%d,%d,%d,%d}", rc.y0, rc.y1, rc.x0, rc.x1)
		}
		out.WriteString("},\n")
	}
	out.WriteString("}\n")
	if err := os.WriteFile("gogpu_glyph_table.go", []byte(out.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote gogpu_glyph_table.go: %d runes, cell %dx%d, baseline %d", len(got), cellW, cellH, baseline)
}

// Benchmarks justify the production scan: the mask scan is ~2x faster and
// allocates less than the cell-image scan, which stays as the reference
// implementation used by TestGenerateGlyphTable.

func glyphRegenBench(b *testing.B, scan func(font.Face, rune, int, int, int) []glyphRect, merged bool) {
	face, w, h, baseline := glyphFace(b)
	b.ReportAllocs()
	total := 0
	for b.Loop() {
		for _, ch := range glyphTableRunes {
			rects := scan(face, ch, w, h, baseline)
			if merged {
				rects = mergeGlyphRectsV(rects)
			}
			total += len(rects)
		}
	}
	n := float64(b.N) * float64(len(glyphTableRunes))
	b.ReportMetric(float64(total)/n, "rects/glyph")
	b.ReportMetric(b.Elapsed().Seconds()/n*1e9, "ns/glyph")
}

func scanMaskBench(f font.Face, ch rune, w, h, bl int) []glyphRect {
	rects, err := scanGlyph(f, ch, w, h, bl)
	if err != nil {
		panic(err)
	}
	return rects
}

func BenchmarkGenerateGlyph_ScanCellAlpha(b *testing.B)  { glyphRegenBench(b, scanCellAlpha, false) }
func BenchmarkGenerateGlyph_ScanMask(b *testing.B)       { glyphRegenBench(b, scanMaskBench, false) }
func BenchmarkGenerateGlyph_ScanMaskMerged(b *testing.B) { glyphRegenBench(b, scanMaskBench, true) }
