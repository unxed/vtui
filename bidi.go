package vtui

import (
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/unxed/vtui/internal/uba"
	"golang.org/x/text/unicode/bidi"
)

// BidiMode selects how much of UAX #9 is applied.
type BidiMode int

const (
	BidiOff     BidiMode = iota // strings are laid out as stored
	BidiDisplay                 // strings are reordered for display
	BidiFull                    // reordering plus caret and input support
)

var DefaultBidiMode = BidiDisplay

// BidiParagraph is the base direction a line is laid out in, the "higher
// level protocol" of UAX #9 HL1.
type BidiParagraph int

const (
	// BidiParagraphLTR lays every line out left to right: a right to left
	// word is reversed in place, the line as a whole is not. This is what
	// Notepad, a browser text field and every left to right user interface
	// do, and it is the default. Detecting the direction from the text
	// instead (P2, P3) turns a line that merely starts with a right to left
	// word inside out, which is what unxed/f4#546 reported as "f4 changed
	// the word order".
	BidiParagraphLTR BidiParagraph = iota
	// BidiParagraphRTL lays every line out right to left.
	BidiParagraphRTL
	// BidiParagraphAuto takes the direction of the first strong character
	// of each line (UAX #9 P2, P3), falling back to left to right.
	BidiParagraphAuto
)

// DefaultBidiParagraph is the base direction used when a caller does not
// specify one. An application localized into a right to left language may
// set it to BidiParagraphRTL or BidiParagraphAuto.
var DefaultBidiParagraph = BidiParagraphLTR

// HasRTL checks if the string contains any strong RTL characters.
func HasRTL(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			for _, r := range s[i:] {
				if isStrongRTLRune(r) {
					return true
				}
			}
			return false
		}
	}
	return false
}

// isStrongRTLRune reports the code points that can make a line need
// reordering: the right to left scripts and the right to left directional
// formatting characters.
func isStrongRTLRune(r rune) bool {
	switch {
	case r < 0x0590:
		return false
	case r <= 0x08FF: // Hebrew, Arabic, Syriac, Thaana, NKo, Samaritan, Mandaic, Arabic Extended
		return true
	case r == 0x200F, r == 0x202B, r == 0x202E, r == 0x2067: // RLM, RLE, RLO, RLI
		return true
	case r >= 0xFB1D && r <= 0xFEFC: // Hebrew and Arabic presentation forms
		return true
	case r >= 0x10800 && r <= 0x10FFF: // the right to left scripts of the SMP
		return true
	case r >= 0x1E800 && r <= 0x1EFFF: // Mende Kikakui, Adlam, Arabic mathematical symbols
		return true
	}
	return false
}

// MirrorRune returns the Bidi_Mirroring_Glyph of r: the code point drawn in
// its place when r is read right to left (UAX #9 L4). ok is false when r
// has no mirrored form.
func MirrorRune(r rune) (mirrored rune, ok bool) {
	i := sort.Search(len(bidiMirrorPairs), func(i int) bool { return bidiMirrorPairs[i][0] >= r })
	if i < len(bidiMirrorPairs) && bidiMirrorPairs[i][0] == r {
		return bidiMirrorPairs[i][1], true
	}
	return r, false
}

// ClusterSpan is the byte range of one grapheme cluster in a logical string.
type ClusterSpan struct {
	Start, End int
}

// BidiLayout is the visual arrangement of the clusters of one line, as
// computed by LayoutBidi.
type BidiLayout struct {
	// Level is the resolved embedding level of each logical cluster (UAX
	// #9); odd levels read right to left.
	Level []int
	// VisualToLogical lists the logical cluster indices in the order the
	// clusters appear on screen, left to right.
	VisualToLogical []int
	// LogicalToVisual is the inverse: the screen position of each logical
	// cluster.
	LogicalToVisual []int
	// Mirrored holds, for the clusters whose glyph is mirrored because they
	// read right to left (a bracket, a guillemet), the text to draw in
	// place of the stored one. It is nil when nothing is mirrored.
	Mirrored map[int]string
}

