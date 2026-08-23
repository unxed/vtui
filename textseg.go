package vtui

import (
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

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
	r, size := utf8.DecodeRuneInString(cluster)
	if size == len(cluster) {
		return uint64(r)
	}

	clusters.mu.RLock()
	id, ok := clusters.byStr[cluster]
	clusters.mu.RUnlock()
	if ok {
		return id
	}

	clusters.mu.Lock()
	defer clusters.mu.Unlock()
	if id, ok := clusters.byStr[cluster]; ok {
		return id
	}
	next := CompCharFlag | uint64(len(clusters.byID)+1)
	if next > MaxCompChar {
		return uint64(r)
	}
	clusters.byID = append(clusters.byID, cluster)
	clusters.byStr[cluster] = next
	return next
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
	if w := runewidth.RuneWidth(r); w > 0 {
		return w
	}
	return 0
}

// ClusterWidth returns how many terminal columns a grapheme cluster occupies.
// The base is the sum of the per rune widths, which is what a wcwidth driven
// terminal does, with the emoji sequences that every terminal special cases
// pinned to two columns.
func ClusterWidth(cluster string) int {
	if cluster == "" {
		return 0
	}
	if r, size := utf8.DecodeRuneInString(cluster); size == len(cluster) {
		// Single rune: no sequence to examine, except the few runes whose
		// width rules flip on a lone occurrence (see the switch below).
		switch r {
		case runeZWJ, runeVS15, runeVS16, runeKeycap:
			// fall through to the sequence rules
		default:
			if w := runeCellWidth(r); w > 0 {
				return w
			}
			return 1
		}
	}

	sum := 0
	regional := 0
	hasZWJ := false
	hasVS16 := false
	hasVS15 := false
	hasKeycap := false
	hasModifier := false

	for _, r := range cluster {
		switch {
		case r == runeZWJ:
			hasZWJ = true
		case r == runeVS16:
			hasVS16 = true
		case r == runeVS15:
			hasVS15 = true
		case r == runeKeycap:
			hasKeycap = true
		case r >= runeModifierFirst && r <= runeModifierLast:
			hasModifier = true
		case r >= runeRegionalFirst && r <= runeRegionalLast:
			regional++
		}
		sum += runeCellWidth(r)
	}

	switch {
	case hasZWJ, hasKeycap, hasModifier, regional >= 2:
		return 2
	case hasVS15:
		return 1
	case hasVS16 && EmojiPresentationWide:
		return 2
	}

	// Terminal emulators shape Indic, Arabic, Hebrew, Thai, and related
	// scripts inside a cell cluster. Spacing marks and virama sequences are
	// therefore not additional terminal columns even though their Unicode
	// categories can make the wcwidth sum look wider. Keep wide base glyphs
	// wide, but count a shaped narrow cluster as one cell.
	if isShapedCluster(cluster) {
		for _, r := range cluster {
			if runeCellWidth(r) >= 2 {
				return 2
			}
		}
		return 1
	}

	if sum <= 0 {
		// A cluster of nothing but combining marks: no base to hang them on,
		// so give it a column of its own rather than dropping it.
		return 1
	}
	return sum
}

// isShapedCluster reports whether a grapheme cluster belongs to a script
// whose combining marks and consonant sequences are laid out as one terminal
// glyph cell. This is a cell-width policy, not a font shaper: the complete
// cluster is retained in CharInfo so the terminal can perform its own glyph
// shaping when the frame is flushed.
func isShapedCluster(cluster string) bool {
	runeCount := 0
	shapedScript := false
	for _, r := range cluster {
		runeCount++
		if unicode.In(r,
			unicode.Arabic,
			unicode.Bengali,
			unicode.Devanagari,
			unicode.Gujarati,
			unicode.Gurmukhi,
			unicode.Hebrew,
			unicode.Kannada,
			unicode.Khmer,
			unicode.Lao,
			unicode.Malayalam,
			unicode.Myanmar,
			unicode.Oriya,
			unicode.Sinhala,
			unicode.Tamil,
			unicode.Telugu,
			unicode.Thai,
		) {
			shapedScript = true
		}
	}
	return shapedScript && runeCount > 1
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
// dropped, other control characters become a visible dot, and a replacement
// character becomes a question mark, as before. The returned width is zero
// when the cluster must not be emitted at all.
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
	if size == len(cluster) {
		switch {
		case r == runeReplacement:
			return "?", 1
		case r < 0x20 || r == 0x7F:
			return string(runeControlVisible), 1
		}
	}
	return cluster, ClusterWidth(cluster)
} // clusterFormingRange is a closed interval of code points that can join a
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
	case r < 0x20 || r == 0x7F:
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
		case r < 0x20 || r == 0x7F:
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
		if havePrevious && endsInIndicVirama(previous) && startsWithLetter(current) {
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
		if havePrevious && endsInIndicVirama(previous) && startsWithLetter(current) {
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

func startsWithLetter(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLetter(r)
}

func endsInIndicVirama(s string) bool {
	r, _ := utf8.DecodeLastRuneInString(s)
	switch r {
	case
		'\u094D',                     // Devanagari
		'\u09CD',                     // Bengali
		'\u0A4D',                     // Gurmukhi
		'\u0ACD',                     // Gujarati
		'\u0B4D',                     // Oriya
		'\u0BCD',                     // Tamil
		'\u0C4D',                     // Telugu
		'\u0CCD',                     // Kannada
		'\u0D3B', '\u0D3C', '\u0D4D', // Malayalam
		'\u0DCA', // Sinhala
		'\u1039', // Myanmar
		'\u1714', // Tagalog
		'\u17D2', // Khmer
		'\u1A60', // Tai Tham
		'\u1BAA', // Sundanese
		'\uA806', // Syloti Nagri
		'\uA8C4', // Saurashtra
		'\uA953', // Rejang
		'\uA9C0', // Javanese
		'\uAAF6', // Meetei Mayek
		'\uABED': // Meetei Mayek
		return true
	default:
		return false
	}
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
// grapheme clusters rather than runes.
func StringWidth(s string) int {
	if !scanNeedsFullSegmentation(s) {
		return measureWidthSimple(s)
	}
	total := 0
	forEachClusterUniseg(s, func(_ string, w, _, _ int) {
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
		case r < 0x20 || r == 0x7F:
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
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		from, to := g.Positions()
		text, cw := SanitizeCluster(s[from:to])
		if cw == 0 {
			continue
		}
		if used+cw > budget {
			break
		}
		sb.WriteString(text)
		used += cw
	}
	return sb.String() + tail, used + tailW
}
