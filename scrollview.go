package vtui

import "github.com/unxed/vtinput"

// ScrollView provides standardized scrolling, positioning, and hit-testing
// for list-based UI elements. It embeds ScreenObject.
type ScrollView struct {
	ScreenObject
	TopPos      int
	SelectPos   int
	ItemCount   int
	ViewHeight  int
	Wrap        bool
	IsSelectable func(int) bool

	ShowScrollBar bool
	ScrollBar     *ScrollBar

	MarginTop    int
	MarginBottom int
	MarginLeft   int
	MarginRight  int

	OnSelect func(int)
	OnAction func(int)
}

// ScrollBy shifts the view and the selection by the same amount, keeping the cursor vertically stable.
// If the view hits a boundary, the remaining scroll delta is applied to the cursor.
func (sv *ScrollView) ScrollBy(delta int) {
	if sv.ItemCount == 0 {
		return
	}

	targetTop := sv.TopPos + delta
	maxTop := sv.ItemCount - sv.ViewHeight
	if maxTop < 0 {
		maxTop = 0
	}
	if targetTop < 0 {
		targetTop = 0
	}
	if targetTop > maxTop {
		targetTop = maxTop
	}

	// Move the cursor by the requested delta to keep it physically stable on the screen,
	// or to push it towards the boundary if the view itself cannot scroll further.
	sv.MoveRelative(delta)

	// Force the view to the calculated target
	sv.TopPos = targetTop

	// Safety check in case MoveRelative jumped out of bounds due to unselectable items
	sv.EnsureVisible()
}

func (sv *ScrollView) InitScrollBar(owner CommandHandler) {
	sv.ScrollBar = NewScrollBar(0, 0, 0)
	sv.ScrollBar.SetOwner(owner)
	sv.ScrollBar.OnScroll = func(v int) {
		delta := v - sv.TopPos
		sv.ScrollBy(delta)
	}
}
func (sv *ScrollView) GetContentWidth() int {
	w := sv.X2 - sv.X1 + 1
	if sv.ShowScrollBar && sv.ItemCount > sv.ViewHeight {
		w--
	}
	return w
}

func (sv *ScrollView) SetPosition(x1, y1, x2, y2 int) {
	sv.ScreenObject.SetPosition(x1, y1, x2, y2)
	sv.ViewHeight = (y2 - y1 + 1) - sv.MarginTop - sv.MarginBottom
	if sv.ViewHeight < 0 {
		sv.ViewHeight = 0
	}
	if sv.ScrollBar != nil {
		sy := y1 + sv.MarginTop
		sv.ScrollBar.SetPosition(x2-sv.MarginRight, sy, x2-sv.MarginRight, sy+sv.ViewHeight-1)
		sv.ScrollBar.PgStep = sv.ViewHeight
	}
}

func (sv *ScrollView) DrawScrollBar(scr *ScreenBuf) {
	if sv.ShowScrollBar && sv.ScrollBar != nil && sv.ItemCount > sv.ViewHeight && sv.ViewHeight > 0 {
		sv.ScrollBar.SetParams(sv.TopPos, 0, sv.ItemCount-sv.ViewHeight)
		sv.ScrollBar.Show(scr)
	}
}

func (sv *ScrollView) HandleMouseScroll(e *vtinput.InputEvent) bool {
	if sv.ShowScrollBar && sv.ScrollBar != nil && sv.ScrollBar.ProcessMouse(e) {
		return true
	}
	if e.WheelDirection != 0 {
		if e.WheelDirection > 0 {
			sv.ScrollBy(-1)
			return true
		} else if e.WheelDirection < 0 {
			sv.ScrollBy(1)
			return true
		}
	}
	return false
}

func (sv *ScrollView) EnsureVisible() {
	if sv.ViewHeight <= 0 { return }
	if sv.SelectPos < sv.TopPos {
		sv.TopPos = sv.SelectPos
	} else if sv.SelectPos >= sv.TopPos+sv.ViewHeight {
		sv.TopPos = sv.SelectPos - sv.ViewHeight + 1
	}
	if sv.TopPos < 0 { sv.TopPos = 0 }
}

