package vtui

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
)

// EmojiPresentationWide tells the layout engine how wide a character that
// carries an emoji presentation selector (U+FE0F) is on screen. Terminals
// disagree: wcwidth based ones keep the width of the base character, while
// most modern emulators advance two columns. Two columns is the common case,
// so it is the default; set this to false for a strictly wcwidth terminal.
var EmojiPresentationWide = true

// Composite grapheme clusters (a base character plus combining marks, an
// emoji ZWJ sequence, a flag) do not fit into a rune, so CharInfo.Char keeps
// an index into a process wide registry instead of a code point. Indices are
// marked with CompCharFlag; anything below it is a plain rune. This mirrors
// far2l's COMP_CHAR, which is why CharInfo.Char is 64 bit wide.
const (
	CompCharFlag uint64 = 1 << 63
	// MaxCompChar is the largest index the registry may hand out. It stays
	// far below WideCharFiller (all bits set) so the two can never collide.
	MaxCompChar uint64 = CompCharFlag | 0x00FFFFFF
)

// The code points the width rules below care about by name.
const (
	runeZWJ            rune = 0x200D
	runeVS15           rune = 0xFE0E
	runeVS16           rune = 0xFE0F
	runeKeycap         rune = 0x20E3
	runeModifierFirst  rune = 0x1F3FB
	runeModifierLast   rune = 0x1F3FF
	runeRegionalFirst  rune = 0x1F1E6
	runeRegionalLast   rune = 0x1F1FF
	runeReplacement    rune = 0xFFFD
	runeMarkBase       rune = 0x25CC // dotted circle: a base for a mark that has none
	runeControlVisible rune = '·'
)

type clusterRegistry struct {
	mu    sync.RWMutex
	byID  []string
	byStr map[string]uint64
}

var clusters = clusterRegistry{byStr: make(map[string]uint64)}

// IsCompChar reports whether a CharInfo.Char value is a registry index rather
// than a plain rune. WideCharFiller shares the high bit and is not one.
func IsCompChar(ch uint64) bool {
	return ch != WideCharFiller && ch&CompCharFlag != 0
}

// RegisterCluster turns a grapheme cluster into a CharInfo.Char value. Single
// rune clusters are stored as the rune itself, so the common path allocates
// nothing and old code comparing a cell against a rune keeps working. Longer
// ones go into the registry. If the registry is ever exhausted the base rune
// is returned, which loses the marks but never corrupts the screen.
func RegisterCluster(cluster string) uint64 {
	if cluster == "" {
		return 0
	}
	if r, size := utf8.DecodeRuneInString(cluster); size == len(cluster) {
		return uint64(r)
	}

	clusters.mu.RLock()
	id, ok := clusters.byStr[cluster]
	clusters.mu.RUnlock()
	if ok {
		return id
	}

	// Cache miss. Fold a decomposed sequence to its precomposed form first
	// (SanitizeCluster already does this for text that went through the
	// layout walk, so this only pays off for direct callers), then cache the
	// result under the raw spelling too: repeat frames stay allocation-free
	// whichever spelling arrives.
	raw := cluster
	if nfc := norm.NFC.String(cluster); nfc != cluster {
		cluster = nfc
	}

	clusters.mu.Lock()
	defer clusters.mu.Unlock()
	if id, ok := clusters.byStr[raw]; ok {
		return id
	}
	if r, size := utf8.DecodeRuneInString(cluster); size == len(cluster) {
		id = uint64(r) //nolint:gosec // DecodeRuneInString never returns a negative rune
	} else if existing, ok := clusters.byStr[cluster]; ok {
		id = existing
	} else {
		next := CompCharFlag | uint64(len(clusters.byID)+1)
		if next > MaxCompChar {
			r, _ := utf8.DecodeRuneInString(cluster)
			return uint64(r) //nolint:gosec // never negative; lossy fallback, deliberately not cached
		}
		clusters.byID = append(clusters.byID, cluster)
		clusters.byStr[cluster] = next
		id = next
	}
	if raw != cluster {
		clusters.byStr[raw] = id
	}
	return id
}

