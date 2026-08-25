# Unicode text in vtui: task, status and instructions

This is the working document for one task: make vtui lay out and draw text
correctly when that text contains zero width characters, double width
characters, or right to left scripts. It is written so that a model picking up
the work later needs nothing but this repository.

Read this file first, then `TEXTSEG.md` for how the existing code works, then
`REVIEW.md` for the loose ends that were left on purpose.

## 0. How to work on this

- One stage per patch. Do not start a stage before the previous one is in.
- Every patch carries: the code, tests for the new code, and the update to
  this file that moves the stage from "to do" to "done".
- Patches are in `ap` format, one patch file per repository.
- Never re-send code that an earlier patch already added. Only the delta.
- Perfectionism is forbidden. If you notice an unrelated problem, write it
  into `REVIEW.md` and go back to the task. Do not refactor neighbouring code.
- When something is genuinely unclear, ask, or write a patch that collects the
  missing information (logging, a dump), rather than guessing.
- If a stage turns out to be bigger than one patch, split it and say so here.

## 1. What was wrong, in the user's words

Three symptoms, reported against an application built on vtui:

1. Emoji behaved strangely.
2. Hindi text broke dialog layout.
3. Right to left scripts were not attempted at all, because there was no
   reason to expect them to work.

After stage 1 the user re-tested. Emoji and Hindi no longer misbehave in the
general case. Two things remain, and one new one appeared:

- **The cursor over a double width character covers only half the cell.** Only
  in the graphical backends. Cosmetic, not urgent, but visible.
- **Syntax highlighting produces artifacts after an emoji on the line.** Found
  and fixed. The highlighter is colorer, and the application was reading its
  region offsets as UTF-16 units while colorer counts code points. vtui's own
  share of the fault was the missing contract, which stage 3 now states.
- **Bidirectional text needs editing, not just display.** The user confirmed
  that typing and moving the caret inside RTL text is in scope. It stays at
  the end of the plan.

## 2. Vocabulary

- **Rune** - one Unicode code point. Go's `rune`.
- **Grapheme cluster** - what a person calls a character: a base plus the
  marks that hang off it, or an emoji built from several code points joined by
  U+200D. Defined by UAX #29; computed here with `rivo/uniseg`.
- **Cell** - one character position on screen. A `CharInfo`.
- **Column / width** - how many cells a cluster claims. Zero is not allowed:
  a cluster always claims at least one.
- **Filler** - `WideCharFiller`, the marker occupying the second cell of a
  double width character. It is `^uint64(0)`.
- **Composite char** - a cluster of more than one rune, stored in a registry
  and referenced from `CharInfo.Char` by an index carrying `CompCharFlag`.
- **Logical order** - the order runes appear in the string.
- **Visual order** - the order they appear on screen. For left to right text
  these are the same; for RTL they are not.

## 3. What far2l does, and what we take from it

far2l is the reference implementation for this problem domain, and the
application vtui serves is a rewrite of it. Its approach, for the record:

- `WinPort/WinCompat.h` defines `CHAR_INFO.Char.UnicodeChar` as a 64 bit
  `COMP_CHAR`. The top bit (`COMPOSITE_CHAR_MARK`) marks it as an index into a
  registry rather than a code point. **vtui copies this exactly**;
  `CompCharFlag` is the same bit and `RegisterCluster` is the same idea as
  `WINPORT(CompositeCharRegister)` in `WinPort/src/APIConsole.cpp`. The
  registry there is likewise never freed.
- `utils/include/CharClasses.h` classifies a code point three ways: `FullWidth`
  (East Asian Width is Wide or Fullwidth), `Suffix` (a character that attaches
  to the previous one: marks, format characters, most joining types) and
  `Prefix`. Composition is then done by gluing `Suffix` characters onto the
  preceding one. **vtui does not copy this**: it uses real UAX #29 grapheme
  clusters instead, which is stricter and needs no hand written joining rules.
- The tables in `utils/src/CharClasses.cpp` are generated offline by
  `utils/src/CharClasses_mk.cpp`, which links against ICU, walks all of
  Unicode, and prints a C++ `switch` with range labels. At runtime there is no
  ICU: just a static table, compressed into 256 entry blocks that are
  deduplicated by hash. **vtui will copy the generation idea in stage 8, but
  never the runtime dependency: no cgo, ever.**
- `g_far2l_use_vs16` is a global switch deciding whether U+FE0F makes the
  preceding character double width. vtui's `EmojiPresentationWide` is the same
  switch with a better name.
