package vtui

import (
	"time"

	"github.com/unxed/vtinput"
)

// Symbols for the scrollbar, similar to Oem2Unicode from far2l
const (
	ScrollUpArrow    = '▲' // 0x25B2
	ScrollDownArrow  = '▼' // 0x25BC
	ScrollBlockLight = '░' // 0x2591 (BS_X_B0)
	ScrollBlockDark  = '▓' // 0x2593 (BS_X_B2)
)

// MathRound performs mathematical rounding of x / y
func MathRound(x, y uint64) uint64 {
	if y == 0 {
		return 0
	}
	return (x + (y / 2)) / y
}

// Max returns the maximum of two numbers
func Max(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// Min returns the minimum of two numbers
func Min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// CalcScrollBar calculates the position and size of the scrollbar thumb.
// Returns caretPos (offset from the top arrow, from 0) and caretLength (thumb size).
func CalcScrollBar(length, topItem, itemsCount int) (caretPos, caretLength int) {
	if length <= 2 || itemsCount <= 0 || length >= itemsCount {
		return 0, 0
	}

	trackLen := uint64(length - 2)
	total := uint64(itemsCount)
	viewHeight := uint64(length)
	top := uint64(topItem)

	// Calculate thumb size (proportional to the visible area)
	cLen := MathRound(trackLen*viewHeight, total)
	cLen = Max(1, cLen)
	if cLen >= trackLen && trackLen > 0 {
		cLen = trackLen -1
	}
	cLen = Min(cLen, trackLen)

	// Calculate maximum values for content scroll and the thumb itself
	maxTop := total - viewHeight
	if top > maxTop {
		top = maxTop
	}

	maxCaret := trackLen - cLen
	cPos := uint64(0)
	if maxTop > 0 {
		// Exact proportion guarantees touching the bottom edge at the very end of the list
		cPos = MathRound(top*maxCaret, maxTop)
	}


	return int(cPos), int(cLen)
}

// DrawScrollBar draws a vertical scrollbar.
// x, y - coordinates of the top character (up arrow).
// length - total scrollbar length (including 2 arrows).
// topItem - index of the first visible element.
// itemsCount - total number of elements in the list.
// attr - color attribute for drawing.
func DrawScrollBar(scr *ScreenBuf, x, y, length int, topItem, itemsCount int, attr uint64) bool {
	caretPos, caretLength := CalcScrollBar(length, topItem, itemsCount)
	if caretLength == 0 {
		return false // Scrollbar is not needed
	}

	trackLen := length - 2

	// 1. Top arrow
	scr.Write(x, y, []CharInfo{{Char: uint64(ScrollUpArrow), Attributes: attr}})

	// 2. Track
	for i := 0; i < trackLen; i++ {
		char := ScrollBlockLight
		if i >= caretPos && i < caretPos+caretLength {
			char = ScrollBlockDark
		}
		scr.Write(x, y+1+i, []CharInfo{{Char: uint64(char), Attributes: attr}})
	}

	// 3. Bottom arrow
	scr.Write(x, y+length-1, []CharInfo{{Char: uint64(ScrollDownArrow), Attributes: attr}})

	return true
}

// ScrollBar is a standalone UIElement for scrolling (analogous to TScrollBar).
type ScrollBar struct {
	ScreenObject
	Value    int
	Min, Max int
	PgStep   int
	OnScroll func(int)
	OnStep   func(int)

	isDragging   bool
	dragStartVal int
	dragStartY   int

	repeatTimer  *time.Timer
	repeatAction int // -1, 1, -2, 2
}


func NewScrollBar(x, y, h int) *ScrollBar {
	sb := &ScrollBar{PgStep: h}
	sb.SetPosition(x, y, x, y+h-1)
	return sb
}

func (sb *ScrollBar) SetParams(val, min, max int) {
	sb.Value, sb.Min, sb.Max = val, min, max
}

func (sb *ScrollBar) Show(scr *ScreenBuf) {
	sb.ScreenObject.Show(scr)
	if !sb.IsVisible() { return }
	h := sb.Y2 - sb.Y1 + 1
	if h < 2 || sb.Max <= sb.Min { return }

	attr := Palette[ColTableBox]
	// Using itemsCount calculation: maxTop = total - viewHeight => total = maxTop + viewHeight
	DrawScrollBar(scr, sb.X1, sb.Y1, h, sb.Value, sb.Max+h, attr)
}

func (sb *ScrollBar) ProcessMouse(e *vtinput.InputEvent) bool {
	if !sb.IsVisible() || sb.Max <= sb.Min {
		return false
	}

	mx, my := int(e.MouseX), int(e.MouseY)
	h := sb.Y2 - sb.Y1 + 1

	// 1. Handle Release
	if e.ButtonState == 0 {
		if sb.isDragging {
			sb.isDragging = false
		}
		if sb.repeatTimer != nil {
			sb.repeatTimer.Stop()
			sb.repeatTimer = nil
		}
		return false
	}

	// 2. Handle Active Dragging
	if sb.isDragging {
		trackLen := h - 2
		itemsCount := sb.Max + h
		_, caretLen := CalcScrollBar(h, sb.Value, itemsCount)
		dragRange := trackLen - caretLen
		if dragRange <= 0 { return true }

		dy := my - sb.dragStartY
		itemsPerPixel := float64(sb.Max) / float64(dragRange)

		newValue := sb.dragStartVal + int(float64(dy)*itemsPerPixel)
		sb.scroll(newValue)
		return true
	}

	// 3. Hit-test for initial click
	if mx != sb.X1 || my < sb.Y1 || my > sb.Y2 {
		return false
	}

	if !e.KeyDown {
		return false
	}

	itemsCount := sb.Max + h
	cPos, cLen := CalcScrollBar(h, sb.Value, itemsCount)
	clickRelY := my - (sb.Y1 + 1)

	action := 0
	if my == sb.Y1 {
		action = -1 // Up arrow
	} else if my == sb.Y2 {
		action = 1  // Down arrow
	} else if clickRelY >= cPos && clickRelY < cPos+cLen {
		sb.isDragging = true
		sb.dragStartY = my
		sb.dragStartVal = sb.Value
		return true
	} else {
		if clickRelY < cPos { action = -2 } else { action = 2 } // PgUp/PgDown
	}

	if action != 0 {
		sb.repeatAction = action
		sb.triggerStep()
		// Start auto-repeat
		sb.repeatTimer = time.AfterFunc(400*time.Millisecond, sb.doRepeat)
	}

	return true
}

func (sb *ScrollBar) triggerStep() {
	switch sb.repeatAction {
	case -1:
		if sb.OnStep != nil { sb.OnStep(-1) } else { sb.scroll(sb.Value - 1) }
	case 1:
		if sb.OnStep != nil { sb.OnStep(1) } else { sb.scroll(sb.Value + 1) }
	case -2:
		if sb.OnStep != nil { sb.OnStep(-2) } else { sb.scroll(sb.Value - sb.PgStep) }
	case 2:
		if sb.OnStep != nil { sb.OnStep(2) } else { sb.scroll(sb.Value + sb.PgStep) }
	}
}

func (sb *ScrollBar) doRepeat() {
	FrameManager.PostTask(func() {
		if sb.repeatTimer == nil { return }
		sb.triggerStep()
		sb.repeatTimer = time.AfterFunc(50*time.Millisecond, sb.doRepeat)
	})
}

func (sb *ScrollBar) scroll(v int) {
	if v < sb.Min { v = sb.Min }
	if v > sb.Max { v = sb.Max }
	if v != sb.Value {
		sb.Value = v
		if sb.OnScroll != nil {
			sb.OnScroll(v)
		}
	}
}
