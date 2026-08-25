package vtui

import (
	"github.com/unxed/vtinput"
	"sort"
	"strings"
)

// TableColumn defines the properties of a single table column.
type TableColumn struct {
	Title string
	// Width in characters. Width <= 0 makes the column flexible: all flexible
	// columns evenly share the space left after fixed-width columns and
	// separators, and are recomputed whenever the table is resized.
	Width int
	// MinWidth is the minimum width of a flexible column (Width <= 0), in
	// characters. If MinWidth <= 0, the title width is used as the minimum.
	// Ignored for fixed-width columns.
	MinWidth  int
	Alignment Alignment
}

// TableRow is an interface for data providers.
type TableRow interface {
	GetCellText(col int) string
}

// TableCellProvider provides direct cell data without allocating TableRow wrappers.
type TableCellProvider interface {
	RowCount() int
	GetCellText(row, col int) string
}

// TableCellAttrProvider allows cell-specific attributes via TableCellProvider.
type TableCellAttrProvider interface {
	GetCellAttr(row, col int, defaultAttr uint64) uint64
}

// TableCellSelectProvider allows row/cell selection via TableCellProvider.
// IsRowSelected(row) only sees a row number, so it cannot tell apart cells
// in grid layouts where several data items share one row across different
// columns (e.g. a multi-column file panel). Implementers of such layouts
// should also implement TableCellColSelectProvider, which the table prefers
// whenever both are present.
type TableCellSelectProvider interface {
	IsRowSelected(row int) bool
}

// TableCellColSelectProvider is the column-aware counterpart of
// TableCellSelectProvider, for TableCellProvider-backed tables whose cells
// at the same row but different columns can belong to different, separately
// selectable data items (grid/multi-column layouts). When a cellProvider
// implements this, the table calls IsCellSelected(row, col) instead of
// IsRowSelected(row).
type TableCellColSelectProvider interface {
	IsCellSelected(row, col int) bool
}

// Table is a generic control for displaying tabular data.
// SelectableRow is an optional interface for rows that can be selected.
type SelectableRow interface {
	IsSelected() bool
}

// MultiColSelectableRow is an interface for multi-column rows where selection is cell-specific.
type MultiColSelectableRow interface {
	IsColSelected(col int) bool
}

// CellColorableRow is an optional interface allowing rows to define custom colors per cell.
type CellColorableRow interface {
	GetCellAttr(col int, defaultAttr uint64) uint64
}

// Table is a generic control for displaying tabular data.
type Table struct {
	ScrollView
	Columns      []TableColumn
	Rows         []TableRow
	cellProvider TableCellProvider

	SelectCol        int
	CellSelection    bool
	ShowHeader       bool
	ShowSeparators   bool
	AlwaysShowCursor bool

	// Sortable enables click-on-header sorting. Default is false: no sorting
	// and header clicks are ignored, so applications doing their own sorting
	// (e.g. by rewriting column titles) are unaffected.
	Sortable bool
	// SortColumn is the column rows are sorted by; -1 (default) means no
	// sorting. SortAscending controls the direction. SortCompare is an
	// optional comparator; when nil, rows are compared by cell text.
	SortColumn    int
	SortAscending bool
	SortCompare   func(a, b TableRow, col int) int

	// QuickSearch enables type-to-filter: while the table is focused,
	// printable characters go into a search string shown in a line above the
	// table header, and rows are filtered by fuzzy match (Myers' bit-vector
	// algorithm) against all columns, best match wins. The filtered list is
	// ranked by (edit distance, match position), best match at the top —
	// closest to the search line. Default is false.
	QuickSearch bool
	// SearchCaseSensitive makes QuickSearch case-sensitive (default false).
	SearchCaseSensitive bool
	// SearchExactOnHit keeps only exact matches when at least one exact match
	// exists. Fuzzy matches remain available while no exact result is present.
	SearchExactOnHit bool
	// OnSearchChange is called whenever the search string changes.
	OnSearchChange func(text string)

	ColorTextIdx             int
	ColorSelectedTextIdx     int
	ColorItemSelectTextIdx   int
	ColorItemSelectCursorIdx int
	ColorTitleIdx            int
	ColorBoxIdx              int
	// ColorHighlightIdx is the QuickSearch match highlight; applied last, on
	// top of every other cell color. Defaults to ColMenuHighlight.
	ColorHighlightIdx int

	// colWidths caches the resolved column widths (flexible columns expanded);
	// reused across frames to avoid allocations in the render hot path.
	colWidths []int
	// order maps display position to Rows index when sorting is active.
	// The Rows slice itself is never reordered.
	order []int
	// searchRunes is the QuickSearch string; searchCursor/searchLeft are the
	// cursor and horizontal scroll positions (in runes) within it.
	searchRunes  []rune
	searchCursor int
	searchLeft   int
	// matchBuf is reused by the search filter to avoid allocations.
	matchBuf []searchMatch
	// matchSpans holds the matched cell span per Rows index for needle
	// highlighting; col == -1 means the row is not matched.
	matchSpans []cellHighlight
	cellBuf    []CharInfo
}

