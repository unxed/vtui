package vtui

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FuzzyMatcher implements approximate substring search using Myers'
// bit-vector algorithm. For needles of up to 64 runes the edit distance is
// computed in O(len(haystack)) bit-parallel operations over a single uint64.
// Longer needles degrade to exact substring search.
//
// The matcher reports the best (lowest) edit distance between the needle and
// any substring of the haystack, plus the starting position of that match.
// Case-insensitive matching is done by indexing both cases of every needle
// character, so the haystack is looked up as-is.
//
// Canonical ranking of match results (guideline for all search UIs, highest
// priority first):
//  1. exact match — needle equals the whole haystack, score remapped to -1;
//  2. prefix match (score 0, start 0);
//  3. substring match (score 0) — the further left, the better;
//  4. everything else by ascending score.
//
// Sorting by (score, match start) with the exact-match remap implements the
// whole list. The exact-match test is matcher.IsMatchExact(), called right
// after matcher.Match(haystack).
type FuzzyMatcher struct {
	needleLen   int // needle length in runes
	maxDistance int // max edit distance accepted as a match

	needle        string
	caseSensitive bool

	ascii    bool
	peqAscii [256]uint64     // L1-resident fast path for ASCII needles and haystacks
	peqRune  map[rune]uint64 // built lazily on the first non-ASCII haystack

	exactOnly  bool   // needle longer than 64 runes
	needleFold string // needle (folded in case-insensitive mode) for exactOnly
	fold       bool   // fold the haystack too (case-insensitive exactOnly)

	rev *FuzzyMatcher // lazy reversed-needle matcher used to locate match starts

	// last match results, consulted by IsMatchExact; haystackLen is refreshed
	// only when an exact hit is possible (score == 0 && start == 0), because
	// IsMatchExact short-circuits on score and start first
	score, start, end, haystackLen int
}

// NewFuzzyMatcher builds a matcher for the given needle. It returns nil for
// an empty needle. The acceptance threshold is len(needle)/3 errors: exact
// substring matches always pass (score 0), short needles stay almost strict,
// longer ones tolerate more typos. Construction precomputes the needle tables
// once; Match is then linear in the haystack length per candidate.
func NewFuzzyMatcher(needle string, caseSensitive bool) *FuzzyMatcher {
	if needle == "" {
		return nil
	}

	fm := &FuzzyMatcher{needleLen: utf8.RuneCountInString(needle), needle: needle, caseSensitive: caseSensitive}
	fm.maxDistance = fm.needleLen / 3

	if fm.needleLen > 64 {
		fm.exactOnly = true
		fm.fold = !caseSensitive
		if caseSensitive {
			fm.needleFold = needle
		} else {
			fm.needleFold = strings.ToLower(needle)
		}
		return fm
	}

	fm.ascii = isASCIIString(needle)
	if fm.ascii {
		for i := 0; i < len(needle); i++ {
			b := needle[i]
			fm.peqAscii[b] |= 1 << uint(i)
			if !caseSensitive {
				if 'a' <= b && b <= 'z' {
					fm.peqAscii[b-'a'+'A'] |= 1 << uint(i)
				} else if 'A' <= b && b <= 'Z' {
					fm.peqAscii[b-'A'+'a'] |= 1 << uint(i)
				}
			}
		}
	} else {
		fm.buildRuneTable()
	}

	return fm
}

// buildRuneTable populates the Unicode peq table. For ASCII needles it is
// built lazily on the first non-ASCII haystack, so the common all-ASCII case
// never pays for the map.
func (fm *FuzzyMatcher) buildRuneTable() {
	fm.peqRune = make(map[rune]uint64, fm.needleLen)
	i := 0
	for _, r := range fm.needle {
		bit := uint64(1) << uint(i)
		fm.peqRune[r] |= bit
		if !fm.caseSensitive {
			fm.peqRune[unicode.ToLower(r)] |= bit
			fm.peqRune[unicode.ToUpper(r)] |= bit
		}
		i++
	}
}

// Match searches the needle inside haystack. It returns the best edit
// distance, the span [start, end] (inclusive, in runes) of the best matching
// substring, and whether the distance is within the acceptance threshold.
// The results are also stored in the matcher for IsMatchExact.
func (fm *FuzzyMatcher) Match(haystack string) (score, start, end int, ok bool) {
	if fm.exactOnly {
		folded := haystack
		if fm.fold {
			folded = strings.ToLower(haystack)
		}

		idx := strings.Index(folded, fm.needleFold)
		if idx < 0 {
			// Keep the stored state a definite non-match for IsMatchExact.
			fm.score, fm.start, fm.end = fm.needleLen, 0, 0
			return 0, 0, 0, false
		}

		start = utf8.RuneCountInString(haystack[:idx])
		fm.score, fm.start, fm.end = 0, start, start+fm.needleLen-1
		if start == 0 {
			fm.haystackLen = utf8.RuneCountInString(haystack)
		}
		return 0, start, start + fm.needleLen - 1, true
	}

	if fm.ascii && isASCIIString(haystack) {
		score, end = fm.matchASCII(haystack)
	} else {
		if fm.peqRune == nil {
			fm.buildRuneTable()
		}

		score, end = fm.matchRunes(haystack)
	}

	score = clampScore(score, fm.needleLen)
	if score == 0 {
		// An exact match spans exactly needleLen runes.
		start = end - fm.needleLen + 1
	} else {
		start = fm.findStart(haystack, end)
	}

	if start < 0 {
		start = 0
	}

	fm.score = score
	fm.start = start
	fm.end = end
	if score == 0 && start == 0 {
		fm.haystackLen = utf8.RuneCountInString(haystack)
	}

	return score, start, end, score <= fm.maxDistance
}

