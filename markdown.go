package vtui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// f4#258 asked for a Markdown browser, and the owner's decision was to base
// it on the existing help engine rather than write a new renderer: HelpTopic
// (help_engine.go) is already a generic "lines of text plus links" document,
// and HelpView/help_wrap.go already know how to scroll, word-wrap and
// navigate one, regardless of where it came from. So a Markdown document is
// turned into the SAME markup HelpTopic's own author-facing format uses
// (`#bold#`, `~text~target@`) and shown by HelpView unchanged.
//
// The Markdown itself is read by goldmark (CommonMark plus the GitHub
// extensions for tables, task lists and bare URLs), not by a line
// regex of our own (f4#1625): the first version knew only headings, bold and
// links, and a '#' anywhere -- a "# comment" line in a code block, "C#" in a
// sentence -- was taken for a heading or a bold toggle. The document's AST is
// laid out here as follows:
//   - a heading is a bold line, with a blank line before it;
//   - **strong**, *emphasis* and `code` are bold: the help palette has one
//     highlight colour, and far2l's help uses it for exactly these;
//   - [text](target), an image's alt text and a bare URL are links, so
//     Enter/click works like any other help link (http(s) targets open in
//     the browser);
//   - a paragraph is one line (HelpView word-wraps it to the window), a hard
//     line break starts a new one;
//   - a code block keeps its lines as they are, indented, with no markup;
//   - list items get a "•" or their number, quotes a "│" bar, a table its
//     columns lined up, a thematic break a rule;
//   - a blank line in the source between two blocks stays one.
//
// Text is written with every character that means something to the help
// markup ('#', '~', '^') escaped by helpLiteral, so it always shows as
// itself.

// markdownParser is shared: goldmark's parser keeps its per-document state in
// a context of each Parse call and is safe to use from several goroutines.
//
// Of GitHub's extensions it takes tables, task lists and bare URLs, but not
// strikethrough: goldmark's takes a single ~ too, so "~/src and ~/bin" or
// "~tilde~" would lose their tildes, and the help palette could not show
// the struck-through text any differently anyway.
var markdownParser parser.Parser = goldmark.New(goldmark.WithExtensions(
	extension.Table, extension.TaskList, extension.Linkify,
)).Parser()

// markdownRuleWidth is how wide a thematic break (---) is drawn.
const markdownRuleWidth = 40

// ParseMarkdownTopic converts a Markdown document into a HelpTopic: the same
// intermediate representation help_engine.go's .hlf loader produces, so
// HelpView (and anything else that already knows how to show a HelpTopic)
// can display it unchanged. name becomes the topic's Name, the same role a
// .hlf file's "@Name" line plays.
func ParseMarkdownTopic(name, markdown string) *HelpTopic {
	topic, _ := ParseMarkdownTopicMap(name, markdown)
	return topic
}

// ParseMarkdownTopicMap is ParseMarkdownTopic that also says where each line
// of the topic came from: srcLine[i] is the 0-based line of markdown that
// topic line i was made from. A host that shows the topic beside the text (f4's
// preview next to its editor) uses it to keep the two in step exactly.
//
// The map is by block: every block starts at its own first source line, and
// the lines the block turned into are spread over the source lines up to the
// next block (a fenced code block, whose lines are its source lines, lands
// exactly). A blank line between blocks belongs to the block after it.
// len(srcLine) == len(topic.Lines), and srcLine never decreases.
func ParseMarkdownTopicMap(name, markdown string) (*HelpTopic, []int) {
	source := []byte(markdown)
	doc := markdownParser.Parse(text.NewReader(source))
	r := &markdownRenderer{source: source}
	lines, srcLine, tables := r.blocksMapped(doc)
	if len(lines) == 0 {
		lines, srcLine = []string{""}, []int{0}
	}
	topic := &HelpTopic{Name: name, Lines: lines, tables: tables}
	for idx, line := range lines {
		parseHelpLinksInto(topic, line, idx)
	}
	return topic, srcLine
}