// searchMatch is one row passing the QuickSearch filter.
type searchMatch struct {
	idx, score, pos int
}

// cellHighlight locates the matched substring inside one cell: column index
// and the [start, end] span in runes of that cell's text.
type cellHighlight struct {
	col, start, end int
}

func NewTable(x, y, w, h int, columns []TableColumn) *Table {
	t := &Table{
		Columns:                  columns,
		Rows:                     []TableRow{},
		ShowHeader:               true,
		ShowSeparators:           true,
		ColorTextIdx:             ColTableText,
		ColorSelectedTextIdx:     ColTableSelectedText,
		ColorItemSelectTextIdx:   ColTableText,
		ColorItemSelectCursorIdx: ColTableSelectedText,
		ColorTitleIdx:            ColTableColumnTitle,
		ColorBoxIdx:              ColTableBox,
		ColorHighlightIdx:        ColMenuHighlight,
		SortColumn:               -1,
	}
	t.canFocus = true
	t.InitScrollBar(t)
	t.SetPosition(x, y, x+w-1, y+h-1)
	return t
}

// resolvedWidths returns the effective width of every column. Columns with
// Width <= 0 are flexible: each gets its minimum width (MinWidth, or the
// title width if unset), then the remaining content width (after fixed-width
// columns and the 1-cell gaps between columns) is distributed between them
// evenly. The result is recomputed on every call, so it follows widget
// resizes automatically.
func (t *Table) resolvedWidths() []int {
	n := len(t.Columns)
	t.colWidths = t.colWidths[:0]
	if n == 0 {
		return t.colWidths
	}

	flexCount := 0
	fixed := 0
	minSum := 0
	for i := range t.Columns {
		if t.Columns[i].Width <= 0 {
			flexCount++
			minSum += t.Columns[i].minWidth()
		} else {
			fixed += t.Columns[i].Width
		}
	}

	extra := 0
	if flexCount > 0 {
		avail := t.GetContentWidth() - fixed - (n - 1)
		if avail < minSum {
			avail = minSum // each flexible column gets at least its minimum
		}
		extra = avail - minSum
	}
	per := 0
	rem := 0
	if flexCount > 0 {
		per = extra / flexCount
		rem = extra % flexCount
	}

	for i := range t.Columns {
		w := t.Columns[i].Width
		if w <= 0 {
			w = t.Columns[i].minWidth() + per
			if rem > 0 {
				w++
				rem--
			}
		}
		t.colWidths = append(t.colWidths, w)
	}
	return t.colWidths
}

// minWidth returns the minimum width of a flexible column: MinWidth if set,
// otherwise the width of the column title.
func (c *TableColumn) minWidth() int {
	if c.MinWidth > 0 {
		return c.MinWidth
	}
	return StringWidth(c.Title)
}

