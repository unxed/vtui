# Text segmentation, cell widths and bidirectional text

How vtui turns a Go string into screen cells, and what is still missing.

## The unit of the screen is a grapheme cluster, not a rune

A cell used to hold one rune. That works until the text contains a combining
mark, and then it does not: the mark has no width of its own, so it either
disappeared or, in vtui's case, was replaced by a visible dot, and every
character after it on the line moved. Hindi suffered the most, because almost
every syllable carries one; emoji suffered next, because a modern emoji is
routinely five code points joined by U+200D.

`textseg.go` splits text into extended grapheme clusters (UAX #29, via
`rivo/uniseg`) and gives one cell to one cluster. `CharInfo.Char` holds either
a plain rune, when the cluster is a single one, or an index into a registry of
cluster strings, marked by `CompCharFlag`. This is why `CharInfo.Char` has been
64 bits wide from the start: it mirrors far2l's `COMP_CHAR`. `CellString`,
`CellRunes` and `CellBaseRune` read a cell back.

The registry only ever grows, and each distinct cluster is stored once. A
screenful of text produces a handful of entries; a file viewer scrolling
through prose in a script that composes heavily could produce thousands, which
is still small. It is never cleared, which is a deliberate simplification for
now, see REVIEW.md.

## Widths are what the terminal advances, summed and clamped to two

