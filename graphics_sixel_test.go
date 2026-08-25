package vtui

import (
	"fmt"
	"strings"
	"testing"
)

// fixedSixel returns an encoder whose cell geometry is pinned to the explicit
// cw x ch the caller passes, so the tests below do not depend on the host
// terminal (Windows Terminal would otherwise force the 10x20 virtual cell).
func fixedSixel() *sixelEncoder {
	e := newSixelEncoderWithOS(func(k string) string {
		if k == "VTUI_SIXEL_PALETTE" {
			return "fixed"
		}
		return ""
	}, "linux")
	e.cellSize = func(cw, ch int) (int, int) { return cw, ch }
	return e
}

func sixelRender(e *sixelEncoder, list []ImagePlacement, cw, ch int) string {
	var sb strings.Builder
	e.Render(&sb, list, cw, ch)
	return sb.String()
}

func TestSixelGrid(t *testing.T) {
	cases := []struct {
		cols, rows, cw, ch int
		dw, dh             int
	}{
		// One cell rasterises to one cell's worth of device pixels.
		{10, 4, 8, 16, 80, 64},
		{10, 4, 10, 20, 100, 80},
		{1, 1, 8, 16, 8, 16},
		{1, 1, 0, 0, 8, 16}, // unknown cell falls back to 8x16
		{0, 0, 8, 16, 1, 1}, // degenerate input clamps to one pixel
		{1, 1, 6, 16, 6, 16},
	}
	for _, tc := range cases {
		dw, dh := sixelGrid(tc.cols, tc.rows, tc.cw, tc.ch)
		if dw != tc.dw || dh != tc.dh {
			t.Errorf("sixelGrid(%d,%d,%d,%d) = %d,%d, want %d,%d",
				tc.cols, tc.rows, tc.cw, tc.ch, dw, dh, tc.dw, tc.dh)
		}
	}
}