func (t *Table) SetRows(rows []TableRow) {
	t.rowProvider = nil
	t.cellProvider = nil
	t.Rows = rows
	t.ItemCount = len(rows)
	t.resort()
	if t.ItemCount == 0 {
		t.SelectPos = 0
	} else if t.SelectPos >= t.ItemCount {
		t.SelectPos = t.ItemCount - 1
	} else if t.SelectPos < 0 {
		t.SelectPos = 0
	}
	t.EnsureVisible()
}

// SetRowProvider configures an on-demand data source for virtualized table display.
func (t *Table) SetRowProvider(p RowProvider) {
	t.Rows = nil
	t.cellProvider = nil
	t.ScrollView.SetRowProvider(p)
	t.resort()
	t.EnsureVisible()
}

// SetCellProvider configures a direct zero-alloc cell provider.
func (t *Table) SetCellProvider(p TableCellProvider) {
	t.Rows = nil
	t.rowProvider = nil
	t.cellProvider = p
	if p != nil {
		t.ItemCount = p.RowCount()
	} else {
		t.ItemCount = 0
	}
	t.resort()
	t.EnsureVisible()
}

// SetRowCount sets the total logical row count for cell providers.
func (t *Table) SetRowCount(n int) {
	t.ItemCount = n
	if t.ItemCount == 0 {
		t.SelectPos = 0
	} else if t.SelectPos >= t.ItemCount {
		t.SelectPos = t.ItemCount - 1
	} else if t.SelectPos < 0 {
		t.SelectPos = 0
	}
	t.EnsureVisible()
}

// SetSort sorts rows by the given column. A negative col disables sorting.
// The header of the sorted column shows a direction arrow (↑/↓).
func (t *Table) SetSort(col int, ascending bool) {
	t.SortColumn = col
	t.SortAscending = ascending
	t.resort()
}

// ClearSort disables sorting and restores the original row order.
func (t *Table) ClearSort() {
	t.SortColumn = -1
	t.resort()
}

// resort rebuilds the display-to-row index mapping. With no active sort it
// is the identity mapping; the Rows slice itself is never reordered.
func (t *Table) resort() {
	n := len(t.Rows)
	if t.cellProvider != nil {
		n = t.cellProvider.RowCount()
	} else if t.rowProvider != nil {
		n = t.rowProvider.RowCount()
	}
	t.ItemCount = n

	if len(t.searchRunes) > 0 {
		t.applySearchFilter()
	} else if col := t.SortColumn; col >= 0 && col < len(t.Columns) && n >= 2 {
		if cap(t.order) < n {
			t.order = make([]int, n)
		} else {
			t.order = t.order[:n]
		}
		for i := range t.order {
			t.order[i] = i
		}
		ascending := t.SortAscending
		cmp := t.SortCompare
		sort.SliceStable(t.order, func(i, j int) bool {
			aIdx, bIdx := t.order[i], t.order[j]
			if cmp != nil && len(t.Rows) == n {
				c := cmp(t.Rows[aIdx], t.Rows[bIdx], col)
				if !ascending {
					c = -c
				}
				return c < 0
			}
			var aText, bText string
			if t.cellProvider != nil {
				aText = t.cellProvider.GetCellText(aIdx, col)
				bText = t.cellProvider.GetCellText(bIdx, col)
			} else if t.rowProvider != nil {
				aCells := t.rowProvider.Row(aIdx)
				bCells := t.rowProvider.Row(bIdx)
				if col < len(aCells) {
					aText = aCells[col]
				}
				if col < len(bCells) {
					bText = bCells[col]
				}
			} else {
				aText = t.Rows[aIdx].GetCellText(col)
				bText = t.Rows[bIdx].GetCellText(col)
			}
			c := strings.Compare(aText, bText)
			if !ascending {
				c = -c
			}
			return c < 0
		})
	} else {
		t.order = t.order[:0]
	}

	if len(t.searchRunes) > 0 {
		t.ItemCount = len(t.order)
		// While a query is being typed, keep the focus on the most
		// relevant row (the top one, right below the search line).
		t.SelectPos = 0
	} else {
		t.ItemCount = n
	}

	if t.SelectPos >= t.ItemCount {
		t.SelectPos = t.ItemCount - 1
	}
	if t.SelectPos < 0 {
		t.SelectPos = 0
	}
}