// CellString returns the text a cell carries. Fillers and empty cells render
// as nothing and a space respectively, which is what every backend wants.
func CellString(ch uint64) string {
	switch {
	case ch == WideCharFiller:
		return ""
	case ch == 0:
		return " "
	case IsCompChar(ch):
		idx := int(ch&^CompCharFlag) - 1
		clusters.mu.RLock()
		if idx >= 0 && idx < len(clusters.byID) {
			s := clusters.byID[idx]
			clusters.mu.RUnlock()
			return s
		}
		clusters.mu.RUnlock()
		return string(runeReplacement)
	default:
		return cellRuneString(ch)
	}
}

// asciiRuneStrings provides single-rune strings for ASCII code points.
var asciiRuneStrings [128]string

func init() {
	for i := 0; i < len(asciiRuneStrings); i++ {
		asciiRuneStrings[i] = string(rune(i))
	}
	buildClusterFormingTables()
}

// cellRuneCache is a direct-mapped cache of single-rune strings for non-ASCII runes.
var cellRuneCache = struct {
	sync.RWMutex
	slots [256]struct {
		ch  uint64
		str string
	}
}{}

func cellRuneString(ch uint64) string {
	if ch < 0x80 {
		return asciiRuneStrings[ch]
	}
	idx := int(ch & 255)
	cellRuneCache.RLock()
	slot := cellRuneCache.slots[idx]
	if slot.ch == ch {
		s := slot.str
		cellRuneCache.RUnlock()
		return s
	}
	cellRuneCache.RUnlock()

	s := string(rune(ch))
	cellRuneCache.Lock()
	cellRuneCache.slots[idx] = struct {
		ch  uint64
		str string
	}{ch, s}
	cellRuneCache.Unlock()
	return s
}

// CellRunes returns the runes a cell carries, base character first.
func CellRunes(ch uint64) []rune {
	if ch == WideCharFiller || ch == 0 {
		return nil
	}
	if !IsCompChar(ch) {
		return []rune{rune(ch)}
	}
	return []rune(CellString(ch))
}

// CellBaseRune returns the base character of a cell, ignoring any combining
// marks. Backends that can only draw one glyph per cell use this.
func CellBaseRune(ch uint64) rune {
	if ch == WideCharFiller {
		return 0
	}
	if !IsCompChar(ch) {
		return rune(ch)
	}
	r, _ := utf8.DecodeRuneInString(CellString(ch))
	return r
}

// runeCellWidth is the per rune column count a wcwidth driven terminal uses.
// go-runewidth does not treat every combining mark as zero wide (the
// Devanagari virama, for one), so the Unicode categories decide first: non
// spacing and enclosing marks and format characters take no room, spacing
// marks do.
func runeCellWidth(r rune) int {
	if unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf) {
		return 0
	}
	if unicode.Is(unicode.Mc, r) {
		// Spacing marks (the Devanagari ा, the Bengali া) advance one
		// column in every terminal, whatever a width table says about them.
		return 1
	}
	if w := runewidth.RuneWidth(r); w > 0 {
		return w
	}
	return 0
}

// ClusterWidth returns how many terminal columns a grapheme cluster occupies.
//
// The rule is the one Windows Terminal and ConPTY apply in their "grapheme
// clusters" measurement mode, and it lands on the same number a wcwidth
// terminal (VTE, xterm, foot, ...) reaches by treating every non spacing mark
// as zero: the columns of the code points are summed and the sum is clamped
// to two. Non spacing marks, enclosing marks and format characters are zero,
// spacing marks one, East Asian wide and fullwidth characters two, and U+FE0F
// two. Summing reproduces every emoji convention without naming it (a ZWJ
// sequence, a keycap, a flag and a skin tone all sum past two) and gives an
// Indic spacing mark or conjunct the columns the terminal really advances by:
// का is two cells and so is स्कृ, however one glyph the font makes of them.
// Counting such a cluster as one cell, as an earlier version did, is exactly
// what made every dialog drawn over Hindi text lean (unxed/f4#546).
func ClusterWidth(cluster string) int {
	if cluster == "" {
		return 0
	}
	if clusterColumns(cluster) >= 2 {
		return 2
	}
	// One column, or none at all: a cluster of nothing but zero width
	// characters still gets a cell, because SanitizeCluster gives it a base
	// to stand on and a caret needs somewhere to be.
	return 1
}

