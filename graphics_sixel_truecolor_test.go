package vtui

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// sixelDecodeLive is a sixel decoder that resolves a colour register at the
// moment it is used rather than at the end of the image. That is the whole
// difference between reading a per-band picture and reading the last band's
// palette painted over all of it — and the test helper next door does the
// latter, which is exactly why it cannot check this encoder.
//
// It returns the colour of every pixel, or -1 where nothing was painted.
func sixelDecodeLive(t *testing.T, s string) (int, int, []int32) {
	t.Helper()
	i := strings.Index(s, "q")
	if i < 0 || !strings.HasPrefix(s, "\x1bP") {
		t.Fatalf("not a sixel image: %q", s)
	}
	body := s[i+1:]
	body = strings.TrimSuffix(body, "\x1b\\")

	var w, h int
	if strings.HasPrefix(body, "\"") {
		end := strings.IndexAny(body, "#!$-?")
		if end < 0 {
			end = len(body)
		}
		parts := strings.Split(body[1:end], ";")
		if len(parts) >= 4 {
			w, _ = strconv.Atoi(parts[2])
			h, _ = strconv.Atoi(parts[3])
		}
		body = body[end:]
	}
	if w <= 0 || h <= 0 {
		t.Fatalf("no raster attributes: %q", s)
	}

	px := make([]int32, w*h)
	for i := range px {
		px[i] = -1
	}
	regs := map[int]int32{}
	cur, band, x := -1, 0, 0

	num := func(p string) (int, string) {
		j := 0
		for j < len(p) && p[j] >= '0' && p[j] <= '9' {
			j++
		}
		v, _ := strconv.Atoi(p[:j])
		return v, p[j:]
	}
	paint := func(bits byte, count int) {
		for ; count > 0; count-- {
			for b := 0; b < 6; b++ {
				y := band*6 + b
				if bits&(1<<uint(b)) == 0 || y >= h || x >= w {
					continue
				}
				px[y*w+x] = regs[cur]
			}
			x++
		}
	}

	for len(body) > 0 {
		switch c := body[0]; {
		case c == '#':
			var n int
			n, body = num(body[1:])
			if strings.HasPrefix(body, ";") {
				var mode, r, g, bl int
				mode, body = num(body[1:])
				r, body = num(body[1:])
				g, body = num(body[1:])
				bl, body = num(body[1:])
				if mode != 2 {
					t.Fatalf("only RGB definitions are expected, got mode %d", mode)
				}
				regs[n] = int32(r*255/100)<<16 | int32(g*255/100)<<8 | int32(bl*255/100)
			}
			cur = n
		case c == '!':
			var cnt int
			cnt, body = num(body[1:])
			if len(body) == 0 {
				t.Fatal("a repeat with nothing to repeat")
			}
			paint(body[0]-'?', cnt)
			body = body[1:]
			continue
		case c == '$':
			x = 0
			body = body[1:]
			continue
		case c == '-':
			x, band = 0, band+1
			body = body[1:]
			continue
		case c >= '?' && c <= '~':
			paint(c-'?', 1)
			body = body[1:]
			continue
		default:
			t.Fatalf("unexpected byte %q", c)
		}
	}
	return w, h, px
}

// trueColorSixel is the default encoder with the cell size pinned, so the
// output does not depend on the host terminal.
func trueColorSixel() *sixelEncoder {
	e := newSixelEncoderWithOS(func(k string) string {
		if k == "VTUI_SIXEL_PALETTE" {
			return "truecolor"
		}
		return ""
	}, "linux")
	e.cellSize = func(cw, ch int) (int, int) { return cw, ch }
	return e
}

// A picture of more colours than a sixel image has registers must come back
// with all of them. This is the whole claim: 256 registers is not 256 colours,
// because a register is a variable and each band redefines it.
func TestSixelTrueColorCarriesMoreThan256Colours(t *testing.T) {
	const w, h = 64, 48
	surf := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// A smooth ramp: 3072 distinct colours, none of them repeated.
			v := y*w + x
			surf.SetPixel(x, y, byte(v>>4), byte(v&0xFF), byte(255-(v>>4)), 255)
		}
	}
	surf.Opaque = true

	out := trueColorSixel().encode(surf, 0, 0, w, h, w, h)
	_, _, px := sixelDecodeLive(t, out)

	distinct := map[int32]bool{}
	for _, c := range px {
		distinct[c] = true
	}
	if len(distinct) <= 256 {
		t.Errorf("only %d distinct colours came through; a single palette would give at most 256", len(distinct))
	}
}

// Redefining registers per band is only useful if the picture is still the
// picture. Every pixel has to come back close to what went in.
func TestSixelTrueColorIsAccurate(t *testing.T) {
	const w, h = 48, 36
	surf := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			surf.SetPixel(x, y, byte(x*5), byte(y*7), byte((x+y)*3), 255)
		}
	}
	surf.Opaque = true

	out := trueColorSixel().encode(surf, 0, 0, w, h, w, h)
	dw, dh, px := sixelDecodeLive(t, out)
	if dw != w || dh != h {
		t.Fatalf("decoded %dx%d, want %dx%d", dw, dh, w, h)
	}

	var worst, total float64
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			got := px[y*w+x]
			if got < 0 {
				t.Fatalf("pixel %d,%d was not painted", x, y)
			}
			wr, wg, wb := float64(x*5), float64(y*7), float64((x+y)*3)
			dr := float64((got>>16)&0xFF) - wr
			dg := float64((got>>8)&0xFF) - wg
			db := float64(got&0xFF) - wb
			d := math.Sqrt(dr*dr + dg*dg + db*db)
			total += d
			if d > worst {
				worst = d
			}
		}
	}
	mean := total / float64(w*h)
	// This is the worst case on purpose: every pixel of every band is a
	// different colour, 288 of them in a band that has 255 registers, so
	// the encoder has to give ground. Two things bound how much. Sixel
	// carries colour as percentages, which is about 1.3 per channel on its
	// own, and dropping a bit to make the colours fit costs about another
	// two. Beyond this the band palettes would not be doing their job.
	if mean > 6 || worst > 14 {
		t.Errorf("colour error: mean %.2f, worst %.2f", mean, worst)
	}
}