// Len returns the number of clusters.
func (l *BidiLayout) Len() int { return len(l.Level) }

// IsRTL reports whether logical cluster i reads right to left.
func (l *BidiLayout) IsRTL(i int) bool { return l.Level[i]&1 == 1 }

// Text returns what to draw for logical cluster i, whose stored text is
// stored: the mirrored glyph if it has one, the stored text otherwise.
func (l *BidiLayout) Text(i int, stored string) string {
	if m, ok := l.Mirrored[i]; ok {
		return m
	}
	return stored
}

// CaretVisual maps a logical caret position, the boundary b between
// clusters b-1 and b (0 to Len), to the visual boundary the caret is drawn
// at. The caret stands at the trailing edge of the cluster it follows: the
// right edge of a left to right cluster, the left edge of a right to left
// one. So the caret moves visually right across Latin, jumps to the far
// right of a right to left word on entering it and walks left through it,
// and after the last letter of a right to left word it sits at that word's
// left edge, which is where the next letter of it will appear. That is the
// convention of Notepad and of the Windows edit controls. At the start of
// the line the caret takes the leading edge of the first cluster.
func (l *BidiLayout) CaretVisual(b int) int {
	n := l.Len()
	if n == 0 {
		return 0
	}
	if b <= 0 {
		if l.IsRTL(0) {
			return l.LogicalToVisual[0] + 1
		}
		return l.LogicalToVisual[0]
	}
	if b > n {
		b = n
	}
	c := b - 1
	if l.IsRTL(c) {
		return l.LogicalToVisual[c]
	}
	return l.LogicalToVisual[c] + 1
}

// CaretLogical is the inverse of CaretVisual for hit testing: the logical
// caret position that a click at visual boundary v (0 to Len) selects. The
// cluster to the right of the boundary decides: its logical start if it
// reads left to right, its logical end if it reads right to left; past the
// last cluster the one to the left decides the other way round.
func (l *BidiLayout) CaretLogical(v int) int {
	n := l.Len()
	if n == 0 {
		return 0
	}
	if v < 0 {
		v = 0
	}
	if v < n {
		c := l.VisualToLogical[v]
		if l.IsRTL(c) {
			return c + 1
		}
		return c
	}
	c := l.VisualToLogical[n-1]
	if l.IsRTL(c) {
		return c
	}
	return c + 1
}

func identityLayout(n int) BidiLayout {
	lay := BidiLayout{
		Level:           make([]int, n),
		VisualToLogical: make([]int, n),
		LogicalToVisual: make([]int, n),
	}
	for i := 0; i < n; i++ {
		lay.VisualToLogical[i] = i
		lay.LogicalToVisual[i] = i
	}
	return lay
}

