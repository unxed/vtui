package vtui

import (
	"reflect"
	"testing"
)

// f4 #378: a help line longer than the window is broken at spaces, as Far and
// far2l do, instead of being cut at the edge.
func TestWrapHelpTopic_BreaksAtSpaces(t *testing.T) {
	src := &HelpTopic{Name: "T", Lines: []string{"one two three four five six"}}
	got, rowSrc := wrapHelpTopic(src, 10)
	want := []string{"one two", "three four", "five six"}
	if !reflect.DeepEqual(got.Lines, want) {
		t.Fatalf("Lines = %q, want %q", got.Lines, want)
	}
	if !reflect.DeepEqual(rowSrc, []int{0, 0, 0}) {
		t.Errorf("rowSrc = %v, want every row from line 0", rowSrc)
	}
}

func TestWrapHelpTopic_KeepsATopicThatFits(t *testing.T) {
	src := &HelpTopic{Name: "T", Lines: []string{"short", "also short"}}
	got, rowSrc := wrapHelpTopic(src, 40)
	if got != src {
		t.Error("a topic that needs no breaking should be returned as it is")
	}
	if !reflect.DeepEqual(rowSrc, []int{0, 1}) {
		t.Errorf("rowSrc = %v, want [0 1]", rowSrc)
	}
}

func TestWrapHelpTopic_BreaksAWordThatIsTooLong(t *testing.T) {
	got, _ := wrapHelpTopic(&HelpTopic{Name: "T", Lines: []string{"abcdefghij"}}, 4)
	if want := []string{"abcd", "efgh", "ij"}; !reflect.DeepEqual(got.Lines, want) {
		t.Fatalf("Lines = %q, want %q", got.Lines, want)
	}
}

func TestWrapHelpTopic_CountsWideCharactersByColumns(t *testing.T) {
	got, _ := wrapHelpTopic(&HelpTopic{Name: "T", Lines: []string{"漢字漢字漢字"}}, 5)
	if want := []string{"漢字", "漢字", "漢字"}; !reflect.DeepEqual(got.Lines, want) {
		t.Fatalf("Lines = %q, want %q", got.Lines, want)
	}
}

// A row is drawn from a clean state, so bold that a break cut is closed at the
// end of the row and opened again on the next.
func TestWrapHelpTopic_KeepsBoldAcrossABreak(t *testing.T) {
	got, _ := wrapHelpTopic(&HelpTopic{Name: "T", Lines: []string{"#bold words here# tail"}}, 10)
	if want := []string{"#bold words#", "#here# tail"}; !reflect.DeepEqual(got.Lines, want) {
		t.Fatalf("Lines = %q, want %q", got.Lines, want)
	}
}

// The name a link points at is not drawn, so it takes no room; and a link that
// a break cut stays a link on both rows.
func TestWrapHelpTopic_KeepsALinkAcrossABreak(t *testing.T) {
	got, _ := wrapHelpTopic(&HelpTopic{Name: "T", Lines: []string{"see ~a link here~Target@ now"}}, 10)
	if want := []string{"see ~a link~Target@", "~here~Target@ now"}; !reflect.DeepEqual(got.Lines, want) {
		t.Fatalf("Lines = %q, want %q", got.Lines, want)
	}
	if len(got.Links) != 2 || got.Links[0].Target != "Target" || got.Links[0].Line != 0 ||
		got.Links[1].Target != "Target" || got.Links[1].Line != 1 {
		t.Fatalf("Links = %+v, want the same target on rows 0 and 1", got.Links)
	}
}

func TestWrapHelpTopic_LeavesStickyAndCenteredLines(t *testing.T) {
	src := &HelpTopic{
		Name:       "T",
		StickyRows: 1,
		Lines: []string{
			"Header that is far too long for this window",
			"^Centered title that is far too long too",
			"plain line that is far too long as well",
		},
	}
	got, rowSrc := wrapHelpTopic(src, 12)
	if got.Lines[0] != src.Lines[0] || got.Lines[1] != src.Lines[1] {
		t.Errorf("sticky and centered lines changed: %q", got.Lines[:2])
	}
	if len(got.Lines) <= 3 || rowSrc[0] != 0 || rowSrc[1] != 1 || rowSrc[2] != 2 || rowSrc[len(rowSrc)-1] != 2 {
		t.Errorf("Lines = %q, rowSrc = %v: the plain line should become several rows", got.Lines, rowSrc)
	}
}

// Zoom makes the text area wider: the topic is laid out again for it, and the
// reader stays where they were in the text (f4 #378).
func TestHelpView_RewrapsWhenTheWindowChangesWidth(t *testing.T) {
	SetDefaultPalette()
	engine := NewHelpEngine(&mockHelpVFS{})
	long := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu"
	lines := []string{"first"}
	for i := 0; i < 30; i++ {
		lines = append(lines, long)
	}
	engine.AddTopic(&HelpTopic{Name: "T", Lines: lines})
	hv := NewHelpView(engine, "T")
	hv.SetPosition(0, 0, 30, 12)
	scr := NewSilentScreenBuf()
	scr.AllocBuf(90, 14)
	hv.Show(scr)

	narrow := len(hv.CurrentTopic().Lines)
	if narrow <= len(lines) {
		t.Fatalf("narrow window laid the topic out in %d rows, want more than its %d lines", narrow, len(lines))
	}
	hv.scrollTop = 20
	topSrc := hv.rowSrc[hv.scrollTop]

	hv.SetPosition(0, 0, 79, 12)
	hv.Show(scr)
	if got := len(hv.CurrentTopic().Lines); got != len(lines) {
		t.Fatalf("wide window laid the topic out in %d rows, want its %d lines", got, len(lines))
	}
	if hv.CurrentTopic() != hv.source {
		t.Error("a topic that fits should be shown as authored")
	}
	if got := hv.rowSrc[hv.scrollTop]; got != topSrc {
		t.Errorf("after widening the top row comes from line %d, want the same line %d as before", got, topSrc)
	}
}

func TestHelpView_RewrapKeepsTheSelectedLink(t *testing.T) {
	SetDefaultPalette()
	engine := NewHelpEngine(&mockHelpVFS{})
	engine.AddTopic(&HelpTopic{Name: "T", Lines: []string{
		"~First~One@ and a long tail of words that has to wrap in a narrow window",
		"~Second~Two@ and another long tail of words that has to wrap as well",
	}})
	hv := NewHelpView(engine, "T")
	hv.SetPosition(0, 0, 30, 12)
	scr := NewSilentScreenBuf()
	scr.AllocBuf(90, 14)
	hv.Show(scr)
	for i, l := range hv.current.Links {
		if l.Target == "Two" {
			hv.selectedIdx = i
		}
	}
	hv.SetPosition(0, 0, 79, 12)
	hv.Show(scr)
	if hv.selectedIdx < 0 || hv.current.Links[hv.selectedIdx].Target != "Two" {
		t.Fatalf("selected link = %d of %+v, want the one to Two", hv.selectedIdx, hv.current.Links)
	}
}
