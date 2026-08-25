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
	surf := gradientSurface(64, 48)

	layers := e.encodeLayers(surf, 0, 0, 64, 48, 64, 48)
	if len(layers) < 2 {
		t.Fatalf("got %d layer(s), want the gradient split across several", len(layers))
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
	surf := gradientSurface(64, 48)
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
		e := newSixelEncoderWith(func(k string) string {
			if k == "VTUI_SIXEL_PALETTE" {
				return mode
			}
			return ""
		})
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

// Windows Terminal is the terminal this exists for, and it must not have to be
// asked. Everywhere else the full-colour stream is smaller and needs no
// compositing promise, so the default has to stay where it was.
func TestWindowsTerminalGetsLayersByDefault(t *testing.T) {
	wt := newSixelEncoderWith(func(k string) string {
		if k == "WT_SESSION" {
			return "e4a1f0c2-0000-0000-0000-000000000000"
		}
		return ""
	})
	if !wt.layered {
		t.Error("Windows Terminal did not get the layered encoder")
	}
	if wt.trueColor {
		t.Error("Windows Terminal got the full-colour stream as well")
	}

	elsewhere := newSixelEncoderWith(func(string) string { return "" })
	if elsewhere.layered {
		t.Error("a terminal that is not Windows Terminal got layers")
	}
	if !elsewhere.trueColor {
		t.Error("the full-colour default moved")
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
	surf := gradientSurface(64, 48)
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
