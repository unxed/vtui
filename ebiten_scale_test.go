//go:build (linux || windows || darwin) && !android && (amd64 || arm64) && !vtui_noebiten

package vtui

import (
	"image"
	"testing"
)

func TestEbitenScale(t *testing.T) {
	for _, tt := range []struct {
		in   float64
		want int
	}{{1, 1}, {1.25, 1}, {1.5, 2}, {2, 2}, {3, 3}, {0, 1}, {0.5, 1}} {
		if got := ebitenScale(tt.in); got != tt.want {
			t.Errorf("ebitenScale(%v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

// Moving the window to a 200% monitor reloads the font at twice the size,
// keeps the grid, and queues a window size that Layout -- given back in
// device-independent pixels -- turns into the same cols x rows.
func TestEbitenHost_CheckScaleKeepsGrid(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	_, cellW, cellH := loadBestFont("", 16, 72)
	host := &EbitenHost{cols: 10, rows: 5, cellW: cellW, cellH: cellH, scale: 1,
		fontSize: 16, scr: scr, winW: 10 * cellW, winH: 5 * cellH}
	renderer := NewEbitenRenderer(host, nil, cellW, cellH, 1)
	renderer.glyphCache[glyphKey{}] = &image.RGBA{}
	host.renderer = renderer
	_, wantW, wantH := loadBestFont("", 32, 72)

	if host.checkScale(1.0) {
		t.Fatal("an unchanged scale was reported as a change")
	}
	if !host.checkScale(2.0) {
		t.Fatal("a scale change from 1 to 2 was not reported")
	}
	if host.scale != 2 || renderer.scale != 2 {
		t.Fatalf("scale host/renderer = %d/%d, want 2/2", host.scale, renderer.scale)
	}
	if host.cellW != wantW || host.cellH != wantH {
		t.Fatalf("cell = %dx%d, want %dx%d", host.cellW, host.cellH, wantW, wantH)
	}
	if cw, ch := scr.Graphics().CellSize(); cw != wantW || ch != wantH {
		t.Fatalf("graphics cell = %dx%d, want %dx%d", cw, ch, wantW, wantH)
	}
	if len(renderer.glyphCache) != 0 {
		t.Error("glyph cache was not cleared")
	}
	if !host.pendingSize.valid || host.pendingSize.w != 10*wantW || host.pendingSize.h != 5*wantH {
		t.Fatalf("pending size = %+v, want %dx%d", host.pendingSize, 10*wantW, 5*wantH)
	}

	// Update sets the window to pending/scale DIPs; Layout then gets them back.
	g := &ebitenGame{host: host}
	g.Layout(ebitenDIPs(host.pendingSize.w, 2), ebitenDIPs(host.pendingSize.h, 2))
	if host.cols != 10 || host.rows != 5 {
		t.Fatalf("grid after Layout = %dx%d, want 10x5", host.cols, host.rows)
	}

	if host.checkScale(2.2) {
		t.Error("a factor rounding to the same scale was reported as a change")
	}
}

// The DIP size handed to SetWindowSize rounds up, so Layout gets back at
// least the device pixels the grid needs.
func TestEbitenDIPs(t *testing.T) {
	for _, tt := range []struct{ px, scale, want int }{
		{165, 2, 83}, {164, 2, 82}, {100, 1, 100}, {100, 3, 34}, {100, 0, 100},
	} {
		if got := ebitenDIPs(tt.px, tt.scale); got != tt.want {
			t.Errorf("ebitenDIPs(%d, %d) = %d, want %d", tt.px, tt.scale, got, tt.want)
		}
		if tt.scale > 0 && ebitenDIPs(tt.px, tt.scale)*tt.scale < tt.px {
			t.Errorf("ebitenDIPs(%d, %d) loses pixels", tt.px, tt.scale)
		}
	}
}
