//go:build windows

package vtui

import "testing"

func csbi(bufW, bufH, curX, curY, winW, winH int) consoleScreenBufferInfo {
	return consoleScreenBufferInfo{
		dwSize:           Coord{X: int16(bufW), Y: int16(bufH)},
		dwCursorPosition: Coord{X: int16(curX), Y: int16(curY)},
		srWindow:         SmallRect{Left: 0, Top: 0, Right: int16(winW) - 1, Bottom: int16(winH) - 1},
	}
}

func ops(plan []fitStep) []fitOp {
	out := make([]fitOp, len(plan))
	for i, s := range plan {
		out[i] = s.op
	}
	return out
}

// The shrink is what kills Windows 10 conhost when the cursor or window is
// still outside the smaller buffer (microsoft/terminal#2366, f4 #397). So on
// any shrink the cursor must be pulled in and the window moved *before* the
// buffer is sized down, and the buffer must be grown first so the window move
// always lands inside it.
func TestPlanFit_ShrinkMovesCursorAndWindowBeforeSizingDown(t *testing.T) {
	// From a tall/wide buffer with the cursor deep inside it, down to 80x25.
	plan := planFit(csbi(150, 300, 149, 299, 150, 50), 80, 25)
	got := ops(plan)
	want := []fitOp{fitGrowBuffer, fitMoveCursor, fitWindow, fitSizeBuffer}
	if len(got) != len(want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d = %v, want %v (full %v)", i, got[i], want[i], got)
		}
	}
	// The cursor must end up inside 80x25.
	for _, s := range plan {
		if s.op == fitMoveCursor {
			if s.coord.X >= 80 || s.coord.Y >= 25 {
				t.Fatalf("cursor moved to %dx%d, still outside 80x25", s.coord.X, s.coord.Y)
			}
		}
		if s.op == fitSizeBuffer && (s.coord.X != 80 || s.coord.Y != 25) {
			t.Fatalf("final buffer %dx%d, want 80x25", s.coord.X, s.coord.Y)
		}
	}
	// Cursor move precedes the size-down.
	ci, si := -1, -1
	for i, s := range plan {
		if s.op == fitMoveCursor {
			ci = i
		}
		if s.op == fitSizeBuffer {
			si = i
		}
	}
	if ci < 0 || ci > si {
		t.Fatalf("cursor move (%d) must come before buffer size-down (%d)", ci, si)
	}
}

// A cursor already inside the target needs no move.
func TestPlanFit_NoCursorMoveWhenInside(t *testing.T) {
	plan := planFit(csbi(150, 50, 3, 3, 150, 50), 80, 25)
	for _, s := range plan {
		if s.op == fitMoveCursor {
			t.Fatalf("cursor moved needlessly: it was at 3x3, inside 80x25")
		}
	}
}

// Growing needs no cursor move and no shrink guard, just grow then window.
func TestPlanFit_Grow(t *testing.T) {
	plan := planFit(csbi(80, 25, 1, 1, 80, 25), 150, 50)
	want := []fitOp{fitGrowBuffer, fitWindow, fitSizeBuffer}
	if got := ops(plan); len(got) != len(want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
}

// Already the right size and origin: nothing to do.
func TestPlanFit_Noop(t *testing.T) {
	if plan := planFit(csbi(80, 25, 5, 5, 80, 25), 80, 25); plan != nil {
		t.Fatalf("expected no work, got %v", ops(plan))
	}
}
