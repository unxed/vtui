package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// ListBox represents a list of strings for selection within a dialog.
type ListBox struct {
	ScreenObject
	ListViewer
	Items    []string
	Command  int
	OnSelect func(int)
	OnAction func(int)

	ColorTextIdx         int
	MultiSelect          bool
	SelectedMap          map[int]bool
	ColorSelectedTextIdx int
	ScrollBar            *ScrollBar
}


func NewListBox(x, y, w, h int, items []string) *ListBox {
	lb := &ListBox{
		Items:                items,
		SelectedMap:          make(map[int]bool),
		ColorTextIdx:         ColTableText,
		ColorSelectedTextIdx: ColTableSelectedText,
	}
	lb.ItemCount = len(items)
	lb.ViewHeight = h
	if lb.ItemCount == 0 {
		lb.SelectPos = 0
	}
	lb.canFocus = true
	lb.SetPosition(x, y, x+w-1, y+h-1)
	lb.ScrollBar = NewScrollBar(x+w-1, y, h)
	lb.ScrollBar.SetOwner(lb)
	lb.ScrollBar.OnScroll = func(v int) {
		lb.TopPos = v
	}
	return lb
}

func (lb *ListBox) GetSelectedIndices() []int {
	var res []int
	for i := range lb.Items {
		if lb.SelectedMap[i] { res = append(res, i) }
	}
	return res
}

func (lb *ListBox) Show(scr *ScreenBuf) {
	lb.ScreenObject.Show(scr)
	lb.DisplayObject(scr)
}

func (lb *ListBox) DisplayObject(scr *ScreenBuf) {
	if !lb.IsVisible() { return }

	width := lb.X2 - lb.X1 + 1
	if len(lb.Items) > lb.ViewHeight { width-- }
	height := lb.Y2 - lb.Y1 + 1

	colText := Palette[lb.ColorTextIdx]
	colSel := Palette[lb.ColorSelectedTextIdx]

	// 1. Elements rendering
	for i := 0; i < height; i++ {
		idx := lb.TopPos + i
		currY := lb.Y1 + i

		attr := colText
		isSelected := lb.SelectedMap[idx]

		if isSelected {
			attr = Palette[ColDialogHighlightText]
		}

		if idx == lb.SelectPos && lb.IsFocused() {
			if isSelected {
				attr = Palette[ColDialogHighlightSelectedButton]
			} else {
				attr = colSel
			}
		}
		if lb.IsDisabled() {
			attr = DimColor(attr)
		}

		if idx < len(lb.Items) {
			text := lb.Items[idx]
			text = runewidth.Truncate(text, width, "")
			vLen := runewidth.StringWidth(text)

			scr.Write(lb.X1, currY, StringToCharInfo(text, attr))
			if vLen < width {
				scr.FillRect(lb.X1+vLen, currY, lb.X1+width-1, currY, ' ', attr)
			}
		} else {
			scr.FillRect(lb.X1, currY, lb.X1+width-1, currY, ' ', colText)
		}
	}

	// 2. Scrollbar
	if len(lb.Items) > height {
		lb.ScrollBar.SetParams(lb.TopPos, 0, len(lb.Items)-height)
		lb.ScrollBar.Show(scr)
	}
}

func (lb *ListBox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || lb.IsDisabled() { return false }

	if e.VirtualKeyCode == vtinput.VK_SPACE || e.VirtualKeyCode == vtinput.VK_INSERT {
		if lb.MultiSelect {
			lb.SelectedMap[lb.SelectPos] = !lb.SelectedMap[lb.SelectPos]
			if e.VirtualKeyCode == vtinput.VK_INSERT && lb.SelectPos < len(lb.Items)-1 {
				lb.SelectPos++
				lb.EnsureVisible()
			}
			return true
		}
	}
	if e.VirtualKeyCode == vtinput.VK_RETURN {
		if lb.OnAction != nil {
			lb.OnAction(lb.SelectPos)
		} else if lb.Command != 0 {
			lb.HandleCommand(lb.Command, lb.SelectPos)
		}
		return true
	}

	if lb.HandleNavKey(e.VirtualKeyCode) {
		if lb.OnSelect != nil {
			lb.OnSelect(lb.SelectPos)
		}
		return true
	}

	return false
}


func (lb *ListBox) ProcessMouse(e *vtinput.InputEvent) bool {
	if lb.IsDisabled() { return false }
	if lb.ScrollBar != nil && lb.ScrollBar.ProcessMouse(e) {
		return true
	}
	if e.WheelDirection != 0 {
		if e.WheelDirection > 0 && lb.TopPos > 0 { lb.TopPos-- }
		if e.WheelDirection < 0 && lb.TopPos < len(lb.Items)-lb.ViewHeight { lb.TopPos++ }
		return true
	}
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		clickIdx := lb.TopPos + (int(e.MouseY) - lb.Y1)
		if clickIdx >= 0 && clickIdx < len(lb.Items) {
			lb.SelectPos = clickIdx
			if lb.OnSelect != nil {
				lb.OnSelect(lb.SelectPos)
			}
			return true
		}
	}
	return false
}