package vtui

import (
	"math/rand"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// refSubstringMatch is the classic dynamic-programming reference for
// approximate substring search: D[0][j] = 0 (a match may start anywhere),
// D[i][0] = i. It returns the best edit distance over all ending positions
// and the earliest ending position achieving it.
func refSubstringMatch(pattern, text string, fold bool) (bestScore, bestEnd int) {
	p := []rune(pattern)
	s := []rune(text)
	if fold {
		for i := range p {
			p[i] = unicode.ToLower(p[i])
		}
		for i := range s {
			s[i] = unicode.ToLower(s[i])
		}
	}
	m := len(p)

	prev := make([]int, m+1)
	for i := range prev {
		prev[i] = i
	}
	cur := make([]int, m+1)

	bestScore = m
	bestEnd = 0
	for j := 1; j <= len(s); j++ {
		cur[0] = 0
		for i := 1; i <= m; i++ {
			sub := prev[i-1]
			if p[i-1] != s[j-1] {
				sub++
			}
			v := prev[i] + 1
			if cur[i-1]+1 < v {
				v = cur[i-1] + 1
			}
			if sub < v {
				v = sub
			}
			cur[i] = v
		}
		if cur[m] < bestScore {
			bestScore = cur[m]
			bestEnd = j - 1
		}
		prev, cur = cur, prev
	}
	return bestScore, bestEnd
}

func TestFuzzyMatcher_AgainstReference(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	alphabet := []rune("abcdAB")

	for iter := 0; iter < 2000; iter++ {
		m := 1 + rng.Intn(8)
		n := rng.Intn(17)
		needle := make([]rune, m)
		for i := range needle {
			needle[i] = alphabet[rng.Intn(len(alphabet))]
		}
		text := make([]rune, n)
		for i := range text {
			text[i] = alphabet[rng.Intn(len(alphabet))]
		}
		// Sometimes embed the needle to guarantee exact matches occur.
		if n > m && rng.Intn(2) == 0 {
			start := rng.Intn(n - m)
			copy(text[start:start+m], needle)
		}

		fm := NewFuzzyMatcher(string(needle), true)
		score, pos, _, _ := fm.Match(string(text))
		wantScore, wantEnd := refSubstringMatch(string(needle), string(text), false)
		wantPos := wantEnd - m + 1
		if wantPos < 0 {
			wantPos = 0
		}
		if score != wantScore {
			t.Errorf("needle=%q text=%q: score %d, want %d", string(needle), string(text), score, wantScore)
		} else if score == 0 && pos != wantPos {
			// Position is exact only for exact (score 0) matches.
			t.Errorf("needle=%q text=%q: pos %d, want %d", string(needle), string(text), pos, wantPos)
		}
	}
}

func TestFuzzyMatcher_Unicode(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	alphabet := []rune("абвгдАБВ")

	for iter := 0; iter < 1000; iter++ {
		m := 1 + rng.Intn(6)
		n := rng.Intn(14)
		needle := make([]rune, m)
		for i := range needle {
			needle[i] = alphabet[rng.Intn(len(alphabet))]
		}
		text := make([]rune, n)
		for i := range text {
			text[i] = alphabet[rng.Intn(len(alphabet))]
		}

		fm := NewFuzzyMatcher(string(needle), true)
		score, _, _, _ := fm.Match(string(text))
		wantScore, _ := refSubstringMatch(string(needle), string(text), false)
		if score != wantScore {
			t.Errorf("needle=%q text=%q: score %d, want %d", string(needle), string(text), score, wantScore)
		}
	}
}

func TestFuzzyMatcher_CaseInsensitive(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	alphabet := []rune("abABюЮ")

	for iter := 0; iter < 1000; iter++ {
		m := 1 + rng.Intn(6)
		n := rng.Intn(14)
		needle := make([]rune, m)
		for i := range needle {
			needle[i] = alphabet[rng.Intn(len(alphabet))]
		}
		text := make([]rune, n)
		for i := range text {
			text[i] = alphabet[rng.Intn(len(alphabet))]
		}

		fm := NewFuzzyMatcher(string(needle), false)
		score, _, _, _ := fm.Match(string(text))
		wantScore, _ := refSubstringMatch(string(needle), string(text), true)
		if score != wantScore {
			t.Errorf("needle=%q text=%q: folded score %d, want %d", string(needle), string(text), score, wantScore)
		}
	}
}

func TestFuzzyMatcher_Threshold(t *testing.T) {
	// k = len/3: needle of 3..5 runes allows 1 error.
	fm := NewFuzzyMatcher("abc", true)
	if fm.maxDistance != 1 {
		t.Fatalf("expected k=1, got %d", fm.maxDistance)
	}
	if _, _, _, ok := fm.Match("xxabcxx"); !ok {
		t.Error("exact substring must pass")
	}
	if _, _, _, ok := fm.Match("xxabdxx"); !ok {
		t.Error("one error must pass for k=1")
	}
	if _, _, _, ok := fm.Match("xxaexx"); ok {
		t.Error("two errors must not pass for k=1")
	}

	// Needles of 1-2 runes are exact-only (k=0).
	fm = NewFuzzyMatcher("ab", true)
	if _, _, _, ok := fm.Match("ac"); ok {
		t.Error("k=0 must reject any error")
	}
	if _, _, _, ok := fm.Match("xxabyy"); !ok {
		t.Error("k=0 must accept exact substring")
	}
}

func TestFuzzyMatcher_ExactPosition(t *testing.T) {
	fm := NewFuzzyMatcher("needle", true)
	score, pos, _, ok := fm.Match("find the needle here")
	if !ok || score != 0 {
		t.Fatalf("expected exact match, score=%d ok=%v", score, ok)
	}
	if pos != 9 {
		t.Errorf("expected pos 9, got %d", pos)
	}
}

func TestFuzzyMatcher_LongNeedleExactOnly(t *testing.T) {
	needle := strings.Repeat("a", 70)
	fm := NewFuzzyMatcher(needle, true)
	if !fm.exactOnly {
		t.Fatal("needle > 64 runes must degrade to exact search")
	}
	if _, _, _, ok := fm.Match("xx" + needle + "yy"); !ok {
		t.Error("exact long needle must match")
	}
	if _, _, _, ok := fm.Match(strings.Repeat("a", 69)); ok {
		t.Error("one error must not pass in exactOnly mode")
	}

	// Case-insensitive long needle.
	fm = NewFuzzyMatcher(strings.Repeat("Ab", 35), false)
	if _, pos, _, ok := fm.Match("zz" + strings.Repeat("aB", 35) + "zz"); !ok || pos != 2 {
		t.Errorf("case-insensitive exactOnly failed: ok=%v pos=%d", ok, pos)
	}
}

func TestFuzzyMatcher_IsMatchExact(t *testing.T) {
	fm := NewFuzzyMatcher("abc", false)

	if _, _, _, ok := fm.Match("abc"); !ok || !fm.IsMatchExact() {
		t.Error("full equality must be exact")
	}
	if _, _, _, ok := fm.Match("ABC"); !ok || !fm.IsMatchExact() {
		t.Error("case-folded full equality must be exact for a case-insensitive matcher")
	}
	if _, _, _, ok := fm.Match("abcd"); !ok || fm.IsMatchExact() {
		t.Error("prefix-only match must not be exact")
	}
	if _, _, _, ok := fm.Match("xabc"); !ok || fm.IsMatchExact() {
		t.Error("substring match must not be exact")
	}
	if _, _, _, ok := fm.Match("abd"); !ok || fm.IsMatchExact() {
		t.Error("inexact fuzzy match must not be exact")
	}
}

func TestFuzzyMatcher_IsMatchExactExactOnly(t *testing.T) {
	needle := strings.Repeat("a", 70)
	fm := NewFuzzyMatcher(needle, true)

	// A failed match must reset the stored state: an equal-length haystack
	// that does not contain the needle is not an exact hit.
	if _, _, _, ok := fm.Match(strings.Repeat("b", 70)); ok {
		t.Fatal("no match expected")
	}
	if fm.IsMatchExact() {
		t.Error("failed exactOnly match must not report exact")
	}

	// A longer haystack starting with the needle is a prefix, not exact.
	if _, _, _, ok := fm.Match(needle + "x"); !ok {
		t.Fatal("expected match")
	}
	if fm.IsMatchExact() {
		t.Error("prefix match in exactOnly mode must not be exact")
	}

	// Full equality is exact.
	if _, _, _, ok := fm.Match(needle); !ok || !fm.IsMatchExact() {
		t.Error("full equality in exactOnly mode must be exact")
	}
}

func TestFuzzyMatcher_Empty(t *testing.T) {
	if fm := NewFuzzyMatcher("", true); fm != nil {
		t.Error("empty needle must produce nil matcher")
	}

	fm := NewFuzzyMatcher("abc", true)
	score, _, _, ok := fm.Match("")
	if ok {
		t.Error("empty text must not match a non-empty needle")
	}
	if score != 3 {
		t.Errorf("empty text distance must equal needle length, got %d", score)
	}
}

func TestFuzzyMatcher_RunePositions(t *testing.T) {
	// Position must be in runes even for the Unicode path.
	fm := NewFuzzyMatcher("game", true)
	_, pos, _, ok := fm.Match("моя game")
	if !ok {
		t.Fatal("expected match")
	}
	if pos != 4 {
		t.Errorf("expected rune pos 4, got %d", pos)
	}
	if got := utf8.RuneCountInString("моя "); got != 4 {
		t.Fatalf("test invariant broken: %d", got)
	}
}
