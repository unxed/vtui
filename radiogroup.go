package vtui

import (
	"unicode"
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

func gridNav(idx, count, cols int, vk uint16) (int, bool) {
	if cols < 1 {
		cols = 1
	}
	row := idx / cols
	col := idx % cols
	rows := (count + cols - 1) / cols

	switch vk {
	case vtinput.VK_UP:
		if row > 0 {
			return idx - cols, true
		} else if col > 0 {
			// Snake navigation: move to the bottom of the previous column
			newIdx := (rows-1)*cols + (col - 1)
			for newIdx >= count {
				newIdx -= cols
			}
			return newIdx, true
		}
	case vtinput.VK_DOWN:
		if row < rows-1 && idx+cols < count {
			return idx + cols, true
		} else if col < cols-1 && col+1 < count {
			// Snake navigation: move to the top of the next column
			return col + 1, true
		}
	case vtinput.VK_LEFT:
		if col > 0 {
			return idx - 1, true
		}
	case vtinput.VK_RIGHT:
		if col < cols-1 && idx < count-1 {
			return idx + 1, true
		}
	}
	return idx, false
}

// RadioGroup is a cluster of radio buttons where only one can be selected.
type RadioGroup struct {
	ScreenObject
	Items     []string
	Selected  int
	OnChange  func(int)
	Columns   int
	colWidths []int
}

func NewRadioGroup(x, y, cols int, items []string) *RadioGroup {
	rg := &RadioGroup{Items: items}
	rg.canFocus = true
	if cols < 1 { cols = 1 }
	rg.Columns = cols

	rows := (len(items) + cols - 1) / cols
	rg.colWidths = make([]int, cols)

	for i, itm := range items {
		c := i % cols
		clean, _, _ := ParseAmpersandString(itm)
		w := 6 + runewidth.StringWidth(clean) // 4 for prefix + 2 padding
		if w > rg.colWidths[c] { rg.colWidths[c] = w }
	}

	totalW := 0
	for _, w := range rg.colWidths { totalW += w }

	rg.SetPosition(x, y, x+totalW-1, y+rows-1)
	return rg
}

func (rg *RadioGroup) Show(scr *ScreenBuf) {
	rg.ScreenObject.Show(scr)
	rg.DisplayObject(scr)
}

func (rg *RadioGroup) DisplayObject(scr *ScreenBuf) {
	if !rg.IsVisible() { return }

	attr := Palette[ColDialogText]
	highAttr := Palette[ColDialogHighlightText]
	if rg.IsFocused() {
		attr = Palette[ColDialogSelectedButton]
		highAttr = Palette[ColDialogHighlightSelectedButton]
	}

	for i, itm := range rg.Items {
		prefix := "( ) "
		if i == rg.Selected { prefix = "(•) " }

		row := i / rg.Columns
		col := i % rg.Columns
		cx := rg.X1
		for c := 0; c < col; c++ { cx += rg.colWidths[c] }

		cells, _ := StringToCharInfoHighlighted(prefix+itm, attr, highAttr)
		scr.Write(cx, rg.Y1+row, cells)
	}
}

func (rg *RadioGroup) GetData() any {
	return rg.Selected
}

func (rg *RadioGroup) SetData(val any) {
	if i, ok := val.(int); ok {
		rg.Selected = i
	}
}

func (rg *RadioGroup) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown {
		return false
	}

	newIdx, moved := gridNav(rg.Selected, len(rg.Items), rg.Columns, e.VirtualKeyCode)
	if moved {
		rg.Selected = newIdx
		if rg.OnChange != nil {
			rg.OnChange(rg.Selected)
		}
		return true
	}

	// Boundary navigation logic: only exit the group if at the absolute start/end
	if e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN ||
		e.VirtualKeyCode == vtinput.VK_LEFT || e.VirtualKeyCode == vtinput.VK_RIGHT {
		movingBack := e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_LEFT
		movingForward := e.VirtualKeyCode == vtinput.VK_DOWN || e.VirtualKeyCode == vtinput.VK_RIGHT

		if movingBack && rg.Selected == 0 {
			return false // Exit to previous control
		}
		if movingForward && rg.Selected == len(rg.Items)-1 {
			return false // Exit to next control
		}
		return true // Stay in group (swallow the key)
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_SPACE, vtinput.VK_RETURN:
		return false // Allow dialog to catch it if needed
	}

	if e.Char != 0 {
		{
			for i, itm := range rg.Items {
				_, hk, _ := ParseAmpersandString(itm)
				if hk == unicode.ToLower(e.Char) {
					rg.Selected = i
					if rg.OnChange != nil { rg.OnChange(rg.Selected) }
					return true
				}
			}
		}
	}

	return false
}