// applySearchFilter keeps only rows fuzzy-matching the search string (best
// match across all columns wins) and ranks them by (distance, position).
// The matched cell span is remembered per row for needle highlighting.
func (t *Table) applySearchFilter() {
	matcher := NewFuzzyMatcher(string(t.searchRunes), t.SearchCaseSensitive)
	t.matchBuf = t.matchBuf[:0]
	rowCount := len(t.Rows)

	if t.cellProvider != nil {
		rowCount = t.cellProvider.RowCount()
	} else if t.rowProvider != nil {
		rowCount = t.rowProvider.RowCount()
	}

	if cap(t.matchSpans) < rowCount {
		t.matchSpans = make([]cellHighlight, rowCount)
	} else {
		t.matchSpans = t.matchSpans[:rowCount]
	}

	for i := range t.matchSpans {
		t.matchSpans[i].col = -1
	}

	hasExact := false

	for i := 0; i < rowCount; i++ {
		bestScore := -1
		bestStart, bestEnd, bestCol := 0, 0, 0
		exactHit := false

		for col := range t.Columns {
			cellText := ""
			if t.cellProvider != nil {
				cellText = t.cellProvider.GetCellText(i, col)
			} else if t.rowProvider != nil {
				cells := t.rowProvider.Row(i)
				if col < len(cells) {
					cellText = cells[col]
				}
			} else {
				cellText = t.Rows[i].GetCellText(col)
			}

			score, start, end, ok := matcher.Match(cellText)
			if ok && (bestScore < 0 || score < bestScore || (score == bestScore && start < bestStart)) {
				bestScore, bestStart, bestEnd, bestCol = score, start, end, col
			}

			exactHit = exactHit || matcher.IsMatchExact()
		}

		if bestScore >= 0 {
			if exactHit {
				bestScore = -1 // exact match always wins sort
				hasExact = true
			}
			t.matchBuf = append(t.matchBuf, searchMatch{i, bestScore, bestStart})
			t.matchSpans[i] = cellHighlight{bestCol, bestStart, bestEnd}
		}
	}

	if t.SearchExactOnHit && hasExact {
		// Exact matches exist: drop rows with a greater distance as irrelevant.
		write := 0
		for _, m := range t.matchBuf {
			if m.score == -1 {
				t.matchBuf[write] = m
				write++
			}
		}
		t.matchBuf = t.matchBuf[:write]
	}

	sort.Slice(t.matchBuf, func(a, b int) bool {
		ma, mb := t.matchBuf[a], t.matchBuf[b]
		if ma.score != mb.score {
			return ma.score < mb.score
		}
		if ma.pos != mb.pos {
			return ma.pos < mb.pos
		}
		return ma.idx < mb.idx
	})

	if cap(t.order) < len(t.matchBuf) {
		t.order = make([]int, len(t.matchBuf))
	} else {
		t.order = t.order[:len(t.matchBuf)]
	}

	for i, m := range t.matchBuf {
		t.order[i] = m.idx
	}
}

// SearchText returns the current QuickSearch string.
func (t *Table) SearchText() string {
	return string(t.searchRunes)
}

// SetSearchText replaces the QuickSearch string and refilters the rows.
func (t *Table) SetSearchText(text string) {
	t.searchRunes = []rune(text)
	t.searchCursor = len(t.searchRunes)
	t.searchLeft = 0
	t.resort()
	t.EnsureVisible()
	t.fireSearchChange()
}

// ClearSearch empties the QuickSearch string and restores the full row list.
func (t *Table) ClearSearch() {
	if len(t.searchRunes) == 0 {
		return
	}
	t.searchRunes = t.searchRunes[:0]
	t.searchCursor = 0
	t.searchLeft = 0
	t.resort()
	t.fireSearchChange()
}