// LayoutBidi runs the Unicode Bidirectional Algorithm (UAX #9) over one line
// and tells where each of its grapheme clusters goes on screen. text is the
// line in logical order and clusters its grapheme clusters, in order,
// covering it; they may come from any walker (a terminal cluster, a UAX #29
// cluster), the algorithm only needs their byte ranges. A cluster takes the
// level of its first code point, rules L1 and L2 are applied over whole
// clusters, and L4 mirroring is recorded per cluster, so a mark never parts
// from its base. Lines that contain no right to left text come back as the
// identity without running the algorithm.
func LayoutBidi(text string, clusters []ClusterSpan, dir BidiParagraph) BidiLayout {
	n := len(clusters)
	if n == 0 {
		return identityLayout(0)
	}
	if dir != BidiParagraphRTL && !HasRTL(text) {
		return identityLayout(n)
	}

	types := make([]uba.Class, 0, len(text))
	pairTypes := make([]uba.Bracket, 0, len(text))
	pairValues := make([]rune, 0, len(text))
	runeCluster := make([]int, 0, len(text))
	ci := 0
	for i, r := range text {
		for ci < n-1 && i >= clusters[ci].End {
			ci++
		}
		props, _ := bidi.LookupRune(r)
		cls := uba.Class(props.Class())
		if cls == uba.B {
			// A line holds no paragraph separator; a stray one (a CR the
			// caller kept) is whitespace here.
			cls = uba.WS
		}
		types = append(types, cls)
		switch {
		case props.IsOpeningBracket():
			pairTypes = append(pairTypes, uba.BracketOpen)
			pairValues = append(pairValues, r)
		case props.IsBracket():
			// BD16 pairs a closer with its opener, so the closer carries the
			// opener's code point.
			opener, _ := MirrorRune(r)
			pairTypes = append(pairTypes, uba.BracketClose)
			pairValues = append(pairValues, opener)
		default:
			pairTypes = append(pairTypes, uba.BracketNone)
			pairValues = append(pairValues, 0)
		}
		runeCluster = append(runeCluster, ci)
	}

	paragraph := uba.ParagraphAuto
	switch dir {
	case BidiParagraphLTR:
		paragraph = uba.ParagraphLTR
	case BidiParagraphRTL:
		paragraph = uba.ParagraphRTL
	}
	levels, base, err := uba.Levels(types, pairTypes, pairValues, paragraph)
	if err != nil {
		return identityLayout(n)
	}

	lay := identityLayout(n)
	for i := range lay.Level {
		lay.Level[i] = base
	}
	seen := make([]bool, n)
	for ri, c := range runeCluster {
		if !seen[c] {
			seen[c] = true
			lay.Level[c] = levels[ri]
		}
	}

	// L2: from the highest level down to the lowest odd level, reverse
	// every maximal run of clusters at that level or higher.
	highest, lowestOdd := 0, -1
	for _, lvl := range lay.Level {
		if lvl > highest {
			highest = lvl
		}
		if lvl&1 == 1 && (lowestOdd < 0 || lvl < lowestOdd) {
			lowestOdd = lvl
		}
	}
	order := lay.VisualToLogical
	if lowestOdd >= 0 {
		for lvl := highest; lvl >= lowestOdd; lvl-- {
			for i := 0; i < n; {
				if lay.Level[order[i]] < lvl {
					i++
					continue
				}
				j := i
				for j < n && lay.Level[order[j]] >= lvl {
					j++
				}
				for a, b := i, j-1; a < b; a, b = a+1, b-1 {
					order[a], order[b] = order[b], order[a]
				}
				i = j
			}
		}
	}
	for v, c := range order {
		lay.LogicalToVisual[c] = v
	}

	// L4: a cluster read right to left draws the mirrored form of its base.
	for c := 0; c < n; c++ {
		if !lay.IsRTL(c) {
			continue
		}
		stored := text[clusters[c].Start:clusters[c].End]
		r, size := utf8.DecodeRuneInString(stored)
		if m, ok := MirrorRune(r); ok {
			if lay.Mirrored == nil {
				lay.Mirrored = make(map[int]string)
			}
			lay.Mirrored[c] = string(m) + stored[size:]
		}
	}
	return lay
}

// terminalClusterSpans walks s with the terminal cluster walker and returns
// the byte span and the logical rune index of each cluster.
func terminalClusterSpans(s string) (spans []ClusterSpan, runeIndex []int) {
	forEachTerminalClusterRaw(s, func(text string, offset, runeIdx int) {
		spans = append(spans, ClusterSpan{Start: offset, End: offset + len(text)})
		runeIndex = append(runeIndex, runeIdx)
	})
	return spans, runeIndex
}

// layoutString is the layout of s for the string level helpers below: the
// identity when bidi is off or the string has no right to left text.
func layoutString(s string, spans []ClusterSpan) BidiLayout {
	if DefaultBidiMode == BidiOff || !HasRTL(s) {
		return identityLayout(len(spans))
	}
	return LayoutBidi(s, spans, DefaultBidiParagraph)
}

// VisualString reorders s from logical to visual order.
func VisualString(s string) string {
	if DefaultBidiMode == BidiOff || !HasRTL(s) {
		return s
	}
	v, _ := VisualStringWithMap(s)
	return v
}