// clusterColumns is the unclamped column sum of a cluster; zero means the
// cluster has no base character of its own.
func clusterColumns(cluster string) int {
	sum := 0
	for _, r := range cluster {
		if r == runeVS16 {
			if EmojiPresentationWide {
				sum += 2
			}
			continue
		}
		sum += runeCellWidth(r)
	}
	return sum
}

// NextCluster splits off the first grapheme cluster of s. It returns the
// cluster, its width in columns and its size in bytes; size is zero only for
// an empty string.
func NextCluster(s string) (cluster string, width int, size int) {
	if s == "" {
		return "", 0, 0
	}
	g := uniseg.NewGraphemes(s)
	if !g.Next() {
		return "", 0, 0
	}
	from, to := g.Positions()
	cluster = s[from:to]
	return cluster, ClusterWidth(cluster), to - from
}

// SanitizeCluster makes a cluster safe to put on screen. Line breaks are
// dropped. Other control characters (C0, DEL, the C1 range, the Unicode line
// and paragraph separators) and lone format characters become a visible dot:
// a terminal either swallows them or executes them (U+0085 is a line feed,
// U+009B starts a control sequence) and in no case advances one column for
// them, which is what vtui has to count. A replacement character becomes a
// question mark, as before. A cluster with no base character (a lone
// combining mark) gets a dotted circle to sit on, so that it really occupies
// the one column it is given. The returned width is zero when the cluster
// must not be emitted at all.
func SanitizeCluster(cluster string) (string, int) {
	if cluster == "" {
		return "", 0
	}
	r, size := utf8.DecodeRuneInString(cluster)
	// Line breaks are dropped whether the cluster is a lone CR/LF or the
	// CRLF pair uniseg groups into one cluster; a raw \r\n must never reach
	// the screen buffer. The rune-by-rune fast path drops each rune
	// separately, so the two paths agree here.
	if r == '\n' || r == '\r' {
		return "", 0
	}
	switch {
	case r == runeReplacement && size == len(cluster):
		return "?", 1
	case isControlRune(r):
		return string(runeControlVisible), 1
	case size == len(cluster) && unicode.Is(unicode.Cf, r):
		return string(runeControlVisible), 1
	}
	// Fold a decomposed sequence to its precomposed form before the width
	// is taken, so width, stored cluster and cursor advance all agree, and
	// so a letter with an NFC equivalent ("и"+U+0306) reaches every backend
	// as the plain rune ("й") terminals render as one ordinary cell.
	if size != len(cluster) && !norm.NFC.IsNormalString(cluster) {
		cluster = norm.NFC.String(cluster)
	}
	if clusterColumns(cluster) == 0 {
		cluster = string(runeMarkBase) + cluster
	}
	return cluster, ClusterWidth(cluster)
}

// isControlRune reports the code points a terminal must never receive as
// text: C0 and C1 controls, DEL, and the Unicode line and paragraph
// separators.
func isControlRune(r rune) bool {
	return r < 0x20 || (r >= 0x7F && r <= 0x9F) || r == '\u2028' || r == '\u2029'
}

// clusterFormingRange is a closed interval of code points that can join a
// multi-rune grapheme cluster under UAX #29.
type clusterFormingRange struct {
	lo, hi rune
}