// blocksMapped is blocks for the document's top level, with the source line
// of every line it lays out (see ParseMarkdownTopicMap).
func (r *markdownRenderer) blocksMapped(doc gast.Node) ([]string, []int, []helpTable) {
	lineStarts := []int{0}
	for i, b := range r.source {
		if b == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}
	total := len(lineStarts)
	if n := len(r.source); n > 0 && r.source[n-1] == '\n' {
		total-- // the text ends with a newline: no line after it
	}
	lineOf := func(offset int) int {
		return sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > offset }) - 1
	}
	var firstLine func(n gast.Node) int
	firstLine = func(n gast.Node) int {
		if seg := n.Lines(); seg != nil && seg.Len() > 0 {
			l := lineOf(seg.At(0).Start)
			if _, ok := n.(*gast.FencedCodeBlock); ok {
				l-- // the opening fence is a line of its own
			}
			return max(l, 0)
		}
		if t, ok := n.(*gast.Text); ok {
			return lineOf(t.Segment.Start)
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if l := firstLine(c); l >= 0 {
				return l
			}
		}
		return -1
	}

	type piece struct {
		start int
		base  int // source line of the first line when each line is a source line (code), else -1
		lines []string
		table *markdownTable // the layout of the table these lines are, if they are one
	}
	var pieces []piece
	prevStart := 0
	var prevLines []string
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		var lines []string
		var table *markdownTable
		if tn, ok := c.(*east.Table); ok {
			table = r.tableLayout(tn)
			lines, _ = table.natural()
		} else {
			lines = r.block(c)
		}
		if len(lines) == 0 {
			continue
		}
		start := firstLine(c)
		if start < prevStart {
			start = prevStart
		}
		if len(prevLines) > 0 && prevLines[len(prevLines)-1] != "" && markdownBlankBefore(c) {
			pieces = append(pieces, piece{start: start, base: -1, lines: []string{""}})
		}
		base := -1
		switch c.(type) {
		case *gast.FencedCodeBlock, *gast.CodeBlock:
			if seg := c.Lines(); seg != nil && seg.Len() > 0 {
				base = lineOf(seg.At(0).Start)
			}
		}
		pieces = append(pieces, piece{start: start, base: base, lines: lines, table: table})
		prevStart, prevLines = start, lines
	}

	var out []string
	var src []int
	var tables []helpTable
	for i, p := range pieces {
		if p.table != nil {
			tables = append(tables, helpTable{first: len(out), count: len(p.lines), table: p.table})
		}
		end := total
		for _, q := range pieces[i+1:] {
			if q.start > p.start {
				end = q.start
				break
			}
		}
		span := max(end-p.start, 1)
		for j, l := range p.lines {
			line := p.start + j*span/len(p.lines)
			if p.base >= 0 {
				line = min(p.base+j, max(total-1, 0))
			}
			if len(src) > 0 && line < src[len(src)-1] {
				line = src[len(src)-1]
			}
			out = append(out, l)
			src = append(src, line)
		}
	}
	return out, src, tables
}

// markdownRun is a piece of a line's text and how it is drawn.
type markdownRun struct {
	text string // as shown: no markup, no escapes
	bold bool
	link string // target, or "" when the run is not a link
}

// markdownRenderer lays out the blocks of a parsed document as help lines.
type markdownRenderer struct {
	source []byte
}

// blocks lays out the block children of n, one after the other.
func (r *markdownRenderer) blocks(n gast.Node) []string {
	var out []string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		lines := r.block(c)
		if len(lines) == 0 {
			continue
		}
		if len(out) > 0 && out[len(out)-1] != "" && markdownBlankBefore(c) {
			out = append(out, "")
		}
		out = append(out, lines...)
	}
	return out
}

