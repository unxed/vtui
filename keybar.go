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
	Bar
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
	kb.Bar.Show(scr)
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

	// Double check: if all labels are empty, maybe we shouldn't show anything?
	// But in Far, numbers 1..12 are always visible.

	width := kb.X2 - kb.X1 + 1
	slotWidth := width / 12
	if slotWidth < 3 { slotWidth = 3 }

	numAttr := Palette[ColKeyBarNum]
	textAttr := Palette[ColKeyBarText]

	// Pre-fill background with the color used for gaps/numbers
	kb.DrawBackground(scr, numAttr)

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

			// If label is not empty, use KeyBarText color
			if label != "" {

				finalAttr := textAttr
				// Just a placeholder check: if the KeyBar is used to emit a command,
				// we should check it. For this generic widget, we'll keep it simple:
				// if a command is disabled, we dim it.

				// Ensure fixed width for the label part by padding it
				for runewidth.StringWidth(label) < labelW {
					label += " "
				}
				scr.Write(labelX, kb.Y1, StringToCharInfo(label, finalAttr))
			} else {
				// For empty labels, just fill the area with the background color
				scr.FillRect(labelX, kb.Y1, labelX+labelW-1, kb.Y1, ' ', textAttr)
			}
		}
		// 3. Gap is naturally provided by DrawBackground
	}
}
