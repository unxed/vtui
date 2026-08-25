package vtui

import (
	"strings"
	"testing"
)

// The layered encoder is about the shape of the stream, not about pixels: a
// terminal composites whatever it is given, and what it has to be given is
// several complete transparent images at one and the same cell.

// gradientSurface is a picture no 255-colour palette can serve well: a smooth
// ramp across three channels at once, which is where banding lives.
func gradientSurface(w, h int) *ImageSurface {
	s := &ImageSurface{
		Width:  w,
		Height: h,
		Stride: w * 4,
		Pix:    make([]byte, w*h*4),
		Opaque: true,
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*s.Stride + x*4
			s.Pix[o] = byte(x * 255 / max(1, w-1))
			s.Pix[o+1] = byte(y * 255 / max(1, h-1))
			s.Pix[o+2] = byte((x + y) * 255 / max(1, w+h-2))
			s.Pix[o+3] = 0xFF
		}
	}
	return s
}

// layeredStressSurface deliberately has more than one palette's worth of
// colours spread across the image. A smooth gradient is too easy for the
// adaptive palette: at this size it can serve every pixel within the layer
// tolerance, so it quite correctly emits one layer and makes a poor fixture
// for tests that need to inspect a stack.
func layeredStressSurface(w, h int) *ImageSurface {
	s := &ImageSurface{
		Width:  w,
		Height: h,
		Stride: w * 4,
		Pix:    make([]byte, w*h*4),
		Opaque: true,
	}
	state := uint32(0x12345678)
	for i := 0; i < len(s.Pix); i += 4 {
		state = state*1664525 + 1013904223
		s.Pix[i] = byte(state >> 24)
		state = state*1664525 + 1013904223
		s.Pix[i+1] = byte(state >> 24)
		state = state*1664525 + 1013904223
		s.Pix[i+2] = byte(state >> 24)
		s.Pix[i+3] = 0xFF
	}
	return s
}

func layeredSixel() *sixelEncoder {
	e := newSixelEncoderWith(func(k string) string {
		if k == "VTUI_SIXEL_PALETTE" {
			return "layered"
		}
		return ""
	})
	e.cellSize = func(cw, ch int) (int, int) { return 10, 20 }
	return e
}

func TestLayeredSendsSeveralImages(t *testing.T) {
	e := layeredSixel()
	surf := layeredStressSurface(64, 48)

	layers := e.encodeLayers(surf, 0, 0, 64, 48, 64, 48)
	if len(layers) < 2 {
		t.Fatalf("got %d layer(s), want the stress image split across several", len(layers))
	}
	if len(layers) > sixelLayerMax {
		t.Errorf("got %d layers, want at most %d", len(layers), sixelLayerMax)
	}
	for i, l := range layers {
		if !strings.HasPrefix(l, "\x1bP0;1;8q") {
			t.Errorf("layer %d does not open a DCS: %.16q", i, l)
		}
		// P2=1 is the whole mechanism: without it a layer paints its
		// untouched pixels with the background and erases the one below.
		if !strings.HasPrefix(l, "\x1bP0;1;") {
			t.Errorf("layer %d does not ask for transparency: %.16q", i, l)
		}
		if !strings.HasSuffix(l, "\x1b\\") {
			t.Errorf("layer %d does not close its DCS", i)
		}
	}
}

// Every layer goes to the same cell. A sixel dump leaves the text cursor at
// the sixel active position, so a stack that does not re-state the position
// walks down the screen.
func TestEveryLayerIsPositionedAgain(t *testing.T) {
	e := layeredSixel()
	surf := layeredStressSurface(64, 48)
	var sb strings.Builder
	e.Render(&sb, []ImagePlacement{{
		Surface: surf, Col: 4, Row: 2, Cols: 6, Rows: 3,
	}}, 10, 20)

	out := sb.String()
	images := sixelLayerCount(out)
	if images < 2 {
		t.Fatalf("got %d image(s) in the stream, want a stack", images)
	}
	if got := strings.Count(out, "\x1b[3;5H"); got != images {
		t.Errorf("got %d cursor moves for %d images, want one each", got, images)
	}
}

// One layer is what every other mode sends, and the layered path must not
// change a single byte of it.
func TestOtherModesStillSendOneImage(t *testing.T) {
	surf := gradientSurface(32, 24)
	for _, mode := range []string{"", "fixed", "adaptive"} {
		e := newSixelEncoderWithOS(func(k string) string {
			if k == "VTUI_SIXEL_PALETTE" {
				return mode
			}
			return ""
		}, "linux")
		layers := e.encodeLayers(surf, 0, 0, 32, 24, 32, 24)
		if len(layers) != 1 {
			t.Errorf("%q: got %d layers, want exactly one", mode, len(layers))
			continue
		}
		if layers[0] != e.encode(surf, 0, 0, 32, 24, 32, 24) {
			t.Errorf("%q: the stack is not what encode says", mode)
		}
	}
}

