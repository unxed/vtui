package vtui

import (
	"bytes"
	"testing"
	"time"
)

// fakeExternal stands in for the X overlay: it records what it was asked to
// draw and how large a cell was said to be.
type fakeExternal struct {
	calls int
	last  []ImagePlacement
	cellW int
	cellH int
	cols  int
	rows  int

	// screen is set when the renderer is given one, so that the test can
	// prove it is never able to reach back into it.
	screen    *ScreenBuf
	reentered bool
}

func (f *fakeExternal) RenderExternal(list []ImagePlacement, cw, ch, cols, rows int) {
	f.calls++
	f.last = append(f.last[:0], list...)
	f.cellW, f.cellH = cw, ch
	f.cols, f.rows = cols, rows

	// Prove the hazard rather than assert it in a comment: ask the screen
	// something from another goroutine and see whether it can answer. It
	// cannot, because this runs with the screen's mutex held, and a
	// renderer that asked from *this* goroutine would deadlock instead.
	if f.screen != nil {
		answered := make(chan struct{})
		go func() {
			_ = f.screen.Width()
			close(answered)
		}()
		select {
		case <-answered:
			f.reentered = true
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Installing an external renderer must make the layer supported, so that
// everything which asks "can this terminal show a picture" starts saying yes.
func TestExternalGraphicsMakesTheLayerSupported(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	g := scr.Graphics()

	g.SetProtocol(GraphicsNone)
	if g.Supported() {
		t.Fatal("a layer with no protocol shows nothing")
	}

	ext := &fakeExternal{}
	g.SetExternalGraphics(ext)
	if !g.Supported() || g.Protocol() != GraphicsExternal {
		t.Errorf("protocol %v supported=%v", g.Protocol(), g.Supported())
	}
	if g.External() != ext {
		t.Error("the renderer must be the one that was installed")
	}
}

// The whole frame reaches the renderer in one call, which is the point: the
// viewer, the thumbnail grid and the built-in terminal all declare their
// images the same way they already do and none of them has to know.
func TestExternalGraphicsReceivesTheWholeFrame(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	g := scr.Graphics()
	g.SetCellSize(16, 34)

	ext := &fakeExternal{}
	g.SetExternalGraphics(ext)

	surf := NewImageSurface(4, 4)
	g.BeginFrame()
	g.DrawImage("a", ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 2, Rows: 2})
	g.DrawImage("b", ImagePlacement{Surface: surf, Col: 9, Row: 1, Cols: 2, Rows: 2})
	g.EndFrame()

	r := &AnsiRenderer{}
	r.RenderGraphics(g, nil, nil, 80, 25, true)

	if ext.calls != 1 {
		t.Fatalf("the renderer must be called once a frame, got %d", ext.calls)
	}
	if len(ext.last) != 2 {
		t.Fatalf("both pictures must arrive together, got %d", len(ext.last))
	}
	if ext.cellW != 16 || ext.cellH != 34 {
		t.Errorf("cell: got %dx%d, want 16x34", ext.cellW, ext.cellH)
	}
	// The grid comes with it, because the renderer runs with the screen
	// locked and must never ask the screen anything.
	if ext.cols != 80 || ext.rows != 25 {
		t.Errorf("grid: got %dx%d, want 80x25", ext.cols, ext.rows)
	}
}

// The renderer runs inside the render pass, with the screen's own mutex held.
// A renderer that calls back into the ScreenBuf deadlocks the application on
// the first frame that carries a picture, which is exactly what happened. This
// drives a real Flush and fails by hanging rather than by returning a wrong
// answer, so the deadline is the assertion.
func TestExternalGraphicsRunsWithTheScreenLocked(t *testing.T) {
	scr := NewScreenBuf()
	var out bytes.Buffer
	scr.Writer = &out
	scr.AllocBuf(80, 25)
	g := scr.Graphics()
	ext := &fakeExternal{screen: scr}
	g.SetExternalGraphics(ext)

	surf := NewImageSurface(4, 4)
	g.BeginFrame()
	g.DrawImage("a", ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 2, Rows: 2})
	g.EndFrame()

	done := make(chan struct{})
	go func() {
		scr.Flush()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Flush did not return: the render pass is deadlocked")
	}
	if ext.calls == 0 {
		t.Error("the renderer must be called from the render pass")
	}
	if ext.reentered {
		t.Error("the screen answered while the render pass held it; " +
			"if that is now true the contract has changed and this test is the wrong one")
	}
}

// Nothing may be drawn when nothing changed: an overlay repainted every frame
// is an overlay that flickers and eats a connection's worth of bandwidth.
func TestExternalGraphicsSkipsAnUnchangedFrame(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	g := scr.Graphics()
	ext := &fakeExternal{}
	g.SetExternalGraphics(ext)

	surf := NewImageSurface(4, 4)
	g.BeginFrame()
	g.DrawImage("a", ImagePlacement{Surface: surf, Col: 1, Row: 1, Cols: 2, Rows: 2})
	g.EndFrame()

	r := &AnsiRenderer{}
	r.RenderGraphics(g, nil, nil, 80, 25, true)
	first := ext.calls
	r.RenderGraphics(g, nil, nil, 80, 25, false)
	if ext.calls != first {
		t.Errorf("a frame with nothing new in it must not be redrawn: %d calls", ext.calls)
	}
}

func TestParseGraphicsProtocolKnowsExternal(t *testing.T) {
	p, ok := ParseGraphicsProtocol("external")
	if !ok || p != GraphicsExternal {
		t.Errorf("got %v %v", p, ok)
	}
	if GraphicsExternal.String() != "external" {
		t.Errorf("name: %q", GraphicsExternal.String())
	}
}
