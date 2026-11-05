package vtui

import (
	"strings"

	"github.com/mattn/go-runewidth"
	east "github.com/yuin/goldmark/extension/ast"
)

// A Markdown table is laid out twice: at its natural width, one line a row,
// when the window is wide enough for it, and at the window's width, with the
// text of a long cell broken inside its column, when it is not (f4#2233: the
// natural lines of a wide table were broken by the generic line wrapping at
// the window's edge, and the columns were no longer readable). The topic keeps
// the table (HelpTopic.tables) so that wrapHelpTopic can lay it out again for
// the width it is given.

// markdownTableSep is drawn between two columns, markdownTableMinCol is the
// narrowest a column is made when a table is fitted to a window.
const (
	markdownTableSep    = " │ "
	markdownTableMinCol = 3
)

type markdownCell struct {
	runs  []markdownRun
	width int
}

// markdownTable is a table's cells, the width of each column at its natural
// layout and the alignment of the columns.
type markdownTable struct {
	rows   [][]markdownCell
	header int // index of the header row, or -1
	widths []int
	align  []east.Alignment
}

// helpTable is the place of a table among the lines of a topic: the lines
// first .. first+count-1 are its natural layout.
type helpTable struct {
	first, count int
	table        *markdownTable
}

// naturalWidth is the width of the table's lines at their natural layout.
func (t *markdownTable) naturalWidth() int {
	w := 0
	for _, cw := range t.widths {
		w += cw
	}
	if len(t.widths) > 1 {
		w += runewidth.StringWidth(markdownTableSep) * (len(t.widths) - 1)
	}
	return w
}

// padOf is how many spaces go before a cell's text in a column of the given
// width, out of the pad spaces it is short of it.
func (t *markdownTable) padOf(col, pad int) int {
	if col < len(t.align) {
		switch t.align[col] {
		case east.AlignRight:
			return pad
		case east.AlignCenter:
			return pad / 2
		}
	}
	return 0
}

// natural lays the table out at the widths of its widest cells, and says
// which of the returned lines each row is (the ruler under the header gets
// the header's row).
func (t *markdownTable) natural() ([]string, []int) {
	var out []string
	var from []int
	for i, cells := range t.rows {
		var line []markdownRun
		for col, c := range cells {
			if col > 0 {
				line = append(line, markdownRun{text: markdownTableSep})
			}
			pad := t.widths[col] - c.width
			left := t.padOf(col, pad)
			line = append(line, markdownRun{text: strings.Repeat(" ", left)})
			line = append(line, c.runs...)
			if col < len(cells)-1 {
				line = append(line, markdownRun{text: strings.Repeat(" ", pad-left)})
			}
		}
		out = append(out, markdownMarkup(line))
		from = append(from, i)
		if i == t.header {
			out = append(out, t.ruler(t.widths))
			from = append(from, i)
		}
	}
	return out, from
}

func (t *markdownTable) ruler(widths []int) string {
	parts := make([]string, len(widths))
	for col, w := range widths {
		parts[col] = strings.Repeat("─", w)
	}
	return strings.Join(parts, "─┼─")
}

// fitWidths narrows the widest columns until the table is width columns wide,
// and says false when the window is too narrow for columns of any use.
func (t *markdownTable) fitWidths(width int) ([]int, bool) {
	n := len(t.widths)
	avail := width - runewidth.StringWidth(markdownTableSep)*(n-1)
	if n == 0 || avail < n*markdownTableMinCol {
		return nil, false
	}
	widths := append([]int(nil), t.widths...)
	sum := 0
	for _, w := range widths {
		sum += w
	}
	for sum > avail {
		widest := 0
		for col, w := range widths {
			if w > widths[widest] {
				widest = col
			}
		}
		if widths[widest] <= markdownTableMinCol {
			return nil, false
		}
		widths[widest]--
		sum--
	}
	return widths, true
}

// fit lays the table out in width columns: a cell that is wider than its
// column is broken at spaces inside it, and the other cells of the row wait
// beside it. It returns nil when the window is too narrow for that. from says
// which row of the table each line is.
func (t *markdownTable) fit(width int) ([]string, []int) {
	widths, ok := t.fitWidths(width)
	if !ok {
		return nil, nil
	}
	var out []string
	var from []int
	for i, cells := range t.rows {
		type piece struct {
			markup string
			width  int
		}
		pieces := make([][]piece, len(cells))
		height := 1
		for col, c := range cells {
			if col >= len(widths) {
				continue
			}
			hc, targets := parseHelpCells(markdownMarkup(c.runs))
			rows := wrapHelpCells(hc, widths[col])
			if len(rows) == 0 {
				rows = [][]helpCell{nil}
			}
			for _, row := range rows {
				pieces[col] = append(pieces[col], piece{helpCellsMarkup(row, targets), helpCellsWidth(row)})
			}
			height = max(height, len(rows))
		}
		for j := 0; j < height; j++ {
			var b strings.Builder
			for col := range pieces {
				if col > 0 {
					b.WriteString(markdownTableSep)
				}
				p := piece{}
				if j < len(pieces[col]) {
					p = pieces[col][j]
				}
				pad := widths[col] - p.width
				left := t.padOf(col, pad)
				b.WriteString(strings.Repeat(" ", left))
				b.WriteString(p.markup)
				if col < len(pieces)-1 {
					b.WriteString(strings.Repeat(" ", pad-left))
				}
			}
			out = append(out, b.String())
			from = append(from, i)
		}
		if i == t.header {
			out = append(out, t.ruler(widths))
			from = append(from, i)
		}
	}
	return out, from
}