// Windows Terminal and native OpenConsole are the terminals this exists for,
// and neither must have to be asked. WezTerm and foot are the known terminals
// where per-band palettes are safe; an unknown terminal gets one adaptive
// palette instead.
func TestWindowsTerminalGetsLayersByDefault(t *testing.T) {
	wt := newSixelEncoderWithOS(func(k string) string {
		if k == "WT_SESSION" {
			return "e4a1f0c2-0000-0000-0000-000000000000"
		}
		return ""
	}, "linux")
	if !wt.layered {
		t.Error("Windows Terminal did not get the layered encoder")
	}
	if wt.trueColor {
		t.Error("Windows Terminal got the full-colour stream as well")
	}

	elsewhere := newSixelEncoderWithOS(func(string) string { return "" }, "linux")
	if elsewhere.layered {
		t.Error("a terminal that is not Windows Terminal got layers")
	}
	if elsewhere.trueColor || !elsewhere.adaptive {
		t.Errorf("unknown terminal got layered=%v trueColor=%v adaptive=%v, want false false true", elsewhere.layered, elsewhere.trueColor, elsewhere.adaptive)
	}

	openConsole := newSixelEncoderWithOS(func(string) string { return "" }, "windows")
	if !openConsole.layered {
		t.Error("native OpenConsole did not get the layered encoder")
	}
	if openConsole.trueColor {
		t.Error("native OpenConsole got the per-band full-colour stream")
	}

	wezterm := newSixelEncoderWithOS(func(k string) string {
		if k == "WEZTERM_PANE" {
			return "0"
		}
		return ""
	}, "windows")
	if wezterm.layered || !wezterm.trueColor {
		t.Errorf("wezterm on Windows got layered=%v trueColor=%v, want false true", wezterm.layered, wezterm.trueColor)
	}

	foot := newSixelEncoderWithOS(func(k string) string {
		if k == "TERM" {
			return "foot-extra"
		}
		return ""
	}, "linux")
	if foot.layered || !foot.trueColor {
		t.Errorf("foot got layered=%v trueColor=%v, want false true", foot.layered, foot.trueColor)
	}
}

// An explicit setting is the reader's, including the one that turns layering
// off where it would otherwise be chosen.
func TestAnExplicitModeBeatsTheTerminal(t *testing.T) {
	e := newSixelEncoderWith(func(k string) string {
		switch k {
		case "WT_SESSION":
			return "1"
		case "VTUI_SIXEL_PALETTE":
			return "adaptive"
		}
		return ""
	})
	if e.layered || !e.adaptive {
		t.Errorf("layered=%v adaptive=%v, want the setting honoured", e.layered, e.adaptive)
	}
}

// The first layer covers everything, so a stack cut short is still a whole
// picture. This is what lets the budget be enforced by simply stopping.
func TestTheFirstLayerCoversThePicture(t *testing.T) {
	e := layeredSixel()
	surf := gradientSurface(48, 36)
	layers := e.encodeLayers(surf, 0, 0, 48, 36, 48, 36)
	if len(layers) == 0 {
		t.Fatal("nothing was encoded")
	}
	// Transparency in the first layer would be a hole: the picture is
	// opaque, so every pixel has to be painted by it.
	if strings.Contains(layers[0], "!") && !strings.Contains(layers[0], "#") {
		t.Fatal("the first layer looks empty")
	}
	if n := strings.Count(layers[0], "#"); n < 2 {
		t.Errorf("the first layer uses %d register(s), want a palette", n)
	}
}

// A layer that costs more than the budget leaves is not sent. The picture is
// already complete without it.
func TestTheBudgetStopsTheStack(t *testing.T) {
	e := layeredSixel()
	surf := layeredStressSurface(64, 48)
	full := e.encodeLayers(surf, 0, 0, 64, 48, 64, 48)
	if len(full) < 2 {
		t.Skip("this picture does not need a second layer")
	}

	e2 := layeredSixel()
	e2.layerBudget = len(full[0]) + 1
	trimmed := e2.encodeLayers(surf, 0, 0, 64, 48, 64, 48)
	if len(trimmed) != 1 {
		t.Errorf("got %d layers under a one-layer budget, want 1", len(trimmed))
	}
	if sixelLayerBytes(trimmed) > len(full[0])+1 {
		t.Errorf("the budget was exceeded: %d bytes", sixelLayerBytes(trimmed))
	}
}