func (t *Table) fireSearchChange() {
	if t.OnSearchChange != nil {
		t.OnSearchChange(string(t.searchRunes))
	}
}

// rowAt maps a display position to the index in Rows, accounting for the
// active sorting. Out-of-range positions are returned unchanged.
func (t *Table) rowAt(pos int) int {
	if len(t.order) > 0 && pos >= 0 && pos < len(t.order) {
		return t.order[pos]
	}
	return pos
}

// RowAt maps a display position (e.g. SelectPos) to the index in Rows,
// accounting for the active sorting. With no sorting it returns pos.
func (t *Table) RowAt(pos int) int {
	return t.rowAt(pos)
}

func (t *Table) Show(scr *ScreenBuf) {
	t.ScreenObject.Show(scr)
	t.DisplayObject(scr)
}

func (t *Table) DisplayObject(scr *ScreenBuf) {
	if !t.IsVisible() {
		return
	}

	// Ensure margins are in sync with ShowHeader/ShowScrollBar before rendering
	t.SetPosition(t.X1, t.Y1, t.X2, t.Y2)

	yOffset := 0

	// 1. Draw the QuickSearch line above everything else
	if t.QuickSearch {
		t.drawSearchLine(scr)
		yOffset++
	}

	// 2. Draw Header
	widths := t.resolvedWidths()
	if t.ShowHeader {
		t.drawRow(scr, t.Y1+yOffset, -1, -1, Palette[t.ColorTitleIdx], widths)
		yOffset++
	}

	// 3. Draw Data Rows (ViewHeight already excludes header and search line).
	// Search results rank top-down: the best match sits right below the
	// header, closest to the search line.
	dataHeight := t.ViewHeight
	if dataHeight < 0 {
		dataHeight = 0
	}
	for i := 0; i < dataHeight; i++ {
		displayPos := t.TopPos + i
		currY := t.Y1 + yOffset + i

		if displayPos < t.ItemCount {
			rowIdx := t.rowAt(displayPos)
			//isSelected := false
			// Calculate standard attribute as a fallback (passed into drawRow)
			attr := Palette[t.ColorTextIdx]
			t.drawRow(scr, currY, rowIdx, displayPos, attr, widths)
		} else {
			// Fill empty space with background color
			scr.FillRect(t.X1, currY, t.X2, currY, ' ', Palette[t.ColorTextIdx])
		}
	}

	// 4. Draw Vertical Separators if needed
	if t.ShowSeparators {
		p := NewPainter(scr)
		currX := t.X1
		sepChar := boxSymbols[bsV] // │
		sepY1 := t.Y1
		if t.QuickSearch {
			sepY1++ // do not cross the search line
		}
		for i := 0; i < len(t.Columns)-1; i++ {
			currX += widths[i]
			p.Fill(currX, sepY1, currX, t.Y2, sepChar, Palette[t.ColorBoxIdx])
			currX++
		}
	}

	// 5. Draw Scrollbar
	t.DrawScrollBar(scr)
}

