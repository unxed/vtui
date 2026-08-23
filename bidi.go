package vtui

import (
	"strings"
	"unicode/utf8"

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

// HasRTL checks if the string contains any strong RTL characters.
func HasRTL(s string) bool {
	isASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			isASCII = false
			break
		}
	}
	if isASCII {
		return false
	}

	for _, r := range s {
		if r >= 0x0590 && r <= 0x08FF { // Hebrew, Arabic, Syriac, Thaana, Samaritan, N'Ko, Mandaic, etc.
			return true
		}
		if r >= 0xFB1D && r <= 0xFEFC { // Presentation Forms (Hebrew, Arabic)
			return true
		}
	}
	return false
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
	if DefaultBidiMode == BidiOff || !HasRTL(s) {
		return s, trivialMap(s)
	}

	type logicalCluster struct {
		text    string
		byteOff int
		runeIdx int
	}

	var logicalClusters []logicalCluster
	forEachTerminalClusterRaw(s, func(text string, byteOff, runeIdx int) {
		logicalClusters = append(logicalClusters, logicalCluster{text: text, byteOff: byteOff, runeIdx: runeIdx})
	})

	p := bidi.Paragraph{}
	_, err := p.SetString(s)
	if err != nil {
		return s, trivialMap(s)
	}
	order, err := p.Order()
	if err != nil {
		return s, trivialMap(s)
	}

	var visualBuilder strings.Builder
	var visualOffsets []int

	numRuns := order.NumRuns()
	for i := 0; i < numRuns; i++ {
		run := order.Run(i)
		start, end := run.Pos()

		// Collect clusters in this run
		var runClusters []logicalCluster
		for _, c := range logicalClusters {
			if c.runeIdx >= start && c.runeIdx <= end {
				runClusters = append(runClusters, c)
			}
		}

		isRTL := run.Direction() == bidi.RightToLeft
		if isRTL {
			// Reverse clusters in RTL run
			for i, j := 0, len(runClusters)-1; i < j; i, j = i+1, j-1 {
				runClusters[i], runClusters[j] = runClusters[j], runClusters[i]
			}
			// Apply bracket mirroring for single-rune clusters
			for i := range runClusters {
				if utf8.RuneCountInString(runClusters[i].text) == 1 {
					runClusters[i].text = bidi.ReverseString(runClusters[i].text)
				}
			}
		}

		for _, c := range runClusters {
			visualBuilder.WriteString(c.text)
			visualOffsets = append(visualOffsets, c.byteOff)
		}
	}

	return visualBuilder.String(), visualOffsets
}

func trivialMap(s string) []int {
	var offsets []int
	forEachTerminalClusterRaw(s, func(_ string, offset, _ int) {
		offsets = append(offsets, offset)
	})
	return offsets
}

// VisualStringWithRuneMap does the same and returns, for each cluster in
// visual order, the logical rune index it had in the original string.
func VisualStringWithRuneMap(s string) (string, []int) {
	if DefaultBidiMode == BidiOff || !HasRTL(s) {
		return s, trivialRuneMap(s)
	}

	type logicalCluster struct {
		text    string
		runeIdx int
	}

	var logicalClusters []logicalCluster
	forEachTerminalClusterRaw(s, func(text string, _, runeIdx int) {
		logicalClusters = append(logicalClusters, logicalCluster{text: text, runeIdx: runeIdx})
	})

	p := bidi.Paragraph{}
	_, err := p.SetString(s)
	if err != nil {
		return s, trivialRuneMap(s)
	}
	order, err := p.Order()
	if err != nil {
		return s, trivialRuneMap(s)
	}

	var visualBuilder strings.Builder
	var visualRuneIndices []int

	numRuns := order.NumRuns()
	for i := 0; i < numRuns; i++ {
		run := order.Run(i)
		start, end := run.Pos()

		// Collect clusters in this run
		var runClusters []logicalCluster
		for _, c := range logicalClusters {
			if c.runeIdx >= start && c.runeIdx <= end {
				runClusters = append(runClusters, c)
			}
		}

		isRTL := run.Direction() == bidi.RightToLeft
		if isRTL {
			// Reverse clusters in RTL run
			for i, j := 0, len(runClusters)-1; i < j; i, j = i+1, j-1 {
				runClusters[i], runClusters[j] = runClusters[j], runClusters[i]
			}
			// Apply bracket mirroring for single-rune clusters
			for i := range runClusters {
				if utf8.RuneCountInString(runClusters[i].text) == 1 {
					runClusters[i].text = bidi.ReverseString(runClusters[i].text)
				}
			}
		}

		for _, c := range runClusters {
			visualBuilder.WriteString(c.text)
			visualRuneIndices = append(visualRuneIndices, c.runeIdx)
		}
	}

	return visualBuilder.String(), visualRuneIndices
}