// VisualStringWithMap does the same and returns, for each cluster in
// visual order, the byte offset it had in the logical string.
func VisualStringWithMap(s string) (string, []int) {
	spans, _ := terminalClusterSpans(s)
	lay := layoutString(s, spans)
	offsets := make([]int, 0, len(spans))
	var sb strings.Builder
	sb.Grow(len(s))
	for _, c := range lay.VisualToLogical {
		sb.WriteString(lay.Text(c, s[spans[c].Start:spans[c].End]))
		offsets = append(offsets, spans[c].Start)
	}
	return sb.String(), offsets
}

// VisualStringWithRuneMap does the same and returns, for each cluster in
// visual order, the logical rune index it had in the original string.
func VisualStringWithRuneMap(s string) (string, []int) {
	spans, runeIndex := terminalClusterSpans(s)
	lay := layoutString(s, spans)
	indices := make([]int, 0, len(spans))
	var sb strings.Builder
	sb.Grow(len(s))
	for _, c := range lay.VisualToLogical {
		sb.WriteString(lay.Text(c, s[spans[c].Start:spans[c].End]))
		indices = append(indices, runeIndex[c])
	}
	return sb.String(), indices
}

// CaretMap translates caret positions of a string between its logical rune
// indices and the visual cluster boundaries of its displayed form.
type CaretMap struct {
	VisualToLogical []int // maps visual boundary (0..N) to logical rune index (0..len(text))
	LogicalToVisual []int // maps logical rune index (0..len(text)) to visual boundary (0..N)
}

// BuildCaretMap computes the CaretMap of s. See BidiLayout.CaretVisual for
// where a caret is placed at a change of direction.
func BuildCaretMap(s string) CaretMap {
	spans, runeIndex := terminalClusterSpans(s)
	lay := layoutString(s, spans)
	n := len(spans)
	totalRunes := utf8.RuneCountInString(s)

	visualToLogical := make([]int, n+1)
	for v := 0; v <= n; v++ {
		if c := lay.CaretLogical(v); c < n {
			visualToLogical[v] = runeIndex[c]
		} else {
			visualToLogical[v] = totalRunes
		}
	}

	logicalToVisual := make([]int, totalRunes+1)
	c := 0
	for r := 0; r <= totalRunes; r++ {
		// The boundary a rune index belongs to: the first cluster starting
		// at or after it, the end of the string when there is none.
		for c < n && runeIndex[c] < r {
			c++
		}
		logicalToVisual[r] = lay.CaretVisual(c)
	}

	return CaretMap{VisualToLogical: visualToLogical, LogicalToVisual: logicalToVisual}
}

// ForEachVisualCluster walks the terminal clusters of s in the order they are
// drawn, left to right, handing the callback the text to draw (mirrored where
// the cluster reads right to left), its width in columns, and the byte offset
// and rune index it had in the logical string. It is the one place widgets
// should get visual order from: reordering the runs of a bidi paragraph by
// hand is what put a line's words in the wrong order in unxed/f4#546, three
// times over, once in each widget that had copied the code.
func ForEachVisualCluster(s string, fn func(cluster string, width, offset, runeIndex int)) {
	if DefaultBidiMode == BidiOff || !HasRTL(s) {
		forEachDisplayCluster(s, fn)
		return
	}
	type cluster struct {
		text      string
		width     int
		offset    int
		runeIndex int
	}
	var logical []cluster
	spans := make([]ClusterSpan, 0, len(s))
	forEachTerminalCluster(s, func(text string, width, offset, runeIndex int) {
		logical = append(logical, cluster{text: text, width: width, offset: offset, runeIndex: runeIndex})
		spans = append(spans, ClusterSpan{Start: offset, End: offset + len(text)})
	})
	lay := LayoutBidi(s, spans, DefaultBidiParagraph)
	if lay.Len() != len(logical) {
		for _, c := range logical {
			fn(c.text, c.width, c.offset, c.runeIndex)
		}
		return
	}
	for _, i := range lay.VisualToLogical {
		c := logical[i]
		fn(lay.Text(i, c.text), c.width, c.offset, c.runeIndex)
	}
}