func (t *Table) drawRow(scr *ScreenBuf, y int, rowIdx int, displayPos int, attr uint64, widths []int) {
	endX := t.X1 + t.GetContentWidth() - 1

	currX := t.X1
	for colIdx, col := range t.Columns {
		text := ""
		if rowIdx == -1 {
			text = col.Title
		} else if t.cellProvider != nil {
			text = t.cellProvider.GetCellText(rowIdx, colIdx)
		} else if t.rowProvider != nil {
			cells := t.rowProvider.Row(rowIdx)
			if colIdx < len(cells) {
				text = cells[colIdx]
			}
		} else if rowIdx < len(t.Rows) {
			text = t.Rows[rowIdx].GetCellText(colIdx)
		}

		isSelected := false
		if rowIdx != -1 {
			if t.cellProvider != nil {
				if csp, ok := t.cellProvider.(TableCellColSelectProvider); ok {
					isSelected = csp.IsCellSelected(rowIdx, colIdx)
				} else if sp, ok := t.cellProvider.(TableCellSelectProvider); ok {
					isSelected = sp.IsRowSelected(rowIdx)
				}
			} else if rowIdx < len(t.Rows) {
				if mcsr, ok := t.Rows[rowIdx].(MultiColSelectableRow); ok {
					isSelected = mcsr.IsColSelected(colIdx)
				} else if selRow, ok := t.Rows[rowIdx].(SelectableRow); ok {
					isSelected = selRow.IsSelected()
				}
			}
		}

		isCursorHere := displayPos == t.SelectPos && (!t.CellSelection || colIdx == t.SelectCol)

		stateAttr := attr
		if rowIdx != -1 {
			if isCursorHere {
				if t.IsFocused() {
					if isSelected {
						stateAttr = Palette[t.ColorItemSelectCursorIdx]
					} else {
						stateAttr = Palette[t.ColorSelectedTextIdx]
					}
				} else {
					if isSelected {
						stateAttr = Palette[t.ColorItemSelectTextIdx]
					} else if t.AlwaysShowCursor {
						stateAttr = Palette[t.ColorSelectedTextIdx]
					}
				}
			} else if isSelected {
				stateAttr = Palette[t.ColorItemSelectTextIdx]
			}

			if t.cellProvider != nil {
				if ap, ok := t.cellProvider.(TableCellAttrProvider); ok {
					stateAttr = ap.GetCellAttr(rowIdx, colIdx, stateAttr)
				}
			} else if rowIdx < len(t.Rows) {
				if cr, ok := t.Rows[rowIdx].(CellColorableRow); ok {
					stateAttr = cr.GetCellAttr(colIdx, stateAttr)
				}
			}
		}

		cellAttr := stateAttr

		if rowIdx == -1 && colIdx == t.SortColumn {
			cellText := t.formatSortedHeader(text, widths[colIdx], col.Alignment)
			t.cellBuf = FillCharInfoString(t.cellBuf, cellText, cellAttr)
		} else {
			t.cellBuf = FillCharInfoAligned(t.cellBuf, text, widths[colIdx], col.Alignment, cellAttr)
		}
		// Paint the matched substring with the highlight color (QuickSearch).
		// Goes last, on top of all other colors.
		if rowIdx >= 0 && len(t.searchRunes) > 0 && rowIdx < len(t.matchSpans) {
			if span := t.matchSpans[rowIdx]; span.col == colIdx {
				t.applyCellHighlight(t.cellBuf, text, widths[colIdx], col.Alignment, span)
			}
		}
		scr.Write(currX, y, t.cellBuf)
		currX += widths[colIdx]

		// Skip separator space if not the last column
		if colIdx < len(t.Columns)-1 {
			currX++
		}
	}

	// Fill remaining horizontal space if any
	lastX := currX - 1
	if lastX < endX {
		scr.FillRect(lastX+1, y, endX, y, ' ', attr)
	}
}

// applyCellHighlight repaints the cells covered by the matched substring with
// the highlight color, overriding whatever state/selection attributes were
// resolved before. span.start/span.end are rune indices in the original cell
// text; they are mapped to display cells accounting for truncation, alignment
// padding and wide runes.
func (t *Table) applyCellHighlight(cis []CharInfo, text string, width int, align Alignment, span cellHighlight) {
	truncated := TruncateString(text, width, "")
	space := width - StringWidth(truncated)
	if space < 0 {
		space = 0
	}
	padLeft := 0
	switch align {
	case AlignRight:
		padLeft = space
	case AlignCenter:
		padLeft = space / 2
	}
	cellPos := padLeft
	runeIdx := 0
	for _, r := range truncated {
		w := ClusterWidth(string(r))
		if w < 1 {
			runeIdx++
			continue // zero-width runes occupy no display cell
		}
		if runeIdx >= span.start && runeIdx <= span.end {
			for k := 0; k < w && cellPos+k < len(cis); k++ {
				cis[cellPos+k].Attributes = Palette[t.ColorHighlightIdx]
			}
		}
		cellPos += w
		runeIdx++
	}
}

