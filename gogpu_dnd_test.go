//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/gogpu/gogpu"
)

// withInlineDropTarget installs a target and makes delivery inline, so a
// test sees the whole gesture without a UI thread to hop to.
func withInlineDropTarget(t *testing.T, f func(ev *DragEvent) DropAction) {
	t.Helper()
	prev := DragDeliverToUI
	DragDeliverToUI = false
	SetDropTarget(DropTargetFunc(f))
	t.Cleanup(func() {
		DragDeliverToUI = prev
		SetDropTarget(nil)
	})
}

func TestGogpuDragPayloadTakesPathsAndURIs(t *testing.T) {
	p := gogpuDragPayload([]string{
		"/tmp/one.txt",
		"   ",
		"file:///tmp/two%20three.txt",
		"https://example.org/x",
	})

	want := []string{"/tmp/one.txt", filepath.FromSlash("/tmp/two three.txt")}
	if !reflect.DeepEqual(p.Paths, want) {
		t.Fatalf("Paths = %v, want %v", p.Paths, want)
	}
	if !reflect.DeepEqual(p.URIs, []string{"https://example.org/x"}) {
		t.Fatalf("URIs = %v, want the remote one alone", p.URIs)
	}
	if !p.HasFiles() || !p.OffersFiles() {
		t.Fatal("a payload holding paths offers files")
	}
	if empty := gogpuDragPayload([]string{"", "  "}); !empty.IsEmpty() || len(empty.Kinds) != 0 {
		t.Fatalf("payload = %+v, want nothing at all", empty)
	}
}

func TestGogpuDropCellDividesByCellSize(t *testing.T) {
	cases := []struct {
		px   float64
		cell int
		want int
	}{
		{0, 8, 0},
		{7.9, 8, 0},
		{8, 8, 1},
		{123.5, 8, 15},
		{-4, 8, 0},
		{100, 0, 0},
	}
	for _, c := range cases {
		if got := gogpuDropCell(c.px, c.cell); got != c.want {
			t.Fatalf("gogpuDropCell(%v, %d) = %d, want %d", c.px, c.cell, got, c.want)
		}
	}
}

func TestGogpuHostDeliversDropAsEnterThenDrop(t *testing.T) {
	var phases []DragPhase
	var last DragEvent
	withInlineDropTarget(t, func(ev *DragEvent) DropAction {
		phases = append(phases, ev.Phase)
		last = *ev
		return DropCopy
	})

	host := &GogpuHost{cellW: 8, cellH: 16}
	host.deliverFileDrop([]string{"/tmp/a.txt"}, 40, 48)

	if !reflect.DeepEqual(phases, []DragPhase{DragEnter, DragDrop}) {
		t.Fatalf("phases = %v, want one enter followed by the drop", phases)
	}
	if last.X != 5 || last.Y != 3 {
		t.Fatalf("cell = %d,%d, want 5,3", last.X, last.Y)
	}
	if last.Allowed != DropCopy || last.Suggested != DropCopy {
		t.Fatalf("actions = %s / %s, want copy and only copy", last.Allowed, last.Suggested)
	}
	if !reflect.DeepEqual(last.Payload.Paths, []string{"/tmp/a.txt"}) {
		t.Fatalf("paths = %v, want the dropped file", last.Payload.Paths)
	}
}

func TestGogpuHostIgnoresEmptyDrop(t *testing.T) {
	asked := false
	withInlineDropTarget(t, func(ev *DragEvent) DropAction {
		asked = true
		return DropNone
	})

	host := &GogpuHost{cellW: 8, cellH: 16}
	host.deliverFileDrop(nil, 10, 10)
	host.deliverFileDrop([]string{"  "}, 10, 10)

	if asked {
		t.Fatal("a drop carrying nothing is not a gesture")
	}
}

