//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package vtui

import (
	"os"
	"io"
	"path/filepath"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
	"github.com/unxed/vtinput"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
)

// RunInX11Window is an alternative entry point for vtui.
func RunInX11Window(cols, rows int, setupApp func()) error {
	fontSize := 22.0
	// Temporary host to detect DPI
	tempConn, _ := xgb.NewConn()
	dpi := 96.0
	if tempConn != nil {
		setup := xproto.Setup(tempConn)
		screen := setup.DefaultScreen(tempConn)
		if screen.WidthInMillimeters > 0 {
			dpi = (float64(screen.WidthInPixels) * 25.4) / float64(screen.WidthInMillimeters)
		}
		tempConn.Close()
	}

	face, cellW, cellH := loadBestFont(fontSize, dpi)

	host, err := NewX11Host(cols, rows, cellW, cellH)
	if err != nil {
		return err
	}
	defer host.Close()

	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = NewX11Renderer(host, face)

	FrameManager.Init(scr)

	// Use a pipe to create a blocking reader that never sends EOF
	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr)
	if reader.NativeEventChan == nil {
		reader.NativeEventChan = make(chan *vtinput.InputEvent, 1024)
	}
	host.reader = reader

	// Override global terminal size source to use X11 window metrics
	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	go host.RunEventLoop()
	setupApp()
	FrameManager.Run(reader)

	return nil
}

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

		DebugLog("X11: Loaded TTF font %s, metrics: %dx%d", filepath.Base(path), cellW, cellH)
		return face, cellW, cellH
	}

	// Fallback to basicfont if no TTF found
	DebugLog("X11: No TTF font found, falling back to basicfont 7x13")
	return basicfont.Face7x13, 7, 13
}