func TestSixelCellSize(t *testing.T) {
	cases := []struct {
		name         string
		env          map[string]string
		goos         string
		cw, ch       int
		wantW, wantH int
	}{
		{"native windows", map[string]string{}, "windows", 0, 0, 10, 20},
		{"wsl in windows terminal", map[string]string{"WT_SESSION": "x"}, "linux", 0, 0, 10, 20},
		{"unknown unix falls back", map[string]string{}, "linux", 0, 0, 8, 16},
		{"reported unix cell wins", map[string]string{}, "linux", 9, 18, 9, 18},
		{"windows overrides reported cell", map[string]string{}, "windows", 9, 18, 10, 20},
		{"wezterm on windows keeps reported cell", map[string]string{"WEZTERM_PANE": "0"}, "windows", 9, 18, 9, 18},
		{"wezterm on windows falls back", map[string]string{"WEZTERM_PANE": "0"}, "windows", 0, 0, 8, 16},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := func(k string) string { return tc.env[k] }
			w, h := sixelCellSizeWith(env, tc.goos, tc.cw, tc.ch)
			if w != tc.wantW || h != tc.wantH {
				t.Errorf("sixelCellSizeWith(...) = %d,%d, want %d,%d", w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

func fillOpaque(s *ImageSurface, r, g, b byte) {
	for y := 0; y < s.Height; y++ {
		for x := 0; x < s.Width; x++ {
			s.SetPixel(x, y, r, g, b, 255)
		}
	}
}

func TestSixelEncodeSolidColour(t *testing.T) {
	surf := NewImageSurface(3, 6)
	fillOpaque(surf, 255, 0, 0)

	out := sixelRender(fixedSixel(), []ImagePlacement{{Surface: surf, Cols: 1, Rows: 1}}, 8, 16)
	if !strings.HasPrefix(out, "\x1b[1;1H\x1bP0;1;8q\"1;1;8;16") {
		t.Fatalf("unexpected header: %q", out)
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Fatal("every sixel image must end with ST")
	}
	// Pure red lands in the colour cube and is the only register.
	if !strings.Contains(out, "#0;2;100;0;0") {
		t.Errorf("missing red register definition: %q", out)
	}

	w, h, grid, pal := sixelDecode(t, out)
	if w != 8 || h != 16 {
		t.Fatalf("decoded size = %dx%d, want 8x16", w, h)
	}
	for i := 0; i < w*h; i++ {
		if grid[i] != 0 {
			t.Fatalf("pixel %d = register %d, want 0", i, grid[i])
		}
	}
	if pal[0] != 0xFF0000 {
		t.Errorf("palette[0] = %#06x, want red", pal[0])
	}
}

func TestSixelKeepsTransparentPixels(t *testing.T) {
	// One cell at the fixed 8x16 geometry, so no resampling is involved.
	surf := NewImageSurface(8, 16) // fully transparent
	surf.SetPixel(3, 3, 255, 0, 0, 255)

	out := sixelRender(fixedSixel(), []ImagePlacement{{Surface: surf, Cols: 1, Rows: 1}}, 8, 16)
	if !strings.Contains(out, "\x1bP0;1;8q") {
		t.Fatal("P2=1 must keep the background behind zero pixels")
	}

	_, _, grid, _ := sixelDecode(t, out)
	for y := 0; y < 16; y++ {
		for x := 0; x < 8; x++ {
			got := grid[y*8+x]
			if x == 3 && y == 3 {
				if got < 0 {
					t.Fatalf("the opaque pixel at 3,3 was dropped")
				}
				continue
			}
			if got != -1 {
				t.Fatalf("pixel (%d,%d) = register %d, want transparent", x, y, got)
			}
		}
	}
}

// TestSixelRoundTrip feeds a two-colour image with interleaved columns through
// the encoder and decodes the result back to pixels. The old encoder skipped
// blank columns without advancing the cursor, shifting every colour's pixels
// left so the colours overlapped; this test pins the fixed behaviour.
func TestSixelRoundTrip(t *testing.T) {
	surf := NewImageSurface(6, 6)
	for y := 0; y < 6; y++ {
		for x := 0; x < 6; x++ {
			if x%2 == 0 {
				surf.SetPixel(x, y, 255, 0, 0, 255)
			} else {
				surf.SetPixel(x, y, 0, 0, 255, 255)
			}
		}
	}

	// Encode at 6x6 with no resampling: one cell, cw=6 ch=6.
	out := sixelRender(fixedSixel(), []ImagePlacement{{Surface: surf, Cols: 1, Rows: 1}}, 6, 6)
	w, h, grid, pal := sixelDecode(t, out)
	if w != 6 || h != 6 {
		t.Fatalf("decoded size = %dx%d, want 6x6", w, h)
	}

	blueReg, redReg := -1, -1
	for r, c := range pal {
		switch c {
		case 0x0000FF:
			blueReg = r
		case 0xFF0000:
			redReg = r
		}
	}
	if blueReg < 0 || redReg < 0 {
		t.Fatalf("decoded palette lost blue or red: %#v", pal)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			want := blueReg
			if x%2 == 0 {
				want = redReg
			}
			if got := grid[y*w+x]; got != want {
				t.Fatalf("pixel (%d,%d) = register %d, want %d", x, y, got, want)
			}
		}
	}
}

func TestSixelEmitParallelMatchesSerial(t *testing.T) {
	dw, dh := 100, 240
	surf := NewImageSurface(dw, dh)
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			surf.SetPixel(x, y, byte(x), byte(y), byte((x+y)/2), 255)
		}
	}
	idx := make([]byte, dw*dh)
	sixelQuantize(surf, idx)
	remap, regs := sixelRegisters(idx)
	nreg, nbands := len(regs), (dh+5)/6

	var serial, parallel strings.Builder
	sixelEmitBands(&serial, idx, remap, nreg, dw, dh, 0, nbands, make([]byte, dw*nreg), make([]bool, nreg))
	newSixelEncoder().sixelEmitData(&parallel, idx, remap, nreg, dw, dh)
	if serial.String() != parallel.String() {
		t.Fatalf("parallel emit diverges from serial (%d vs %d bytes)", len(serial.String()), len(parallel.String()))
	}
}

func TestSixelReusesCachedBytes(t *testing.T) {
	surf := NewImageSurface(3, 6)
	fillOpaque(surf, 10, 20, 30)

	e := fixedSixel()
	first := sixelRender(e, []ImagePlacement{{Surface: surf, Cols: 1, Rows: 1}}, 8, 16)
	second := sixelRender(e, []ImagePlacement{{Surface: surf, Col: 5, Cols: 1, Rows: 1}}, 8, 16)

	if !strings.Contains(second, "\x1b[1;6H") {
		t.Errorf("a moved image must re-position the cursor: %q", second)
	}
	first = first[strings.Index(first, "\x1bP"):]
	second = second[strings.Index(second, "\x1bP"):]
	if first != second {
		t.Error("the same crop at the same size must reuse cached sixel bytes")
	}
}

func TestSixelDifferentSizeReEncodes(t *testing.T) {
	surf := NewImageSurface(4, 8)
	fillOpaque(surf, 40, 90, 200)

	e := fixedSixel()
	small := sixelRender(e, []ImagePlacement{{Surface: surf, Cols: 1, Rows: 1}}, 8, 16)
	wide := sixelRender(e, []ImagePlacement{{Surface: surf, Cols: 3, Rows: 2}}, 8, 16)

	if small == wide {
		t.Fatal("different destination sizes must not share a cache entry")
	}
	if !strings.Contains(wide, fmt.Sprintf("\"1;1;%d;%d", 3*8, 2*16)) {
		t.Errorf("3x2 cells must raster to 24x32 sixel pixels: %q", wide)
	}
}

func TestSixelSkipsInvalidPlacements(t *testing.T) {
	out := sixelRender(fixedSixel(), []ImagePlacement{
		{Surface: nil, Cols: 2, Rows: 2},
		{Surface: NewImageSurface(4, 4), Cols: 0, Rows: 2},
		{Surface: NewImageSurface(0, 0), Cols: 2, Rows: 2},
	}, 8, 16)
	if out != "" {
		t.Errorf("nothing drawable must produce no output, got %q", out)
	}
}

func TestSixelEvictsOldEntries(t *testing.T) {
	e := fixedSixel()
	for i := 0; i < sixelCacheLimit+3; i++ {
		surf := NewImageSurface(3, 6)
		fillOpaque(surf, byte(i), byte(i>>8), 0)
		e.Render(&strings.Builder{}, []ImagePlacement{{Surface: surf, Cols: 1, Rows: 1}}, 8, 16)
	}
	if len(e.cache) > sixelCacheLimit {
		t.Errorf("cache grew to %d entries", len(e.cache))
	}
}

func TestScreenBufFlushEmitsSixel(t *testing.T) {
	scr, out := newGraphicsTestScreen(t)
	scr.Graphics().SetProtocol(GraphicsSixel)
	surf := NewImageSurface(3, 6)
	fillOpaque(surf, 255, 0, 0)
	scr.Graphics().Add(ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 1, Rows: 1})

	scr.Flush()
	if !strings.Contains(out.String(), "\x1bP0;1;8q") {
		t.Fatalf("first flush must emit sixel data: %q", out.String())
	}

	out.Reset()
	scr.Flush()
	if strings.Contains(out.String(), "\x1bP") {
		t.Errorf("an unchanged frame must not re-send sixel data: %q", out.String())
	}
}

