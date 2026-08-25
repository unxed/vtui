package vtui

import (
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
)

// AutoCompleteItem is a single entry of the autocomplete list.
type AutoCompleteItem struct {
	Text string
	// Display is shown instead of Text when non-empty (e.g. the final path
	// element instead of the full path). MatchStart/MatchEnd then refer to
	// Display. Text is still what gets inserted on accept.
	Display string
	// Cells renders the item as multiple columns (panel-style). Cells[0] is
	// the main text (falls back to Display/Text when shorter); extra cells
	// get fixed columns sized to their widest content.
	Cells []string
	// Attr overrides the item foreground (file-highlight colors); 0 uses the
	// default list colors.
	Attr uint64
	// Separator draws a non-selectable divider line between item groups.
	Separator bool
	// MatchStart/MatchEnd mark the needle span in the displayed string (rune
	// indices, end inclusive) that gets highlighted. MatchStart -1 means no
	// highlight.
	MatchStart int
	MatchEnd   int
	// ReplaceFrom/ReplaceTo define the rune span of the Edit text replaced
	// when the item is accepted. The zero span (both 0) keeps the legacy
	// behavior: the whole text is replaced.
	ReplaceFrom int
	ReplaceTo   int
}

// PathHintProvider is installed by the host application (f4) to contribute
// file path suggestions to the autocomplete menu. word is the whitespace
// bounded token under the cursor, from/to its rune span in the edit text.
// Returning nil or an empty slice means "no path suggestions".
var PathHintProvider func(edit *Edit, word string, from, to int) []AutoCompleteItem

// maxAutoCompleteVisible caps the visible rows in total mode; longer lists
// scroll. In per-category mode each separator-delimited group (path hints of
// each panel, then history) contributes up to this many rows.
var autoCompleteMaxVisible = 5

// autoCompletePerCategory selects how the visible height is computed:
// false = the whole list shares one window of autoCompleteMaxVisible rows,
// true = each category shows up to autoCompleteMaxVisible rows of its own.
var autoCompletePerCategory bool

// SetAutoCompleteMaxVisible sets the visible row cap. Values below 1 are
// ignored.
func SetAutoCompleteMaxVisible(n int) {
	if n >= 1 {
		autoCompleteMaxVisible = n
	}
}

// SetAutoCompletePerCategory switches between one shared window and a
// per-category row budget (see autoCompletePerCategory).
func SetAutoCompletePerCategory(on bool) {
	autoCompletePerCategory = on
}

type AutoCompleteMenu struct {
	Window
	Edit    *Edit
	lb      *ListBox
	Matches []string // texts of items ("" for separators), kept for API compatibility
	items   []AutoCompleteItem
	// hasPathItems marks that the leading group came from PathHintProvider;
	// the menu then anchors to the trigger separator instead of the cursor.
	hasPathItems bool
}

func NewAutoCompleteMenu(edit *Edit) *AutoCompleteMenu {
	ac := &AutoCompleteMenu{
		Window: *NewWindow(0, 0, 10, 10, ""),
		Edit:   edit,
	}
	ac.ShowClose = false
	ac.ShowZoom = false
	ac.Modal = true
	ac.frame.boxType = SingleBox

	ac.lb = NewListBox(0, 0, 10, 10, nil)
	ac.lb.ColorTextIdx = ColDialogText
	ac.lb.ColorSelectedTextIdx = ColMenuBarSelected
	ac.lb.ShowScrollBar = true
	ac.lb.IsSelectable = func(i int) bool {
		return i >= 0 && i < len(ac.items) && !ac.items[i].Separator
	}
	ac.lb.OnAction = func(idx int) {
		// A click on a path item only inserts it; a click on a history item
		// (whole-text legacy) also executes, as before.
		ac.accept(idx, true)
	}
	ac.AddItem(ac.lb)

	ac.UpdateMatches()
	return ac
}

