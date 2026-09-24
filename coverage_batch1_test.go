package vtui

import (
	"image"
	"image/color"
	"testing"

	"github.com/unxed/vtinput"
)

func TestKeysymToVKDirectMapping(t *testing.T) {
	if got := keysymToVK(0xffbe); got != vtinput.VK_F1 {
		t.Fatalf("F1 keysym mapped to %#x", got)
	}
}

func TestKeysymToVKLetters(t *testing.T) {
	cases := map[uint32]uint16{'a': 'A', 'z': 'Z', 'A': 'A', 'Z': 'Z'}
	for keysym, want := range cases {
		if got := keysymToVK(keysym); got != want {
			t.Errorf("keysym %#x mapped to %#x, want %#x", keysym, got, want)
		}
	}
}

func TestKeysymToVKDigits(t *testing.T) {
	for keysym := uint32('0'); keysym <= '9'; keysym++ {
		if got := keysymToVK(keysym); got != uint16(keysym) {
			t.Errorf("digit keysym %#x mapped to %#x", keysym, got)
		}
	}
}

func TestEnhancedKeyForX11Keysym(t *testing.T) {
	if got := enhancedKeyForX11Keysym(0xffff); got != vtinput.EnhancedKey {
		t.Fatalf("Delete enhancement = %#x", got)
	}
	if got := enhancedKeyForX11Keysym('A'); got != 0 {
		t.Fatalf("letter enhancement = %#x", got)
	}
}

func TestClearFrameMarginsNil(t *testing.T) {
	clearFrameMargins(nil, 1, 1)
}

func TestClearFrameMarginsClipsRight(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for i := range img.Pix {
		img.Pix[i] = 77
	}
	clearFrameMargins(img, 2, 3)
	if got := img.RGBAAt(1, 1); got != (color.RGBA{77, 77, 77, 77}) {
		t.Fatalf("grid pixel changed to %#v", got)
	}
	if got := img.RGBAAt(3, 1); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("right margin = %#v", got)
	}
}

func TestClearFrameMarginsClipsBottom(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	for i := range img.Pix {
		img.Pix[i] = 77
	}
	clearFrameMargins(img, 4, 1)
	if got := img.RGBAAt(1, 2); got != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("bottom margin = %#v", got)
	}
}

func TestDndCellInvalidSize(t *testing.T) {
	if got := dndCell(20, 0); got != 0 {
		t.Fatalf("zero cell size = %d", got)
	}
}

func TestDndCellNonPositivePixel(t *testing.T) {
	if got := dndCell(-1, 10); got != 0 {
		t.Fatalf("negative pixel = %d", got)
	}
}

func TestDndCellDividesPixels(t *testing.T) {
	if got := dndCell(25, 10); got != 2 {
		t.Fatalf("25px at 10px/cell = %d", got)
	}
}