// TestScreenBufFlushSixelGeometry runs the whole ScreenBuf -> AnsiRenderer ->
// sixel path with default cell-size resolution and decodes the emitted stream,
// checking the raster a terminal receives, not just the encoder in isolation.
func TestScreenBufFlushSixelGeometry(t *testing.T) {
	scr, out := newGraphicsTestScreen(t)
	scr.Graphics().SetProtocol(GraphicsSixel)
	surf := NewImageSurface(40, 20)
	fillOpaque(surf, 30, 120, 200)
	scr.Graphics().Add(ImagePlacement{Surface: surf, Col: 2, Row: 3, Cols: 20, Rows: 10})

	scr.Flush()
	raw := out.String()
	start := strings.Index(raw, "\x1bP0;1;8q")
	if start < 0 {
		t.Fatalf("no sixel DCS in flush output: %q", raw)
	}
	w, h, _, _ := sixelDecode(t, raw[start:])

	cw, ch := sixelCellSize(0, 0)
	if wantW, wantH := 20*cw, 10*ch; w != wantW || h != wantH {
		t.Fatalf("sixel raster = %dx%d, want %dx%d (cell %dx%d)", w, h, wantW, wantH, cw, ch)
	}
}

// sixelDecode is the test oracle for the encoder: a dependency-free decoder
// for the emitted subset (DCS 0;1;8q, "1;1;W;H, #r;2;R;G;B definitions, #r
// selects, $, -, !N runs). It returns the raster size, one register per pixel
// (-1 = transparent) and the register palette as packed RGB.
func sixelDecode(t *testing.T, s string) (w, h int, grid []int, pal []uint32) {
	t.Helper()

	i := strings.Index(s, "\x1bP")
	if i < 0 {
		t.Fatalf("no DCS in %q", s)
	}
	s = s[i:]
	if !strings.HasPrefix(s, "\x1bP0;1;8q") {
		t.Fatalf("unexpected DCS header: %q", s)
	}
	pos := len("\x1bP0;1;8q")
	if !strings.HasPrefix(s[pos:], "\"1;1;") {
		t.Fatalf("unexpected raster attributes: %q", s[pos:])
	}
	pos += len("\"1;1;")
	w = readSixelNum(t, s, &pos)
	if pos >= len(s) || s[pos] != ';' {
		t.Fatalf("expected ';' after raster width in %q", s)
	}
	pos++
	h = readSixelNum(t, s, &pos)

	pal = make([]uint32, 256)
	grid = make([]int, w*((h+5)/6)*6)
	for i := range grid {
		grid[i] = -1
	}

	cur := -1
	x, y := 0, 0
	for pos < len(s) {
		c := s[pos]
		switch {
		case c == 0x1b && strings.HasPrefix(s[pos:], "\x1b\\"):
			return w, h, grid, pal
		case c == '#':
			pos++
			reg := readSixelNum(t, s, &pos)
			if pos < len(s) && s[pos] == ';' {
				// Colour definition: #r;2;R;G;B
				pos++
				if model := readSixelNum(t, s, &pos); model != 2 {
					t.Fatalf("only RGB colour definitions are emitted, got model %d", model)
				}
				pos++
				r := readSixelNum(t, s, &pos)
				pos++
				g := readSixelNum(t, s, &pos)
				pos++
				b := readSixelNum(t, s, &pos)
				pal[reg] = uint32(r*255/100)<<16 | uint32(g*255/100)<<8 | uint32(b*255/100)
			}
			cur = reg
		case c == '!':
			pos++
			n := readSixelNum(t, s, &pos)
			if pos >= len(s) {
				t.Fatal("repeat introducer without a sixel char")
			}
			v := int(s[pos]) - 63
			pos++
			for k := 0; k < n; k++ {
				drawSixelPixel(w, grid, x, y, v, cur)
				x++
			}
		case c == '$':
			x = 0
			pos++
		case c == '-':
			y += 6
			x = 0
			pos++
		case c >= '?' && c <= '~':
			drawSixelPixel(w, grid, x, y, int(c)-63, cur)
			x++
			pos++
		default:
			pos++
		}
	}
	t.Fatal("sixel stream ended without ST")
	return w, h, grid, pal
}

func readSixelNum(t *testing.T, s string, pos *int) int {
	t.Helper()
	n := 0
	for *pos < len(s) && s[*pos] >= '0' && s[*pos] <= '9' {
		n = n*10 + int(s[*pos]-'0')
		*pos = *pos + 1
	}
	return n
}

func drawSixelPixel(w int, grid []int, x, y, v, cur int) {
	for p := 0; p < 6; p++ {
		if v&(1<<p) == 0 {
			continue
		}
		idx := (y+p)*w + x
		if idx >= 0 && idx < len(grid) {
			grid[idx] = cur
		}
	}
}