- The wx backend, `WinPort/src/Backend/WX/Paint.cpp`, draws the cursor
  rectangle as `FontWidth() * nx` wide, where `nx` is the cell count of the
  character under it. That is precisely the fix stage 2 needs.

## 4. Status board

| Stage | What | State |
|---|---|---|
| 1 | Cluster layer, composite registry, cluster aware cell producers, ANSI renderer | **done** |
| 2 | Cursor covers the whole double width cell in graphical backends | **done** |
| 3 | A defined contract for `Highlighter` attributes, and a mapper onto cells | **done** |
| 4 | Graphical backends draw whole clusters, not just the base rune | **done** |
| 5 | Remaining `go-runewidth` callers, and the three per rune writers | **done** |
| 6 | BiDi for display only | **done**, redone on the real algorithm (see `TEXTSEG.md`) |
| 7 | BiDi for editing: caret, selection, input | **done** |
| 8 | Own width tables generated from the UCD, replacing `go-runewidth` | optional |

## 5. Stage 1 - done

Delivered in `textseg.go`, with tests in `textseg_test.go`. What exists now,
so that later stages do not reinvent it:

| Function | Use |
|---|---|
| `RegisterCluster(string) uint64` | text to cell value |
| `CellString(uint64) string` | cell value to text; filler gives `""`, zero gives `" "` |
| `CellRunes(uint64) []rune` | cell value to runes |
| `CellBaseRune(uint64) rune` | base character only, for one glyph backends |
| `IsCompChar(uint64) bool` | is this cell value a registry index |
| `ClusterWidth(string) int` | columns for one cluster |
| `StringWidth(string) int` | columns for a whole string |
| `TruncateString(s string, w int, tail string) string` | cut to width without splitting a cluster |
| `NextCluster(string) (string, int, int)` | first cluster, its width, its byte size |
| `SanitizeCluster(string) (string, int)` | control characters to placeholders |
| `ForEachCluster` / `ForEachClusterAt` | walk a string cluster by cluster |
| `AppendCluster` | put a cluster plus its fillers into a cell slice |
| `EmojiPresentationWide` | the U+FE0F width switch |

`StringToCharInfo`, `FillCharInfo`, `FillCharInfoWithSelection`,
`StringToCharInfoHighlighted`, and the painter's text drawing all go through
this layer. So does the ANSI renderer and `ScreenBuf.Dump`.

Widths follow wcwidth: Unicode categories Mn, Me and Cf take no columns,
everything else takes what `go-runewidth` says, and the emoji sequences every
terminal special cases are pinned to two. The details and the reasoning are in
`TEXTSEG.md`.

## 6. Stage 2 - the cursor over a double width character - done

**What the symptom was.** In the x11, wayland and gogpu backends the cursor
rectangle was one cell wide even when it sat on a character occupying two, and
a cursor parked on the right half of such a character was not drawn at all,
because those loops skip filler cells before reaching the cursor test.

**What the cause turned out to be.** Wider than the cursor. x11 and wayland
took the cell count from `runewidth.RuneWidth` applied to the base rune of the
cell. That number is the width of a character, not the number of cells the
layout engine gave the cluster, and the two differ for exactly the text this
task is about: a cluster carrying U+FE0F occupies two cells while its base rune
measures one. So the renderer could disagree with the layout, and the cursor
was only the visible half of it.

**What was done.**

`CellSpanAt` in `screenbuf.go` is the single answer to "what character is in
this cell and how wide is it":

    func CellSpanAt(buf []CharInfo, width, x, y int) (startX, span int)

It walks left off any filler to find the character's first column, then right
to count the fillers that follow. `span` is never below one, and out of range
coordinates answer as a plain one column cell so callers need no bounds check
of their own. Tests are in `cellspan_test.go`, including the composite cluster
case that motivated it.

The three backends now use it:

- `x11_renderer.go` and `wayland_renderer.go` take `rw` from `CellSpanAt`
  instead of from `runewidth`, so glyph placement and cursor both follow the
  buffer. The cursor test became a range test - true when the cursor column
  falls anywhere inside the character - and the invert loop runs `cw*rw`
  pixels wide. `go-runewidth` is no longer imported by either.
- `gogpu_renderer.go` draws its cursor once, after the cell loop, so it calls
  `CellSpanAt` on `r.renderBuf` to find the character's first column and span,
  and scales the rectangle by it. Its glyph loop still derives the span inline;
  that code is already correct and stage 4 will be inside it anyway.

