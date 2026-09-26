package vtui

import (
	"github.com/unxed/vtinput"
	"unicode"
)

// RadioGroup is a cluster of radio buttons where only one can be selected.
type RadioGroup struct {
	ScreenObject
	Items     []string
	Selected  int
	focusIdx  int
	OnChange  func(int)
	Columns   int
	colWidths []int
}

func NewRadioGroup(x, y, cols int, items []string) *RadioGroup {
	rg := &RadioGroup{Items: items}
	rg.canFocus = true
	if cols < 1 {
		cols = 1
	}
	rg.Columns = cols

	rows := (len(items) + cols - 1) / cols
	rg.colWidths = calcGridColWidths(cols, items)

	totalW := 0
	for _, w := range rg.colWidths {
		totalW += w
	}

	rg.SetPosition(x, y, x+totalW-1, y+rows-1)
	return rg
}

func (rg *RadioGroup) Show(scr *ScreenBuf) {
	rg.ScreenObject.Show(scr)
	rg.DisplayObject(scr)
}

func (rg *RadioGroup) DisplayObject(scr *ScreenBuf) {
	if !rg.IsVisible() {
		return
	}

	attr := rg.GetStateAttr(ColDialogText, ColDialogText)
	highAttr := rg.GetStateAttr(ColDialogHighlightText, ColDialogHighlightText)
	selAttr := rg.GetStateAttr(ColDialogSelectedButton, ColDialogSelectedButton)
	selHighAttr := rg.GetStateAttr(ColDialogHighlightSelectedButton, ColDialogHighlightSelectedButton)

	p := NewPainter(scr)
	for i, itm := range rg.Items {
		curAttr, curHigh := attr, highAttr
		if rg.IsFocused() && i == rg.focusIdx {
			curAttr, curHigh = selAttr, selHighAttr
		}
		if rg.IsDisabled() {
			curAttr, curHigh = DimColor(curAttr), DimColor(curHigh)
		}

		sym := SymRadioOff
		if i == rg.Selected {
			sym = SymRadioOn
		}

		row := i / rg.Columns
		col := i % rg.Columns
		cx := rg.X1
		for c := 0; c < col; c++ {
			cx += rg.colWidths[c]
		}

		// "( )" (3 cells) plus the padding space that used to be part of the
		// literal "( ) "/"(•) " prefix, then the item text.
		p.DrawSymGlyph(cx, rg.Y1+row, sym, curAttr)
		p.DrawString(cx+3, rg.Y1+row, " ", curAttr)
		p.DrawStringHighlighted(cx+4, rg.Y1+row, itm, curAttr, curHigh)
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
	if rg.IsDisabled() {
		return false
	}

	newIdx, moved := gridNav(rg.focusIdx, len(rg.Items), rg.Columns, e.VirtualKeyCode)
	if moved {
		rg.focusIdx = newIdx
		return true
	}

	if handleGridBoundaryNav(e.VirtualKeyCode, rg.focusIdx, len(rg.Items)) {
		return true
	}

	switch e.VirtualKeyCode {
	case vtinput.VK_SPACE:
		if rg.Selected != rg.focusIdx {
			rg.Selected = rg.focusIdx
			if rg.OnChange != nil {
				rg.OnChange(rg.Selected)
			}
			rg.FireAction(nil, rg.Selected)
		}
		return true
	}

	if e.Char == ' ' {
		if rg.Selected != rg.focusIdx {
			rg.Selected = rg.focusIdx
			if rg.OnChange != nil {
				rg.OnChange(rg.Selected)
			}
			rg.FireAction(nil, rg.Selected)
		}
		return true
	}

	if e.Char != 0 {
		hkChar := unicode.ToLower(e.Char)
		xlatChar := unicode.ToLower(GlobalXlator.Translate(e.Char))
		for i, itm := range rg.Items {
			hk := ExtractHotkey(itm)
			if hk != 0 && (hk == hkChar || hk == xlatChar) {
				rg.focusIdx = i
				rg.Selected = i
				if rg.OnChange != nil {
					rg.OnChange(rg.Selected)
				}
				rg.FireAction(nil, rg.Selected)
				return true
			}
		}
	}

	return false
}

func (rg *RadioGroup) ProcessMouse(e *vtinput.InputEvent) bool {
	if rg.IsDisabled() {
		return false
	}
	if e.Type == vtinput.MouseEventType && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)
		if rg.HitTest(mx, my) {
			idx := getGridIndexFromMouse(rg.X1, rg.Y1, mx, my, rg.Columns, rg.colWidths)
			if idx >= 0 && idx < len(rg.Items) {
				rg.focusIdx = idx
				if rg.Selected != idx {
					rg.Selected = idx
					if rg.OnChange != nil {
						rg.OnChange(rg.Selected)
					}
					rg.FireAction(nil, rg.Selected)
				}
				return true
			}
		}
	}
	return false
}