// formatSortedHeader renders a column header with a sort direction arrow
// (↑/↓) at the right edge of the cell, reserving space for it so the title
// itself is truncated first.
func (t *Table) formatSortedHeader(title string, width int, align Alignment) string {
	arrow := " ↓"
	if t.SortAscending {
		arrow = " ↑"
	}
	arrowWidth := StringWidth(arrow)
	if width <= arrowWidth {
		return TruncateString(arrow, width, "")
	}
	return t.formatCell(title, width-arrowWidth, align) + arrow
}

func (t *Table) formatCell(text string, width int, align Alignment) string {
	var vLen int
	text, vLen = truncateStringWidth(text, width, "")
	if vLen >= width {
		return text
	}

	space := width - vLen
	switch align {
	case AlignLeft:
		return text + strings.Repeat(" ", space)
	case AlignRight:
		return strings.Repeat(" ", space) + text
	case AlignCenter:
		left := space / 2
		right := space - left
		return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
	}
	return text
}

func (t *Table) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || t.IsDisabled() {
		return false
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		if t.SelectPos == 0 {
			return false
		}
	case vtinput.VK_DOWN:
		if t.SelectPos == t.ItemCount-1 {
			return false
		}
	}

	if t.QuickSearch && t.processSearchKey(e) {
		return true
	}

	if t.CellSelection {
		switch e.VirtualKeyCode {
		case vtinput.VK_LEFT:
			if t.SelectCol > 0 {
				t.SelectCol--
				return true
			}
			if t.SelectPos == 0 {
				return false
			}
			if t.MoveSelection(-1) {
				t.SelectCol = len(t.Columns) - 1
				return true
			}
		case vtinput.VK_RIGHT:
			if t.SelectCol < len(t.Columns)-1 {
				t.SelectCol++
				return true
			}
			if t.SelectPos == t.ItemCount-1 {
				return false
			}
			if t.MoveSelection(1) {
				t.SelectCol = 0
				return true
			}
		}
	}

	return t.HandleKey(e)
}

// processSearchKey handles QuickSearch editing keys. It returns true if the
// event was consumed. Up/Down and all other navigation keys fall through to
// the default table handling; with CellSelection enabled it wins plain
// Left/Right, and the search cursor moves with Ctrl+Left/Right instead.
func (t *Table) processSearchKey(e *vtinput.InputEvent) bool {
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0

	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE:
		if len(t.searchRunes) > 0 {
			t.ClearSearch()
			return true
		}
		return false
	case vtinput.VK_BACK:
		if t.searchCursor > 0 {
			t.searchRunes = append(t.searchRunes[:t.searchCursor-1], t.searchRunes[t.searchCursor:]...)
			t.searchCursor--
			t.resort()
			t.EnsureVisible()
			t.fireSearchChange()
		}
		return true
	case vtinput.VK_DELETE:
		if t.searchCursor < len(t.searchRunes) {
			t.searchRunes = append(t.searchRunes[:t.searchCursor], t.searchRunes[t.searchCursor+1:]...)
			t.resort()
			t.EnsureVisible()
			t.fireSearchChange()
		}
		return true
	case vtinput.VK_LEFT:
		if ctrl || !t.CellSelection {
			if t.searchCursor > 0 {
				t.searchCursor--
			}
			return true
		}
		return false
	case vtinput.VK_RIGHT:
		if ctrl || !t.CellSelection {
			if t.searchCursor < len(t.searchRunes) {
				t.searchCursor++
			}
			return true
		}
		return false
	case vtinput.VK_HOME:
		if ctrl {
			t.searchCursor = 0
			return true
		}
		return false
	case vtinput.VK_END:
		if ctrl {
			t.searchCursor = len(t.searchRunes)
			return true
		}
		return false
	}

	// Printable characters are appended at the cursor position.
	if e.Char >= ' ' && !ctrl && !alt {
		t.searchRunes = append(t.searchRunes, 0)
		copy(t.searchRunes[t.searchCursor+1:], t.searchRunes[t.searchCursor:])
		t.searchRunes[t.searchCursor] = e.Char
		t.searchCursor++
		t.resort()
		t.EnsureVisible()
		t.fireSearchChange()
		return true
	}
	return false
}