**How to check it by hand.** Put a wide character in an edit field, move the
caret onto it from the left and from the right, in each graphical backend. The
cursor must cover both columns either way, and must not vanish. `❤\uFE0F` is
the interesting one, because it is the case the old code got wrong even in
principle.

**What this stage deliberately did not do.** It did not touch the terminal
backend, which was already correct: the terminal draws its own cursor and only
needs the cell position, which it always had.

## 7. Stage 3 - the highlighter contract - done

**What the symptom was.** Syntax colouring went wrong from the first emoji on
a line onwards, and stayed wrong to the end of that line.

**What the cause turned out to be.** Not clustering, and not this repository.
`Highlighter.Highlight` never said what `attrs` was indexed by, so the two
sides of it had each quietly picked an answer, and the answers differed.
colorer keeps a line in its legacy `UnicodeString`, one element per code
point, and reports region offsets in those elements. The application read them
as UTF-16 units and mapped them through a surrogate aware table first. Inside
the BMP the two readings agree, which is why nobody noticed for so long; an
astral character - and an emoji is one - shifted every offset after it one
position to the left.

Three independent readings of the colorer sources agree, which is why this was
fixed rather than guessed at:

- `strings/legacy/CString.cpp` decodes UTF-8 into one `wchar` per code point
  and never builds a surrogate pair.
- `strings/legacy/Character.h` states outright that the library has no
  surrogate support and would treat a pair as two distinct characters.
- `strings/legacy/Encodings.cpp` carries a `wc > 0xFFFF` branch, guarded by
  `__WCHAR_MAX__ > 0xffff`, so one element holds a whole code point. Under
  wasi-sdk `wchar_t` is 32 bits.

**The contract, decided.** `attrs` is indexed by **rune**. `attrs[i]` colours
the i-th rune of `line`; `len(attrs)` is the rune count of the line, except
that nil and a short slice are allowed and the remainder takes `baseAttr`. It
is written on the interface in `types.go` and repeated in `TEXTSEG.md`.

This is not the byte offset contract this section used to recommend. Rune
indices are what every producer already emits and what every consumer already
reads. Bytes would have been a flag day across three repositories, to fix a
bug that was about code points versus UTF-16 units and never about bytes. The
reasoning is in `REVIEW.md`, as this section asked for.

**The mapper.** `highlight.go`:

    func StringToCharInfoWithAttrs(s string, attrs []uint64, baseAttr uint64) []CharInfo

It walks clusters, gives each cluster the attribute of its first rune, and
lets `AppendCluster` repeat that attribute over the fillers of a wide one. A
combining mark coloured differently from its base gets no cell of its own, so
it cannot shift anything. Tests are in `highlight_test.go`, including the
invariant that the cell count equals `StringWidth`.

**Where the other half of the fix lives.** In the application, which now reads
colorer's offsets as the rune indices they are. vtui's obligation was the
contract and the mapper, and both are here.

## 8. Stage 4 - clusters in the graphical backends

**Symptom.** In x11, wayland and gogpu a composed character loses its marks: a
letter with an accent draws as the bare letter. Column counts are already
right, so nothing shifts; only the marks are missing. Terminal output is
correct.

**Cause.** Stage 1 pointed those backends at `CellBaseRune` as a deliberate
stopgap, so that they would keep compiling and keep their layout while the
cluster layer landed.

**What to do.** Each backend caches glyphs by rune and blits one glyph per
cell. The smallest honest change is to render the whole cluster string into
the glyph cache instead of a single rune, keying the cache by the cell value
(`uint64`) rather than by `rune`, and letting the font stack place the marks.
`golang.org/x/image/font` will draw a string with combining marks by advancing
zero for them, which is what is wanted, and is already a dependency.

Concretely, per backend:

- Change the glyph cache key from `rune` to `uint64`.
- Replace `CellBaseRune(currCell.Char)` with `CellString(currCell.Char)` at
  the draw call, keeping `CellBaseRune` only where a single rune is genuinely
  needed - the box drawing detection in `gogpu_renderer.go`, for one.
- Clip the drawn string to the cell's column span so a font that does not
  compose cannot bleed into the neighbour.

Do **not** attempt real shaping (Arabic joining forms, Indic reordering) here.
That needs a shaping engine and is out of scope for this task; note it in
`REVIEW.md` and move on.

