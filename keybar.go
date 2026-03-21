package vtui

import (
	"fmt"
	"github.com/mattn/go-runewidth"
)

// KeyBarLabels stores labels for F1-F12 for a specific modifier state.
type KeyBarLabels [12]string
// KeySet represents a full collection of KeyBar labels for all modifier states.
type KeySet struct {
	Normal KeyBarLabels
	Shift  KeyBarLabels
	Ctrl   KeyBarLabels
	Alt    KeyBarLabels
}

// KeyBar implements the bottom row of function key hints.
type KeyBar struct {
	ScreenObject
	Normal KeyBarLabels
	Shift  KeyBarLabels
	Ctrl   KeyBarLabels
	Alt    KeyBarLabels

	shiftState bool
	ctrlState  bool
	altState   bool
}

func NewKeyBar() *KeyBar {
	kb := &KeyBar{}
	return kb
}

func (kb *KeyBar) SetModifiers(shift, ctrl, alt bool) {
	if kb.shiftState != shift || kb.ctrlState != ctrl || kb.altState != alt {
		kb.shiftState = shift
		kb.ctrlState = ctrl
		kb.altState = alt
	}
}

func (kb *KeyBar) Show(scr *ScreenBuf) {
	kb.ScreenObject.Show(scr)
	kb.DisplayObject(scr)
}

func (kb *KeyBar) DisplayObject(scr *ScreenBuf) {
	if !kb.IsVisible() { return }

	labels := kb.Normal
	if kb.shiftState {
		labels = kb.Shift
	} else if kb.ctrlState {
		labels = kb.Ctrl
	} else if kb.altState {
		labels = kb.Alt
	}

	width := kb.X2 - kb.X1 + 1
	slotWidth := width / 12
	if slotWidth < 3 { slotWidth = 3 }

	numAttr := Palette[ColKeyBarNum]
	textAttr := Palette[ColKeyBarText]

	for i := 0; i < 12; i++ {
		x := kb.X1 + (i * slotWidth)
		if x > kb.X2 { break }

		// 1. Draw number
		numStr := fmt.Sprintf("%d", i+1)
		numW := runewidth.StringWidth(numStr)
		scr.Write(x, kb.Y1, StringToCharInfo(numStr, numAttr))

		// 2. Draw label block (occupies slot minus gap)
		labelX := x + numW
		labelW := slotWidth - numW - 1
		if i == 11 {
			labelW = (kb.X2 - labelX) + 1
		}

		if labelW > 0 {
			label := labels[i]
			label = runewidth.Truncate(label, labelW, "")
			for runewidth.StringWidth(label) < labelW {
				label += " "
			}
			scr.Write(labelX, kb.Y1, StringToCharInfo(label, textAttr))
		}

		// 3. Gap (using number's background color)
		if i < 11 {
			scr.Write(x+slotWidth-1, kb.Y1, StringToCharInfo(" ", numAttr))
		}
	}
}