// clusterFormingBMP is a 65536-bit bitmap of the BMP code points that can
// join a multi-rune grapheme cluster; clusterFormingSup is the sorted,
// coalesced range list for the supplementary planes. Both are built once from
// the Mn/Me/Mc/Cf categories plus the sequence runes they miss (Hangul jamo;
// emoji modifiers and regional indicators). A membership test is one bitmap
// word load in the BMP and one binary search above it — a single rule for
// every script, no per-language cases.
const bmpClusterWords = 1 << (16 - 6)

var (
	clusterFormingBMP [bmpClusterWords]uint64
	clusterFormingSup []clusterFormingRange
)

func buildClusterFormingTables() {
	setBMPRange := func(lo, hi rune) {
		for cp := lo; cp <= hi; cp++ {
			clusterFormingBMP[cp>>6] |= 1 << (cp & 63)
		}
	}
	addTable := func(rt *unicode.RangeTable) {
		for _, r := range rt.R16 {
			stride := rune(r.Stride)
			if stride <= 1 {
				setBMPRange(rune(r.Lo), rune(r.Hi))
			} else {
				for cp := rune(r.Lo); cp <= rune(r.Hi); cp += stride {
					clusterFormingBMP[cp>>6] |= 1 << (cp & 63)
				}
			}
		}
		for _, r := range rt.R32 {
			clusterFormingSup = append(clusterFormingSup, clusterFormingRange{lo: rune(r.Lo), hi: rune(r.Hi)})
		}
	}
	addTable(unicode.Mn)
	addTable(unicode.Me)
	addTable(unicode.Mc)
	addTable(unicode.Cf)
	// Sequence runes outside those categories.
	setBMPRange(0x1100, 0x11FF) // Hangul jamo L/V/T
	setBMPRange(0xA960, 0xA97F) // Hangul jamo extended-A
	setBMPRange(0xD7B0, 0xD7FF) // Hangul jamo extended-B
	clusterFormingSup = append(clusterFormingSup,
		clusterFormingRange{lo: runeModifierFirst, hi: runeModifierLast}, // emoji modifiers (skin tones)
		clusterFormingRange{lo: runeRegionalFirst, hi: runeRegionalLast}, // regional indicators (flags)
	)
	sort.Slice(clusterFormingSup, func(i, j int) bool { return clusterFormingSup[i].lo < clusterFormingSup[j].lo })
	// Coalesce overlapping and adjacent intervals into a compact table.
	out := clusterFormingSup[:0]
	for _, r := range clusterFormingSup {
		if n := len(out); n > 0 && r.lo <= out[n-1].hi+1 {
			if r.hi > out[n-1].hi {
				out[n-1].hi = r.hi
			}
		} else {
			out = append(out, r)
		}
	}
	clusterFormingSup = out
}

// isClusterFormingRune reports whether r can join its neighbours into a
// multi-rune grapheme cluster.
func isClusterFormingRune(r rune) bool {
	if r < 0x80 {
		return false
	}
	if r < 0x10000 {
		return clusterFormingBMP[r>>6]&(1<<(r&63)) != 0
	}
	tbl := clusterFormingSup
	i := sort.Search(len(tbl), func(i int) bool { return tbl[i].hi >= r })
	return i < len(tbl) && tbl[i].lo <= r
}

// scanNeedsFullSegmentation reports whether s contains a rune that can join a
// multi-rune grapheme cluster. ASCII bytes are skipped without decoding, so
// plain names scan at byte speed; the full uniseg walk is reserved for the
// strings that actually need it.
func scanNeedsFullSegmentation(s string) bool {
	for offset := 0; offset < len(s); {
		c := s[offset]
		if c < 0x80 {
			offset++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[offset:])
		if isClusterFormingRune(r) {
			return true
		}
		offset += size
	}
	return false
}

