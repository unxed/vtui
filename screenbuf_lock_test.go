package vtui

import (
	"sync"
	"testing"
)

// spyRenderer is a minimal SurfaceRenderer that records how many times it
// was asked to compose a frame, and a deep copy of the cell buffer it saw
// each time. It exists to prove, from outside ScreenBuf, exactly what a
// backend (X11/Wayland/etc.) would have presented to the display for a
// given Lock/Unlock bracket -- see f4#254 (render lock to avoid presenting
// half-drawn intermediate frames while a panel is toggled).
type spyRenderer struct {
	mu    sync.Mutex
	calls int
	last  []CharInfo
}

func (r *spyRenderer) Render(buf, shadow []CharInfo, width, height int, forceRedraw bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.last = append([]CharInfo(nil), buf...)
}

func (r *spyRenderer) SetCursor(x, y int, visible bool, shape CursorShape) {}
func (r *spyRenderer) SetPalette(palette *[256]uint32)                     {}
func (r *spyRenderer) SetWindowTitle(title string)                         {}
func (r *spyRenderer) Flush()                                              {}

func (r *spyRenderer) renderCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

func (r *spyRenderer) lastFrame() []CharInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]CharInfo(nil), r.last...)
}

func (r *spyRenderer) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = 0
	r.last = nil
}

func newLockTestScreen(w, h int) (*ScreenBuf, *spyRenderer) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(w, h)
	spy := &spyRenderer{}
	scr.Renderer = spy
	// AllocBuf/NewSilentScreenBuf leave the buffer marked dirty, which would
	// otherwise be delivered on the first Flush below and skew every call
	// count in this file by one. Consume that implicit flush, then reset
	// the spy so every test starts from a known, empty call count.
	scr.Flush()
	spy.reset()
	return scr, spy
}

// TestScreenBuf_FlushWithoutLockIsUnaffected is the regression guard: a
// caller that never touches Lock/Unlock must see exactly the pre-existing
// behavior -- one composed-and-delivered frame per Flush call, with no
// suppression and no extra work introduced by the new lockCount default.
func TestScreenBuf_FlushWithoutLockIsUnaffected(t *testing.T) {
	scr, spy := newLockTestScreen(2, 2)

	scr.Flush()
	scr.Flush()
	scr.Flush()

	if got := spy.renderCalls(); got != 3 {
		t.Fatalf("got %d render calls across 3 unlocked Flush calls, want 3", got)
	}
}

// TestScreenBuf_LockSuppressesFlush proves that while locked, Flush is a
// complete no-op as far as the Renderer is concerned: nothing is composed,
// nothing is delivered, so no intermediate state can reach the display.
func TestScreenBuf_LockSuppressesFlush(t *testing.T) {
	scr, spy := newLockTestScreen(4, 2)

	scr.Lock()
	scr.FillRect(0, 0, 3, 0, 'A', 0)
	scr.Flush()
	if got := spy.renderCalls(); got != 0 {
		t.Fatalf("Flush delivered %d frame(s) while locked, want 0", got)
	}

	scr.Unlock()
	if got := spy.renderCalls(); got != 1 {
		t.Fatalf("got %d render calls after Unlock, want exactly 1", got)
	}
}

// TestScreenBuf_LockNests proves Lock/Unlock nest correctly: delivery stays
// suppressed until every matching Lock has been undone, exactly the
// semantics a redraw that starts while another is still bracketed needs in
// order to be safely sequenced rather than racing the display.
func TestScreenBuf_LockNests(t *testing.T) {
	scr, spy := newLockTestScreen(4, 2)

	scr.Lock()
	scr.Lock()
	scr.FillRect(0, 0, 3, 0, 'A', 0)
	scr.Unlock() // still locked once
	if got := spy.renderCalls(); got != 0 {
		t.Fatalf("got %d render calls after inner Unlock, want 0 (still locked)", got)
	}

	scr.Unlock() // fully unlocked now
	if got := spy.renderCalls(); got != 1 {
		t.Fatalf("got %d render calls after outer Unlock, want exactly 1", got)
	}
}

