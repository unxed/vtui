package vtui

import (
	"strings"
	"testing"
)

const wideTable = "| Name | Description | Link |\n" +
	"|------|:-----------:|-----:|\n" +
	"| alpha | A fairly long description of the first row that cannot fit | [site](https://example.com) |\n" +
	"| beta | Short | x |\n"

func plainOf(line string) string {
	cells, _ := parseHelpCells(line)
	var b strings.Builder
	for _, c := range cells {
		b.WriteRune(c.r)
	}
	return b.String()
}

// f4#2233: a table wider than the window keeps its columns and breaks the text
// inside the cells instead of breaking the lines at the window's edge.
func TestWrapHelpTopic_FitsAWideTableToTheWindow(t *testing.T) {
	topic := ParseMarkdownTopic("T", wideTable)
	natural := topic.Lines
	const width = 40
	got, rowSrc := wrapHelpTopic(topic, width)
	if got == topic {
		t.Fatal("a table wider than the window should be laid out again")
	}
	if len(got.Lines) != len(rowSrc) {
		t.Fatalf("%d lines but %d sources", len(got.Lines), len(rowSrc))
	}
	var text []string
	for i, l := range got.Lines {
		cells, _ := parseHelpCells(l)
		if w := helpCellsWidth(cells); w > width {
			t.Errorf("line %d is %d columns wide, window is %d: %q", i, w, width, plainOf(l))
		}
		if rowSrc[i] < 0 || rowSrc[i] >= len(natural) || (i > 0 && rowSrc[i] < rowSrc[i-1]) {
			t.Errorf("rowSrc = %v is out of range or goes back", rowSrc)
		}
		text = append(text, plainOf(l))
	}
	all := strings.Join(text, "\n")
	for _, want := range []string{"Name", "Description", "alpha", "beta", "Short", "┼", "site"} {
		if !strings.Contains(all, want) {
			t.Errorf("%q is lost from the table:\n%s", want, all)
		}
	}
	for _, word := range strings.Fields("A fairly long description of the first row that cannot fit") {
		if !strings.Contains(all, word) {
			t.Errorf("word %q is lost from the table:\n%s", word, all)
		}
	}
	// Every line of a wrapped row has the columns at the same places.
	cols := -1
	for _, l := range text {
		if strings.Contains(l, "─┼─") {
			continue
		}
		if n := strings.Count(l, " │ "); cols < 0 {
			cols = n
		} else if n != cols {
			t.Errorf("line %q has %d column separators, others %d", l, n, cols)
		}
	}
	if len(got.Links) != 1 || got.Links[0].Target != "https://example.com" {
		t.Errorf("Links = %+v, want the one link of the table kept", got.Links)
	}
}

func TestWrapHelpTopic_KeepsATableThatFits(t *testing.T) {
	topic := ParseMarkdownTopic("T", wideTable)
	got, _ := wrapHelpTopic(topic, 200)
	if got != topic {
		t.Error("a table that fits should be returned as it is")
	}
}

func TestWrapHelpTopic_WindowTooNarrowForTheColumns(t *testing.T) {
	topic := ParseMarkdownTopic("T", wideTable)
	got, rowSrc := wrapHelpTopic(topic, 8)
	if len(got.Lines) != len(rowSrc) || len(got.Lines) == 0 {
		t.Fatalf("%d lines, %d sources", len(got.Lines), len(rowSrc))
	}
	for _, l := range got.Lines {
		cells, _ := parseHelpCells(l)
		if w := helpCellsWidth(cells); w > 8 {
			t.Errorf("line %q is %d columns wide, window is 8", plainOf(l), w)
		}
	}
}

func TestWrapHelpTopic_TextAroundAWideTableIsKept(t *testing.T) {
	topic := ParseMarkdownTopic("T", "Before.\n\n"+wideTable+"\nAfter.")
	got, _ := wrapHelpTopic(topic, 40)
	all := ""
	for _, l := range got.Lines {
		all += plainOf(l) + "\n"
	}
	if !strings.HasPrefix(all, "Before.\n") || !strings.HasSuffix(all, "After.\n") {
		t.Errorf("text around the table changed:\n%s", all)
	}
}
