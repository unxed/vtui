package vtui

import (
	"github.com/unxed/vtinput"
	"github.com/mattn/go-runewidth"
)

// ListBox represents a list of strings for selection within a dialog.
type ListBox struct {
	ScreenObject
	Items      []string
	SelectPos  int
	TopPos     int
	OnChange   func(int)
	OnAction   func(int)

	ColorTextIdx         int
	ColorSelectedTextIdx int
}

func NewListBox(x, y, w, h int, items []string) *ListBox {
	lb := &ListBox{
		Items:                items,
		ColorTextIdx:         ColTableText,
		ColorSelectedTextIdx: ColTableSelectedText,
	}
	lb.canFocus = true
	lb.SetPosition(x, y, x+w-1, y+h-1)
	return lb
}

func (lb *ListBox) Show(scr *ScreenBuf) {
	lb.ScreenObject.Show(scr)
	lb.DisplayObject(scr)
}

func (lb *ListBox) DisplayObject(scr *ScreenBuf) {
	if !lb.IsVisible() { return }

	width := lb.X2 - lb.X1 + 1
	height := lb.Y2 - lb.Y1 + 1

	colText := Palette[lb.ColorTextIdx]
	colSel := Palette[lb.ColorSelectedTextIdx]
	colBox := Palette[ColTableBox]

	// 1. Elements rendering
	for i := 0; i < height; i++ {
		idx := lb.TopPos + i
		currY := lb.Y1 + i

		attr := colText
		if idx == lb.SelectPos && lb.IsFocused() {
			attr = colSel
		}

		if idx < len(lb.Items) {
			text := lb.Items[idx]
			text = runewidth.Truncate(text, width, "")
			vLen := runewidth.StringWidth(text)

			// Write text and fill the rest of the line with spaces
			scr.Write(lb.X1, currY, StringToCharInfo(text, attr))
			if vLen < width {
				scr.FillRect(lb.X1+vLen, currY, lb.X2, currY, ' ', attr)
			}
		} else {
			// Empty lines at the end
			scr.FillRect(lb.X1, currY, lb.X2, currY, ' ', colText)
		}
	}

	// 2. Scrollbar
	if len(lb.Items) > height {
		DrawScrollBar(scr, lb.X2, lb.Y1, height, lb.TopPos, len(lb.Items), colBox)
	}
}

func (lb *ListBox) ProcessKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }

	height := lb.Y2 - lb.Y1 + 1
	oldPos := lb.SelectPos

	switch e.VirtualKeyCode {
	case vtinput.VK_UP:
		if lb.SelectPos > 0 { lb.SelectPos-- }
	case vtinput.VK_RETURN:
		if lb.OnAction != nil {
			lb.OnAction(lb.SelectPos)
		}
		return true
	case vtinput.VK_DOWN:
		if lb.SelectPos < len(lb.Items)-1 { lb.SelectPos++ }
	case vtinput.VK_PRIOR: // PgUp
		lb.SelectPos -= height
		if lb.SelectPos < 0 { lb.SelectPos = 0 }
	case vtinput.VK_NEXT: // PgDn
		lb.SelectPos += height
		if lb.SelectPos >= len(lb.Items) { lb.SelectPos = len(lb.Items) - 1 }
	case vtinput.VK_HOME:
		lb.SelectPos = 0
	case vtinput.VK_END:
		lb.SelectPos = len(lb.Items) - 1
	default:
		return false
	}

	if lb.SelectPos != oldPos {
		lb.EnsureVisible()
		if lb.OnChange != nil {
			lb.OnChange(lb.SelectPos)
		}
		return true
	}

	return false
}

func (lb *ListBox) EnsureVisible() {
	height := lb.Y2 - lb.Y1 + 1
	if lb.SelectPos < lb.TopPos {
		lb.TopPos = lb.SelectPos
	} else if lb.SelectPos >= lb.TopPos+height {
		lb.TopPos = lb.SelectPos - height + 1
	}
}

func (lb *ListBox) ProcessMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType { return false }

	mx, my := int(e.MouseX), int(e.MouseY)
	height := lb.Y2 - lb.Y1 + 1

	// Mouse wheel processing
	if e.WheelDirection > 0 {
		if lb.TopPos > 0 { lb.TopPos-- }
		return true
	} else if e.WheelDirection < 0 {
		if lb.TopPos < len(lb.Items)-height { lb.TopPos++ }
		return true
	}

	// Click
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		if mx >= lb.X1 && mx <= lb.X2 && my >= lb.Y1 && my <= lb.Y2 {
			// Check click on the scrollbar
			if len(lb.Items) > height && mx == lb.X2 {
				if my == lb.Y1 {
					lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP})
				} else if my == lb.Y2 {
					lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
				} else {
	// Page-wise scrolling
					if my < lb.Y1 + height/2 {
						lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
					} else {
						lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
					}
				}
				return true
			}

			// Item selection
			clickIdx := lb.TopPos + (my - lb.Y1)
			if clickIdx >= 0 && clickIdx < len(lb.Items) {
				lb.SelectPos = clickIdx
				if lb.OnChange != nil { lb.OnChange(lb.SelectPos) }
				return true
			}
		}
	}
	return false
}