func (ac *AutoCompleteMenu) HasShadow() bool {
	return false
}

func (ac *AutoCompleteMenu) SetPosition(x1, y1, x2, y2 int) {
	ac.Window.SetPosition(x1, y1, x2, y2)
	ac.X1, ac.Y1, ac.X2, ac.Y2 = x1, y1, x2, y2
	if ac.frame != nil {
		ac.frame.SetPosition(x1, y1, x2, y2)
	}
	if ac.rootGroup != nil {
		ac.rootGroup.SetPosition(x1+1, y1+1, x2-1, y2-1)
	}
	if ac.lb != nil {
		ac.lb.SetPosition(x1+1, y1+1, x2-1, y2-1)
		// ListBox.SetPosition pins the first column to the full width;
		// restore the multi-column layout.
		ac.applyColumns()
	}
}

// applyColumns rebuilds the list box columns from the items: column 0 is
// flexible, extra columns (from AutoCompleteItem.Cells) are fixed to their
// widest content.
func (ac *AutoCompleteMenu) applyColumns() {
	n := 1
	for i := range ac.items {
		if len(ac.items[i].Cells) > n {
			n = len(ac.items[i].Cells)
		}
	}
	cols := make([]TableColumn, n)
	cols[0] = TableColumn{MinWidth: 8} // flexible
	for c := 1; c < n; c++ {
		w := 0
		for i := range ac.items {
			if c < len(ac.items[i].Cells) {
				if cw := StringWidth(ac.items[i].Cells[c]); cw > w {
					w = cw
				}
			}
		}
		cols[c] = TableColumn{Width: w}
	}
	ac.lb.Columns = cols
	ac.lb.ShowSeparators = n > 1
}

func (ac *AutoCompleteMenu) HasMatches() bool {
	return len(ac.Matches) > 0
}

// SelectPos returns the index of the currently selected item in the menu.
func (ac *AutoCompleteMenu) SelectPos() int {
	if ac.lb != nil {
		return ac.lb.SelectPos
	}
	return -1
}

// IsBusy reports true if the underlying frame is busy (e.g. PanelsFrame in console view),
// suppressing full-screen Flush passes that would otherwise overwrite host console history.
func (ac *AutoCompleteMenu) IsBusy() bool {
	if FrameManager != nil && len(FrameManager.frames) > 1 {
		under := FrameManager.frames[len(FrameManager.frames)-2]
		if under != nil && under.IsBusy() {
			return true
		}
	}
	return false
}

// capAutoCompleteGroups keeps at most n non-separator items in each
// separator-delimited group.
func capAutoCompleteGroups(items []AutoCompleteItem, n int) []AutoCompleteItem {
	out := make([]AutoCompleteItem, 0, len(items))
	count := 0
	for _, it := range items {
		if it.Separator {
			count = 0
			out = append(out, it)
			continue
		}
		if count < n {
			out = append(out, it)
			count++
		}
	}
	return out
}

// acRow adapts AutoCompleteItem to the ListBox table model.
type acRow struct {
	ac  *AutoCompleteMenu
	idx int
}

func (r acRow) GetCellText(col int) string {
	it := r.ac.items[r.idx]
	if it.Separator {
		if col == 0 {
			return strings.Repeat("─", 64) // truncated to the cell width on draw
		}
		return ""
	}
	if col < len(it.Cells) {
		return it.Cells[col]
	}
	if col == 0 {
		if it.Display != "" {
			return it.Display
		}
		return it.Text
	}
	return ""
}

func (r acRow) GetCellAttr(col int, def uint64) uint64 {
	it := r.ac.items[r.idx]
	if it.Separator {
		return DimColor(def)
	}
	if it.Attr == 0 {
		return def
	}
	// Keep the state background (cursor/selection), take the item foreground.
	if it.Attr&IsFgRGB != 0 {
		return SetRGBFore(def, GetRGBFore(it.Attr))
	}
	return SetIndexFore(def, GetIndexFore(it.Attr))
}