// findStart locates the beginning of the best fuzzy match ending at end
// (rune index). It runs a second bit-vector pass with the reversed needle
// over the reversed haystack prefix; edit distance is symmetric under
// reversal, so the reverse pass reproduces the same score and its end
// position maps back to the start of the forward match. Only used for
// inexact matches, so it runs at most once per matched row per filter
// rebuild — never in the render hot path.
func (fm *FuzzyMatcher) findStart(haystack string, end int) int {
	rev := fm.reverseMatcher()

	runes := []rune(haystack)
	if end+1 < len(runes) {
		runes = runes[:end+1]
	}

	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}

	if rev.peqRune == nil {
		rev.buildRuneTable()
	}

	_, revEnd := rev.matchRunes(string(runes))
	return end - revEnd
}

// reverseMatcher lazily builds the matcher for the reversed needle.
func (fm *FuzzyMatcher) reverseMatcher() *FuzzyMatcher {
	if fm.rev == nil {
		r := []rune(fm.needle)
		for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
			r[i], r[j] = r[j], r[i]
		}
		fm.rev = NewFuzzyMatcher(string(r), fm.caseSensitive)
	}

	return fm.rev
}

// matchASCII runs the bit-vector scan over bytes; haystack must be pure
// ASCII, so byte positions equal rune positions. Returns the best score and
// the end position of the best match.
func (fm *FuzzyMatcher) matchASCII(haystack string) (bestScore, bestEnd int) {
	needleLen := fm.needleLen
	top := uint64(1) << uint(needleLen-1)
	pv := ^uint64(0)
	mv := uint64(0)
	score := needleLen
	bestScore = needleLen + 1
	bestEnd = 0

	for j := 0; j < len(haystack); j++ {
		eq := fm.peqAscii[haystack[j]]
		xv := eq | mv
		xh := (((eq & pv) + pv) ^ pv) | eq
		ph := mv | ^(xh | pv)
		mh := pv & xh

		if ph&top != 0 {
			score++
		} else if mh&top != 0 {
			score--
		}

		// Substring search: D[0][j] = 0, so the horizontal delta injected
		// into row 0 is 0 and both vectors shift in a zero bit.
		ph <<= 1
		mh <<= 1
		pv = mh | ^(xv | ph)
		mv = ph & xv

		if score < bestScore {
			bestScore = score
			bestEnd = j
			if bestScore == 0 {
				break // cannot do better; first exact match ends here
			}
		} else if score == bestScore {
			// Among equal-scoring matches prefer the latest end: it yields
			// the fullest span (e.g. "abxc" for needle "abc", not just "ab").
			bestEnd = j
		}
	}
	return bestScore, bestEnd
}

// matchRunes is the Unicode fallback of the same algorithm.
func (fm *FuzzyMatcher) matchRunes(haystack string) (bestScore, bestEnd int) {
	needleLen := fm.needleLen
	top := uint64(1) << uint(needleLen-1)
	pv := ^uint64(0)
	mv := uint64(0)
	score := needleLen
	bestScore = needleLen + 1
	bestEnd = 0
	j := 0

	for _, r := range haystack {
		eq := fm.peqRune[r]
		xv := eq | mv
		xh := (((eq & pv) + pv) ^ pv) | eq
		ph := mv | ^(xh | pv)
		mh := pv & xh

		if ph&top != 0 {
			score++
		} else if mh&top != 0 {
			score--
		}

		ph <<= 1
		mh <<= 1
		pv = mh | ^(xv | ph)
		mv = ph & xv

		if score < bestScore {
			bestScore = score
			bestEnd = j
			if bestScore == 0 {
				break
			}
		} else if score == bestScore {
			bestEnd = j // see matchASCII: latest end gives the fullest span
		}

		j++
	}

	return bestScore, bestEnd
}

// IsMatchExact reports whether the last Match call found the needle equal to
// the whole haystack (the canonical exact-hit test of the ranking guideline
// above).
func (fm *FuzzyMatcher) IsMatchExact() bool {
	return fm.score == 0 && fm.start == 0 && fm.haystackLen == fm.needleLen
}

// clampScore converts the "no text scanned" sentinel into the real distance
// (needle length: deleting the whole needle matches the empty substring).
func clampScore(bestScore, needleLen int) int {
	if bestScore > needleLen {
		return needleLen
	}
	return bestScore
}

func isASCIIString(str string) bool {
	for i := 0; i < len(str); i++ {
		if str[i] >= 0x80 {
			return false
		}
	}
	return true
}
