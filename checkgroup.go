package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
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

// GetData returns bitmask of states (TV style)
func (cg *CheckGroup) GetData() uint32 {
	var mask uint32
	for i, s := range cg.States {
		if s { mask |= (1 << i) }
	}
	return mask
}

func (cg *CheckGroup) SetData(mask uint32) {
	for i := range cg.States {
		cg.States[i] = (mask & (1 << i)) != 0
	}
}

func NewCheckGroup(x, y, cols int, items []string) *CheckGroup {
	cg := &CheckGroup{Items: items, States: make([]bool, len(items))}
	cg.canFocus = true
	if cols < 1 { cols = 1 }
	cg.Columns = cols
	
	rows := (len(items) + cols - 1) / cols
	cg.colWidths = make([]int, cols)
	
	for i, itm := range items {
		c := i % cols
		clean, _, _ := ParseAmpersandString(itm)
		w := 6 + runewidth.StringWidth(clean) // 4 for prefix + 2 padding
		if w > cg.colWidths[c] { cg.colWidths[c] = w }
	}
	
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

		prefix := "[ ] "
		if cg.States[i] { prefix = "[x] " }

		row := i / cg.Columns
		col := i % cg.Columns
		cx := cg.X1
		for c := 0; c < col; c++ { cx += cg.colWidths[c] }

		cells, _ := StringToCharInfoHighlighted(prefix+itm, curAttr, curHigh)
		scr.Write(cx, cg.Y1+row, cells)
	}
}

func (cg *CheckGroup) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}

	newIdx, moved := gridNav(cg.focusIdx, len(cg.Items), cg.Columns, e.VirtualKeyCode)
	if moved {
		cg.focusIdx = newIdx
		return true
	}

	// Boundary navigation logic: only exit the group if at the absolute start/end
	if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN ||
		e.VirtualKeyCode == vtinput.VK_LEFT || e.VirtualKeyCode == vtinput.VK_RIGHT {
		movingBack := e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_LEFT
		movingForward := e.VirtualKeyCode == vtinput.VK_DOWN || e.VirtualKeyCode == vtinput.VK_RIGHT

		if movingBack && cg.focusIdx == 0 {
			return false // Exit to previous control
		}
		if movingForward && cg.focusIdx == len(cg.Items)-1 {
			return false // Exit to next control
		}
		return true // Stay in group (swallow the key)
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_SPACE, vtinput.VK_RETURN:
		cg.States[cg.focusIdx] = !cg.States[cg.focusIdx]
		return true
	}

	if e.Char != 0 {
		hkChar := unicode.ToLower(e.Char)
		for i, itm := range cg.Items {
			_, hk, _ := ParseAmpersandString(itm)
			if hk == hkChar {
				cg.focusIdx = i
				cg.States[i] = !cg.States[i]
				return true
			}
		}
	}

	return false
}

func (cg *CheckGroup) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		my, mx := int(e.MouseY), int(e.MouseX)
		if my >= cg.Y1 && my <= cg.Y2 && mx >= cg.X1 && mx <= cg.X2 {
			row := my - cg.Y1
			col := 0
			cx := cg.X1
			for c := 0; c < cg.Columns; c++ {
				if mx >= cx && mx < cx+cg.colWidths[c] {
					col = c
					break
				}
				cx += cg.colWidths[c]
			}
			idx := row*cg.Columns + col
			if idx >= 0 && idx < len(cg.Items) {
				cg.focusIdx = idx
				cg.States[idx] = !cg.States[idx]
				return true
			}
		}
	}
	return false
}