func (ac *AutoCompleteMenu) UpdateMatches() {
	ac.items = nil
	ac.Matches = nil

	// Path suggestions come first when the host provider has any.
	ac.hasPathItems = false
	if ac.Edit.PathHintsEnabled && PathHintProvider != nil {
		from, to, word := ac.Edit.WordUnderCursor()
		ac.items = append(ac.items, PathHintProvider(ac.Edit, word, from, to)...)
		ac.hasPathItems = len(ac.items) > 0
	}
	pathCount := len(ac.items)

	// Legacy history: matched by the fuzzy matcher, deduplicated. Ranking
	// (highest first): exact match (score remapped to -1), prefix, substring
	// (the further left, the better), everything else by ascending score —
	// a plain (score, match start) ordering implements all of it.
	text := ac.Edit.GetText()
	if text != "" {
		matcher := NewFuzzyMatcher(text, false)
		seen := make(map[string]bool)
		type histMatch struct {
			text       string
			score      int
			start, end int
		}

		var hist []histMatch
		for _, h := range ac.Edit.History {
			if seen[h] {
				continue
			}
			score, start, end, ok := matcher.Match(h)
			if !ok {
				continue
			}
			seen[h] = true
			if matcher.IsMatchExact() {
				score = -1 // exact match always wins sort
			}
			hist = append(hist, histMatch{h, score, start, end})
		}

		sort.SliceStable(hist, func(a, b int) bool {
			if hist[a].score != hist[b].score {
				return hist[a].score < hist[b].score
			}

			return hist[a].start < hist[b].start
		})

		for i, hm := range hist {
			if pathCount > 0 && i == 0 {
				ac.items = append(ac.items, AutoCompleteItem{Separator: true, MatchStart: -1, MatchEnd: -1})
			}
			ac.items = append(ac.items, AutoCompleteItem{Text: hm.text, MatchStart: hm.start, MatchEnd: hm.end})
		}
	}

	for _, it := range ac.items {
		ac.Matches = append(ac.Matches, it.Text)
	}

	if autoCompletePerCategory {
		ac.items = capAutoCompleteGroups(ac.items, autoCompleteMaxVisible)
		ac.Matches = ac.Matches[:0]
		for _, it := range ac.items {
			ac.Matches = append(ac.Matches, it.Text)
		}
	}

	if len(ac.items) == 0 {
		return
	}

	rows := make([]TableRow, len(ac.items))
	for i := range ac.items {
		rows[i] = acRow{ac: ac, idx: i}
	}
	ac.lb.Items = ac.Matches
	ac.lb.SetRows(rows)

	// Keep the selection on a selectable row.
	if ac.lb.SelectPos < 0 || ac.lb.SelectPos >= len(ac.items) || !ac.lb.IsSelectable(ac.lb.SelectPos) {
		sel := -1
		for i := range ac.items {
			if ac.lb.IsSelectable(i) {
				sel = i
				break
			}
		}
		if sel < 0 {
			ac.Matches = nil
			return
		}
		ac.lb.SetSelectPos(sel)
	}

	ac.reposition()
}

