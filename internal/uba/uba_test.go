package uba

import (
	"testing"

	"golang.org/x/text/unicode/bidi"
)

func classify(s string) (types []Class, pairTypes []Bracket, pairValues []rune) {
	for _, r := range s {
		props, _ := bidi.LookupRune(r)
		types = append(types, Class(props.Class()))
		switch {
		case props.IsOpeningBracket():
			pairTypes = append(pairTypes, BracketOpen)
			pairValues = append(pairValues, r)
		case props.IsBracket():
			pairTypes = append(pairTypes, BracketClose)
			pairValues = append(pairValues, []rune(bidi.ReverseString(string(r)))[0])
		default:
			pairTypes = append(pairTypes, BracketNone)
			pairValues = append(pairValues, 0)
		}
	}
	return
}

func levelsOf(t *testing.T, s string, paragraph int) ([]int, int) {
	t.Helper()
	types, pt, pv := classify(s)
	lv, base, err := Levels(types, pt, pv, paragraph)
	if err != nil {
		t.Fatalf("Levels(%q): %v", s, err)
	}
	return lv, base
}

func TestLevels_ParagraphDirection(t *testing.T) {
	// Hebrew first: auto detection picks RTL, a fixed LTR paragraph does not.
	s := "שלום abc"
	if _, base := levelsOf(t, s, ParagraphAuto); base != 1 {
		t.Errorf("auto: paragraph level %d, want 1", base)
	}
	lv, base := levelsOf(t, s, ParagraphLTR)
	if base != 0 {
		t.Errorf("ltr: paragraph level %d, want 0", base)
	}
	want := []int{1, 1, 1, 1, 0, 0, 0, 0}
	for i := range want {
		if lv[i] != want[i] {
			t.Fatalf("ltr levels = %v, want %v", lv, want)
		}
	}
}

func TestLevels_NumberInsideRTLRun(t *testing.T) {
	// A European number inside Hebrew sits at level 2 (rule I2), which is
	// what lets L2 keep its digits in reading order while the words swap.
	lv, _ := levelsOf(t, "אב 12 גד", ParagraphLTR)
	want := []int{1, 1, 1, 2, 2, 1, 1, 1}
	for i := range want {
		if lv[i] != want[i] {
			t.Fatalf("levels = %v, want %v", lv, want)
		}
	}
}

func TestLevels_BracketPairs(t *testing.T) {
	// N0: brackets enclosing Hebrew inside Hebrew are Hebrew too.
	lv, _ := levelsOf(t, "א(ב)ג", ParagraphLTR)
	for i, l := range lv {
		if l != 1 {
			t.Fatalf("position %d level %d, want 1 (%v)", i, l, lv)
		}
	}
}