// A band whose colours fit in the registers is not approximated at all: the
// only loss is sixel's own percentages.
func TestSixelTrueColorIsExactWhenItFits(t *testing.T) {
	const w, h = 32, 12
	surf := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// 32 colours a band, well inside 255.
			surf.SetPixel(x, y, byte(x*8), byte(x*8), byte(x*8), 255)
		}
	}
	surf.Opaque = true

	out := trueColorSixel().encode(surf, 0, 0, w, h, w, h)
	_, _, px := sixelDecodeLive(t, out)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			got := float64((px[y*w+x] >> 16) & 0xFF)
			if d := math.Abs(got - float64(x*8)); d > 2 {
				t.Fatalf("pixel %d,%d is %.0f, want %d", x, y, got, x*8)
			}
		}
	}
}

// A flat picture must not pay for the machinery: one colour is one register.
func TestSixelTrueColorSolidStaysOneRegister(t *testing.T) {
	surf := NewImageSurface(8, 12)
	fillOpaque(surf, 255, 0, 0)

	out := trueColorSixel().encode(surf, 0, 0, 8, 12, 8, 12)
	if strings.Count(out, ";2;") != 2 {
		t.Errorf("two bands of one colour need two definitions: %q", out)
	}
	if !strings.Contains(out, "#0;2;100;0;0") {
		t.Errorf("red must be exact: %q", out)
	}
	_, _, px := sixelDecodeLive(t, out)
	for i, c := range px {
		if c != 0xFF0000 {
			t.Fatalf("pixel %d is %#06x, not red", i, c)
		}
	}
}

// Transparency survives: a pixel with no alpha gets no register and no bit, so
// whatever is behind it stays visible.
func TestSixelTrueColorKeepsTransparency(t *testing.T) {
	surf := NewImageSurface(4, 6)
	for y := 0; y < 6; y++ {
		for x := 0; x < 4; x++ {
			a := byte(255)
			if x == 1 {
				a = 0
			}
			surf.SetPixel(x, y, 10, 200, 30, a)
		}
	}

	out := trueColorSixel().encode(surf, 0, 0, 4, 6, 4, 6)
	_, _, px := sixelDecodeLive(t, out)
	for y := 0; y < 6; y++ {
		if px[y*4+1] >= 0 {
			t.Errorf("the transparent column was painted at row %d", y)
		}
		if px[y*4+0] < 0 {
			t.Errorf("the opaque column was not painted at row %d", y)
		}
	}
}

// The band that ends the picture is usually shorter than six rows, and its
// rows must be the right ones.
func TestSixelTrueColorHandlesAShortLastBand(t *testing.T) {
	const w, h = 4, 8 // one full band and one of two rows
	surf := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			surf.SetPixel(x, y, byte(y*30), 0, 0, 255)
		}
	}
	surf.Opaque = true

	out := trueColorSixel().encode(surf, 0, 0, w, h, w, h)
	dw, dh, px := sixelDecodeLive(t, out)
	if dw != w || dh != h {
		t.Fatalf("decoded %dx%d, want %dx%d", dw, dh, w, h)
	}
	for y := 0; y < h; y++ {
		c := px[y*w]
		if c < 0 {
			t.Fatalf("row %d was not painted", y)
		}
		want := float64(y * 30)
		got := float64((c >> 16) & 0xFF)
		if math.Abs(got-want) > 3 {
			t.Errorf("row %d: red %.0f, want %.0f", y, got, want)
		}
	}
}

// Enough bands to be split across workers must come out the same as when they
// are not, or the picture depends on how many cores the machine has.
func TestSixelTrueColorParallelMatchesSerial(t *testing.T) {
	const w, h = 32, 240
	surf := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			surf.SetPixel(x, y, byte(x*8), byte(y), byte(x+y), 255)
		}
	}
	surf.Opaque = true

	parallel := trueColorSixel().encode(surf, 0, 0, w, h, w, h)

	saved := scaleWorkers
	scaleWorkers = 1
	defer func() { scaleWorkers = saved }()
	serial := trueColorSixel().encode(surf, 0, 0, w, h, w, h)

	if parallel != serial {
		t.Errorf("split across workers the output differs: %d bytes against %d",
			len(parallel), len(serial))
	}
}

// The single-palette encoders stay reachable for a decoder that resolves its
// registers at the end of the image instead of as it goes.
func TestSixelPaletteModeFromEnvironment(t *testing.T) {
	cases := map[string][2]bool{ // {trueColor, adaptive}
		"":          {false, true},
		"fixed":     {false, false},
		"adaptive":  {false, true},
		"ADAPTIVE":  {false, true},
		"truecolor": {true, false},
		"per-band":  {true, false},
		"nonsense":  {true, false},
	}
	for value, want := range cases {
		e := newSixelEncoderWithOS(func(string) string { return value }, "linux")
		if e.trueColor != want[0] || e.adaptive != want[1] {
			t.Errorf("%q: trueColor=%v adaptive=%v, want %v", value, e.trueColor, e.adaptive, want)
		}
	}
}