// TestScreenBuf_LockDeliversFullyComposedFrameOnly is the direct test of the
// bug report: several Write/FillRect calls building up one panel redraw
// (e.g. a big blue panel over a black background) must never be observed by
// the Renderer half-finished. Exactly one frame is delivered, and it must be
// the final, fully composed one -- not row A alone, not A+B, but A+B+C.
func TestScreenBuf_LockDeliversFullyComposedFrameOnly(t *testing.T) {
	scr, spy := newLockTestScreen(3, 3)

	scr.Lock()
	scr.FillRect(0, 0, 2, 0, 'A', 0)
	scr.Flush() // attempted mid-composition flush: must not deliver
	scr.FillRect(0, 1, 2, 1, 'B', 0)
	scr.Flush() // attempted mid-composition flush: must not deliver
	scr.FillRect(0, 2, 2, 2, 'C', 0)
	scr.Unlock()

	if got := spy.renderCalls(); got != 1 {
		t.Fatalf("got %d render calls, want exactly 1 (no intermediate frame delivered)", got)
	}

	snap := spy.lastFrame()
	for y := 0; y < 3; y++ {
		want := uint64('A' + y)
		for x := 0; x < 3; x++ {
			if got := snap[y*3+x].Char; got != want {
				t.Fatalf("cell (%d,%d) = %q, want %q -- delivered frame is not the fully composed one", x, y, rune(got), rune(want))
			}
		}
	}
}

// TestScreenBuf_UnlockWithoutLockDoesNotPanicAndFlushes covers an unbalanced
// Unlock call: it must not drive the counter negative (which would then
// require an extra, unmatched Lock just to reach zero again) and, since
// nothing is actually locked, it behaves like a normal Flush.
func TestScreenBuf_UnlockWithoutLockDoesNotPanicAndFlushes(t *testing.T) {
	scr, spy := newLockTestScreen(2, 2)

	scr.FillRect(0, 0, 1, 1, 'X', 0)
	scr.Unlock()

	if got := spy.renderCalls(); got != 1 {
		t.Fatalf("got %d render calls after unbalanced Unlock, want 1", got)
	}

	// The counter must still be exactly zero: a real Lock/Unlock pair
	// afterwards must suppress then deliver exactly once, not be left
	// permanently unbalanced by the earlier stray Unlock.
	scr.Lock()
	scr.FillRect(0, 0, 1, 1, 'Y', 0)
	scr.Flush()
	if got := spy.renderCalls(); got != 1 {
		t.Fatalf("got %d render calls while locked, want still 1", got)
	}
	scr.Unlock()
	if got := spy.renderCalls(); got != 2 {
		t.Fatalf("got %d render calls after Unlock, want 2", got)
	}
}

// TestScreenBuf_LockSerializesConcurrentComposition is the concurrency
// guarantee: a redraw bracketed by Lock/Unlock on one goroutine must not
// let a Flush from another goroutine slip a partial or unrelated frame
// through while it is in progress. The second goroutine's Flush call must
// see nothing delivered until the first goroutine's Unlock completes the
// bracket, at which point exactly one frame -- the fully composed one --
// goes out.
func TestScreenBuf_LockSerializesConcurrentComposition(t *testing.T) {
	scr, spy := newLockTestScreen(4, 2)

	scr.Lock()

	proceed := make(chan struct{})
	done := make(chan struct{})
	go func() {
		<-proceed
		scr.FillRect(0, 0, 3, 0, 'Z', 0)
		scr.Unlock()
		close(done)
	}()

	// The bracket opened by Lock above is still open (the goroutine hasn't
	// even started its writes yet), so this Flush -- coming from a
	// completely different call site -- must be fully suppressed.
	scr.Flush()
	if got := spy.renderCalls(); got != 0 {
		t.Fatalf("got %d render calls while another redraw is locked, want 0", got)
	}

	close(proceed)
	<-done

	if got := spy.renderCalls(); got != 1 {
		t.Fatalf("got %d render calls after the bracket closed, want exactly 1", got)
	}
}
