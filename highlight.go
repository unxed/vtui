package vtui

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
	res := make([]CharInfo, 0, len(s))
	ForEachVisualCluster(s, func(cluster string, w, _, runeIdx int) {
		attr := baseAttr
		if runeIdx >= 0 && runeIdx < len(attrs) {
			attr = attrs[runeIdx]
		}
		res = AppendCluster(res, cluster, w, attr)
	})
	return res
}