func TestGogpuHostReportsDragDirections(t *testing.T) {
	host := &GogpuHost{}
	if host.AcceptsDrops() {
		t.Fatal("without a window there is nothing to drop on")
	}
	if host.CanStartDrag() {
		t.Fatal("without a window there is nothing to drag out of")
	}
	_, err := host.StartDrag(DragPayload{Paths: []string{"/tmp/a.txt"}}, DropCopy)
	if !errors.Is(err, ErrDragUnsupported) {
		t.Fatalf("err = %v, want ErrDragUnsupported", err)
	}
} // testDragPath is an absolute path on whatever this is running on, which is
// what gogpu requires and what filepath.IsAbs agrees with on Windows too.
func testDragPath(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(name)
	if err != nil {
		t.Fatalf("cannot build an absolute path: %v", err)
	}
	return p
}

func TestGogpuDragPathsKeepsAbsoluteLocalFiles(t *testing.T) {
	abs := testDragPath(t, "dragged.txt")
	got := gogpuDragPaths(DragPayload{
		Paths: []string{abs, "   ", "relative.txt"},
		URIs:  []string{"https://example.org/x"},
	})
	if !reflect.DeepEqual(got, []string{abs}) {
		t.Fatalf("paths = %v, want the absolute one alone", got)
	}
	if got := gogpuDragPaths(DragPayload{URIs: []string{"https://example.org/x"}}); len(got) != 0 {
		t.Fatalf("paths = %v, want nothing draggable", got)
	}
}

func TestGogpuDropActionOfResult(t *testing.T) {
	cases := []struct {
		result gogpu.DragResult
		want   DropAction
	}{
		{gogpu.DragCopied, DropCopy},
		{gogpu.DragMoved, DropMove},
		{gogpu.DragCancelled, DropNone},
	}
	for _, c := range cases {
		if got := gogpuDropActionOf(c.result); got != c.want {
			t.Fatalf("result %v = %s, want %s", c.result, got, c.want)
		}
	}
}

func TestGogpuDragOutReportsWhatTheReceiverDid(t *testing.T) {
	host := &GogpuHost{}
	abs := testDragPath(t, "dragged.txt")

	req, err := host.queueDragOut(DragPayload{Paths: []string{abs}})
	if err != nil {
		t.Fatalf("queueing the gesture: %v", err)
	}
	if _, err := host.queueDragOut(DragPayload{Paths: []string{abs}}); !errors.Is(err, ErrDragBusy) {
		t.Fatalf("err = %v, want ErrDragBusy: there is one pointer", err)
	}

	go host.finishDragOut(req, gogpuDragOutcome{action: DropCopy})

	action, err := host.awaitDragOut(req)
	if err != nil || action != DropCopy {
		t.Fatalf("gesture ended as %s, err %v, want copy", action, err)
	}
	host.mu.Lock()
	left := host.dragOut
	host.mu.Unlock()
	if left != nil {
		t.Fatal("a finished gesture leaves nothing behind")
	}
}

func TestGogpuDragOutNeedsLocalFiles(t *testing.T) {
	host := &GogpuHost{}
	if _, err := host.queueDragOut(DragPayload{URIs: []string{"https://example.org/x"}}); !errors.Is(err, ErrDragNoData) {
		t.Fatalf("err = %v, want ErrDragNoData", err)
	}
}

func TestGogpuDragOutGivesUpAndReleasesTheButton(t *testing.T) {
	prev := gogpuDragTimeout
	gogpuDragTimeout = 20 * time.Millisecond
	defer func() { gogpuDragTimeout = prev }()

	host := &GogpuHost{mouseBtn: 1}
	req, err := host.queueDragOut(DragPayload{Paths: []string{testDragPath(t, "dragged.txt")}})
	if err != nil {
		t.Fatalf("queueing the gesture: %v", err)
	}

	action, err := host.awaitDragOut(req)
	if err != nil || action != DropNone {
		t.Fatalf("gesture ended as %s, err %v, want nothing", action, err)
	}
	host.mu.Lock()
	left, btn := host.dragOut, host.mouseBtn
	host.mu.Unlock()
	if left != nil {
		t.Fatal("giving up forgets the gesture")
	}
	if btn != 0 {
		t.Fatal("giving up releases the button the drag was started with")
	}
}
func TestGogpuPumpDragOutHandsTheGestureOverOnce(t *testing.T) {
	host := &GogpuHost{}
	req, err := host.queueDragOut(DragPayload{Paths: []string{testDragPath(t, "dragged.txt")}})
	if err != nil {
		t.Fatalf("queueing the gesture: %v", err)
	}

	// There is no window behind this host, so the only thing the loop can
	// report is that there is nothing to drag with. Report it it must,
	// though: a gesture nobody answers is a UI goroutine standing still.
	host.pumpDragOut()

	action, err := host.awaitDragOut(req)
	if action != DropNone || !errors.Is(err, ErrDragUnsupported) {
		t.Fatalf("gesture ended as %s, err %v, want ErrDragUnsupported", action, err)
	}

	// Nothing waits any more, so a later frame does nothing at all.
	host.pumpDragOut()
	host.mu.Lock()
	left := host.dragOut
	host.mu.Unlock()
	if left != nil {
		t.Fatal("an empty pump leaves no gesture behind")
	}
}

