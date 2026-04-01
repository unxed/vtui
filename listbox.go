package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// ListBox represents a list of strings for selection within a dialog.
type ListBox struct {
	ScrollView
	Items    []string
	OnSelect func(int)
	OnAction func(int)

	ColorTextIdx         int
	MultiSelect          bool
	SelectedMap          map[int]bool
	ColorSelectedTextIdx int
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
	lb.ShowScrollBar = true
	lb.InitScrollBar(lb)
	lb.SetPosition(x, y, x+w-1, y+h-1)
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

	width := lb.GetContentWidth()
	height := lb.Y2 - lb.Y1 + 1

	// 1. Elements rendering
	for i := 0; i < height; i++ {
		idx := lb.TopPos + i
		currY := lb.Y1 + i

		attr := lb.ResolveColor(lb.ColorTextIdx, lb.ColorSelectedTextIdx)
		isSelected := lb.SelectedMap[idx]

		if isSelected {
			attr = lb.ResolveColor(ColDialogHighlightText, ColDialogHighlightSelectedButton)
		} else if idx == lb.SelectPos && lb.IsFocused() {
			// ResolveColor already handled this via the base/selected indices above
		} else if idx == lb.SelectPos && !lb.IsFocused() {
			// Keep it normal text if not focused
			attr = lb.ResolveColor(lb.ColorTextIdx, lb.ColorTextIdx)
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
			scr.FillRect(lb.X1, currY, lb.X1+width-1, currY, ' ', lb.ResolveColor(lb.ColorTextIdx, lb.ColorTextIdx))
		}
	}

	// 2. Scrollbar
	lb.DrawScrollBar(scr)
}

func (lb *ListBox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown || lb.IsDisabled() { return false }

	switch e.VirtualKeyCode {
	case vtinput.VK_SPACE, vtinput.VK_INSERT:
		if lb.MultiSelect {
			lb.SelectedMap[lb.SelectPos] = !lb.SelectedMap[lb.SelectPos]
			if e.VirtualKeyCode == vtinput.VK_INSERT { lb.MoveRelative(1) }
			return true
		}
	case vtinput.VK_RETURN:
		if lb.OnAction != nil {
			lb.OnAction(lb.SelectPos)
		} else if lb.Command != 0 {
			lb.HandleCommand(lb.Command, lb.SelectPos)
		}
		return true
	}

	if lb.HandleNavKey(e.VirtualKeyCode) {
		if lb.OnSelect != nil { lb.OnSelect(lb.SelectPos) }
		return true
	}

	return false
}


func (lb *ListBox) ProcessMouse(e *vtinput.InputEvent) bool {
	if lb.IsDisabled() || e.Type != vtinput.MouseEventType { return false }
	if lb.HandleMouseScroll(e) { return true }

	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		clickIdx := lb.GetClickIndex(int(e.MouseY))
		if clickIdx != -1 {
			lb.SelectPos = clickIdx
			if lb.OnSelect != nil {
				lb.OnSelect(lb.SelectPos)
			}
			return true
		}
	}
	return false
}