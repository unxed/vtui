package vtui

import (
	"github.com/unxed/vtinput"
	"unicode"
)

// CheckGroup is a cluster of checkboxes managed as a single widget.
type CheckGroup struct {
	ScreenObject
	Items     []string
	States    []bool
	focusIdx  int
	Columns   int
	colWidths []int
}

func (cg *CheckGroup) GetData() any {
	var mask uint32
	for i, s := range cg.States {
		if s {
			mask |= (1 << i)
		}
	}
	return mask
}

func (cg *CheckGroup) SetData(val any) {
	if mask, ok := val.(uint32); ok {
		for i := range cg.States {
			cg.States[i] = (mask & (1 << i)) != 0
		}
	}
}

func NewCheckGroup(x, y, cols int, items []string) *CheckGroup {
	cg := &CheckGroup{Items: items, States: make([]bool, len(items))}
	cg.canFocus = true
	if cols < 1 { cols = 1 }
	cg.Columns = cols
	
	rows := (len(items) + cols - 1) / cols
	cg.colWidths = calcGridColWidths(cols, items)

	totalW := 0
	for _, w := range cg.colWidths { totalW += w }
	
	cg.SetPosition(x, y, x+totalW-1, y+rows-1)
	return cg
}

func (cg *CheckGroup) Show(scr *ScreenBuf) {
	cg.ScreenObject.Show(scr)
	cg.DisplayObject(scr)
}

func (cg *CheckGroup) DisplayObject(scr *ScreenBuf) {
	if !cg.IsVisible() { return }

	attr := Palette[ColDialogText]
	highAttr := Palette[ColDialogHighlightText]
	selAttr := Palette[ColDialogSelectedButton]
	selHighAttr := Palette[ColDialogHighlightSelectedButton]

	for i, itm := range cg.Items {
		curAttr, curHigh := attr, highAttr
		if cg.IsFocused() && i == cg.focusIdx {
			curAttr, curHigh = selAttr, selHighAttr
		}
		if cg.IsDisabled() {
			curAttr, curHigh = DimColor(curAttr), DimColor(curHigh)
		}

		prefix := "[ ] "
		if cg.States[i] { prefix = "[x] " }

		row := i / cg.Columns
		col := i % cg.Columns
		cx := cg.X1
		for c := 0; c < col; c++ { cx += cg.colWidths[c] }

		p := NewPainter(scr)
		p.DrawStringHighlighted(cx, cg.Y1+row, prefix+itm, curAttr, curHigh)
	}
}

func (cg *CheckGroup) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}
	if cg.IsDisabled() { return false }

	newIdx, moved := gridNav(cg.focusIdx, len(cg.Items), cg.Columns, e.VirtualKeyCode)
	if moved {
		cg.focusIdx = newIdx
		return true
	}

	if handleGridBoundaryNav(e.VirtualKeyCode, cg.focusIdx, len(cg.Items)) {
		return true
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_SPACE, vtinput.VK_RETURN:
		cg.States[cg.focusIdx] = !cg.States[cg.focusIdx]
		return true
	}

	if e.Char != 0 {
		hkChar := unicode.ToLower(e.Char)
		for i, itm := range cg.Items {
			if ExtractHotkey(itm) == hkChar {
				cg.focusIdx = i
				cg.States[i] = !cg.States[i]
				return true
			}
		}
	}

	return false
}

func (cg *CheckGroup) ProcessMouse(e *vtinput.InputEvent) bool {
	if cg.IsDisabled() { return false }
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)
		if cg.HitTest(mx, my) {
			idx := getGridIndexFromMouse(cg.X1, cg.Y1, mx, my, cg.Columns, cg.colWidths)
			if idx >= 0 && idx < len(cg.Items) {
				cg.focusIdx = idx
				cg.States[idx] = !cg.States[idx]
				return true
			}
		}
	}
	return false
}