// markdownBlankBefore says whether a blank line separates c from the block
// before it: when the source has one there, and always before a heading or a
// table (goldmark does not record the blank line for a table, which it makes
// out of a paragraph after parsing).
func markdownBlankBefore(c gast.Node) bool {
	switch c.Kind() {
	case gast.KindHeading, east.KindTable:
		return true
	}
	return c.HasBlankPreviousLines()
}

func (r *markdownRenderer) block(n gast.Node) []string {
	switch n := n.(type) {
	case *gast.Heading:
		return r.inlineLines(n, true)
	case *gast.Paragraph, *gast.TextBlock:
		return r.inlineLines(n, false)
	case *gast.FencedCodeBlock:
		return r.rawLines(n.Lines(), "    ")
	case *gast.CodeBlock:
		return r.rawLines(n.Lines(), "    ")
	case *gast.HTMLBlock:
		if n.HTMLBlockType == gast.HTMLBlockType2 {
			return nil // an <!-- comment --> is not shown by a browser either
		}
		lines := r.rawLines(n.Lines(), "")
		if n.HasClosure() {
			closure := n.ClosureLine
			lines = append(lines, markdownEscape(markdownRawLine(closure.Value(r.source))))
		}
		return lines
	case *gast.ThematicBreak:
		return []string{strings.Repeat("─", markdownRuleWidth)}
	case *gast.Blockquote:
		return markdownPrefix(r.blocks(n), "│ ", "│ ")
	case *gast.List:
		return r.list(n)
	case *east.Table:
		return r.table(n)
	case *gast.LinkReferenceDefinition:
		return nil
	}
	return r.blocks(n)
}

// list lays out a list's items under their bullet or number.
func (r *markdownRenderer) list(n *gast.List) []string {
	var out []string
	num := n.Start
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		marker := "• "
		if n.IsOrdered() {
			marker = strconv.Itoa(num) + string(n.Marker) + " "
			num++
		}
		body := r.blocks(item)
		if len(body) == 0 {
			body = []string{""}
		}
		if len(out) > 0 && !n.IsTight {
			out = append(out, "")
		}
		indent := strings.Repeat(" ", runewidth.StringWidth(marker))
		out = append(out, markdownPrefix(body, marker, indent)...)
	}
	return out
}

// markdownPrefix puts first before the first line and rest before the others
// (an empty line gets rest without its trailing spaces).
func markdownPrefix(lines []string, first, rest string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		p := rest
		if i == 0 {
			p = first
		}
		if line == "" {
			p = strings.TrimRight(p, " ")
		}
		out[i] = p + line
	}
	return out
}

// rawLines is a code or HTML block: every line as it is, no markup.
func (r *markdownRenderer) rawLines(lines *text.Segments, indent string) []string {
	out := make([]string, 0, lines.Len())
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		line := markdownRawLine(seg.Value(r.source))
		if line == "" {
			out = append(out, "")
			continue
		}
		out = append(out, indent+markdownEscape(line))
	}
	return out
}

// markdownRawLine is one source line without its line ending and with its
// tabs expanded to the next multiple of 4 columns.
func markdownRawLine(b []byte) string {
	line := strings.TrimRight(string(b), "\r\n")
	if !strings.Contains(line, "\t") {
		return line
	}
	var sb strings.Builder
	col := 0
	for _, c := range line {
		if c == '\t' {
			n := 4 - col%4
			sb.WriteString(strings.Repeat(" ", n))
			col += n
			continue
		}
		sb.WriteRune(c)
		col += runewidth.RuneWidth(c)
	}
	return sb.String()
}

// table lays out a table with its columns lined up, the header bold and
// ruled off from the body.
func (r *markdownRenderer) table(n *east.Table) []string {
	lines, _ := r.tableLayout(n).natural()
	return lines
}

