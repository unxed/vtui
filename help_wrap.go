package vtui

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Help lines are authored as markup: #text# is bold, ~text~Target@ is a link
// to Target whose name is not drawn. A line longer than the window used to be
// cut at its edge, so a zoomed window changed nothing about it, and the words
// past the edge could not be read at all. Far and far2l break such a line at a
// space instead (f4 #378); wrapHelpTopic does that on a copy of the topic, the
// way the window is now, so nothing downstream of HelpView has to know.

// helpLiteral before a character makes it plain text: "\x10#" draws a '#'
// instead of toggling bold, "\x10~" a '~' instead of starting a link. The .hlf
// format has no escape (its authors write around the markup), but a Markdown
// document can have any of these characters in its text (markdown.go), so the
// markup needs one. A control character is used because no help text has one.
const helpLiteral = '\x10'

// helpCell is one drawn character of a help line and the markup it was under.
type helpCell struct {
	r    rune
	w    int
	bold bool
	link int  // index into the line's link targets, or -1
	lit  bool // came escaped by helpLiteral, and is written back so
}

// parseHelpCells reads a line of help markup into the characters it draws and
// the targets of its links. Only a complete ~text~target@ is a link; a stray ~
// is text. Bold inside a link is dropped: the link colour wins there anyway.
func parseHelpCells(line string) ([]helpCell, []string) {
	runes := []rune(line)
	var cells []helpCell
	var targets []string
	bold := false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == helpLiteral && i+1 < len(runes) {
			i++
			r = runes[i]
			cells = append(cells, helpCell{r: r, w: helpRuneWidth(r), bold: bold, link: -1, lit: true})
			continue
		}
		if r == '#' {
			bold = !bold
			continue
		}
		if r == '~' {
			close1 := indexRune(runes, '~', i+1)
			at := -1
			if close1 >= 0 {
				at = indexRune(runes, '@', close1+1)
			}
			if close1 >= 0 && at >= 0 {
				link := len(targets)
				targets = append(targets, string(runes[close1+1:at]))
				for j := i + 1; j < close1; j++ {
					t, lit := runes[j], false
					if t == helpLiteral && j+1 < close1 {
						j++
						t, lit = runes[j], true
					} else if t == '#' {
						continue
					}
					cells = append(cells, helpCell{r: t, w: helpRuneWidth(t), link: link, lit: lit})
				}
				i = at
				continue
			}
		}
		cells = append(cells, helpCell{r: r, w: helpRuneWidth(r), bold: bold, link: -1})
	}
	return cells, targets
}

// indexRune finds want in runes from from on, passing over a character
// escaped by helpLiteral.
func indexRune(runes []rune, want rune, from int) int {
	for i := from; i < len(runes); i++ {
		if runes[i] == helpLiteral {
			i++
			continue
		}
		if runes[i] == want {
			return i
		}
	}
	return -1
}

func helpRuneWidth(r rune) int {
	if w := runewidth.RuneWidth(r); w > 0 {
		return w
	}
	return 1
}

func helpCellsWidth(cells []helpCell) int {
	w := 0
	for _, c := range cells {
		w += c.w
	}
	return w
}

// wrapHelpCells breaks cells into rows of at most width columns, at the last
// space that fits, or in the middle of a word too long for a row. The space a
// row is broken at is not carried over to the next one.
func wrapHelpCells(cells []helpCell, width int) [][]helpCell {
	var rows [][]helpCell
	start := 0
	for start < len(cells) {
		w, end, lastSpace := 0, start, -1
		for end < len(cells) && w+cells[end].w <= width {
			if cells[end].r == ' ' {
				lastSpace = end
			}
			w += cells[end].w
			end++
		}
		if end >= len(cells) {
			rows = append(rows, cells[start:])
			break
		}
		if end == start {
			end = start + 1 // a cell wider than the row still takes a row
		}
		cut, next := end, end
		switch {
		case cells[end].r == ' ':
			next = end + 1 // the row ends exactly where a space is
		case lastSpace > start:
			cut, next = lastSpace, lastSpace+1
		}
		rows = append(rows, cells[start:cut])
		start = next
	}
	return rows
}

// helpCellsMarkup writes a row back as markup. Bold and a link that a break
// cut are closed at the end of the row and opened again on the next: every line
// is drawn from a clean state.
func helpCellsMarkup(row []helpCell, targets []string) string {
	var b strings.Builder
	bold, link := false, -1
	closeLink := func() {
		if link >= 0 {
			b.WriteString("~" + targets[link] + "@")
			link = -1
		}
	}
	for _, c := range row {
		if c.link != link {
			closeLink()
		}
		if c.bold != bold {
			b.WriteRune('#')
			bold = c.bold
		}
		if c.link != link {
			b.WriteRune('~')
			link = c.link
		}
		if c.lit {
			b.WriteRune(helpLiteral)
		}
		b.WriteRune(c.r)
	}
	closeLink()
	if bold {
		b.WriteRune('#')
	}
	return b.String()
}

// wrapHelpTopic returns the topic as it reads in a text area width columns wide,
// and for each of its lines the line of src it came from. A line that fits, a
// sticky header and a centered title stay as they are. When nothing needs
// breaking the answer is src itself.
func wrapHelpTopic(src *HelpTopic, width int) (*HelpTopic, []int) {
	rowSrc := make([]int, 0, len(src.Lines))
	if width < 4 {
		for i := range src.Lines {
			rowSrc = append(rowSrc, i)
		}
		return src, rowSrc
	}
	lines := make([]string, 0, len(src.Lines))
	changed := false
	tableAt := make(map[int]helpTable, len(src.tables))
	for _, t := range src.tables {
		if t.table.naturalWidth() > width {
			tableAt[t.first] = t
		}
	}
	skipUntil := 0
	for idx, line := range src.Lines {
		if idx < skipUntil {
			continue
		}
		if t, ok := tableAt[idx]; ok {
			if fitted, from := t.table.fit(width); fitted != nil {
				changed = true
				skipUntil = t.first + t.count
				lines = append(lines, fitted...)
				for _, row := range from {
					rowSrc = append(rowSrc, t.first+natRow(t, row))
				}
				continue
			}
		}
		if idx < src.StickyRows || strings.HasPrefix(line, "^") {
			lines = append(lines, line)
			rowSrc = append(rowSrc, idx)
			continue
		}
		cells, targets := parseHelpCells(line)
		if helpCellsWidth(cells) <= width {
			lines = append(lines, line)
			rowSrc = append(rowSrc, idx)
			continue
		}
		changed = true
		for _, row := range wrapHelpCells(cells, width) {
			lines = append(lines, helpCellsMarkup(row, targets))
			rowSrc = append(rowSrc, idx)
		}
	}
	if !changed {
		return src, rowSrc
	}
	derived := &HelpTopic{Name: src.Name, Lines: lines, StickyRows: src.StickyRows}
	for idx, line := range lines {
		if idx >= derived.StickyRows {
			parseHelpLinksInto(derived, line, idx)
		}
	}
	return derived, rowSrc
}

// natRow is the line of the natural layout of a table that its row i is on.
func natRow(t helpTable, i int) int {
	if t.table.header >= 0 && i > t.table.header {
		return i + 1 // the ruler under the header is a line of its own
	}
	return i
}