// reposition places the menu next to the text cursor: below the edit line,
// flipped above when there is more room there; clamped to the screen.
func (ac *AutoCompleteMenu) reposition() {
	scrW, scrH := 80, 25
	if FrameManager != nil && FrameManager.scr != nil {
		scrW = FrameManager.scr.width
		scrH = FrameManager.scr.height
	}

	vis := len(ac.items)
	limit := autoCompleteMaxVisible
	if autoCompletePerCategory {
		// Each of the three possible categories (active panel, passive
		// panel, history) gets its own budget, plus up to two separators.
		limit = 3*autoCompleteMaxVisible + 2
	}
	if vis > limit {
		vis = limit
	}
	h := vis + 2

	w := 24
	for i := range ac.items {
		if iw := runewidth.StringWidth(ac.items[i].Text) + 2; iw > w {
			w = iw
		}
	}
	if w > scrW {
		w = scrW
	}

	cur := ac.Edit.curPos
	if cur > len(ac.Edit.text) {
		cur = len(ac.Edit.text)
	}
	if ac.Edit.leftPos > cur {
		ac.Edit.leftPos = cur
	}
	cursorX := ac.Edit.X1 + runewidth.StringWidth(string(ac.Edit.text[ac.Edit.leftPos:cur]))
	if ac.hasPathItems {
		// Anchor to the trigger separator of the current token, so the menu
		// does not drift right as the user keeps typing.
		from, _, _ := ac.Edit.WordUnderCursor()
		sep := -1
		for i := cur - 1; i >= from && i < len(ac.Edit.text); i-- {
			if ac.Edit.text[i] == '/' || ac.Edit.text[i] == '\\' {
				sep = i
				break
			}
		}
		if sep >= 0 {
			anchor := sep
			if anchor < ac.Edit.leftPos {
				anchor = ac.Edit.leftPos
			}
			cursorX = ac.Edit.X1 + runewidth.StringWidth(string(ac.Edit.text[ac.Edit.leftPos:anchor]))
		}
	}

	x1 := cursorX
	if x1+w > scrW {
		x1 = scrW - w
	}
	if x1 < 0 {
		x1 = 0
	}
	x2 := x1 + w - 1

	y1 := ac.Edit.Y1 + 1
	if y1+h > scrH && ac.Edit.Y1 >= h {
		y1 = ac.Edit.Y1 - h // flip above the edit line
	} else if y1+h > scrH {
		h = scrH - y1
		if h < 3 {
			h = 3
		}
	}
	y2 := y1 + h - 1

	ac.SetPosition(x1, y1, x2, y2)
}

// accept applies the chosen item: it replaces either the item's rune span of
// the edit text (path hints) or the whole text (legacy history). Directory
// paths keep the menu open for drill-down; injectEnter only applies to
// legacy whole-text items.
func (ac *AutoCompleteMenu) accept(idx int, injectEnter bool) {
	if idx < 0 || idx >= len(ac.items) {
		ac.Close()
		return
	}
	it := ac.items[idx]
	if it.Separator {
		return
	}
	e := ac.Edit
	legacy := it.ReplaceTo <= it.ReplaceFrom
	if legacy {
		e.SetText(it.Text)
		e.curPos = len(e.text)
	} else {
		newText := make([]rune, 0, len(e.text)+len([]rune(it.Text)))
		newText = append(newText, e.text[:it.ReplaceFrom]...)
		newText = append(newText, []rune(it.Text)...)
		newText = append(newText, e.text[it.ReplaceTo:]...)
		e.SetText(string(newText))
		e.curPos = it.ReplaceFrom + len([]rune(it.Text))
	}
	e.clearFlag = false
	e.HistoryPos = -1

	if !legacy && (strings.HasSuffix(it.Text, "/") || strings.HasSuffix(it.Text, "\\")) {
		// Directory selected: refresh the hint for the new token.
		ac.UpdateMatches()
		if !ac.HasMatches() {
			ac.Close()
		}
		return
	}

	ac.Close()
	if legacy && injectEnter && FrameManager != nil {
		FrameManager.InjectEvents([]*vtinput.InputEvent{
			{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN},
		})
	}
}