// tableLayout reads a table into its cells; natural and fit lay it out.
func (r *markdownRenderer) tableLayout(n *east.Table) *markdownTable {
	t := &markdownTable{header: -1}
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		isHeader := row.Kind() == east.KindTableHeader
		if isHeader {
			t.header = len(t.rows)
		}
		var cells []markdownCell
		for c := row.FirstChild(); c != nil; c = c.NextSibling() {
			in := &markdownInline{r: r}
			if isHeader {
				in.bold = 1
			}
			in.children(c)
			var runs []markdownRun
			for i, line := range in.lines {
				if i > 0 {
					runs = append(runs, markdownRun{text: " "})
				}
				runs = append(runs, line...)
			}
			w := 0
			for _, run := range runs {
				w += runewidth.StringWidth(run.text)
			}
			col := len(cells)
			if col >= len(t.widths) {
				t.widths = append(t.widths, 0)
			}
			t.widths[col] = max(t.widths[col], w)
			cells = append(cells, markdownCell{runs: runs, width: w})
		}
		t.rows = append(t.rows, cells)
	}
	t.align = n.Alignments
	return t
}

// inlineLines lays out the inline content of a paragraph or a heading.
func (r *markdownRenderer) inlineLines(n gast.Node, bold bool) []string {
	in := &markdownInline{r: r}
	if bold {
		in.bold = 1
	}
	in.children(n)
	out := make([]string, 0, len(in.lines))
	for _, line := range in.lines {
		out = append(out, markdownMarkup(line))
	}
	return out
}

// markdownInline collects the runs of inline content, line by line.
type markdownInline struct {
	r     *markdownRenderer
	lines [][]markdownRun
	bold  int    // depth of bold spans the walk is inside
	link  string // target of the link the walk is inside
}

func (in *markdownInline) add(s string) {
	if s == "" {
		return
	}
	if len(in.lines) == 0 {
		in.lines = append(in.lines, nil)
	}
	last := len(in.lines) - 1
	in.lines[last] = append(in.lines[last], markdownRun{text: s, bold: in.bold > 0, link: in.link})
}

// newline ends the line (a hard line break), without the spaces before it.
func (in *markdownInline) newline() {
	if len(in.lines) == 0 {
		in.lines = append(in.lines, nil)
	}
	last := len(in.lines) - 1
	for runs := in.lines[last]; len(runs) > 0; runs = runs[:len(runs)-1] {
		end := &runs[len(runs)-1]
		end.text = strings.TrimRight(end.text, " ")
		if end.text != "" {
			in.lines[last] = runs
			break
		}
		in.lines[last] = runs[:len(runs)-1]
	}
	in.lines = append(in.lines, nil)
}

func (in *markdownInline) children(n gast.Node) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		in.node(c)
	}
}

func (in *markdownInline) node(n gast.Node) {
	source := in.r.source
	switch n := n.(type) {
	case *gast.Text:
		v := n.Segment.Value(source)
		if n.IsRaw() {
			in.add(string(v))
		} else {
			in.add(markdownUnescape(v))
		}
		if n.HardLineBreak() {
			in.newline()
		} else if n.SoftLineBreak() {
			in.add(" ")
		}
	case *gast.String:
		if n.IsCode() || n.IsRaw() {
			in.add(string(n.Value))
		} else {
			in.add(markdownUnescape(n.Value))
		}
	case *gast.CodeSpan:
		in.bold++
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *gast.Text:
				in.add(strings.ReplaceAll(string(t.Segment.Value(source)), "\n", " "))
			case *gast.String:
				in.add(strings.ReplaceAll(string(t.Value), "\n", " "))
			}
		}
		in.bold--
	case *gast.Emphasis:
		in.bold++
		in.children(n)
		in.bold--
	case *gast.Link:
		in.linked(markdownUnescape(n.Destination), n)
	case *gast.Image:
		in.linked(markdownUnescape(n.Destination), n)
	case *gast.AutoLink:
		url := string(n.URL(source))
		if n.AutoLinkType == gast.AutoLinkEmail && !strings.HasPrefix(strings.ToLower(url), "mailto:") {
			url = "mailto:" + url
		}
		outer := in.link
		if outer == "" {
			in.link = url
		}
		in.add(string(n.Label(source)))
		in.link = outer
	case *gast.RawHTML:
		for i := 0; i < n.Segments.Len(); i++ {
			seg := n.Segments.At(i)
			in.add(strings.ReplaceAll(string(seg.Value(source)), "\n", " "))
		}
	case *east.TaskCheckBox:
		if n.IsChecked {
			in.add("[x] ")
		} else {
			in.add("[ ] ")
		}
	default:
		in.children(n)
	}
}

