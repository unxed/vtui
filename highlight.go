package vtui

import (
	"unicode/utf8"

	"golang.org/x/text/unicode/bidi"
)

// StringToCharInfoWithAttrs lays s out into cells and colours them from attrs,
// the slice a Highlighter returns.
//
// attrs is indexed by rune, as the Highlighter interface documents. Each
// grapheme cluster takes the attribute of its first rune, and the extra
// columns of a wide cluster repeat it. Runes past the end of attrs, and a nil
// attrs, take baseAttr.
//
// The cell count is whatever the layout says it is: StringWidth(s). Attributes
// never move a character.
func StringToCharInfoWithAttrs(s string, attrs []uint64, baseAttr uint64) []CharInfo {
	if DefaultBidiMode == BidiOff || !HasRTL(s) {
		res := make([]CharInfo, 0, len(s))
		forEachDisplayCluster(s, func(cluster string, w, _, runeIdx int) {
			attr := baseAttr
			if runeIdx >= 0 && runeIdx < len(attrs) {
				attr = attrs[runeIdx]
			}
			res = AppendCluster(res, cluster, w, attr)
		})
		return res
	}

	type logicalCluster struct {
		text    string
		runeIdx int
		width   int
		attr    uint64
	}

	var logicalClusters []logicalCluster
	forEachTerminalCluster(s, func(clText string, width, _, runeIdx int) {
		attr := baseAttr
		if runeIdx >= 0 && runeIdx < len(attrs) {
			attr = attrs[runeIdx]
		}
		logicalClusters = append(logicalClusters, logicalCluster{
			text:    clText,
			runeIdx: runeIdx,
			width:   width,
			attr:    attr,
		})
	})

	p := bidi.Paragraph{}
	_, err := p.SetString(s)
	if err != nil {
		res := make([]CharInfo, 0, len(s))
		for _, c := range logicalClusters {
			res = AppendCluster(res, c.text, c.width, c.attr)
		}
		return res
	}
	order, err := p.Order()
	if err != nil {
		res := make([]CharInfo, 0, len(s))
		for _, c := range logicalClusters {
			res = AppendCluster(res, c.text, c.width, c.attr)
		}
		return res
	}

	var res []CharInfo
	numRuns := order.NumRuns()
	for i := 0; i < numRuns; i++ {
		run := order.Run(i)
		start, end := run.Pos()

		var runClusters []logicalCluster
		for _, c := range logicalClusters {
			if c.runeIdx >= start && c.runeIdx <= end {
				runClusters = append(runClusters, c)
			}
		}

		isRTL := run.Direction() == bidi.RightToLeft
		if isRTL {
			for i, j := 0, len(runClusters)-1; i < j; i, j = i+1, j-1 {
				runClusters[i], runClusters[j] = runClusters[j], runClusters[i]
			}
			for i := range runClusters {
				if utf8.RuneCountInString(runClusters[i].text) == 1 {
					runClusters[i].text = bidi.ReverseString(runClusters[i].text)
				}
			}
		}

		for _, c := range runClusters {
			res = AppendCluster(res, c.text, c.width, c.attr)
		}
	}

	return res
}
