//go:build windows

package vtui

import (
	"strconv"
	"testing"
)

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

var fitOpNames = [...]string{
	fitGrowBuffer: "growBuffer",
	fitMoveCursor: "moveCursor",
	fitWindow:     "window",
	fitSizeBuffer: "sizeBuffer",
}

// String is what makes a failed plan readable -- [growBuffer moveCursor]
// rather than [0 1]. It lives in the test because nothing in the package
// itself ever formats a fitOp. An op added without a name here prints as a
// number rather than blanking or crashing the diagnostic.
func (o fitOp) String() string {
	if o < 0 || int(o) >= len(fitOpNames) || fitOpNames[o] == "" {
		return "fitOp(" + strconv.Itoa(int(o)) + ")"
	}
	return fitOpNames[o]
}

// checkOps fails unless the plan is exactly want, in that order: for this
// plan the order is the whole point, not just the set of calls.
func checkOps(t *testing.T, plan []fitStep, want ...fitOp) {
	t.Helper()
	got := ops(plan)
	if len(got) != len(want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("step %d = %v, want %v (full %v)", i, got[i], want[i], got)
		}
	}
}

// The shrink is what kills Windows 10 conhost when the cursor or window is
// still outside the smaller buffer (microsoft/terminal#2366, f4 #397). So on
// any shrink the cursor must be pulled in and the window moved *before* the
// buffer is sized down. A buffer that already covers the target on both axes
// has nothing to grow, so this plan opens with the cursor move; the
// grow-first half of the invariant is pinned by the mixed-resize test below.
func TestPlanFit_ShrinkMovesCursorAndWindowBeforeSizingDown(t *testing.T) {
	// From a tall/wide buffer with the cursor deep inside it, down to 80x25.
	plan := planFit(csbi(150, 300, 149, 299, 150, 50), 80, 25)
	checkOps(t, plan, fitMoveCursor, fitWindow, fitSizeBuffer)
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
	// Both moves precede the size-down. The step-by-step check above already
	// pins that, but these two orderings are the whole of #397 -- conhost
	// faults on a buffer set smaller than where the cursor or the window
	// still sit -- so they are spelled out as well, against a later and
	// looser check of the steps.
	ci, wi, si := -1, -1, -1
	for i, s := range plan {
		if s.op == fitMoveCursor {
			ci = i
		}
		if s.op == fitWindow {
			wi = i
		}
		if s.op == fitSizeBuffer {
			si = i
		}
	}
	if ci < 0 || ci > si {
		t.Fatalf("cursor move (%d) must come before buffer size-down (%d)", ci, si)
	}
	if wi < 0 || wi > si {
		t.Fatalf("window move (%d) must come before buffer size-down (%d)", wi, si)
	}
}

// A resize that cuts one axis while widening the other still moves the window
// to the whole target before the buffer comes down, so the buffer has to cover
// the old size and the target at once first: grown short of the target leaves
// the window move hanging outside the buffer, and grown short of the old size
// is itself the shrink the ordering exists to defer -- one
// SetConsoleScreenBufferSize sets both axes.
func TestPlanFit_MixedResizeGrowsTheBufferFirst(t *testing.T) {
	for _, c := range []struct {
		name       string
		info       consoleScreenBufferInfo
		w, h       int
		wantGrow   Coord
		wantWindow Coord // inclusive bottom-right
	}{
		{
			// Narrow and very tall, widened to 80 columns and cut to 25 rows,
			// with the cursor sitting on the last row.
			name: "wider and shorter", info: csbi(60, 300, 59, 299, 60, 50), w: 80, h: 25,
			wantGrow: Coord{X: 80, Y: 300}, wantWindow: Coord{X: 79, Y: 24},
		},
		{
			// The same the other way up: wide and short, cut to 80 columns and
			// grown to 50 rows, with the cursor out past column 80.
			name: "narrower and taller", info: csbi(150, 25, 149, 24, 150, 25), w: 80, h: 50,
			wantGrow: Coord{X: 150, Y: 50}, wantWindow: Coord{X: 79, Y: 49},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			plan := planFit(c.info, c.w, c.h)
			checkOps(t, plan, fitGrowBuffer, fitMoveCursor, fitWindow, fitSizeBuffer)
			if plan[0].coord != c.wantGrow {
				t.Fatalf("grew the buffer to %dx%d, want %dx%d",
					plan[0].coord.X, plan[0].coord.Y, c.wantGrow.X, c.wantGrow.Y)
			}
			if plan[2].coord != c.wantWindow {
				t.Fatalf("window moved to %dx%d inclusive, want %dx%d",
					plan[2].coord.X, plan[2].coord.Y, c.wantWindow.X, c.wantWindow.Y)
			}
		})
	}
}