// linked walks the text of a link (or an image's alt text) as a link to
// target. A link with no text of its own shows its target.
func (in *markdownInline) linked(target string, n gast.Node) {
	if target == "" || in.link != "" {
		in.children(n)
		return
	}
	in.link = target
	before := in.runCount()
	in.children(n)
	if in.runCount() == before {
		in.add(target)
	}
	in.link = ""
}

func (in *markdownInline) runCount() int {
	count := 0
	for _, line := range in.lines {
		count += len(line)
	}
	return count
}

// markdownUnescape resolves what a CommonMark renderer resolves in text:
// backslash escapes and entity/numeric character references.
func markdownUnescape(b []byte) string {
	b = util.UnescapePunctuations(b)
	b = util.ResolveNumericReferences(b)
	b = util.ResolveEntityNames(b)
	return string(b)
}

// markdownMarkup writes a line of runs as help markup: #bold#, ~text~target@
// and the text escaped. Bold is not written inside a link, where the link
// colour wins anyway (help_wrap.go drops it too).
func markdownMarkup(runs []markdownRun) string {
	var b strings.Builder
	bold, link := false, ""
	for _, run := range runs {
		if run.text == "" {
			continue
		}
		if run.link != link {
			if link != "" {
				b.WriteString("~" + markdownTarget(link) + "@")
			}
			if run.link != "" {
				if bold {
					b.WriteByte('#')
					bold = false
				}
				b.WriteByte('~')
			}
			link = run.link
		}
		if link == "" && run.bold != bold {
			b.WriteByte('#')
			bold = run.bold
		}
		markdownEscapeInto(&b, run.text)
	}
	if link != "" {
		b.WriteString("~" + markdownTarget(link) + "@")
	}
	if bold {
		b.WriteByte('#')
	}
	return b.String()
}

// markdownTarget is a link target as the help markup can carry it: it ends
// at the first '@', so one inside the target is percent-encoded (what it
// means in a URL anyway), and it cannot hold control characters.
func markdownTarget(target string) string {
	target = strings.ReplaceAll(target, "@", "%40")
	return strings.Map(func(c rune) rune {
		if c < ' ' || c == 0x7f {
			return -1
		}
		return c
	}, target)
}

// markdownEscape is text written so the help markup shows it as it is.
func markdownEscape(s string) string {
	var b strings.Builder
	markdownEscapeInto(&b, s)
	return b.String()
}

func markdownEscapeInto(b *strings.Builder, s string) {
	for _, c := range s {
		switch {
		case c == '#' || c == '~' || c == '^':
			b.WriteRune(helpLiteral)
			b.WriteRune(c)
		case c == '\t' || c == '\n' || c == '\r':
			b.WriteByte(' ')
		case c < ' ' || c == 0x7f:
			// A control character draws as garbage, and helpLiteral itself
			// would escape the character after it.
		default:
			b.WriteRune(c)
		}
	}
}

// NewMarkdownView renders markdown as a scrollable, navigable HelpView --
// the same widget f4's own F1 help uses -- without a preloaded .hlf file or
// a shared *HelpEngine: it builds a private one-topic engine just for this
// document. title becomes both the topic's name and, through HelpView's own
// SwitchTopic, the window's title bar text.
func NewMarkdownView(title, markdown string) *HelpView {
	engine := NewHelpEngine(nil)
	engine.AddTopic(ParseMarkdownTopic(title, markdown))
	return NewHelpView(engine, title)
}