**Tests.** The glyph cache key change is testable: draw nothing, just assert
that two different clusters do not collide in the cache and that the same
cluster hits it. Rendering itself is not unit testable without a window.

**Done when.** A letter with a combining accent shows the accent in the
graphical backends, and column counts are unchanged.

## 9. Stage 5 - the remaining rune oriented code

Two groups, both mechanical.

**Group A: `go-runewidth` called directly.** These files still ask
`go-runewidth` for widths and so disagree with the layout engine on exactly
the strings this task is about:

    treeview.go  text_utils.go  multilineedit.go  menubar.go  autocomplete.go
    keybar.go    edit.go        layout_validator.go  grid_nav.go  table.go
    checkbox.go  text.go        common_dialogs.go   help_view.go  semantic.go
    vmenu.go     button.go      statusline.go       framemanager.go  combobox.go
    vtext.go     x11_renderer.go  wayland_renderer.go

Replace `runewidth.StringWidth` with `StringWidth` and `runewidth.Truncate`
with `TruncateString`. `runewidth.RuneWidth` calls that operate on a cell's
base rune inside a renderer may stay for now; everything else should go
through the cluster layer. `text_utils.go` needs more than a substitution: its
`WrapText` splits words by rune and `TruncateMiddle` counts runes, so both
need to count clusters instead.

Do this file by file, in more than one patch if it is long, and run the whole
test suite after each. Layout tests will catch mistakes.

**Group B: text written one rune at a time.** Three places build cells by
looping over runes and writing each separately, which puts a combining mark
into its own cell:

- `edit.go`, around the visible text loop
- `multilineedit.go`, in the display loop
- `vtext.go`, in the vertical text drawing

Each must iterate clusters with `ForEachCluster` and write with
`AppendCluster`. `edit.go` is the delicate one because the caret position is
computed in the same loop; the caret must land on the first column of a
cluster, never on a filler.

**Tests.** For each widget, one test asserting that a string with a combining
mark and a string with a wide character occupy the expected number of cells,
and that the caret in `edit.go` cannot come to rest on a filler.

## 10. Stage 6 - bidirectional text, display only

**Goal.** A dialog label, a menu item, a status line, a list item or a table
cell containing Hebrew or Arabic reads correctly: the RTL run is reversed,
brackets are mirrored, and embedded Latin or digits stay left to right.

**Tool.** `golang.org/x/text/unicode/bidi`, already an indirect dependency of
this module. Promote it to a direct one, the same way `rivo/uniseg` was
promoted in stage 1. Two pieces of it matter:

- `bidi.Paragraph` with `SetString` and `Order()` gives an `Ordering` whose
  runs are already in visual order, each with a `Direction()`.
- `bidi.AppendReverse(out, in []byte) []byte` reverses a run and swaps paired
  brackets for their mirror images, keeping combining marks with the runes
  they modify. This is exactly the RTL run transformation, and it means no
  mirroring table has to be written.

**What to build.** A new file `bidi.go`:

    // BidiMode selects how much of UAX #9 is applied.
    type BidiMode int
    const (
        BidiOff BidiMode = iota   // strings are laid out as stored
        BidiDisplay               // strings are reordered for display
        BidiFull                  // reordering plus caret and input support
    )
    var DefaultBidiMode = BidiDisplay

    // VisualString reorders s from logical to visual order.
    func VisualString(s string) string

    // VisualStringWithMap does the same and returns, for each cluster in
    // visual order, the byte offset it had in the logical string.
    func VisualStringWithMap(s string) (visual string, logicalOffsets []int)

`VisualString` is what stage 6 needs. `VisualStringWithMap` is what stage 7
needs; build both now, because the map falls out of the same walk and
retrofitting it later means writing the function twice.

Then route the read only widgets through it. The cleanest seam is
`StringToCharInfo` and `StringToCharInfoHighlighted`, since almost everything
draws through them: when `DefaultBidiMode` is not `BidiOff` and the string
contains any strong RTL character, reorder before clustering. Add the cheap
guard - a string of pure ASCII must not pay for a bidi pass - and measure
nothing until someone complains.

**Careful.** Selection and highlighting use byte offsets into the logical
string. Once the visual order differs, an attribute range that was contiguous
logically can become two or three separate spans visually. For stage 6 it is
acceptable to apply attributes before reordering, so that they travel with
their clusters. Say so in `TEXTSEG.md`.

