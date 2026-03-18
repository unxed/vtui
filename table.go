package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
	"strings"
)

// Alignment defines text alignment within a cell.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignCenter
	AlignRight
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
type Table struct {
	ScreenObject
	Columns      []TableColumn
	Rows         []TableRow
	SelectPos    int
	TopPos       int
	ShowHeader   bool
	ShowSeparators bool

	ColorTextIdx         int
	ColorSelectedTextIdx int
	ColorTitleIdx        int
	ColorBoxIdx          int
}

func NewTable(x, y, w, h int, columns []TableColumn) *Table {
	t := &Table{
		Columns:        columns,
		Rows:           []TableRow{},
		ShowHeader:     true,
		ShowSeparators: true,
		ColorTextIdx:         ColTableText,
		ColorSelectedTextIdx: ColTableSelectedText,
		ColorTitleIdx:        ColTableColumnTitle,
		ColorBoxIdx:          ColTableBox,
	}
	t.canFocus = true
	t.SetPosition(x, y, x+w-1, y+h-1)
	return t
}

func (t *Table) SetRows(rows []TableRow) {
	t.Rows = rows
	DebugLog("  Table: Rows set, count=%d", len(rows))
	if t.SelectPos >= len(rows) {
		t.SelectPos = len(rows) - 1
	}
	if t.SelectPos < 0 && len(rows) > 0 {
		t.SelectPos = 0
	}
}

func (t *Table) Show(scr *ScreenBuf) {
	t.ScreenObject.Show(scr)
	t.DisplayObject(scr)
}

func (t *Table) DisplayObject(scr *ScreenBuf) {
	if !t.IsVisible() { return }

	yOffset := 0
	height := t.Y2 - t.Y1 + 1

	// 1. Draw Header
	if t.ShowHeader {
		t.drawRow(scr, t.Y1, -1, Palette[t.ColorTitleIdx])
		yOffset++
	}

	// 2. Draw Data Rows
	dataHeight := height - yOffset
	for i := 0; i < dataHeight; i++ {
		rowIdx := t.TopPos + i
		currY := t.Y1 + yOffset + i

		if rowIdx < len(t.Rows) {
			attr := Palette[t.ColorTextIdx]
			if rowIdx == t.SelectPos {
				if t.IsFocused() {
					attr = Palette[t.ColorSelectedTextIdx]
				} else {
					attr = Palette[t.ColorTextIdx]
				}
			}
			t.drawRow(scr, currY, rowIdx, attr)
		} else {
			// Fill empty space with background color
			scr.FillRect(t.X1, currY, t.X2, currY, ' ', Palette[t.ColorTextIdx])
		}
	}

	// 3. Draw Vertical Separators if needed
	if t.ShowSeparators {
		t.drawSeparators(scr)
	}

	// 4. Draw Scrollbar
	if len(t.Rows) > dataHeight {
		scrollbarX := t.X2
		scrollbarY := t.Y1 + yOffset
		scrollbarLen := dataHeight
		DrawScrollBar(scr, scrollbarX, scrollbarY, scrollbarLen, t.TopPos, len(t.Rows), Palette[t.ColorBoxIdx])
	}
}

func (t *Table) SetFocus(f bool) {
	DebugLog("  Table: SetFocus(%v)", f)
	t.focused = f
}

func (t *Table) drawRow(scr *ScreenBuf, y int, rowIdx int, attr uint64) {
	currX := t.X1
	for colIdx, col := range t.Columns {
		text := ""
		if rowIdx == -1 {
			text = col.Title
		} else {
			text = t.Rows[rowIdx].GetCellText(colIdx)
		}

		// Prepare cell text with alignment
		cellText := t.formatCell(text, col.Width, col.Alignment)
		scr.Write(currX, y, StringToCharInfo(cellText, attr))
		currX += col.Width

		// Skip separator space if not the last column
		if colIdx < len(t.Columns)-1 {
			currX++
		}
	}

	// Fill remaining horizontal space if any.
	// If scrollbar is drawn, we must not overwrite the rightmost column.
	headerOffset := 0
	if t.ShowHeader { headerOffset = 1 }
	dataHeight := (t.Y2 - t.Y1 + 1) - headerOffset

	endX := t.X2
	if len(t.Rows) > dataHeight {
		endX-- // Leave space for scrollbar
	}
	lastX := currX - 1
	if lastX < endX {
		scr.FillRect(lastX+1, y, endX, y, ' ', attr)
	}
}

func (t *Table) drawSeparators(scr *ScreenBuf) {
	currX := t.X1
	sepChar := boxSymbols[bsV] // │
	for i := 0; i < len(t.Columns)-1; i++ {
		currX += t.Columns[i].Width
		scr.FillRect(currX, t.Y1, currX, t.Y2, sepChar, Palette[t.ColorBoxIdx])
		currX++
	}
}

func (t *Table) formatCell(text string, width int, align Alignment) string {
	// 1. Truncate if visual width exceeds column width
	text = runewidth.Truncate(text, width, "")

	// 2. Calculate visual space remaining
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
	if !e.KeyDown { return false }

	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		if t.SelectPos > 0 {
			t.SelectPos--
			t.EnsureVisible()
			return true
		}
	case vtinput.VK_DOWN:
		if t.SelectPos < len(t.Rows)-1 {
			t.SelectPos++
			t.EnsureVisible()
			return true
		}
	case vtinput.VK_HOME:
		t.SelectPos = 0
		t.EnsureVisible()
		return true
	case vtinput.VK_END:
		t.SelectPos = len(t.Rows) - 1
		t.EnsureVisible()
		return true
	}
	return false
}

func (t *Table) EnsureVisible() {
	headerOffset := 0
	if t.ShowHeader { headerOffset = 1 }
	height := (t.Y2 - t.Y1 + 1) - headerOffset

	if height <= 0 { return }

	if t.SelectPos < t.TopPos {
		t.TopPos = t.SelectPos
	} else if t.SelectPos >= t.TopPos+height {
		t.TopPos = t.SelectPos - height + 1
	}
}

func (t *Table) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType { return false }

	headerOffset := 0
	if t.ShowHeader { headerOffset = 1 }
	dataHeight := (t.Y2 - t.Y1 + 1) - headerOffset

	// Check scrollbar click
	if len(t.Rows) > dataHeight && int(e.MouseX) == t.X2 {
		if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
			my := int(e.MouseY)
			startY := t.Y1 + headerOffset
			if my == startY {
				t.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
				return true
			} else if my == startY+dataHeight-1 {
				t.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
				return true
			} else if my > startY && my < startY+dataHeight-1 {
				if my < startY+dataHeight/2 {
					t.TopPos -= dataHeight
					if t.TopPos < 0 { t.TopPos = 0 }
				} else {
					t.TopPos += dataHeight
					if t.TopPos > len(t.Rows)-dataHeight { t.TopPos = len(t.Rows) - dataHeight }
				}
				return true
			}
		}
	}

	if e.WheelDirection > 0 {
		if t.TopPos > 0 { t.TopPos-- }
		return true
	} else if e.WheelDirection < 0 {
		headerOffset := 0
		if t.ShowHeader { headerOffset = 1 }
		if t.TopPos < len(t.Rows)-((t.Y2-t.Y1+1)-headerOffset) {
			t.TopPos++
		}
		return true
	}

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		my := int(e.MouseY)

		clickIdx := t.TopPos + (my - t.Y1 - headerOffset)
		if clickIdx >= 0 && clickIdx < len(t.Rows) {
			t.SelectPos = clickIdx
			return true
		}
	}
	return false
}