`ClusterWidth` sums the columns of the code points of a cluster and clamps
the sum to two. Non spacing marks, enclosing marks and format characters are
zero; spacing marks (the Devanagari `ा`) one; East Asian wide and fullwidth
characters two; U+FE0F two (one on a strictly wcwidth terminal, set
`EmojiPresentationWide` to false there). This is the rule of Windows Terminal
and ConPTY in their "grapheme clusters" measurement mode, and a wcwidth
terminal (VTE, xterm, foot) lands on the same count by never clustering at
all, so it is the one number every terminal agrees on. Summing reproduces the
emoji conventions without naming them: a ZWJ sequence, a keycap, a flag and a
skin tone all sum past two. It also means an Indic cluster is *not* one cell
because a font draws one glyph for it: `का` is two columns and so is the
conjunct `स्कृ`, and pretending otherwise made every dialog drawn over Hindi
text lean by one column per conjunct (unxed/f4#546).

`SanitizeCluster` guarantees that what reaches the terminal advances by what
was counted: C0 and C1 controls, DEL, U+2028/2029 and lone format characters
(a terminal executes U+0085 and U+009B and swallows the rest) become a visible
dot, and a cluster with no base character gets a dotted circle to stand on.
The ANSI renderer adds a second line of defence, in `screenbuf.go`: after any
cell whose advance a terminal might measure differently (anything past
Latin, Greek and Cyrillic, box drawing excepted) the next cell is placed with
an absolute cursor position, so a disagreement can shift nothing but that
cell.

## What to use where

- `StringWidth`, `TruncateString` instead of the `go-runewidth` equivalents.
- `ForEachCluster` / `ForEachClusterAt` to walk text for drawing.
- `AppendCluster` to put a cluster and its fillers into a cell slice.
- `SanitizeCluster` instead of `SanitizeRune`, whenever the surrounding text
  is at hand. `SanitizeRune` remains for callers that only have one rune.

### Caret boundaries use the terminal clusters, not the UAX ones

`ForEachCluster` / `ForEachClusterAt` are the public UAX #29 walkers as
`rivo/uniseg` implements them. They are right for measuring and for callers
that count runes, and wrong for anything that positions a caret: their
tables predate rule GB9c, which keeps an Indic virama with the consonant that
follows it, while Windows Terminal and ConPTY apply GB9c from the Unicode
16.0 tables. If a widget paints with one walker and moves the caret with the
other, the two disagree about how many cells the text occupies — the cursor
appears on the right glyph but edits a fraction of it, and Backspace or
Delete eats part of a shaped unit (unxed/f4#546).

`forEachTerminalCluster` therefore glues the pair back together with
`JoinsConjunct`, which is GB9c the way the terminal applies it: pairwise, a
virama that is `Indic_Conjunct_Break=Linker` (Devanagari, Bengali, Gujarati,
Oriya, Telugu, Malayalam) followed by a consonant of those scripts. Other
viramas (Kannada, Tamil, Sinhala, ...) do not join under GB9c and the
terminal keeps them apart, so nothing here joins them either.

`Edit.prevClusterBoundary`, `Edit.nextClusterBoundary` and
`Edit.cursorPositionAtX` use `forEachTerminalCluster`, the same walker as
`Edit.DisplayObject` and `MultiLineEdit`. Any new caret, selection or
hit-testing code belongs on that side too.

### Backspace deletes a code point, everything else a cluster

Cursoring, selection and forward Delete step over a whole cluster. Backspace
does not: it peels one code point off the end of the preceding cluster, which
is what UAX #29 explicitly allows and what Windows edit controls, Notepad and
the browsers do. Deleting a base character forwards takes its marks with it, so
nothing is orphaned; deleting a trailing mark backwards is safe on its own and
lets a mistyped composition be corrected without retyping the syllable. The
W3C i18n note "Cursor Movement and Deletion of Unicode Text" is the readable
write-up of the de facto behaviour.

Emoji are the exception everyone makes: ZWJ sequences, keycaps, flags and
skin-tone modifiers stay atomic in both directions. `backspace.go` holds the
rule; `Edit` and `MultiLineEdit` narrow their deletion range through
`backspaceStart` and nothing else has to know about it.

**Known gap:** in `BidiFull` mode `Edit` picks the cluster to backspace by its
*visual* neighbour, so a caret at the logical end of a purely RTL string sits
at visual position 0 and Backspace becomes a no-op. That is a bidi-mapping
question, not a segmentation one, and is untouched here.

## Highlighter attributes are indexed by rune

`Highlighter.Highlight` returns one attribute per rune of the line. Not per
byte, and not per cell. Those three counts stopped agreeing as soon as text
with emoji or combining marks arrived: a cluster is one cell, one or two
columns, one to seven runes and up to twenty five bytes.

`StringToCharInfoWithAttrs` is the mapper. It walks clusters, gives each
cluster the attribute of its first rune, and repeats that attribute over the
fillers of a wide cluster. Runes past the end of the slice, and a nil slice,
take the base attribute.

An attribute sitting on a combining mark is therefore ignored, because the
mark has no cell of its own to put it in. That is the point rather than a
shortcoming: a highlighter that colours a mark differently from its base
cannot move the rest of the line.

For bidirectional (BiDi) text, attributes are resolved against the logical
string first. During visual reordering, these attributes travel with their
respective clusters. This guarantees that logical highlighting boundaries
remain perfectly attached to the text content even when a logically contiguous
span is split and reordered into visually separate runs.
## Bidirectional text is laid out per line, left to right by default

`bidi.go` runs the Unicode Bidirectional Algorithm over one line at a time.
The algorithm itself is the reference implementation that
`golang.org/x/text/unicode/bidi` carries in `core.go` and `bracket.go`,
copied verbatim into `internal/uba`, because that package's public API
cannot fix the paragraph direction to left to right, does not expose the
embedding levels that rule L2 needs, and never pairs a bracket (it stores
each bracket as its own pair value). Its tables are still what classify the
characters.

`LayoutBidi(text, clusters, dir)` is the one entry point: it takes the line
and its grapheme clusters, whichever walker produced them, and returns the
level of every cluster, the visual order (rule L2 applied over clusters, so
a mark never parts from its base) and the mirrored glyphs (L4). Everything
else — `VisualString`, `VisualStringWithMap`, `BuildCaretMap` — is built on
it, and an application that keeps its own cluster boundaries should call it
directly rather than reimplement the reordering.

The base direction is `DefaultBidiParagraph`, and it is **left to right**
unless the application says otherwise. Detecting it from the first strong
character (UAX #9 P2, P3) is what a plain text viewer does, and it turns a
left to right line that merely starts with a right to left word inside out:
`ދިވެހިބަސް - Divehi` became `- Divehi ...` followed by the Thaana word. Notepad,
a browser text field and every left to right interface fix the paragraph to
left to right and reverse the right to left words in place; so does vtui.

Caret positions inside mixed text follow `BidiLayout.CaretVisual`: the caret
stands at the trailing edge of the cluster it logically follows, so it walks
right through Latin, jumps to the far right of a right to left word on
entering it, walks left through it, and rests at that word's left edge after
its last letter, where the next letter of it will appear. This is the
convention of the Windows edit controls and Notepad; a caret that always
moves in the screen direction is the browser convention, and neither is
mandated by the standard.

The terminal is told that the rows it receives are already in visual order
(`terminal_env.go` sends BDSM reset and SCP left to right, the terminal-wg
BiDi recommendation's "explicit LTR" mode, whenever `DefaultBidiMode` is not
`BidiOff`). A terminal that runs the algorithm itself, VTE for one, would
otherwise reorder the row a second time; Windows Terminal and the other
emulators without bidi support ignore the sequences, and they are the ones
for which reordering here is the only way to read Hebrew or Arabic at all.
Arabic contextual shaping on such a terminal is a separate problem: the
terminal shapes runs in the order it receives them, which is reversed.

## Where the rest of the work is written down

This file describes the machinery. The task it belongs to, what is done, what
is left and how to do it, lives in `UNICODE_PLAN.md`. Start there.

The far2l equivalents, for anyone comparing the two: `COMP_CHAR` and
`COMPOSITE_CHAR_MARK` in `WinPort/WinCompat.h`, the registry in
`WinPort/src/APIConsole.cpp`, and the width and composition classification in
`utils/include/CharClasses.h`. vtui takes the cell representation from them
almost unchanged, and deliberately does not take the composition rules: real
grapheme clusters replace far2l's prefix and suffix classes, and the ICU
dependency their tables are generated from has no place in a Go program.