// SetSelectPos manually sets the selection index and updates TopPos to keep it visible.
func (sv *ScrollView) SetSelectPos(pos int) {
	if sv.ItemCount == 0 {
		sv.SelectPos = 0
		sv.TopPos = 0
		return
	}
	if pos < 0 { pos = 0 }
	if pos >= sv.ItemCount { pos = sv.ItemCount - 1 }
	sv.SelectPos = pos
	sv.EnsureVisible()
}

// MoveRelative shifts the selection by delta and updates TopPos.
func (sv *ScrollView) MoveRelative(delta int) bool {
	if sv.ItemCount == 0 {
		return false
	}
	oldPos := sv.SelectPos
	newPos := oldPos

	step := 1
	if delta < 0 {
		step = -1
	}
	absDelta := delta
	if absDelta < 0 {
		absDelta = -absDelta
	}

	// Move one 'selectable' unit at a time
	for i := 0; i < absDelta; i++ {
		testPos := newPos
		found := false
		// Internal loop to skip unselectable items
		for j := 0; j < sv.ItemCount; j++ {
			testPos += step
			if testPos < 0 {
				if sv.Wrap { testPos = sv.ItemCount - 1 } else { testPos = 0; break }
			}
			if testPos >= sv.ItemCount {
				if sv.Wrap { testPos = 0 } else { testPos = sv.ItemCount - 1; break }
			}
			if sv.IsSelectable == nil || sv.IsSelectable(testPos) {
				newPos = testPos
				found = true
				break
			}
			if !sv.Wrap && (testPos <= 0 || testPos >= sv.ItemCount-1) {
				break
			}
		}
		if !found {
			break
		}
	}

	sv.SetSelectPos(newPos)
	return sv.SelectPos != oldPos
}

func (sv *ScrollView) HandleNavKey(vk uint16) bool {
	switch vk {
	case vtinput.VK_UP:
		sv.MoveRelative(-1)
	case vtinput.VK_DOWN:
		sv.MoveRelative(1)
	case vtinput.VK_PRIOR:
		sv.MoveRelative(-sv.ViewHeight)
	case vtinput.VK_NEXT:
		sv.MoveRelative(sv.ViewHeight)
	case vtinput.VK_HOME:
		sv.SetSelectPos(0)
	case vtinput.VK_END:
		sv.SetSelectPos(sv.ItemCount - 1)
	default:
		return false
	}
	return true
}

// GetClickIndex returns the data index that was clicked, or -1 if invalid
func (sv *ScrollView) GetClickIndex(my int) int {
	relY := my - (sv.Y1 + sv.MarginTop)
	if relY >= 0 && relY < sv.ViewHeight {
		idx := sv.TopPos + relY
		if idx >= 0 && idx < sv.ItemCount {
			return idx
		}
	}
	return -1
}

func (sv *ScrollView) HandleKey(e *vtinput.InputEvent) bool {
	if !e.KeyDown { return false }

	oldPos := sv.SelectPos
	handled := false

	switch e.VirtualKeyCode {
	case vtinput.VK_RETURN:
		if sv.OnAction != nil {
			sv.OnAction(sv.SelectPos)
			return true
		}
	default:
		handled = sv.HandleNavKey(e.VirtualKeyCode)
	}

	if handled && sv.SelectPos != oldPos && sv.OnSelect != nil {
		sv.OnSelect(sv.SelectPos)
	}
	return handled
}

func (sv *ScrollView) HandleMouse(e *vtinput.InputEvent) bool {
	if e.Type != vtinput.MouseEventType { return false }
	if sv.HandleMouseScroll(e) { return true }

	if e.ButtonState != 0 && e.KeyDown {
		clickIdx := sv.GetClickIndex(int(e.MouseY))
		if clickIdx != -1 {
			oldPos := sv.SelectPos
			sv.SelectPos = clickIdx

			if sv.SelectPos != oldPos && sv.OnSelect != nil {
				sv.OnSelect(sv.SelectPos)
			}

			// Trigger actions only for the primary (left) button
			if e.ButtonState == vtinput.FromLeft1stButtonPressed && (e.MouseEventFlags & vtinput.DoubleClick) != 0 && sv.OnAction != nil {
				sv.OnAction(sv.SelectPos)
			}
			return true
		}
	}
	return false
}
