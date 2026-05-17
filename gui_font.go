//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows

package vtui

import (
	"os"
	"fmt"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

// loadBestFont attempts to find a suitable monospace TTF font on the system.
// If none is found, it falls back to a built-in bitmap font.
func loadBestFont(size float64, dpi float64) (font.Face, int, int) {
	// Paths for common Linux distributions and Windows
	fontPaths := []string{
		`C:\Windows\Fonts\consola.ttf`,
		`C:\Windows\Fonts\lucon.ttf`,
		`C:\Windows\Fonts\cour.ttf`,
		`C:\Windows\Fonts\arial.ttf`,
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
			fmt.Fprintf(os.Stderr, "GUI_FONT: Error parsing %s: %v\n", path, err)
			continue
		}

		face, err := opentype.NewFace(f, &opentype.FaceOptions{
			Size:    size,
			DPI:     dpi,
			Hinting: font.HintingFull,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "GUI_FONT: Error creating face for %s: %v\n", path, err)
			continue
		}

		metrics := face.Metrics()
		cellH := (metrics.Ascent + metrics.Descent).Ceil()
		advance, _ := face.GlyphAdvance('A')
		cellW := advance.Ceil()

		msg := fmt.Sprintf("GUI_FONT: Successfully loaded %s (%dx%d)", path, cellW, cellH)
		fmt.Fprintln(os.Stderr, msg)
		DebugLog(msg)
		return face, cellW, cellH
	}

	// Fallback to basicfont if no TTF found
	DebugLog("GUI_FONT: CRITICAL - No TTF font found! Falling back to basicfont 7x13 (ASCII only!)")
	return basicfont.Face7x13, 7, 13
}
