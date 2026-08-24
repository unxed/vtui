package vtui

// Backspace granularity.
//
// Cursor movement, selection and forward Delete all step over a whole
// user-perceived character; Backspace does not. The de facto behaviour, which
// UAX #29 explicitly allows ("the backspace key might delete by code point,
// while the delete key may delete an entire cluster") and which Windows edit
// controls, Notepad and the browsers implement, is to peel one code point off
// the end of the preceding cluster. Deleting the base character forwards takes
// its combining marks with it, so nothing is orphaned; deleting a trailing
// mark backwards is safe on its own and lets a mistyped composition be
// corrected without retyping the whole syllable.
//
// The exception everyone also makes is emoji: a ZWJ sequence, a keycap, a flag
// or a skin-tone modifier is atomic in both directions, because its parts are
// not something the user composed on purpose one keystroke at a time.
//
// See w3c/i18n-drafts "Cursor Movement and Deletion of Unicode Text" and
// unxed/f4#546.

// clusterIsAtomicForBackspace reports whether a cluster must be removed whole
// rather than one code point at a time.
func clusterIsAtomicForBackspace(cluster []rune) bool {
	if len(cluster) <= 1 {
		return true
	}
	regional := 0
	for _, r := range cluster {
		switch {
		case r == runeZWJ, r == runeVS16, r == runeKeycap:
			return true
		case r >= runeModifierFirst && r <= runeModifierLast:
			return true
		case r >= runeRegionalFirst && r <= runeRegionalLast:
			regional++
		}
	}
	return regional >= 2
}

// backspaceStart narrows the range [start,end) that Backspace is about to
// remove down to the last code point of that range, unless the range is one of
// the sequences that stay atomic. Callers pass a whole cluster; the caret
// therefore still only ever sits on a cluster boundary once the cluster has
// been consumed completely.
func backspaceStart(text []rune, start, end int) int {
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	if end-start <= 1 {
		return start
	}
	if clusterIsAtomicForBackspace(text[start:end]) {
		return start
	}
	return end - 1
}
