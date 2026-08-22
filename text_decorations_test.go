package vtui

import (
	"image"
	"testing"
)

func TestDrawUnderlineUsesForegroundOnBottomRows(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	drawUnderline(img, 2, 1, 4, 5, 1, 0x123456)

	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			p := img.RGBAAt(x, y)
			underlined := y == 5 && x >= 2 && x < 6
			if underlined && (p.R != 0x12 || p.G != 0x34 || p.B != 0x56 || p.A != 0xff) {
				t.Fatalf("pixel (%d,%d) = %#v, want underline colour", x, y, p)
			}
			if !underlined && p.A != 0 {
				t.Fatalf("pixel (%d,%d) = %#v, want untouched pixel", x, y, p)
			}
		}
	}
}

func TestDrawUnderlineScalesThickness(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	drawUnderline(img, 0, 0, 4, 6, 2, 0xffffff)
	for y := 0; y < 4; y++ {
		if img.RGBAAt(1, y).A != 0 {
			t.Fatalf("row %d unexpectedly contains underline pixels", y)
		}
	}
	for y := 4; y < 6; y++ {
		if img.RGBAAt(1, y).A != 0xff {
			t.Fatalf("row %d is missing scaled underline", y)
		}
	}
}