// simpleRuneWidth is the display width of a single rune that needs no
// grapheme segmentation, applying the same sanitize rules as
// SanitizeCluster: line breaks are dropped (width 0), other control
// characters and the replacement character get a visible width, everything
// else takes its rune width (at least 1).
func simpleRuneWidth(r rune) int {
	switch {
	case r == '\n' || r == '\r':
		return 0
	case r == runeReplacement:
		return 1
	case isControlRune(r):
		return 1
	default:
		if w := runewidth.RuneWidth(r); w > 0 {
			return w
		}
		return 1
	}
}

// forEachClusterSimple is ForEachClusterAt for strings that need no grapheme
// segmentation: every rune is its own cluster. Only call it when
// scanNeedsFullSegmentation(s) is false; its output matches the uniseg walk
// exactly for such strings.
func forEachClusterSimple(s string, fn func(cluster string, width, offset, runeIndex int)) {
	runeIndex := 0
	for offset := 0; offset < len(s); {
		r, size := utf8.DecodeRuneInString(s[offset:])
		switch {
		case r == '\n' || r == '\r':
			// dropped, like SanitizeCluster
		case r == runeReplacement:
			fn("?", 1, offset, runeIndex)
		case isControlRune(r):
			fn(string(runeControlVisible), 1, offset, runeIndex)
		default:
			w := runewidth.RuneWidth(r)
			if w < 1 {
				w = 1
			}
			fn(s[offset:offset+size], w, offset, runeIndex)
		}
		runeIndex++
		offset += size
	}
}

// forEachClusterUniseg is the reference UAX #29 walk, kept separate from the
// simple path so tests can compare the two on the same input.
func forEachClusterUniseg(s string, fn func(cluster string, width, offset, runeIndex int)) {
	runeIndex := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, to := g.Positions()
		raw := s[from:to]
		text, w := SanitizeCluster(raw)
		if w > 0 {
			fn(text, w, from, runeIndex)
		}
		runeIndex += utf8.RuneCountInString(raw)
	}
}

// ForEachCluster walks s cluster by cluster, handing the callback the
// sanitized text, its width and the byte offset the cluster started at in s.
// Clusters that must not be drawn are skipped.
func ForEachCluster(s string, fn func(cluster string, width int, offset int)) {
	ForEachClusterAt(s, func(cluster string, width, offset, _ int) {
		fn(cluster, width, offset)
	})
}

// ForEachClusterAt is ForEachCluster with the index of the cluster's first
// rune in s as well. Positions coming from code that counts runes, such as
// the hotkey position of an ampersand string, need it.
//
// Strings that cannot form multi-rune clusters (no combining marks, ZWJ,
// emoji sequences, Hangul jamo) take a rune-by-rune path that skips the
// uniseg state machine entirely; the full segmentation is reserved for the
// strings that need it.
func ForEachClusterAt(s string, fn func(cluster string, width, offset, runeIndex int)) {
	if !scanNeedsFullSegmentation(s) {
		forEachClusterSimple(s, fn)
		return
	}
	forEachClusterUniseg(s, fn)
}

// forEachDisplayCluster walks the terminal-level clusters used by layout and
// screen rendering. It keeps the fast path for strings that cannot contain a
// multi-rune cluster, while joining Indic virama sequences in the same way as
// the editor and bidi code.
func forEachDisplayCluster(s string, fn func(cluster string, width, offset, runeIndex int)) {
	if !scanNeedsFullSegmentation(s) {
		forEachClusterSimple(s, fn)
		return
	}
	forEachTerminalCluster(s, fn)
}