func TestGogpuPumpDragOutSkipsAGestureAlreadyUnderWay(t *testing.T) {
	host := &GogpuHost{}
	req, err := host.queueDragOut(DragPayload{Paths: []string{testDragPath(t, "dragged.txt")}})
	if err != nil {
		t.Fatalf("queueing the gesture: %v", err)
	}
	req.started = true

	host.pumpDragOut()

	select {
	case out := <-req.result:
		t.Fatalf("a gesture already under way was started again: %+v", out)
	default:
	}
}
func TestGogpuStartDragWithoutWindowIsRefused(t *testing.T) {
	host := &GogpuHost{}
	action, err := host.StartDrag(DragPayload{Paths: []string{testDragPath(t, "x.txt")}}, DropCopy)
	if action != DropNone || !errors.Is(err, ErrDragUnsupported) {
		t.Fatalf("gesture = %s, err %v, want ErrDragUnsupported", action, err)
	}
	if !memLogHas("no window to drag from") {
		t.Fatal("a refused drag out must say why in the log")
	}
}

func TestGogpuStartDragWithoutWindowIsLogged(t *testing.T) {
	host := &GogpuHost{}
	action, err := host.StartDrag(DragPayload{Paths: []string{testDragPath(t, "x.txt")}}, DropCopy)
	if action != DropNone || !errors.Is(err, ErrDragUnsupported) {
		t.Fatalf("gesture = %s, err %v, want ErrDragUnsupported", action, err)
	}
	if !memLogHas("no window to drag from") {
		t.Fatal("a refused drag out must say why in the log")
	}
}
func TestGogpuDragResultNameCoversTheKnownOnes(t *testing.T) {
	cases := map[gogpu.DragResult]string{
		gogpu.DragCopied:    "copied",
		gogpu.DragMoved:     "moved",
		gogpu.DragCancelled: "cancelled",
	}
	for r, want := range cases {
		if got := gogpuDragResultName(r); got != want {
			t.Fatalf("name of %v = %q, want %q", r, got, want)
		}
	}
}

func TestGogpuDropUsesThePositionGogpuReports(t *testing.T) {
	var last DragEvent
	withInlineDropTarget(t, func(ev *DragEvent) DropAction {
		last = *ev
		return DropCopy
	})

	host := &GogpuHost{cellW: 8, cellH: 16}
	host.deliverFileDrop([]string{"/tmp/a.txt"}, 40, 48)

	if last.X != 5 || last.Y != 3 {
		t.Fatalf("cell = %d,%d, want 5,3 from the reported 40,48", last.X, last.Y)
	}
}

func TestGogpuDropPixelsKeepAPositionThatWasReported(t *testing.T) {
	host := &GogpuHost{}
	if x, y := host.dropPixels(40, 48); x != 40 || y != 48 {
		t.Fatalf("position = %.1f,%.1f, want the reported 40,48", x, y)
	}
}

func TestGogpuDropPixelsKeepTheOriginWithNoPointerToTake(t *testing.T) {
	host := &GogpuHost{}
	if x, y := host.dropPixels(0, 0); x != 0 || y != 0 {
		t.Fatalf("position = %.1f,%.1f, want the origin when there is no window to ask", x, y)
	}
}
