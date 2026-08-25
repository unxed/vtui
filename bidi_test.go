package vtui

import "testing"

func TestBidi_ASCII_Guard(t *testing.T) {
	s := "Hello World!"
	vis := VisualString(s)
	if vis != s {
		t.Errorf("ASCII guard failed: expected %q, got %q", s, vis)
	}
}

func TestBidi_Hebrew_Simple(t *testing.T) {
	s := "שלום"
	vis, offsets := VisualStringWithMap(s)
	expected := "םולש"
	if vis != expected {
		t.Errorf("Hebrew reversal failed: expected %q, got %q", expected, vis)
	}
	if len(offsets) != 4 || offsets[0] != 6 {
		t.Errorf("RTL offset mapping failed: %v", offsets)
	}
}

func TestBidi_Hebrew_With_Latin(t *testing.T) {
	s := "שלום hello"
	vis := VisualString(s)
	// A left to right line: the Hebrew word is reversed in place, the
	// line is not turned around (that would need a right to left paragraph).
	expected := "םולש hello"
	if vis != expected {
		t.Errorf("Mixed LTR/RTL failed: expected %q, got %q", expected, vis)
	}
}

func TestBidi_Parentheses_Mirroring(t *testing.T) {
	s := "(שלום)"
	vis := VisualString(s)
	expected := "(םולש)"
	if vis != expected {
		t.Errorf("Bracket mirroring failed: expected %q, got %q", expected, vis)
	}
}

func TestBidi_Width_Invariant(t *testing.T) {
	tests := []string{
		"שלום",
		"שלום hello",
		"hello שלום",
		"(שלום)",
		"abc مرحبا def",
	}
	for _, tc := range tests {
		wLogical := StringWidth(tc)
		vis := VisualString(tc)
		wVisual := StringWidth(vis)
		if wLogical != wVisual {
			t.Errorf("Width invariant violated for %q: logical width %d != visual width %d (visual: %q)", tc, wLogical, wVisual, vis)
		}
	}
}

func TestBidi_LeadingRTLWordKeepsLineOrder(t *testing.T) {
	// unxed/f4#546: a line that merely starts with a right to left word
	// must not be laid out as a right to left paragraph.
	s := "ދިވެހިބަސް - Divehi (Maldivian) - BiDi"
	want := "ސްބަހިވެދި - Divehi (Maldivian) - BiDi"
	if got := VisualString(s); got != want {
		t.Errorf("leading Thaana: got %q, want %q", got, want)
	}
	if got := VisualString("Divehi (Maldivian) ދިވެހިބަސް BiDi"); got != "Divehi (Maldivian) ސްބަހިވެދި BiDi" {
		t.Errorf("embedded Thaana: got %q", got)
	}
}

func TestBidi_ParagraphDirection(t *testing.T) {
	old := DefaultBidiParagraph
	t.Cleanup(func() { DefaultBidiParagraph = old })
	s := "שלום hello"
	DefaultBidiParagraph = BidiParagraphRTL
	if got := VisualString(s); got != "hello םולש" {
		t.Errorf("rtl paragraph: got %q", got)
	}
	DefaultBidiParagraph = BidiParagraphAuto
	if got := VisualString(s); got != "hello םולש" {
		t.Errorf("auto paragraph (first strong is Hebrew): got %q", got)
	}
	if got := VisualString("hello שלום"); got != "hello םולש" {
		t.Errorf("auto paragraph (first strong is Latin): got %q", got)
	}
}

func TestBidi_RuleL2_NumbersInsideRTL(t *testing.T) {
	// The digits stay in reading order while the Hebrew words swap: that
	// needs the real levels (the number sits at level 2), not a reversal of
	// each run in place, which gave "גבא 123 והד".
	if got := VisualString("אבג 123 דהו"); got != "והד 123 גבא" {
		t.Errorf("got %q", got)
	}
}

func TestBidi_MirroringInsideRTL(t *testing.T) {
	// L4: a mirrored character read right to left draws its counterpart;
	// brackets are paired (N0) and other Bidi_Mirrored characters too.
	if got := VisualString("א(ב)ג"); got != "ג(ב)א" {
		t.Errorf("brackets: got %q", got)
	}
	if got := VisualString("א<ב"); got != "ב>א" {
		t.Errorf("less-than: got %q", got)
	}
	if m, ok := MirrorRune('«'); !ok || m != '»' {
		t.Errorf("MirrorRune('«') = %q, %v", m, ok)
	}
	if _, ok := MirrorRune('a'); ok {
		t.Error("'a' has no mirrored form")
	}
}

func TestBidi_LayoutCaret(t *testing.T) {
	text := "abc אבג"
	spans, _ := terminalClusterSpans(text)
	lay := LayoutBidi(text, spans, BidiParagraphLTR)
	if got := lay.VisualToLogical; len(got) != 7 || got[4] != 6 || got[6] != 4 {
		t.Fatalf("visual order = %v", got)
	}
	wantVisual := []int{0, 1, 2, 3, 4, 6, 5, 4}
	for b, want := range wantVisual {
		if got := lay.CaretVisual(b); got != want {
			t.Errorf("CaretVisual(%d) = %d, want %d", b, got, want)
		}
	}
	wantLogical := []int{0, 1, 2, 3, 7, 6, 5, 4}
	for v, want := range wantLogical {
		if got := lay.CaretLogical(v); got != want {
			t.Errorf("CaretLogical(%d) = %d, want %d", v, got, want)
		}
	}
	// The map keyed by rune index agrees with the cluster boundaries.
	cm := BuildCaretMap(text)
	if cm.LogicalToVisual[4] != 4 || cm.LogicalToVisual[5] != 6 || cm.LogicalToVisual[7] != 4 {
		t.Errorf("rune caret map = %v", cm.LogicalToVisual)
	}
	if cm.VisualToLogical[4] != 7 || cm.VisualToLogical[7] != 4 {
		t.Errorf("visual caret map = %v", cm.VisualToLogical)
	}
}

func TestBidi_MarksStayWithBase(t *testing.T) {
	// Thaana: each consonant carries a vowel sign; clusters are reversed as
	// units, the marks never part from their letters.
	text := "ދިވެ"
	spans, _ := terminalClusterSpans(text)
	lay := LayoutBidi(text, spans, BidiParagraphLTR)
	if len(spans) != 2 || lay.VisualToLogical[0] != 1 || lay.VisualToLogical[1] != 0 {
		t.Fatalf("spans %v order %v", spans, lay.VisualToLogical)
	}
	if got, _ := VisualStringWithMap(text); got != "ވެދި" {
		t.Errorf("got %q", got)
	}
}