**Tests.** Hebrew alone; Hebrew with an embedded Latin word; an RTL string
containing parentheses, asserting they are mirrored; a neutral string
unchanged; and the guard, asserting an ASCII string comes back identical.
Widths must be identical before and after reordering - that is the invariant
worth asserting hardest, because if it ever fails, every layout breaks.

## 11. Stage 7 - bidirectional text, editing

The user needs input, not just display. This is the hard part and it is last
on purpose.

**The problem.** The buffer is in logical order. The screen is in visual
order. The caret lives on screen but edits happen in the buffer, and the two
orders do not agree, so every caret operation needs a translation.

**What to build.**

1. A position map per displayed line, from `VisualStringWithMap`: visual
   cluster index to logical byte offset, and the inverse.
2. Caret movement. Left and right arrows move by one visual cluster, which
   means the logical offset can jump backwards on screen-right inside an RTL
   run. Home and End are visual. Word movement (`word_nav.go`) is logical.
3. Insertion. A character typed at a caret sitting between two runs of
   opposite direction has two defensible insertion points. Pick the one
   matching the direction of the run the caret arrived from, remember that
   direction as caret state, and document the choice. Do not try to be clever.
4. Selection. A visually contiguous selection can be logically discontiguous.
   Store the selection logically, render it as however many spans it takes.
5. The caret must never land on a filler, and never inside a cluster.

**Scope control.** `BidiFull` is a mode, not the default. Ship `BidiDisplay`
as the default and let an application opt in. If stage 7 turns out to need
more than two patches, split it: caret movement first, insertion second,
selection third.

**Tests.** These are unit testable and should be tested heavily, because
manual testing of bidi editing is miserable. Build an `Edit` with a mixed
direction string, drive it with synthetic key events, and assert the logical
buffer after each. Test that typing an ASCII letter inside an RTL run puts it
where the mode says it should, and that pressing left `n` times and right `n`
times returns the caret exactly where it started.

## 12. Stage 8 - our own width tables

**Why.** `go-runewidth` disagrees with terminals on real characters. Stage 1
already had to override it with Unicode categories because it gives the
Devanagari virama a column that no terminal gives it. That patch treats a
symptom.

**What far2l does.** Generates the table offline from ICU and compiles it in:
`utils/src/CharClasses_mk.cpp` walks every code point, groups consecutive
matches into ranges, and prints a `switch`. At runtime there is no ICU.

**What we would do.** A `go generate` step that reads the UCD data files -
`EastAsianWidth.txt`, `UnicodeData.txt`, `emoji-data.txt` - and emits a Go
file containing sorted range tables, looked up by binary search. No cgo, no
runtime dependency, and the Unicode version we target becomes a fact recorded
in the generated file rather than a property of whatever `go-runewidth`
happens to be pinned to.

**Why it is optional.** It is a correctness upgrade with no visible symptom
attached right now. Do it when a width disagreement is reported that the
category overrides cannot fix, or when someone wants a specific Unicode
version. Not before.

A related and more ambitious idea - making all applications and terminals
agree on one table rather than each shipping its own - is written up
separately in `WIDTH_NEGOTIATION.md`. It is a proposal, not a scheduled stage.

## 13. Deliberately out of scope

State these when asked, rather than quietly attempting them:

- **Shaping.** Arabic joining forms, Indic reordering and ligatures are the
  font stack's job. vtui counts columns and hands text to a backend.
- **Vertical text.** Not a terminal concept.
- **The colorer bug itself.** vtui's obligation ends at a documented interface
  and a working mapper, stage 3. The rest is another repository and another
  conversation.
- **Freeing the cluster registry.** See `REVIEW.md`; the ceiling is far away.

## 14. Test strings to keep using

Reuse these across stages, so results stay comparable:

| String | Expected columns | Why |
|---|---|---|
| `नमस्ते` | 4 | virama and vowel signs, the original layout bug |
| `का` | 2 | spacing mark, which does take a column |
| `👨\u200D👩\u200D👦` | 2 | ZWJ sequence |
| `❤\uFE0F` | 2, or 1 with `EmojiPresentationWide` off | the ambiguous case |
| `🇩🇪` | 2 | regional indicator pair |
| `1\uFE0F\u20E3` | 2 | keycap |
| `e\u0301` | 1 | the simplest composition |
| `A世B` | 4 | plain double width |
| `مرحبا` | 5 | RTL, no reordering needed within it |
| `abc مرحبا def` | 13 | mixed direction, width must not change |