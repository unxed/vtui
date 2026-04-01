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

// calcGridColWidths calculates column widths for grid-based UI groups.
func calcGridColWidths(cols int, items []string) []int {
	widths := make([]int, cols)
	for i, itm := range items {
		c := i % cols
		clean, _, _ := ParseAmpersandString(itm)
		w := 6 + runewidth.StringWidth(clean) // 4 for prefix + 2 padding
		if w > widths[c] {
			widths[c] = w
		}
	}
	return widths
}

// getGridIndexFromMouse maps a mouse click coordinate to a grid index.
func getGridIndexFromMouse(x1, y1, mx, my, columns int, colWidths []int) int {
	row := my - y1
	col := 0
	cx := x1
	for c := 0; c < columns; c++ {
		if mx >= cx && mx < cx+colWidths[c] {
			col = c
			break
		}
		cx += colWidths[c]
	}
	return row*columns + col
}
// handleGridBoundaryNav determines if a navigation key should be swallowed
// or allowed to pass through for exiting the group.
func handleGridBoundaryNav(vk uint16, currentIndex, itemCount int) bool {
	if vk == vtinput.VK_UP || vk == vtinput.VK_DOWN ||
		vk == vtinput.VK_LEFT || vk == vtinput.VK_RIGHT {
		movingBack := vk == vtinput.VK_UP || vk == vtinput.VK_LEFT
		movingForward := vk == vtinput.VK_DOWN || vk == vtinput.VK_RIGHT

		if movingBack && currentIndex == 0 {
			return false // Exit to previous control
		}
		if movingForward && currentIndex == itemCount-1 {
			return false // Exit to next control
		}
		return true // Stay in group (swallow the key)
	}
	return false
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
	rg.colWidths = calcGridColWidths(cols, items)

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

	attr, highAttr := rg.ResolveColors(ColDialogText, ColDialogSelectedButton, ColDialogHighlightText, ColDialogHighlightSelectedButton)

	p := NewPainter(scr)
	for i, itm := range rg.Items {
		prefix := "( ) "
		if i == rg.Selected { prefix = "(•) " }

		row := i / rg.Columns
		col := i % rg.Columns
		cx := rg.X1
		for c := 0; c < col; c++ { cx += rg.colWidths[c] }

		p.DrawStringHighlighted(cx, rg.Y1+row, prefix+itm, attr, highAttr)
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

	if rg.IsDisabled() { return false }

	newIdx, moved := gridNav(rg.Selected, len(rg.Items), rg.Columns, e.VirtualKeyCode)
	if moved {
		rg.Selected = newIdx
		var onClick func()
		if rg.OnChange != nil {
			onClick = func() { rg.OnChange(rg.Selected) }
		}
		rg.FireAction(onClick, rg.Selected)
		return true
	}

	if handleGridBoundaryNav(e.VirtualKeyCode, rg.Selected, len(rg.Items)) {
		return true
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_SPACE, vtinput.VK_RETURN:
		return false // Allow dialog to catch it if needed
	}

	if e.Char != 0 {
		hkChar := unicode.ToLower(e.Char)
		{
			for i, itm := range rg.Items {
				if ExtractHotkey(itm) == hkChar {
					rg.Selected = i
				var onClick func()
				if rg.OnChange != nil {
					onClick = func() { rg.OnChange(rg.Selected) }
				}
				rg.FireAction(onClick, rg.Selected)
					return true
				}
			}
		}
	}

	return false
}

func (rg *RadioGroup) ProcessMouse(e *vtinput.InputEvent) bool {
	if rg.IsDisabled() { return false }
	if e.Type == vtinput.MouseEventType && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)
		if rg.HitTest(mx, my) {
			idx := getGridIndexFromMouse(rg.X1, rg.Y1, mx, my, rg.Columns, rg.colWidths)
			if idx >= 0 && idx < len(rg.Items) {
				if rg.Selected != idx {
					rg.Selected = idx
					var onClick func()
					if rg.OnChange != nil {
						onClick = func() { rg.OnChange(rg.Selected) }
					}
					rg.FireAction(onClick, rg.Selected)
				}
				return true
			}
		}
	}
	return false
}

