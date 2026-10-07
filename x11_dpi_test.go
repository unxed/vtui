//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"encoding/binary"
	"image"
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestParseXftDPI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want float64
	}{
		{"absent", "Xcursor.size:\t24\n", 0},
		{"plain", "Xft.dpi:\t192\n", 192},
		{"among others", "Xcursor.theme:\tAdwaita\nXft.antialias:\t1\nXft.dpi:\t144\nXft.hinting:\t1\n", 144},
		{"spaces around", "  Xft.dpi :  120  \n", 120},
		{"fractional", "Xft.dpi: 115.2", 115.2},
		{"not a number", "Xft.dpi: lots", 0},
		{"zero", "Xft.dpi: 0", 0},
		{"similar name", "Xft.dpiX: 300\n", 0},
	}
	for _, tt := range tests {
		if got := parseXftDPI(tt.in); got != tt.want {
			t.Errorf("%s: parseXftDPI = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// xsettingsBlob builds an _XSETTINGS_SETTINGS property in the given byte
// order from (type, name, value) entries, per the freedesktop spec.
type xsettingsEntry struct {
	typ   byte
	name  string
	ival  int32
	sval  string
	color [4]uint16
}

func xsettingsBlob(order binary.AppendByteOrder, entries []xsettingsEntry) []byte {
	pad := func(b []byte) []byte {
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
		return b
	}
	b := []byte{0, 0, 0, 0}
	if order == binary.AppendByteOrder(binary.BigEndian) {
		b[0] = 1
	}
	b = order.AppendUint32(b, 7) // serial
	b = order.AppendUint32(b, uint32(len(entries))) // #nosec G115 -- test fixture: a handful of entries
	for _, e := range entries {
		b = append(b, e.typ, 0)
		b = order.AppendUint16(b, uint16(len(e.name))) // #nosec G115 -- test fixture: short setting names
		b = pad(append(b, e.name...))
		b = order.AppendUint32(b, 1) // last-change serial
		switch e.typ {
		case xsettingsTypeInteger:
			b = order.AppendUint32(b, uint32(e.ival)) // #nosec G115 -- deliberate: XSETTINGS stores the signed value as its bit pattern
		case xsettingsTypeString:
			b = order.AppendUint32(b, uint32(len(e.sval))) // #nosec G115 -- test fixture: short string values
			b = pad(append(b, e.sval...))
		case xsettingsTypeColor:
			for _, c := range e.color {
				b = order.AppendUint16(b, c)
			}
		}
	}
	return b
}

func TestParseXSettingsDPI(t *testing.T) {
	gnome2x := []xsettingsEntry{
		{typ: xsettingsTypeString, name: "Net/ThemeName", sval: "Adwaita"},
		{typ: xsettingsTypeColor, name: "Gtk/Color", color: [4]uint16{1, 2, 3, 4}},
		{typ: xsettingsTypeInteger, name: "Gdk/WindowScalingFactor", ival: 2},
		{typ: xsettingsTypeInteger, name: "Xft/DPI", ival: 192 * 1024},
	}
	for _, order := range []binary.AppendByteOrder{binary.LittleEndian, binary.BigEndian} {
		if got := parseXSettingsDPI(xsettingsBlob(order, gnome2x)); got != 192 {
			t.Errorf("%v: Xft/DPI = %v, want 192", order, got)
		}
	}

	noDPI := []xsettingsEntry{{typ: xsettingsTypeString, name: "Net/ThemeName", sval: "Adwaita"}}
	if got := parseXSettingsDPI(xsettingsBlob(binary.LittleEndian, noDPI)); got != 0 {
		t.Errorf("no Xft/DPI: got %v, want 0", got)
	}

	// The "unset" value -1 and malformed data must not produce a DPI.
	unset := []xsettingsEntry{{typ: xsettingsTypeInteger, name: "Xft/DPI", ival: -1}}
	if got := parseXSettingsDPI(xsettingsBlob(binary.LittleEndian, unset)); got != 0 {
		t.Errorf("Xft/DPI -1: got %v, want 0", got)
	}
	full := xsettingsBlob(binary.LittleEndian, gnome2x)
	for n := 0; n < len(full); n++ {
		// Truncation anywhere must neither panic nor invent a value.
		if got := parseXSettingsDPI(full[:n]); got != 0 {
			t.Errorf("truncated to %d bytes: got %v, want 0", n, got)
		}
	}
	if got := parseXSettingsDPI(nil); got != 0 {
		t.Errorf("nil: got %v, want 0", got)
	}
}

func TestX11FontDPI(t *testing.T) {
	tests := []struct {
		dpi         float64
		wantFontDPI float64
		wantScale   int
	}{
		{96, 72, 1},
		{192, 144, 2},
		{144, 108, 2},
		{120, 90, 1},
		{0, 72, 1},
	}
	for _, tt := range tests {
		fontDPI, scale := x11FontDPI(tt.dpi)
		if fontDPI != tt.wantFontDPI || scale != tt.wantScale {
			t.Errorf("x11FontDPI(%v) = %v, %d; want %v, %d", tt.dpi, fontDPI, scale, tt.wantFontDPI, tt.wantScale)
		}
	}
}

// A desktop DPI change reloads the font at the new DPI, keeps the grid and
// resets the renderer's caches; the same DPI again changes nothing.
func TestX11Host_ApplyDPIKeepsGrid(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	_, cellW, cellH := loadBestFont("", 16, 72)
	host := &X11Host{cols: 10, rows: 5, cellW: cellW, cellH: cellH, scale: 1,
		fontSize: 16, dpi: 72, scr: scr}
	renderer := NewX11Renderer(host, nil)
	renderer.glyphCache[glyphKey{}] = &image.RGBA{}
	renderer.gfxKnown = true
	host.renderer = renderer
	_, wantW, wantH := loadBestFont("", 16, 144)

	host.mu.Lock()
	changed := host.applyDPILocked(192)
	host.mu.Unlock()

	if !changed {
		t.Fatal("applyDPILocked(192) on a 96 DPI host reported no change")
	}
	if host.dpi != 144 || host.scale != 2 {
		t.Fatalf("dpi/scale = %v/%d, want 144/2", host.dpi, host.scale)
	}
	if host.cellW != wantW || host.cellH != wantH {
		t.Fatalf("cell = %dx%d, want %dx%d", host.cellW, host.cellH, wantW, wantH)
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

	host.mu.Lock()
	again := host.applyDPILocked(192)
	host.mu.Unlock()
	if again {
		t.Error("the same DPI twice reported a second change")
	}
}

// After a rescale, the ConfigureNotify answering the grid-sized
// ConfigureWindow keeps cols x rows, and mouse coordinates map to cells at
// the new size.
func TestX11Host_ConfigureAfterRescaleKeepsGrid(t *testing.T) {
	host := &X11Host{cols: 10, rows: 5, cellW: 9, cellH: 17, width: 90, height: 85}
	host.cellW, host.cellH = 18, 34 // as applyDPILocked would at 200%

	if !host.handleConfigure(180, 170) {
		t.Fatal("a new pixel size was not reported as a change")
	}
	if host.cols != 10 || host.rows != 5 {
		t.Fatalf("grid = %dx%d, want 10x5", host.cols, host.rows)
	}
	if host.handleConfigure(180, 170) {
		t.Error("the same pixel size was reported as a change")
	}
	if cw, ch := host.cellSize(); cw != 18 || ch != 34 {
		t.Errorf("cellSize = %dx%d, want 18x34", cw, ch)
	}
}

func TestX11DPIWatch_Matching(t *testing.T) {
	w := &x11DPIWatch{root: 1, resourceManager: 10, manager: 11, xsettingsSel: 12,
		xsettingsProp: 13, xsettingsOwner: 5}

	if !w.isDPIProperty(1, 10) {
		t.Error("RESOURCE_MANAGER on the root was not recognised")
	}
	if !w.isDPIProperty(5, 13) {
		t.Error("_XSETTINGS_SETTINGS on the manager window was not recognised")
	}
	if w.isDPIProperty(5, 10) || w.isDPIProperty(1, 13) || w.isDPIProperty(2, 10) {
		t.Error("a property on the wrong window was recognised")
	}

	ev := xproto.ClientMessageEvent{Format: 32, Window: 1, Type: 11}
	ev.Data = xproto.ClientMessageDataUnionData32New([]uint32{0, 12, 99, 0, 0})
	if !w.isNewXSettingsManager(&ev) {
		t.Error("MANAGER for _XSETTINGS_S0 was not recognised")
	}
	ev.Data = xproto.ClientMessageDataUnionData32New([]uint32{0, 77, 99, 0, 0})
	if w.isNewXSettingsManager(&ev) {
		t.Error("MANAGER for another selection was recognised")
	}

	// A watch whose atoms could not be interned (all 0) matches nothing.
	empty := &x11DPIWatch{root: 1}
	if empty.isDPIProperty(1, 0) || empty.isDPIProperty(0, 0) {
		t.Error("a watch without atoms matched a property")
	}
}

// A maximized or tiled window cannot be resized to keep the grid: the
// window manager answers the grid-sized ConfigureWindow with the size it
// keeps. The grid must then follow the window, although its pixel size did
// not change, or at 200% only a quarter of the UI would show.
func TestX11Host_RefusedResizeAfterRescaleRefitsGrid(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	host := &X11Host{cols: 10, rows: 5, cellW: 9, cellH: 17, width: 90, height: 85,
		fontSize: 16, dpi: 72, scr: scr}
	host.renderer = NewX11Renderer(host, nil)

	host.mu.Lock()
	host.applyDPILocked(192)
	cellW, cellH := host.cellW, host.cellH
	host.mu.Unlock()
	if !host.gridStale {
		t.Fatal("a font reload did not mark the grid stale")
	}

	// The WM keeps the window at 90x85.
	if !host.handleConfigure(90, 85) {
		t.Fatal("a refused resize after a rescale did not refit the grid")
	}
	if wantCols, wantRows := 90/cellW, 85/cellH; host.cols != wantCols || host.rows != wantRows {
		t.Fatalf("grid = %dx%d, want %dx%d (window 90x85 at %dx%d cells)", host.cols, host.rows, wantCols, wantRows, cellW, cellH)
	}
	if host.gridStale {
		t.Error("the grid stayed stale after being refitted")
	}
	if host.handleConfigure(90, 85) {
		t.Error("a second same-size configure was reported as a change")
	}
}
