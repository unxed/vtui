//go:build windows

package vtui

import (
	"image"
	"testing"
)

func TestWin32DPIScale(t *testing.T) {
	tests := []struct {
		dpi         float64
		wantFontDPI float64
		wantScale   int
	}{
		{96, 72, 1},
		{120, 90, 1},
		{144, 108, 2},
		{192, 144, 2},
		{288, 216, 3},
		// Below 100% the font is not scaled down, as before.
		{72, 72, 1},
		// An unreadable DPI falls back to 100%.
		{0, 72, 1},
	}
	for _, tt := range tests {
		fontDPI, scale := win32DPIScale(tt.dpi)
		if fontDPI != tt.wantFontDPI || scale != tt.wantScale {
			t.Errorf("win32DPIScale(%v) = %v, %d; want %v, %d", tt.dpi, fontDPI, scale, tt.wantFontDPI, tt.wantScale)
		}
	}
}

func newWin32DPITestHost(t *testing.T) (*Win32GuiHost, *Win32GuiRenderer, *ScreenBuf) {
	t.Helper()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	fontDPI, scale := win32DPIScale(96)
	_, cellW, cellH := loadBestFont("", 16, fontDPI)
	host := &Win32GuiHost{
		cols:     10,
		rows:     5,
		cellW:    cellW,
		cellH:    cellH,
		scale:    scale,
		fontName: "",
		fontSize: 16,
		fontDPI:  fontDPI,
		scr:      scr,
	}
	renderer := NewWin32GuiRenderer(host, nil, cellW, cellH)
	renderer.glyphCache[glyphKey{}] = &image.RGBA{}
	renderer.gfxKnown = true
	host.renderer = renderer
	return host, renderer, scr
}

// Moving from a 100% to a 200% monitor reloads the font at twice the DPI,
// hands the new cell size and line scale to the renderer and the graphics
// layer, and keeps the grid: the window is resized around it, the panels
// are not reflowed.
func TestWin32GuiHost_ApplyDPIRescalesAndKeepsGrid(t *testing.T) {
	host, renderer, scr := newWin32DPITestHost(t)
	oldW, oldH := host.cellW, host.cellH
	_, wantW, wantH := loadBestFont("", 16, 144)

	host.mu.Lock()
	changed := host.applyDPILocked(192)
	host.mu.Unlock()

	if !changed {
		t.Fatal("applyDPILocked(192) on a 96 DPI host reported no change")
	}
	if host.fontDPI != 144 || host.scale != 2 {
		t.Fatalf("fontDPI/scale = %v/%d, want 144/2", host.fontDPI, host.scale)
	}
	if host.cellW != wantW || host.cellH != wantH {
		t.Fatalf("host cell = %dx%d, want %dx%d", host.cellW, host.cellH, wantW, wantH)
	}
	if renderer.cellW != wantW || renderer.cellH != wantH || renderer.scale != 2 {
		t.Fatalf("renderer cell/scale = %dx%d/%d, want %dx%d/2", renderer.cellW, renderer.cellH, renderer.scale, wantW, wantH)
	}
	if cw, ch := scr.Graphics().CellSize(); cw != wantW || ch != wantH {
		t.Fatalf("graphics cell = %dx%d, want %dx%d", cw, ch, wantW, wantH)
	}
	if len(renderer.glyphCache) != 0 || renderer.gfxKnown {
		t.Error("renderer caches were not reset")
	}
	if host.cols != 10 || host.rows != 5 {
		t.Fatalf("grid = %dx%d, want unchanged 10x5", host.cols, host.rows)
	}
	t.Logf("cell %dx%d at 100%% -> %dx%d at 200%%", oldW, oldH, host.cellW, host.cellH)
	// With a real font installed the cells must grow; the built-in bitmap
	// fallback (7x13) has a single size and cannot.
	if (oldW != 7 || oldH != 13) && (host.cellW <= oldW || host.cellH <= oldH) {
		t.Errorf("cells did not grow at 200%%: %dx%d -> %dx%d", oldW, oldH, host.cellW, host.cellH)
	}
}

// A DPI that maps to the font DPI already in use changes nothing, so a
// WM_DPICHANGED between two monitors of the same scale, or a resolution
// change that keeps the scale, does not reload the font.
func TestWin32GuiHost_ApplySameDPIIsNoop(t *testing.T) {
	host, renderer, _ := newWin32DPITestHost(t)
	host.mu.Lock()
	changed := host.applyDPILocked(96)
	host.mu.Unlock()
	if changed {
		t.Fatal("applyDPILocked(96) on a 96 DPI host reported a change")
	}
	if len(renderer.glyphCache) == 0 || !renderer.gfxKnown {
		t.Error("renderer caches were reset although nothing changed")
	}
}

// Going back down from 200% to 100% restores the original cell size.
func TestWin32GuiHost_ApplyDPIRoundTrip(t *testing.T) {
	host, _, _ := newWin32DPITestHost(t)
	origW, origH := host.cellW, host.cellH
	host.mu.Lock()
	host.applyDPILocked(192)
	host.applyDPILocked(96)
	host.mu.Unlock()
	if host.cellW != origW || host.cellH != origH || host.scale != 1 {
		t.Fatalf("after 192 -> 96: cell %dx%d scale %d, want %dx%d scale 1", host.cellW, host.cellH, host.scale, origW, origH)
	}
}

// handleDPIChange works before a window exists (hwnd 0, as in every unit
// test here): the font is rescaled and nothing panics.
func TestWin32GuiHost_HandleDPIChangeWithoutWindow(t *testing.T) {
	host, _, _ := newWin32DPITestHost(t)
	suggested := &win32Rect{left: 100, top: 50, right: 900, bottom: 650}
	host.handleDPIChange(0, 192, suggested)
	if host.fontDPI != 144 {
		t.Fatalf("fontDPI = %v, want 144", host.fontDPI)
	}
	if host.cols != 10 || host.rows != 5 {
		t.Fatalf("grid = %dx%d, want unchanged 10x5", host.cols, host.rows)
	}
}

// The outer window rectangle holds the whole grid and sits where asked.
func TestGridWindowRect(t *testing.T) {
	for _, dpi := range []float64{96, 144, 192} {
		rc := gridWindowRect(100, 40, 80, 25, 9, 18, dpi)
		if rc.left != 100 || rc.top != 40 {
			t.Errorf("dpi %v: origin = %d,%d, want 100,40", dpi, rc.left, rc.top)
		}
		if w, h := rc.right-rc.left, rc.bottom-rc.top; w < 80*9 || h < 25*18 {
			t.Errorf("dpi %v: outer %dx%d smaller than the %dx%d client area", dpi, w, h, 80*9, 25*18)
		}
	}
}

func TestWin32DPIQueriesReturnUsableValues(t *testing.T) {
	if dpi := primaryMonitorDPI(); dpi <= 0 {
		t.Errorf("primaryMonitorDPI() = %v, want > 0", dpi)
	}
	if dpi := win32WindowDPI(0); dpi <= 0 {
		t.Errorf("win32WindowDPI(0) = %v, want > 0", dpi)
	}
}
