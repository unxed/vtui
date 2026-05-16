//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd

package vtui

import (
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

// loadBestFont attempts to find a suitable monospace TTF font on the system.
// If none is found, it falls back to a built-in bitmap font.
func loadBestFont(size float64, dpi float64) (font.Face, int, int) {
	// Paths for common Linux distributions
	fontPaths := []string{
		"/usr/share/fonts/truetype/ubuntu/UbuntuMono-R.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf",
		"/usr/share/fonts/truetype/liberation/LiberationMono-Regular.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono.ttf",
		"/System/Library/Fonts/Supplemental/Courier New.ttf", // macOS path
	}

	for _, path := range fontPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		f, err := opentype.Parse(data)
		if err != nil {
			continue
		}

		face, err := opentype.NewFace(f, &opentype.FaceOptions{
			Size:    size,
			DPI:     dpi,
			Hinting: font.HintingFull,
		})
		if err != nil {
			continue
		}

		// Calculate cell size from metrics
		metrics := face.Metrics()
		cellH := (metrics.Ascent + metrics.Descent).Ceil()

		// For monospaced fonts, advance of any character (e.g. 'A') is the cell width
		advance, _ := face.GlyphAdvance('A')
		cellW := advance.Ceil()

		DebugLog("GUI: Loaded TTF font %s, metrics: %dx%d", filepath.Base(path), cellW, cellH)
		return face, cellW, cellH
	}

	// Fallback to basicfont if no TTF found
	DebugLog("GUI: No TTF font found, falling back to basicfont 7x13")
	return basicfont.Face7x13, 7, 13
}