// forEachTerminalCluster walks the clusters that the terminal treats as one
// cell-level editing and rendering unit. Unicode grapheme tables used by
// older terminal stacks can split an Indic virama from the following
// consonant, even though the shaping engine renders the pair as one glyph.
// Keep that virama-consonant join in the same path used by editors, bidi, and
// screen rendering so a caret can never land in the middle of a shaped unit.
func forEachTerminalCluster(s string, fn func(cluster string, width, offset, runeIndex int)) {
	var previous string
	previousOffset, previousRuneIndex := 0, 0
	havePrevious := false
	emit := func() {
		if !havePrevious {
			return
		}
		cluster, width := SanitizeCluster(previous)
		if width > 0 {
			fn(cluster, ClusterWidth(cluster), previousOffset, previousRuneIndex)
		}
	}

	runeIndex := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, to := g.Positions()
		current := s[from:to]
		if havePrevious && JoinsConjunct(previous, current) {
			previous += current
		} else {
			emit()
			previous = current
			previousOffset, previousRuneIndex = from, runeIndex
			havePrevious = true
		}
		runeIndex += utf8.RuneCountInString(current)
	}
	emit()
}

func forEachTerminalClusterRaw(s string, fn func(cluster string, offset, runeIndex int)) {
	var previous string
	previousOffset, previousRuneIndex := 0, 0
	havePrevious := false
	emit := func() {
		if havePrevious {
			fn(previous, previousOffset, previousRuneIndex)
		}
	}

	runeIndex := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, to := g.Positions()
		current := s[from:to]
		if havePrevious && JoinsConjunct(previous, current) {
			previous += current
		} else {
			emit()
			previous, previousOffset, previousRuneIndex = current, from, runeIndex
			havePrevious = true
		}
		runeIndex += utf8.RuneCountInString(current)
	}
	emit()
}

// JoinsConjunct reports whether a terminal draws prev and next as one cell
// level unit: prev ends in a virama that Unicode classes as
// Indic_Conjunct_Break=Linker and next starts with a consonant of the same
// script (Devanagari, Bengali, Gujarati, Oriya, Telugu or Malayalam in the
// 16.0 tables). That is UAX #29 rule GB9c the way Windows Terminal and ConPTY
// apply it, pairwise, from their Unicode 16.0 tables; uniseg's older tables
// split the pair, so the walkers here glue it back together. Viramas of the
// other Indic scripts (Kannada, Tamil, Sinhala, ...) do not join under GB9c,
// and the terminal keeps them apart too, so neither does this.
func JoinsConjunct(prev, next string) bool {
	last, _ := utf8.DecodeLastRuneInString(prev)
	first, _ := utf8.DecodeRuneInString(next)
	return isConjunctLinker(last) && isConjunctConsonant(first)
}

// isConjunctLinker is Indic_Conjunct_Break=Linker, Unicode 16.0.
func isConjunctLinker(r rune) bool {
	switch r {
	case '\u094D', '\u09CD', '\u0ACD', '\u0B4D', '\u0C4D', '\u0D4D':
		return true
	}
	return false
}

// conjunctConsonants is Indic_Conjunct_Break=Consonant, Unicode 16.0, as
// closed ranges sorted by their first code point.
var conjunctConsonants = [...][2]rune{
	{0x0915, 0x0939}, {0x0958, 0x095F}, {0x0978, 0x097F}, // Devanagari
	{0x0995, 0x09A8}, {0x09AA, 0x09B0}, {0x09B2, 0x09B2}, {0x09B6, 0x09B9}, {0x09DC, 0x09DD}, {0x09DF, 0x09DF}, {0x09F0, 0x09F1}, // Bengali
	{0x0A95, 0x0AA8}, {0x0AAA, 0x0AB0}, {0x0AB2, 0x0AB3}, {0x0AB5, 0x0AB9}, {0x0AF9, 0x0AF9}, // Gujarati
	{0x0B15, 0x0B28}, {0x0B2A, 0x0B30}, {0x0B32, 0x0B33}, {0x0B35, 0x0B39}, {0x0B5C, 0x0B5D}, {0x0B5F, 0x0B5F}, {0x0B71, 0x0B71}, // Oriya
	{0x0C15, 0x0C28}, {0x0C2A, 0x0C39}, {0x0C58, 0x0C5A}, // Telugu
	{0x0D15, 0x0D3A}, // Malayalam
}

