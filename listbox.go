package vtui

import (
	"github.com/unxed/vtinput"
)

// listRow wraps a string for Table compatibility.
type listRow struct {
	lb  *ListBox
	idx int
}

func (r listRow) GetCellText(col int) string { return r.lb.Items[r.idx] }
func (r listRow) IsSelected() bool           { return r.lb.SelectedMap[r.idx] }
func (r listRow) GetCellAttr(_ int, attr uint64) uint64 {
	if r.lb.hovered && r.lb.hoverIdx == r.idx && !r.lb.IsFocused() && !r.lb.IsDisabled() && roundedButtonsActive() {
		return roundedListHoverAttr(attr)
	}
	return attr
}

// ListBox is a single-column Table for simple string selection.
type ListBox struct {
	Table
	Items       []string
	SelectedMap map[int]bool
	MultiSelect bool
	OnKeyDown   func(e *vtinput.InputEvent) bool
	hovered     bool
	hoverIdx    int
}

func NewListBox(x, y, w, h int, items []string) *ListBox {
	lb := &ListBox{
		Table:       *NewTable(x, y, w, h, []TableColumn{{Width: w}}),
		Items:       items,
		SelectedMap: make(map[int]bool),
		hoverIdx:    -1,
	}
	lb.ShowHeader = false
	lb.ShowSeparators = false
	lb.canFocus = true
	lb.UpdateRows()
	return lb
}

func (lb *ListBox) UpdateRows() {
	rows := make([]TableRow, len(lb.Items))
	for i := range lb.Items {
		rows[i] = listRow{lb: lb, idx: i}
	}
	// Sync MarginTop/ViewHeight before calculating widths
	lb.Table.SetPosition(lb.X1, lb.Y1, lb.X2, lb.Y2)
	lb.SetRows(rows)
	if len(lb.Columns) > 0 {
		lb.Columns[0].Width = lb.GetContentWidth()
	}
}

func (lb *ListBox) GetSelectedIndices() []int {
	var res []int
	for i := range lb.Items {
		if lb.SelectedMap[i] {
			res = append(res, i)
		}
	}
	return res
}

// SelectName searches for an item by its string value and moves the cursor to it.
func (lb *ListBox) SelectName(name string) {
	for i, item := range lb.Items {
		if item == name {
			lb.SetSelectPos(i)
			return
		}
	}
}

func (lb *ListBox) SetRowProvider(p RowProvider) {
	lb.Table.SetRowProvider(p)
	if len(lb.Columns) > 0 {
		lb.Columns[0].Width = lb.GetContentWidth()
	}
}

func (lb *ListBox) SetPosition(x1, y1, x2, y2 int) {
	lb.Table.SetPosition(x1, y1, x2, y2)
	if len(lb.Columns) > 0 {
		lb.Columns[0].Width = lb.GetContentWidth()
	}
}

// SetMouseHovered tracks whether the pointer is over this list. Group calls it
// on entry and exit so a row tint is cleared even when the exit event lands
// outside the list bounds.
func (lb *ListBox) SetMouseHovered(hovered bool) {
	lb.hovered = hovered && !lb.IsDisabled()
	if !lb.hovered {
		lb.hoverIdx = -1
	}
}

func (lb *ListBox) ProcessMouse(e *vtinput.InputEvent) bool {
	oldHoverIdx := lb.hoverIdx
	lb.hoverIdx = -1
	if lb.hovered && e.Type == vtinput.MouseEventType && lb.HitTest(int(e.MouseX), int(e.MouseY)) {
		if displayPos := lb.GetClickIndex(int(e.MouseY)); displayPos >= 0 {
			lb.hoverIdx = lb.rowAt(displayPos)
		}
	}
	handled := lb.Table.ProcessMouse(e)
	return handled || oldHoverIdx != lb.hoverIdx
}

func (lb *ListBox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || lb.IsDisabled() {
		return false
	}
	if !e.TextInput && lb.OnKeyDown != nil && lb.OnKeyDown(e) {
		return true
	}
	e = TextInputView(e)
	switch e.VirtualKeyCode {
	case vtinput.VK_SPACE, vtinput.VK_INSERT:
		if lb.MultiSelect {
			lb.SelectedMap[lb.SelectPos] = !lb.SelectedMap[lb.SelectPos]
			if e.VirtualKeyCode == vtinput.VK_INSERT {
				lb.MoveSelection(1)
			}
			return true
		}
	}
	return lb.Table.ProcessKey(e)
}