// A cursor already inside the target needs no move, and a buffer already
// covering it needs no grow: window, then size-down, and nothing else.
func TestPlanFit_NoCursorMoveWhenInside(t *testing.T) {
	plan := planFit(csbi(150, 50, 3, 3, 150, 50), 80, 25)
	for _, s := range plan {
		if s.op == fitMoveCursor {
			t.Fatalf("cursor moved needlessly: it was at 3x3, inside 80x25")
		}
	}
	checkOps(t, plan, fitWindow, fitSizeBuffer)
}

// The cursor is pulled in per axis: the coordinate hanging outside the target
// is clamped to the last cell, the one already inside is left where the user
// put it.
func TestPlanFit_ClampsOnlyTheAxisOutsideTheTarget(t *testing.T) {
	for _, c := range []struct {
		name string
		info consoleScreenBufferInfo
		want Coord
	}{
		{"column outside", csbi(150, 25, 149, 3, 150, 25), Coord{X: 79, Y: 3}},
		{"row outside", csbi(80, 300, 3, 299, 80, 50), Coord{X: 3, Y: 24}},
	} {
		t.Run(c.name, func(t *testing.T) {
			moved := false
			for _, s := range planFit(c.info, 80, 25) {
				if s.op != fitMoveCursor {
					continue
				}
				moved = true
				if s.coord != c.want {
					t.Fatalf("cursor moved to %dx%d, want %dx%d",
						s.coord.X, s.coord.Y, c.want.X, c.want.Y)
				}
			}
			if !moved {
				t.Fatal("the cursor was outside 80x25 and was not moved")
			}
		})
	}
}

// Growing needs no cursor move: grow, window, and the size-down, which on a
// pure grow asks for the size the grow has already set.
func TestPlanFit_Grow(t *testing.T) {
	plan := planFit(csbi(80, 25, 1, 1, 80, 25), 150, 50)
	checkOps(t, plan, fitGrowBuffer, fitWindow, fitSizeBuffer)
	if plan[0].coord != (Coord{X: 150, Y: 50}) {
		t.Fatalf("grew the buffer to %dx%d, want 150x50", plan[0].coord.X, plan[0].coord.Y)
	}
}

// Already the right size and origin: nothing to do.
func TestPlanFit_Noop(t *testing.T) {
	if plan := planFit(csbi(80, 25, 5, 5, 80, 25), 80, 25); len(plan) != 0 {
		t.Fatalf("expected no work, got %v", ops(plan))
	}
}

// The shortcut out needs every part to match: the buffer, the viewport, and
// the viewport origin. The origin is belt and braces -- a console window
// cannot reach past its own buffer, so a viewport already the size of the
// buffer is at 0,0 -- but planFit checks it, so both halves are pinned here
// against a viewport nudged off the origin.
func TestPlanFit_NotANoopWhenAnythingIsOff(t *testing.T) {
	scrolledRight := csbi(80, 25, 0, 0, 80, 25)
	scrolledRight.srWindow.Left, scrolledRight.srWindow.Right = 1, 80
	scrolledDown := csbi(80, 25, 0, 0, 80, 25)
	scrolledDown.srWindow.Top, scrolledDown.srWindow.Bottom = 1, 25
	for _, c := range []struct {
		name string
		info consoleScreenBufferInfo
	}{
		{"buffer one column wider", csbi(81, 25, 0, 0, 80, 25)},
		{"buffer one row taller", csbi(80, 26, 0, 0, 80, 25)},
		{"viewport one column narrower", csbi(80, 25, 0, 0, 79, 25)},
		{"viewport one row shorter", csbi(80, 25, 0, 0, 80, 24)},
		{"viewport scrolled right of the origin", scrolledRight},
		{"viewport scrolled below the origin", scrolledDown},
	} {
		t.Run(c.name, func(t *testing.T) {
			if plan := planFit(c.info, 80, 25); len(plan) == 0 {
				t.Fatal("expected a plan, got none")
			}
		})
	}
}

// A COORD is a pair of int16, so a size the console cannot express is not
// worth a single call: planFit backs out and leaves the buffer alone. The
// bound is exclusive at the top -- the largest size a COORD does hold is
// planned like any other.
func TestPlanFit_OnlyPlansSizesACoordCanHold(t *testing.T) {
	for _, c := range []struct {
		name string
		w, h int
	}{
		{"no columns", 0, 25},
		{"no rows", 80, 0},
		{"columns past int16", 0x8000, 25},
		{"rows past int16", 80, 0x8000},
	} {
		t.Run(c.name, func(t *testing.T) {
			if plan := planFit(csbi(150, 300, 149, 299, 150, 50), c.w, c.h); len(plan) != 0 {
				t.Fatalf("planned %v for %dx%d, want nothing", ops(plan), c.w, c.h)
			}
		})
	}
	// The bound is exclusive: the largest size a COORD can hold is planned.
	t.Run("the largest size int16 holds", func(t *testing.T) {
		if plan := planFit(csbi(80, 25, 0, 0, 80, 25), 0x7fff, 0x7fff); len(plan) == 0 {
			t.Fatal("0x7fff by 0x7fff fits in a COORD and must be planned")
		}
	})
}
