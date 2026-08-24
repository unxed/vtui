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

## Widths follow wcwidth, with the emoji exceptions everyone makes

`ClusterWidth` sums the width of the runes of a cluster the way a wcwidth
terminal does. Non spacing marks, enclosing marks and format characters take
no columns; spacing marks (the Devanagari `ा`, for one) take one. Categories
decide before `go-runewidth` does, because `go-runewidth` gives the Devanagari
virama a column and terminals do not.

On top of that, sequences every terminal treats as one double wide glyph are
pinned to two columns: ZWJ sequences, keycaps, skin tone modifiers and pairs
of regional indicators. A cluster carrying U+FE0F is two columns as well,
which is what modern emulators do; set `EmojiPresentationWide` to false on a
strictly wcwidth terminal.

## What to use where

- `StringWidth`, `TruncateString` instead of the `go-runewidth` equivalents.
- `ForEachCluster` / `ForEachClusterAt` to walk text for drawing.
- `AppendCluster` to put a cluster and its fillers into a cell slice.
- `SanitizeCluster` instead of `SanitizeRune`, whenever the surrounding text
  is at hand. `SanitizeRune` remains for callers that only have one rune.

### Caret boundaries use the terminal clusters, not the UAX ones

`ForEachCluster` / `ForEachClusterAt` are the public UAX #29 walkers. They are
right for measuring and for callers that count runes, and wrong for anything
that positions a caret: UAX #29 splits an Indic virama from the consonant that
follows it, while `forEachTerminalCluster` joins the pair the way a shaping
terminal draws it. If a widget paints with one walker and moves the caret with
the other, the two disagree about how many cells the text occupies — the
cursor appears on the right glyph but edits a fraction of it, and Backspace or
Delete eats part of a shaped unit (unxed/f4#546).

`Edit.prevClusterBoundary`, `Edit.nextClusterBoundary` and
`Edit.cursorPositionAtX` therefore use `forEachTerminalCluster`, the same
walker as `Edit.DisplayObject` and `MultiLineEdit`. Any new caret, selection
or hit-testing code belongs on that side too.

**Open question, deliberately left alone:** deletion *granularity* still
differs between hosts. `Edit` deletes a whole cluster on Backspace, while
f4's editor peels a trailing Indic/Thaana mark off first
(`textlayout.TrailingModifierStart`), which is what Windows edit controls do.
Unifying the two is a behaviour decision, not a segmentation bug, and is not
part of this change.

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