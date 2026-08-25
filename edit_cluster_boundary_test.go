package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// The reference sequences come from the Notepad transcript attached to
// unxed/f4#546: Delete removes one whole shaped unit at a time. The editing
// boundaries therefore have to be the terminal clusters that the widget also
// paints, not the raw UAX #29 clusters (which cut a Devanagari virama loose
// from its consonant).

const (
	sanskritSample = "\u0938\u0902\u0938\u094D\u0915\u0943\u0924\u092E\u094D" // संस्कृतम्
	thaanaSample   = "\u078B\u07A8\u0788\u07AC\u0780\u07A8\u0784\u07A6\u0790\u07B0"
)

func newBoundaryEdit(text string) *Edit {
	e := NewEdit(0, 0, 40, text)
	e.ClearSelection()
	e.clearFlag = false
	e.curPos = 0
	return e
}

func forwardBoundaries(e *Edit) []int {
	var got []int
	for pos := 0; pos < len(e.text); {
		next := e.nextClusterBoundary(pos)
		if next <= pos {
			break
		}
		got = append(got, next)
		pos = next
	}
	return got
}

func backwardBoundaries(e *Edit) []int {
	var got []int
	for pos := len(e.text); pos > 0; {
		prev := e.prevClusterBoundary(pos)
		if prev >= pos {
			break
		}
		got = append(got, prev)
		pos = prev
	}
	return got
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEditClusterBoundariesJoinIndicVirama(t *testing.T) {
	e := newBoundaryEdit(sanskritSample)

	// सं | स्कृ | त | म्
	want := []int{2, 6, 7, 9}
	if got := forwardBoundaries(e); !equalInts(got, want) {
		t.Fatalf("nextClusterBoundary walk = %v, want %v", got, want)
	}
	wantBack := []int{7, 6, 2, 0}
	if got := backwardBoundaries(e); !equalInts(got, wantBack) {
		t.Fatalf("prevClusterBoundary walk = %v, want %v", got, wantBack)
	}
}

func TestEditClusterBoundariesKeepThaanaVowelSigns(t *testing.T) {
	e := newBoundaryEdit(thaanaSample)

	want := []int{2, 4, 6, 8, 10}
	if got := forwardBoundaries(e); !equalInts(got, want) {
		t.Fatalf("nextClusterBoundary walk = %v, want %v", got, want)
	}
}

// A caret boundary that the painter does not treat as a cell start puts the
// cursor inside a glyph; that is the visible half of unxed/f4#546.
func TestEditClusterBoundariesMatchPaintedCells(t *testing.T) {
	for _, sample := range []string{sanskritSample, thaanaSample} {
		painted := map[int]bool{}
		forEachTerminalCluster(sample, func(_ string, _, _, runeIndex int) {
			painted[runeIndex] = true
		})
		e := newBoundaryEdit(sample)
		for _, pos := range forwardBoundaries(e) {
			if pos == len(e.text) {
				continue
			}
			if !painted[pos] {
				t.Errorf("caret boundary %d in %q is not a painted cell start", pos, sample)
			}
		}
	}
}

func TestEditDeleteRemovesWholeTerminalCluster(t *testing.T) {
	saved := DefaultBidiMode
	DefaultBidiMode = BidiDisplay
	defer func() { DefaultBidiMode = saved }()

	e := newBoundaryEdit(sanskritSample)
	want := []string{
		"\u0938\u094D\u0915\u0943\u0924\u092E\u094D", // स्कृतम्
		"\u0924\u092E\u094D",                         // तम्
		"\u092E\u094D",                               // म्
		"",
	}
	for i, expected := range want {
		e.ProcessKey(&vtinput.InputEvent{
			Type:           vtinput.KeyEventType,
			KeyDown:        true,
			VirtualKeyCode: vtinput.VK_DELETE,
		})
		if got := e.GetText(); got != expected {
			t.Fatalf("Delete #%d = %q, want %q", i+1, got, expected)
		}
	}
}

func TestEditCursorPositionAtXLandsOnClusterStart(t *testing.T) {
	saved := DefaultBidiMode
	DefaultBidiMode = BidiDisplay
	defer func() { DefaultBidiMode = saved }()

	e := newBoundaryEdit(sanskritSample)
	painted := map[int]bool{len(e.text): true}
	forEachTerminalCluster(sanskritSample, func(_ string, _, _, runeIndex int) {
		painted[runeIndex] = true
	})
	for x := e.X1; x <= e.X1+6; x++ {
		if pos := e.cursorPositionAtX(x); !painted[pos] {
			t.Errorf("cursorPositionAtX(%d) = %d, not a cluster start", x, pos)
		}
	}
}
