package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
	"strings"
)


// TableColumn defines the properties of a single table column.
type TableColumn struct {
	Title     string
	Width     int // Width in characters
	Alignment Alignment
}

// TableRow is an interface for data providers.
type TableRow interface {
	GetCellText(col int) string
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
	Columns        []TableColumn
	Rows           []TableRow

	SelectCol      int
	CellSelection  bool
	ShowHeader     bool
	ShowSeparators bool
	AlwaysShowCursor bool

	ColorTextIdx             int
	ColorSelectedTextIdx     int
	ColorItemSelectTextIdx   int
	ColorItemSelectCursorIdx int
	ColorTitleIdx            int
	ColorBoxIdx              int
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
	}
	t.canFocus = true
	t.InitScrollBar(t)
	t.SetPosition(x, y, x+w-1, y+h-1)
	return t
}

func (t *Table) SetRows(rows []TableRow) {
	t.Rows = rows
	t.ItemCount = len(rows)
	if t.ItemCount == 0 {
		t.SelectPos = 0
	} else if t.SelectPos >= t.ItemCount {
		t.SelectPos = t.ItemCount - 1
	} else if t.SelectPos < 0 {
		t.SelectPos = 0
	}
	t.EnsureVisible()
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
	height := t.Y2 - t.Y1 + 1

	// 1. Draw Header
	if t.ShowHeader {
		t.drawRow(scr, t.Y1, -1, Palette[t.ColorTitleIdx])
		yOffset++
	}

	// 2. Draw Data Rows
	dataHeight := height - yOffset
	if dataHeight < 0 {
		dataHeight = 0
	}
	for i := 0; i < dataHeight; i++ {
		rowIdx := t.TopPos + i
		currY := t.Y1 + yOffset + i

		if rowIdx < len(t.Rows) {
			//isSelected := false
			// Calculate standard attribute as a fallback (passed into drawRow)
			attr := Palette[t.ColorTextIdx]
			t.drawRow(scr, currY, rowIdx, attr)
		} else {
			// Fill empty space with background color
			scr.FillRect(t.X1, currY, t.X2, currY, ' ', Palette[t.ColorTextIdx])
		}
	}

	// 3. Draw Vertical Separators if needed
	if t.ShowSeparators {
		p := NewPainter(scr)
		currX := t.X1
		sepChar := boxSymbols[bsV] // │
		for i := 0; i < len(t.Columns)-1; i++ {
			currX += t.Columns[i].Width
			p.Fill(currX, t.Y1, currX, t.Y2, sepChar, Palette[t.ColorBoxIdx])
			currX++
		}
	}

	// 4. Draw Scrollbar
	t.DrawScrollBar(scr)
}