func trivialRuneMap(s string) []int {
	var indices []int
	forEachTerminalClusterRaw(s, func(_ string, _, runeIdx int) {
		indices = append(indices, runeIdx)
	})
	return indices
}

type CaretMap struct {
	VisualToLogical []int // maps visual boundary (0..N) to logical rune index (0..len(text))
	LogicalToVisual []int // maps logical rune index (0..len(text)) to visual boundary (0..N)
}

type bidiClusterInfo struct {
	logicalIdx int // index in logicalClusters
	runeIdx    int // logical rune index before this cluster
}

func BuildCaretMap(s string) CaretMap {
	var logicalClusters []bidiClusterInfo
	forEachTerminalClusterRaw(s, func(_ string, _, runeIdx int) {
		logicalClusters = append(logicalClusters, bidiClusterInfo{logicalIdx: len(logicalClusters), runeIdx: runeIdx})
	})
	totalRunes := utf8.RuneCountInString(s)
	N := len(logicalClusters)

	p := bidi.Paragraph{}
	_, err := p.SetString(s)
	if err != nil || N == 0 {
		return trivialCaretMap(N, totalRunes, logicalClusters)
	}
	order, err := p.Order()
	if err != nil {
		return trivialCaretMap(N, totalRunes, logicalClusters)
	}

	logicalToVisualBoundaries := make([]int, N+1)
	visualClusterIdx := 0

	numRuns := order.NumRuns()
	for i := 0; i < numRuns; i++ {
		run := order.Run(i)
		start, end := run.Pos()

		var runClusters []bidiClusterInfo
		for _, c := range logicalClusters {
			if c.runeIdx >= start && c.runeIdx <= end {
				runClusters = append(runClusters, c)
			}
		}

		if len(runClusters) == 0 {
			continue
		}

		runEndLogicalIdx := runClusters[len(runClusters)-1].logicalIdx

		isRTL := run.Direction() == bidi.RightToLeft
		if !isRTL {
			for idx, c := range runClusters {
				logicalToVisualBoundaries[c.logicalIdx] = visualClusterIdx + idx
			}
			logicalToVisualBoundaries[runEndLogicalIdx+1] = visualClusterIdx + len(runClusters)
		} else {
			for idx, c := range runClusters {
				logicalToVisualBoundaries[c.logicalIdx] = visualClusterIdx + (len(runClusters) - idx)
			}
			logicalToVisualBoundaries[runEndLogicalIdx+1] = visualClusterIdx
		}

		visualClusterIdx += len(runClusters)
	}

	visualToLogicalRune := make([]int, N+1)
	logicalRuneToVisual := make([]int, totalRunes+1)

	visualToLogicalBoundary := make([]int, N+1)
	for l, v := range logicalToVisualBoundaries {
		if v >= 0 && v <= N {
			visualToLogicalBoundary[v] = l
		}
	}

	for v := 0; v <= N; v++ {
		l := visualToLogicalBoundary[v]
		var rIdx int
		if l < N {
			rIdx = logicalClusters[l].runeIdx
		} else {
			rIdx = totalRunes
		}
		visualToLogicalRune[v] = rIdx
	}

	for r := 0; r <= totalRunes; r++ {
		closestL := N
		for i, c := range logicalClusters {
			if r <= c.runeIdx {
				closestL = i
				break
			}
		}
		logicalRuneToVisual[r] = logicalToVisualBoundaries[closestL]
	}

	return CaretMap{
		VisualToLogical: visualToLogicalRune,
		LogicalToVisual: logicalRuneToVisual,
	}
}

func trivialCaretMap(N int, totalRunes int, logicalClusters []bidiClusterInfo) CaretMap {
	vToL := make([]int, N+1)
	lToV := make([]int, totalRunes+1)

	for i := 0; i < N; i++ {
		vToL[i] = logicalClusters[i].runeIdx
	}
	vToL[N] = totalRunes

	for r := 0; r <= totalRunes; r++ {
		closest := N
		for i, c := range logicalClusters {
			if r <= c.runeIdx {
				closest = i
				break
			}
		}
		lToV[r] = closest
	}

	return CaretMap{
		VisualToLogical: vToL,
		LogicalToVisual: lToV,
	}
}