func isConjunctConsonant(r rune) bool {
	if r < 0x0915 || r > 0x0D3A {
		return false
	}
	for _, rg := range conjunctConsonants {
		if r < rg[0] {
			return false
		}
		if r <= rg[1] {
			return true
		}
	}
	return false
}

// AppendCluster puts a cluster into a cell slice, following it with as many
// fillers as the extra columns it claims.
func AppendCluster(target []CharInfo, cluster string, width int, attr uint64) []CharInfo {
	if width <= 0 {
		return target
	}
	target = append(target, CharInfo{Char: RegisterCluster(cluster), Attributes: attr})
	for i := 1; i < width; i++ {
		target = append(target, CharInfo{Char: WideCharFiller, Attributes: attr})
	}
	return target
}

// StringWidth returns the width of a string in terminal columns, counting
// terminal display clusters rather than runes.
func StringWidth(s string) int {
	total := 0
	forEachDisplayCluster(s, func(_ string, w, _, _ int) {
		total += w
	})
	return total
}

// measureWidthSimple returns the sanitized display width of s for strings
// that need no grapheme segmentation. Only call it when
// scanNeedsFullSegmentation(s) is false.
func measureWidthSimple(s string) int {
	total := 0
	for offset := 0; offset < len(s); {
		r, size := utf8.DecodeRuneInString(s[offset:])
		total += simpleRuneWidth(r)
		offset += size
	}
	return total
}

// truncateSimple truncates s without grapheme segmentation (call only when
// scanNeedsFullSegmentation(s) is false). It returns the truncated string
// and its width; the width equals measureWidthSimple of the result.
func truncateSimple(s string, w int, tail string) (string, int) {
	total := 0
	fits := true
	for offset := 0; offset < len(s); {
		r, size := utf8.DecodeRuneInString(s[offset:])
		if cw := simpleRuneWidth(r); cw > 0 {
			total += cw
			if total > w {
				fits = false
				break
			}
		}
		offset += size
	}
	if fits {
		return s, total
	}

	tailW := measureWidthSimple(tail)
	budget := w - tailW
	if budget < 0 {
		return tail, tailW
	}

	var sb strings.Builder
	used := 0
	for offset := 0; offset < len(s); {
		r, size := utf8.DecodeRuneInString(s[offset:])
		cw := simpleRuneWidth(r)
		if cw == 0 {
			offset += size
			continue
		}
		if used+cw > budget {
			break
		}
		switch {
		case r == runeReplacement:
			sb.WriteByte('?')
		case isControlRune(r):
			sb.WriteRune(runeControlVisible)
		default:
			sb.WriteRune(r)
		}
		used += cw
		offset += size
	}
	return sb.String() + tail, used + tailW
}

// TruncateString shortens s so that it plus tail fits into w columns. It never
// cuts a grapheme cluster in half and never leaves a wide character with only
// one of its two columns on screen.
func TruncateString(s string, w int, tail string) string {
	res, _ := truncateStringWidth(s, w, tail)
	return res
}

// truncateStringWidth is TruncateString plus the display width of the result.
// Returning the width saves callers a second measurement; the common path
// (the string fits) costs a single rune walk.
func truncateStringWidth(s string, w int, tail string) (string, int) {
	if w <= 0 {
		return "", 0
	}
	if !scanNeedsFullSegmentation(s) {
		return truncateSimple(s, w, tail)
	}
	if sw := StringWidth(s); sw <= w {
		return s, sw
	}
	tailW := StringWidth(tail)
	budget := w - tailW
	if budget < 0 {
		return tail, tailW
	}

	var sb strings.Builder
	used := 0
	stopped := false
	forEachDisplayCluster(s, func(text string, cw, _, _ int) {
		if stopped {
			return
		}
		if used+cw > budget {
			stopped = true
			return
		}
		sb.WriteString(text)
		used += cw
	})
	return sb.String() + tail, used + tailW
}