func (ac *AutoCompleteMenu) Show(scr *ScreenBuf) {
	ac.Window.Show(scr)

	// Needle highlight over the plain rows drawn by the list box.
	for i := 0; i < ac.lb.ViewHeight; i++ {
		idx := ac.lb.TopPos + i
		if idx >= len(ac.items) {
			break
		}
		it := &ac.items[idx]
		if it.Separator || it.MatchStart < 0 {
			continue
		}
		disp := it.Text
		if it.Display != "" {
			disp = it.Display
		}
		if len(it.Cells) > 0 {
			disp = it.Cells[0]
		}
		runes := []rune(disp)
		if it.MatchStart >= len(runes) {
			continue
		}
		end := it.MatchEnd
		if end >= len(runes) {
			end = len(runes) - 1
		}
		x := ac.lb.X1 + runewidth.StringWidth(string(runes[:it.MatchStart]))
		w := runewidth.StringWidth(string(runes[it.MatchStart : end+1]))
		for cx := x; cx < x+w && cx <= ac.lb.X2; cx++ {
			cell := scr.GetCell(cx, ac.lb.Y1+i)
			cell.Attributes = InvertColors(cell.Attributes)
			scr.Write(cx, ac.lb.Y1+i, []CharInfo{cell})
		}
	}

	footer := " Up/Down Enter Esc Tab Shift+Del "
	p := NewPainter(scr)
	p.DrawTitle(ac.X1, ac.Y2, ac.X2, footer, Palette[ColDialogBoxTitle])

	if ac.Edit.curPos > len(ac.Edit.text) {
		ac.Edit.curPos = len(ac.Edit.text)
	}
	if ac.Edit.leftPos > ac.Edit.curPos {
		ac.Edit.leftPos = ac.Edit.curPos
	}

	if ac.IsFocused() {
		headText := string(ac.Edit.text[ac.Edit.leftPos:ac.Edit.curPos])
		vOffset := runewidth.StringWidth(headText)
		scr.SetCursorPos(ac.Edit.X1+vOffset, ac.Edit.Y1)
		scr.SetCursorVisible(true)
	}
}

func (ac *AutoCompleteMenu) ProcessKey(e *vtinput.InputEvent) bool {
	if e.Type == vtinput.FocusEventType {
		return ac.Window.ProcessKey(e)
	}
	if !e.KeyDown {
		return false
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_PRIOR, vtinput.VK_NEXT:
		return ac.lb.ProcessKey(e)
	case vtinput.VK_ESCAPE:
		ac.Close()
		return true
	case vtinput.VK_TAB:
		ac.accept(ac.lb.SelectPos, false)
		return true
	case vtinput.VK_RETURN:
		inject := (e.ControlKeyState & vtinput.ShiftPressed) == 0
		ac.accept(ac.lb.SelectPos, inject)
		return true
	case vtinput.VK_DELETE:
		if (e.ControlKeyState & vtinput.ShiftPressed) != 0 {
			if ac.lb.SelectPos >= 0 && ac.lb.SelectPos < len(ac.items) {
				it := ac.items[ac.lb.SelectPos]
				// Only legacy history items can be removed from history.
				if !it.Separator && it.ReplaceTo <= it.ReplaceFrom {
					newHist := []string{}
					for _, h := range ac.Edit.History {
						if h != it.Text {
							newHist = append(newHist, h)
						}
					}
					ac.Edit.History = newHist
					if ac.Edit.HistoryID != "" && GlobalHistoryProvider != nil {
						GlobalHistoryProvider.SaveHistory(ac.Edit.HistoryID, newHist)
					}
					ac.UpdateMatches()
					if !ac.HasMatches() {
						ac.Close()
					}
				}
			}
			return true
		}
	}

	oldText := ac.Edit.GetText()
	handled := ac.Edit.ProcessKey(e)
	if handled {
		newText := ac.Edit.GetText()
		if newText != oldText {
			if newText == "" {
				ac.Close()
			} else {
				ac.UpdateMatches()
				if !ac.HasMatches() {
					ac.Close()
				}
			}
		}
	}
	return handled
}

func (ac *AutoCompleteMenu) ProcessMouse(e *vtinput.InputEvent) bool {
	if ac.lb.ProcessMouse(e) {
		return true
	}
	// Consume all mouse events within the menu bounds to prevent
	// the parent Window class from initiating a drag or resize operation.
	return true
}