func (t *Table) drawRow(scr *ScreenBuf, y int, rowIdx int, attr uint64) {
	endX := t.X1 + t.GetContentWidth() - 1

	currX := t.X1
	for colIdx, col := range t.Columns {
		text := ""
		if rowIdx == -1 {
			text = col.Title
		} else {
			text = t.Rows[rowIdx].GetCellText(colIdx)
		}

		isSelected := false
		if rowIdx != -1 && rowIdx < len(t.Rows) {
			if mcsr, ok := t.Rows[rowIdx].(MultiColSelectableRow); ok {
				isSelected = mcsr.IsColSelected(colIdx)
			} else if selRow, ok := t.Rows[rowIdx].(SelectableRow); ok {
				isSelected = selRow.IsSelected()
			}
		}

		isCursorHere := rowIdx == t.SelectPos && (!t.CellSelection || colIdx == t.SelectCol)

		cellAttr := attr
		if rowIdx != -1 {
			baseAttr := Palette[t.ColorTextIdx]
			if cr, ok := t.Rows[rowIdx].(CellColorableRow); ok {
				baseAttr = cr.GetCellAttr(colIdx, baseAttr)
			}
			cellAttr = baseAttr

			if isSelected {
				cellAttr = Palette[t.ColorItemSelectTextIdx]
			}
			if isCursorHere {
				if t.IsFocused() {
					if isSelected {
						cellAttr = Palette[t.ColorItemSelectCursorIdx]
					} else {
						cursorAttr := Palette[t.ColorSelectedTextIdx]
						standardAttr := Palette[t.ColorTextIdx]
						if baseAttr != standardAttr {
							if baseAttr&IsFgRGB != 0 {
								cellAttr = SetRGBFore(cursorAttr, GetRGBFore(baseAttr))
							} else {
								cellAttr = SetIndexFore(cursorAttr, GetIndexFore(baseAttr))
							}
							const styleMask = ForegroundIntensity | ForegroundDim | CommonLvbUnderscore | CommonLvbStrikeout
							cellAttr |= (baseAttr & styleMask)
						} else {
							cellAttr = cursorAttr
						}
					}
				} else {
					if isSelected {
						cellAttr = Palette[t.ColorItemSelectTextIdx]
					} else {
						if t.AlwaysShowCursor {
							cursorAttr := Palette[t.ColorSelectedTextIdx]
							standardAttr := Palette[t.ColorTextIdx]
							if baseAttr != standardAttr {
								if baseAttr&IsFgRGB != 0 {
									cellAttr = SetRGBFore(cursorAttr, GetRGBFore(baseAttr))
								} else {
									cellAttr = SetIndexFore(cursorAttr, GetIndexFore(baseAttr))
								}
								const styleMask = ForegroundIntensity | ForegroundDim | CommonLvbUnderscore | CommonLvbStrikeout
								cellAttr |= (baseAttr & styleMask)
							} else {
								cellAttr = cursorAttr
							}
						} else {
							cellAttr = baseAttr
						}
					}
				}
			}
		}

		// Prepare cell text with alignment
		cellText := t.formatCell(text, col.Width, col.Alignment)
		scr.Write(currX, y, StringToCharInfo(cellText, cellAttr))
		currX += col.Width

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

func (t *Table) formatCell(text string, width int, align Alignment) string {
	text = runewidth.Truncate(text, width, "")
	vLen := runewidth.StringWidth(text)
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
	if !e.KeyDown || t.IsDisabled() { return false }

	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		if t.SelectPos == 0 { return false }
	case vtinput.VK_DOWN:
		if t.SelectPos == t.ItemCount-1 { return false }
	}

	if t.CellSelection {
		switch e.VirtualKeyCode {
		case vtinput.VK_LEFT:
			if t.SelectCol > 0 { t.SelectCol--; return true }
			if t.SelectPos == 0 { return false }
			if t.MoveRelative(-1) { t.SelectCol = len(t.Columns) - 1; return true }
		case vtinput.VK_RIGHT:
			if t.SelectCol < len(t.Columns)-1 { t.SelectCol++; return true }
			if t.SelectPos == t.ItemCount-1 { return false }
			if t.MoveRelative(1) { t.SelectCol = 0; return true }
		}
	}

	return t.HandleKey(e)
}

func (t *Table) ProcessMouse(e *vtinput.InputEvent) bool {
	if t.IsDisabled() { return false }
	
	// Pre-process for CellSelection before generic HandleMouse
	if e.Type == vtinput.MouseEventType && e.ButtonState != 0 && e.KeyDown {
		if t.CellSelection && t.HitTest(int(e.MouseX), int(e.MouseY)) {
			currX := t.X1
			for i, col := range t.Columns {
				if int(e.MouseX) >= currX && int(e.MouseX) < currX+col.Width {
					t.SelectCol = i
					break
				}
				currX += col.Width
				if i < len(t.Columns)-1 { currX++ }
			}
		}
	}
	
	return t.HandleMouse(e)
}

func (t *Table) SetPosition(x1, y1, x2, y2 int) {
	t.MarginTop = map[bool]int{true: 1, false: 0}[t.ShowHeader]
	t.ScrollView.SetPosition(x1, y1, x2, y2)
}
