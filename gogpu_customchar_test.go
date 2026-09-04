//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"testing"

	"github.com/gogpu/gg"
)

// Proves which glyphs the GPU renderer draws as vector shapes vs which fall
// back to dc.DrawString. Fallback renders as tofu when the font lacks the
// codepoint - a real symptom on Windows (boxes instead of ┌┐└┘ ↑ ▓ ▒).
func TestGogpuRenderer_CustomCharVectorCoverage(t *testing.T) {
	cases := []struct {
		name string
		ch   rune
		want bool // true = must be vector-drawn, false = font fallback is OK
	}{
		// Everything the vtui/f4 widget set actually writes:
		{"h", '─', true}, {"v", '│', true},
		{"tl", '┌', true}, {"tr", '┐', true}, {"bl", '└', true}, {"br", '┘', true},
		{"tee-l", '├', true}, {"tee-r", '┤', true}, {"tee-t", '┬', true}, {"tee-b", '┴', true}, {"cross", '┼', true},
		{"dh", '═', true}, {"dv", '║', true}, {"dtl", '╔', true}, {"dtr", '╗', true},
		{"dbl", '╚', true}, {"dbr", '╝', true}, {"dtee-l", '╠', true}, {"dtee-r", '╣', true},
		{"dtee-t", '╦', true}, {"dtee-b", '╩', true}, {"dcross", '╬', true},
		{"vmenu-l", '╟', true}, {"vmenu-r", '╢', true},
		{"up", '↑', true}, {"down", '↓', true}, {"updown", '↕', true},
		{"tri-up", '▲', true}, {"tri-down", '▼', true},
		{"full", '█', true}, {"upper", '▀', true}, {"lower", '▄', true},
		{"left", '▌', true}, {"right", '▐', true},
		{"shade-light", '░', true}, {"shade-med", '▒', true}, {"shade-dark", '▓', true},
		{"arrow-r", '→', true}, {"arrow-l", '←', true}, {"arrow-both", '↔', true},
		{"q-tl", '▘', true}, {"q-tr", '▗', true}, {"q-bl", '▖', true}, {"q-br", '▝', true},
		{"bar-top", '▔', true}, {"bar-right", '▕', true},
		{"space", ' ', false}, {"letter", 'A', false},
	}

	// Assert the whole used set is vector-drawn; anything outside the switch
	// falls back to the font (possible tofu).
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewGogpuRenderer(nil, nil, 8, 16)
			dc := gg.NewContext(8, 16)
			dc.SetRGB(1, 1, 1)
			dc.Clear()
			ok := r.drawCustomChar(dc, tc.ch, 0, 0, 8, 16, 0)
			if ok != tc.want {
				t.Errorf("drawCustomChar(%q) = %v, want %v", tc.ch, ok, tc.want)
			}
			if !ok {
				return
			}
			img := dc.Image()
			nonWhite := 0
			for y := 0; y < 16; y++ {
				for x := 0; x < 8; x++ {
					r32, g32, b32, _ := img.At(x, y).RGBA()
					if r32 != 0xFFFF || g32 != 0xFFFF || b32 != 0xFFFF {
						nonWhite++
					}
				}
			}
			if nonWhite == 0 {
				t.Errorf("drawCustomChar(%q) claimed success but drew nothing", tc.ch)
			}
		})
	}
}
