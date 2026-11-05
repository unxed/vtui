# Word Navigation Rules

`f4` reproduces the word boundary logic of `far2l` (and therefore of Far Manager)
literally. The rules are subtle, they are not the same for plain cursor movement
and for selection, and the selecting variants differ further between the
multi-line editor and single-line input fields: in `far2l` these live in
different classes and were never unified. All of it is written down here.

## Character classes

Every character belongs to exactly one of three classes:

*   **space** — the space and the tab character.
*   **divider** — any character from the word divider set.
*   **word** — everything else: letters, digits, and also `_`, `$`, `#`, `@` and
    any non-ASCII character.

The default divider set is the same as `Opt.strWordDiv` in `far2l`:

    ~!%^&*()+|{}:"<>?`-=\[];',./

A space is never a member of that set. In `far2l` whitespace is checked
separately from the divider list, and `f4` keeps that distinction.

## Ctrl+Left and Ctrl+Right

These behave identically in the editor and in input fields.

A jump always moves at least one character, then keeps going until a stop
condition holds. Conditions are evaluated for a pair of neighbouring characters
`(prev, curr)`, where `curr` is the character the cursor would land on.

`Ctrl+Right` stops when:

*   `prev` is a space and `curr` is not — the start of the next token; or
*   `prev` is a word character and `curr` is a divider — the end of a word.

`Ctrl+Left` stops when:

*   `prev` is a space and `curr` is not; or
*   `prev` is a divider and `curr` is a word character — the start of a word.

Nothing else is a boundary. In particular, a run of dividers is never split,
even if it mixes different divider characters: `...///` is crossed in a single
jump. And `foo.bar` takes two jumps to the right, not three:

    |foo.bar  ->  foo|.bar  ->  foo.bar|

## Ctrl+Shift+Left and Ctrl+Shift+Right

While a selection is being extended, `far2l` switches to a different rule set,
and not the same one everywhere. Pick the subsection that matches the widget.

### In the editor

The editor uses a coarser rule than plain movement: dividers are treated exactly
like spaces, so a selection always covers whole words.

*   `Ctrl+Shift+Right` stops when `prev` is a word character and `curr` is not.
*   `Ctrl+Shift+Left` stops when `prev` is not a word character and `curr` is.

The practical consequence is an asymmetry with plain movement. On `foo bar`,
`Ctrl+Shift+Right` selects `foo` and then `foo bar`, never leaving the cursor on
the space in between, whereas plain `Ctrl+Right` does stop there. This is not an
oversight: `far2l` behaves the same way, and muscle memory from Far depends on
it.

### In single-line input fields

The command line and dialog input fields go the other way and become finer than
plain movement: every boundary inside a run of dividers becomes a stop.

*   `Ctrl+Shift+Right` stops everywhere except inside a word and before a space.
*   `Ctrl+Shift+Left` stops everywhere except inside a word, before a space, and
    on a divider that directly follows a word character.

So on `a.-/b` plain `Ctrl+Right` goes to `a|.-/b` and then straight to the end of
the field, while `Ctrl+Shift+Right` visits every position of the divider run in
turn.

## Line boundaries

A jump does not run past the end of a logical line: `Ctrl+Right` at the end of a
line moves to the beginning of the next one, and `Ctrl+Left` at the beginning of
a line moves to the end of the previous one. When word wrap is on, a jump also
stops at the end of the current visual line rather than continuing on the next
screen row.

## Word deletion in input fields (`FarKeys`)

A single-line `Edit` with `FarKeys` set (off by default, so dialog fields keep
plain Backspace/Delete) takes three more keys: `Ctrl+K` deletes from the cursor
to the end of the text, `Ctrl+Backspace` and `Ctrl+Delete` delete the word to
the left and to the right of the cursor. A "word" here is the run of one
character class that the double-click selection takes, so `/usr/bin` is the
three runs `/`, `usr`, `/`, `bin` and a run of spaces is a word of its own. A
selection, if any, is deleted instead. The cursor is readable and settable
through `Edit.CursorPos` and `Edit.SetCursorPos` (runes), so the owner can build
what `Edit` does not do itself, such as completing the line from a history.