func (t *Table) ProcessMouse(e *vtinput.InputEvent) bool {
	if t.IsDisabled() {
		return false
	}

	// Pre-process for CellSelection before generic HandleMouse
	originalCol := t.SelectCol
	colChanged := false

	if e.Type == vtinput.MouseEventType && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		// Click on a column header toggles sorting (only when Sortable).
		// Clicks on separator cells are consumed but do not change the sort.
		if t.Sortable && t.ShowHeader && int(e.MouseY) == t.Y1 &&
			int(e.MouseX) >= t.X1 && int(e.MouseX) <= t.X2 {
			widths := t.resolvedWidths()
			currX := t.X1
			for i := range t.Columns {
				if int(e.MouseX) >= currX && int(e.MouseX) < currX+widths[i] {
					if t.SortColumn == i {
						t.SortAscending = !t.SortAscending
					} else {
						t.SortColumn = i
						t.SortAscending = true
					}
					t.resort()
					break
				}
				currX += widths[i]
				if i < len(t.Columns)-1 {
					currX++
				}
			}
			return true
		}

		if t.CellSelection && t.HitTest(int(e.MouseX), int(e.MouseY)) {
			widths := t.resolvedWidths()
			currX := t.X1
			for i := range t.Columns {
				if int(e.MouseX) >= currX && int(e.MouseX) < currX+widths[i] {
					if t.SelectCol != i {
						t.SelectCol = i
						colChanged = true
					}
					break
				}
				currX += widths[i]
				if i < len(t.Columns)-1 {
					currX++
				}
			}
		}
	}

	handled := t.HandleMouse(e)
	if !handled && colChanged {
		t.SelectCol = originalCol
	}
	return handled
}

func (t *Table) SetPosition(x1, y1, x2, y2 int) {
	t.MarginTop = map[bool]int{true: 1, false: 0}[t.ShowHeader] + map[bool]int{true: 1, false: 0}[t.QuickSearch]
	t.MarginBottom = 0
	t.ScrollView.SetPosition(x1, y1, x2, y2)
}

// drawSearchLine renders the QuickSearch string in the top line of the
// table, above the header: "> text" with a hardware cursor while the table
// is focused.
func (t *Table) drawSearchLine(scr *ScreenBuf) {
	y := t.Y1
	attr := Palette[t.ColorTextIdx]
	scr.FillRect(t.X1, y, t.X2, y, ' ', attr)

	fullWidth := t.X2 - t.X1 + 1
	visibleWidth := fullWidth - 2 // "> " prefix
	if visibleWidth < 0 {
		visibleWidth = 0
	}

	// Horizontal scroll: keep the cursor visible (rune-width aware).
	if t.searchCursor < t.searchLeft {
		t.searchLeft = t.searchCursor
	}
	for t.searchLeft < t.searchCursor && StringWidth(string(t.searchRunes[t.searchLeft:t.searchCursor])) >= visibleWidth {
		t.searchLeft++
	}

	scr.Write(t.X1, y, StringToCharInfo("> ", attr))
	text := string(t.searchRunes[t.searchLeft:])
	text = TruncateString(text, visibleWidth, "")
	scr.Write(t.X1+2, y, StringToCharInfo(text, attr))

	if t.IsFocused() {
		scr.SetCursorVisible(true)
		scr.SetCursorShape(CursorShapeUnderline)
		cursorX := t.X1 + 2 + StringWidth(string(t.searchRunes[t.searchLeft:t.searchCursor]))
		if cursorX > t.X2 {
			cursorX = t.X2
		}
		scr.SetCursorPos(cursorX, y)
	}
}
