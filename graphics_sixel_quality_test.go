package vtui

import (
	"math"
	"strings"
	"testing"
)

// bandingSkySurface is a banding-prone synthetic picture: three smooth
// gradients in narrow hue ranges (sky, foliage, skin), where a fixed colour
// cube has only a few entries and an adaptive palette can concentrate all of
// its colours.
func bandingSkySurface() *ImageSurface {
	const w, h = 512, 256
	s := NewImageSurface(w, h)
	bands := []struct {
		y0, y1 int
		c0, c1 [3]float64
	}{
		{0, 85, [3]float64{95, 135, 175}, [3]float64{150, 190, 230}},
		{85, 170, [3]float64{18, 55, 28}, [3]float64{70, 110, 60}},
		{170, 256, [3]float64{175, 135, 95}, [3]float64{230, 190, 150}},
	}
	for _, b := range bands {
		for y := b.y0; y < b.y1; y++ {
			for x := 0; x < w; x++ {
				u := float64(x) / float64(w-1)
				r := b.c0[0] + (b.c1[0]-b.c0[0])*u
				g := b.c0[1] + (b.c1[1]-b.c0[1])*u
				bl := b.c0[2] + (b.c1[2]-b.c0[2])*u
				s.SetPixel(x, y,
					byte(clamp255(int32(r+0.5))),
					byte(clamp255(int32(g+0.5))),
					byte(clamp255(int32(bl+0.5))),
					255)
			}
		}
	}
	s.Opaque = true
	return s
}

// quantizeFixed reconstructs the RGB the fixed-palette ditherer would paint.
func quantizeFixed(surf *ImageSurface) (rgb []byte, nColors int) {
	idx := make([]byte, surf.Width*surf.Height)
	sixelQuantize(surf, idx)
	rgb = make([]byte, 0, surf.Width*surf.Height*3)
	used := make([]bool, 256)
	for _, p := range idx {
		c := sixelPalette[p]
		rgb = append(rgb, c[0], c[1], c[2])
		if !used[p] {
			used[p] = true
			nColors++
		}
	}
	return rgb, nColors
}

// quantizeAdaptive reconstructs the RGB of the production adaptive path.
func quantizeAdaptive(surf *ImageSurface) (rgb []byte, nColors int) {
	palI, _, lut := adaptiveSixelPalette(surf)
	idx := make([]byte, surf.Width*surf.Height)
	sixelQuantizePal(surf, idx, lut, palI)

	rgb = make([]byte, 0, surf.Width*surf.Height*3)
	used := make([]bool, len(palI))
	for _, p := range idx {
		c := palI[p]
		rgb = append(rgb, byte(c[0]), byte(c[1]), byte(c[2]))
		if !used[p] {
			used[p] = true
			nColors++
		}
	}
	return rgb, nColors
}

// psnrRGB reports the peak signal-to-noise ratio of the reconstruction
// against the source surface. Higher is better; 30 dB is already a visible
// dither, 40 dB and above looks smooth.
func psnrRGB(surf *ImageSurface, got []byte) float64 {
	var sum float64
	for y := 0; y < surf.Height; y++ {
		for x := 0; x < surf.Width; x++ {
			r, g, b, _ := surf.PixelAt(x, y)
			o := (y*surf.Width + x) * 3
			dr := float64(int(r) - int(got[o]))
			dg := float64(int(g) - int(got[o+1]))
			db := float64(int(b) - int(got[o+2]))
			sum += dr*dr + dg*dg + db*db
		}
	}
	mse := sum / float64(surf.Width*surf.Height*3)
	return 10 * math.Log10(255*255/mse)
}

func TestSixelAdaptivePaletteQuality(t *testing.T) {
	surf := bandingSkySurface()

	fixedRGB, fixedColors := quantizeFixed(surf)
	adaptRGB, adaptColors := quantizeAdaptive(surf)
	fixedPSNR := psnrRGB(surf, fixedRGB)
	adaptPSNR := psnrRGB(surf, adaptRGB)

	// The fixed cube has ~1 colour per narrow gamut, so it bands; the
	// adaptive palette must clear that by a wide margin.
	if adaptPSNR <= fixedPSNR+10 {
		t.Errorf("adaptive palette should beat the fixed cube on banding content: %.2f dB vs %.2f dB (%d vs %d colours)",
			adaptPSNR, fixedPSNR, adaptColors, fixedColors)
	}
}

func TestSixelAdaptivePaletteSelection(t *testing.T) {
	unknownTerminal := func(string) string { return "" }
	if !newSixelEncoderWithOS(unknownTerminal, "linux").adaptive {
		t.Error("adaptive must be the conservative default for an unknown terminal")
	}
	if !newSixelEncoderWithOS(func(string) string { return "adaptive" }, "linux").adaptive {
		t.Error("VTUI_SIXEL_PALETTE=adaptive must enable adaptive mode")
	}
	if !newSixelEncoderWithOS(func(string) string { return " ADAPTIVE " }, "linux").adaptive {
		t.Error("adaptive must be case and space insensitive")
	}
}

func TestSixelAdaptiveEncoderUsesImagePalette(t *testing.T) {
	surf := NewImageSurface(4, 4)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			surf.SetPixel(x, y, 128, 128, 128, 255)
		}
	}
	surf.Opaque = true

	// The fixed cube and its greys never contain 128 (nearest greys are 127
	// and 134), so it cannot emit the image's exact colour as 50%. It has
	// to be asked for explicitly: the per-band encoder would reproduce the
	// grey exactly, which is the point of it.
	fixed := newSixelEncoder()
	fixed.trueColor = false
	fixed.adaptive = false
	fixedOut := fixed.encode(surf, 0, 0, 4, 4, 4, 4)
	if strings.Contains(fixedOut, ";2;50;50;50") {
		t.Errorf("fixed palette must not quantise 128 grey to itself: %q", fixedOut)
	}

	adapt := newSixelEncoder()
	adapt.trueColor = false
	adapt.adaptive = true
	adaptOut := adapt.encode(surf, 0, 0, 4, 4, 4, 4)
	if !strings.Contains(adaptOut, ";2;50;50;50") {
		t.Errorf("adaptive palette must include the image's exact 128 grey: %q", adaptOut)
	}
	if !strings.HasPrefix(adaptOut, "\x1bP0;1;8q\"1;1;4;4") {
		t.Errorf("adaptive sixel header malformed: %q", adaptOut)
	}
	if !strings.HasSuffix(adaptOut, "\x1b\\") {
		t.Error("adaptive sixel must end with ST")
	}
}
