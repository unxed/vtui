package vtui

import (
	"testing"
)

func TestWin32Gui_DIBInfoGeneration(t *testing.T) {
	bmi := makeTopDownDIBInfo(100, 50)
	if bmi.bmiHeader.biWidth != 100 {
		t.Errorf("expected DIB width 100, got %d", bmi.bmiHeader.biWidth)
	}
	if bmi.bmiHeader.biHeight != -50 {
		t.Errorf("expected negative top-down DIB height -50, got %d", bmi.bmiHeader.biHeight)
	}
	if bmi.bmiHeader.biBitCount != 32 {
		t.Errorf("expected 32-bit DIB, got %d", bmi.bmiHeader.biBitCount)
	}
}

func TestWin32Gui_RGBAToBGRAConversion(t *testing.T) {
	src := []byte{
		0x11, 0x22, 0x33, 0xFF, // Pixel 0: R=11, G=22, B=33
		0xAA, 0xBB, 0xCC, 0xFF, // Pixel 1: R=AA, G=BB, B=CC
	}
	dst := make([]byte, len(src))
	rgbaToBGRA(dst, src, len(src))

	// Pixel 0 BGRA: B=33, G=22, R=11, A=FF
	if dst[0] != 0x33 || dst[1] != 0x22 || dst[2] != 0x11 || dst[3] != 0xFF {
		t.Errorf("pixel 0 BGRA mismatch: %02X %02X %02X %02X", dst[0], dst[1], dst[2], dst[3])
	}
	// Pixel 1 BGRA: B=CC, G=BB, R=AA, A=FF
	if dst[4] != 0xCC || dst[5] != 0xBB || dst[6] != 0xAA || dst[7] != 0xFF {
		t.Errorf("pixel 1 BGRA mismatch: %02X %02X %02X %02X", dst[4], dst[5], dst[6], dst[7])
	}
}

func TestWin32GuiRenderer_Lifecycle(t *testing.T) {
	r := NewWin32GuiRenderer(nil, nil, 8, 16)
	buf, shadow := mkGrid(10, 4, ' ', 0)

	r.Render(buf, shadow, 10, 4, true)
	if r.imgBuf == nil {
		t.Fatal("expected allocated imgBuf after Render")
	}
	if r.imgBuf.Rect.Dx() != 80 || r.imgBuf.Rect.Dy() != 64 {
		t.Errorf("expected 80x64 pixels, got %dx%d", r.imgBuf.Rect.Dx(), r.imgBuf.Rect.Dy())
	}

	w, h, ok := r.syncBGRA()
	if !ok || w != 80 || h != 64 {
		t.Errorf("syncBGRA returned %dx%d (ok=%v)", w, h, ok)
	}
	if len(r.bgraBuf) != 80*64*4 {
		t.Errorf("bgraBuf length = %d, want %d", len(r.bgraBuf), 80*64*4)
	}
}

func TestWin32GuiRenderer_CursorDirty(t *testing.T) {
	r := NewWin32GuiRenderer(nil, nil, 8, 16)
	r.dirty = false

	r.SetCursor(2, 3, true, CursorShapeBlock)
	if !r.dirty {
		t.Error("SetCursor should mark renderer as dirty")
	}

	r.dirty = false
	r.SetCursor(2, 3, true, CursorShapeUnderline)
	if !r.dirty {
		t.Error("changing cursor shape should mark renderer as dirty")
	}
}

// The GDI backend paints the CommonLvbUnderscore decoration itself, like the
// other pixel backends: it is how a hovered URL is underlined (f4 #459).
func TestWin32GuiRenderer_UnderlineAttribute(t *testing.T) {
	r := NewWin32GuiRenderer(nil, nil, 8, 16)
	buf, shadow := mkGrid(4, 2, ' ', 0)
	attr := uint64(0x07) | CommonLvbUnderscore
	buf[1] = CharInfo{Char: 'a', Attributes: attr}
	buf[2] = CharInfo{Char: ' ', Attributes: attr}

	r.Render(buf, shadow, 4, 2, true)

	fg := ThemePalette[GetIndexFore(attr)]
	want := [3]uint8{uint8(fg >> 16), uint8(fg >> 8), uint8(fg)}
	for x := 8; x < 24; x++ {
		p := r.imgBuf.RGBAAt(x, 15)
		if [3]uint8{p.R, p.G, p.B} != want {
			t.Fatalf("pixel (%d,15) = %v, want underline colour %v", x, p, want)
		}
	}
	bg := ThemePalette[GetIndexBack(attr)]
	if p := r.imgBuf.RGBAAt(20, 14); [3]uint8{p.R, p.G, p.B} != [3]uint8{uint8(bg >> 16), uint8(bg >> 8), uint8(bg)} {
		t.Fatalf("pixel (20,14) = %v, want background above the underline", p)
	}
	if p := r.imgBuf.RGBAAt(4, 15); [3]uint8{p.R, p.G, p.B} == want && ThemePalette[0] != fg {
		t.Fatalf("pixel (4,15) = %v: underline leaked into a cell without the attribute", p)
	}
}

func TestWin32GuiRenderer_ImplementsInterfaces(t *testing.T) {
	var _ SurfaceRenderer = (*Win32GuiRenderer)(nil)
	var _ GraphicsRenderer = (*Win32GuiRenderer)(nil)
}

func TestWin32GuiRenderer_RenderGraphics(t *testing.T) {
	r := NewWin32GuiRenderer(nil, nil, 8, 16)
	buf, shadow := mkGrid(4, 2, ' ', 0)
	r.Render(buf, shadow, 4, 2, true)

	layer := &GraphicsLayer{}
	layer.SetProtocol(GraphicsNative)
	layer.Add(ImagePlacement{
		Surface: NewImageSurface(16, 16),
		Col:     0,
		Row:     0,
		Cols:    2,
		Rows:    1,
	})

	r.RenderGraphics(layer, buf, shadow, 4, 2, true)
	if !r.dirty {
		t.Error("RenderGraphics with native layer should mark renderer dirty")
	}
}

func TestFrameManager_GetBackendName_Win32Gui(t *testing.T) {
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)

	scr.Renderer = NewWin32GuiRenderer(nil, nil, 8, 16)
	if name := fm.GetBackendName(); name != "GUI (Win32)" {
		t.Errorf("GetBackendName() = %q, want 'GUI (Win32)'", name)
	}
}

func TestWin32Gui_PostQuitState(t *testing.T) {
	host := &Win32GuiHost{
		closeChan: make(chan struct{}),
	}

	if host.closed {
		t.Error("host should not be closed initially")
	}

	host.PostQuit()

	if !host.closed {
		t.Error("PostQuit should mark host as closed")
	}

	select {
	case <-host.closeChan:
		// success: closeChan closed
	default:
		t.Error("closeChan was not closed on PostQuit")
	}

	// Calling PostQuit again should be safe and idempotent
	host.PostQuit()
}
