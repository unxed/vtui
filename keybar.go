package vtui

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
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

// LatchModifiers records the state reported by a modifier key's own press or
// release event.
//
// Such an event is the only proof we ever get that a modifier is physically
// held down: it arrives when Shift goes down and it arrives again when Shift
// comes back up, so a row switched on here is guaranteed to be switched off
// again. Left and right variations of the same modifier (e.g. Left/Right Ctrl)
// are treated as equivalent (we do not support or require independent
// left/right states).
func (kb *KeyBar) LatchModifiers(shift, ctrl, alt bool) {
	kb.shiftState = shift
	kb.ctrlState = ctrl
	kb.altState = alt
}

// SetModifiers folds the modifier flags carried by an ordinary event into the
// bar. It can only clear a modifier, never light one up.
//
// A plain terminal has no key release reporting at all: Shift+F1 arrives as a
// single F1 keypress with the Shift bit set, and nothing whatsoever follows
// when the user lets Shift go. Lighting the Shift row from that bit left the
// bar on a row that is mostly empty -- and empty slots are drawn as filled
// blocks, so it reads as a band of greyed out keys. It stayed that way until
// some unrelated keystroke happened along. When the chord itself had nothing
// visible to show for it (a command that declines to run and opens no dialog),
// that stuck row was the only thing that changed on screen, which looked
// exactly like an invisible window opening over the panels: f4 issue #983.
//
// Clearing stays honoured, because the flags of an ordinary event are reliable
// about what is *not* held. That is what lets go of a modifier whose release
// was swallowed by a focus change, and what lets a key remapping rule retire
// the row belonging to the chord it rewrote.
func (kb *KeyBar) SetModifiers(shift, ctrl, alt bool) {
	kb.shiftState = kb.shiftState && shift
	kb.ctrlState = kb.ctrlState && ctrl
	kb.altState = kb.altState && alt
}

func (kb *KeyBar) Show(scr *ScreenBuf) {
	kb.Bar.Show(scr)
	kb.DisplayObject(scr)
	_, cy := scr.GetCursorPos()
	if cy == kb.Y1 {
		scr.SetCursorVisible(false)
	}
}
func (kb *KeyBar) ProcessMouse(e *vtinput.InputEvent) bool {
	if !kb.IsVisible() || e.Type != vtinput.MouseEventType {
		return false
	}
	if e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx := int(e.MouseX)
		if kb.HitTest(mx, int(e.MouseY)) {
			width := kb.X2 - kb.X1 + 1
			slotWidth := width / 12
			if slotWidth < 3 {
				slotWidth = 3
			}

			slot := (mx - kb.X1) / slotWidth
			if slot > 11 {
				slot = 11
			}
			if slot >= 0 {
				vk := uint16(vtinput.VK_F1 + slot)
				// Синтезируем событие нажатия клавиши с учетом текущих модификаторов (Shift/Ctrl/Alt)
				ev := &vtinput.InputEvent{
					Type:            vtinput.KeyEventType,
					KeyDown:         true,
					VirtualKeyCode:  vk,
					ControlKeyState: e.ControlKeyState,
				}
				FrameManager.InjectEvents([]*vtinput.InputEvent{ev})
				return true
			}
		}
	}
	return false
}

func (kb *KeyBar) DisplayObject(scr *ScreenBuf) {
	if !kb.IsVisible() {
		return
	}

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
	if slotWidth < 3 {
		slotWidth = 3
	}

	numAttr := Palette[ColKeyBarNum]
	textAttr := Palette[ColKeyBarText]

	// Pre-fill background with the color used for gaps/numbers
	kb.DrawBackground(scr, numAttr)

	for i := 0; i < 12; i++ {
		x := kb.X1 + (i * slotWidth)
		if x > kb.X2 {
			break
		}

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
