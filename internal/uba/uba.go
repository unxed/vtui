// Package uba is the core of the Unicode Bidirectional Algorithm (UAX #9):
// given the bidi classes of the characters of one paragraph it resolves
// their embedding levels. It is the reference implementation that
// golang.org/x/text/unicode/bidi carries in core.go and bracket.go, copied
// verbatim, because that package's public Paragraph API neither lets a
// caller fix the paragraph direction to left to right (HL1) nor exposes the
// levels that rule L2 needs, and pairs no brackets at all (it stores each
// bracket as its own pair value, so an opener never matches its closer).
// The classes and bracket properties still come from x/text; only the
// algorithm lives here.
package uba

import "golang.org/x/text/unicode/bidi"

// Class is the bidi class of a character. Its values are those of
// golang.org/x/text/unicode/bidi.Class, so a class looked up there converts
// directly; it is a distinct type only because the algorithm defines a
// method on it.
type Class uint

// The classes, by their UAX #9 names.
const (
	L       = Class(bidi.L)
	R       = Class(bidi.R)
	EN      = Class(bidi.EN)
	ES      = Class(bidi.ES)
	ET      = Class(bidi.ET)
	AN      = Class(bidi.AN)
	CS      = Class(bidi.CS)
	B       = Class(bidi.B)
	S       = Class(bidi.S)
	WS      = Class(bidi.WS)
	ON      = Class(bidi.ON)
	BN      = Class(bidi.BN)
	NSM     = Class(bidi.NSM)
	AL      = Class(bidi.AL)
	Control = Class(bidi.Control)
	LRO     = Class(bidi.LRO)
	RLO     = Class(bidi.RLO)
	LRE     = Class(bidi.LRE)
	RLE     = Class(bidi.RLE)
	PDF     = Class(bidi.PDF)
	LRI     = Class(bidi.LRI)
	RLI     = Class(bidi.RLI)
	FSI     = Class(bidi.FSI)
	PDI     = Class(bidi.PDI)

	unknownClass = ^Class(0)
)

// Bracket is the Bidi_Paired_Bracket_Type of a character.
type Bracket = bracketType

// The bracket types.
const (
	BracketNone  = bpNone
	BracketOpen  = bpOpen
	BracketClose = bpClose
)

// Paragraph directions for Levels.
const (
	ParagraphLTR  = 0
	ParagraphRTL  = 1
	ParagraphAuto = -1
)

// Levels resolves the embedding level of every character of one paragraph
// that is displayed as a single line: rules P, X, W, N0-N2 and I of UAX #9,
// then L1. types holds the class of each character, pairTypes its bracket
// type and pairValues, for an opening or closing bracket, the code point of
// the opening bracket of the pair (so that ')' carries '('). paragraph is
// ParagraphLTR, ParagraphRTL, or ParagraphAuto to detect the direction from
// the first strong character (P2, P3) with left to right as the fallback.
// The second result is the resolved paragraph level.
func Levels(types []Class, pairTypes []Bracket, pairValues []rune, paragraph int) ([]int, int, error) {
	base := implicitLevel
	switch {
	case paragraph == ParagraphLTR:
		base = 0
	case paragraph > 0:
		base = 1
	}
	p, err := newParagraph(types, pairTypes, pairValues, base)
	if err != nil {
		return nil, 0, err
	}
	resolved := p.getLevels([]int{len(types)})
	out := make([]int, len(resolved))
	for i, l := range resolved {
		out[i] = int(l)
	}
	return out, int(p.embeddingLevel), nil
}
