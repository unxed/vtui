package vtui

import "github.com/unxed/vtinput"

// ListViewer is a mixin for handling list navigation logic (analogous to TListViewer).
type ListViewer struct {
	TopPos      int
	SelectPos   int
	ItemCount   int
	ViewHeight  int
	Wrap        bool
	// Callback to check if an item is selectable (e.g. not a separator)
	IsSelectable func(int) bool

	ShowScrollBar bool
	ScrollBar     *ScrollBar
}

func (lv *ListViewer) InitScrollBar(owner CommandHandler) {
	lv.ScrollBar = NewScrollBar(0, 0, 0)
	lv.ScrollBar.SetOwner(owner)
	lv.ScrollBar.OnScroll = func(v int) {
		lv.TopPos = v
	}
}

func (lv *ListViewer) UpdateScrollBar(x, y, h int) {
	if lv.ScrollBar != nil {
		lv.ScrollBar.SetPosition(x, y, x, y+h-1)
		lv.ScrollBar.PgStep = h
	}
}

func (lv *ListViewer) DrawScrollBar(scr *ScreenBuf) {
	if lv.ShowScrollBar && lv.ScrollBar != nil && lv.ItemCount > lv.ViewHeight && lv.ViewHeight > 0 {
		lv.ScrollBar.SetParams(lv.TopPos, 0, lv.ItemCount-lv.ViewHeight)
		lv.ScrollBar.Show(scr)
	}
}

func (lv *ListViewer) HandleMouseScroll(e *vtinput.InputEvent) bool {
	if lv.ShowScrollBar && lv.ScrollBar != nil && lv.ScrollBar.ProcessMouse(e) {
		return true
	}
	if e.WheelDirection != 0 {
		if e.WheelDirection > 0 && lv.TopPos > 0 {
			lv.TopPos--
			return true
		} else if e.WheelDirection < 0 && lv.TopPos < lv.ItemCount - lv.ViewHeight {
			lv.TopPos++
			return true
		}
	}
	return false
}

func (lv *ListViewer) EnsureVisible() {
	if lv.ViewHeight <= 0 { return }
	if lv.SelectPos < lv.TopPos {
		lv.TopPos = lv.SelectPos
	} else if lv.SelectPos >= lv.TopPos+lv.ViewHeight {
		lv.TopPos = lv.SelectPos - lv.ViewHeight + 1
	}
	if lv.TopPos < 0 { lv.TopPos = 0 }
}

// SetSelectPos manually sets the selection index and updates TopPos to keep it visible.
func (lv *ListViewer) SetSelectPos(pos int) {
	if lv.ItemCount == 0 {
		lv.SelectPos = 0
		lv.TopPos = 0
		return
	}
	if pos < 0 { pos = 0 }
	if pos >= lv.ItemCount { pos = lv.ItemCount - 1 }
	lv.SelectPos = pos
	lv.EnsureVisible()
}

// MoveRelative shifts the selection by delta and updates TopPos.
func (lv *ListViewer) MoveRelative(delta int) bool {
	if lv.ItemCount == 0 {
		return false
	}
	oldPos := lv.SelectPos
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
		for j := 0; j < lv.ItemCount; j++ {
			testPos += step
			if testPos < 0 {
				if lv.Wrap { testPos = lv.ItemCount - 1 } else { testPos = 0; break }
			}
			if testPos >= lv.ItemCount {
				if lv.Wrap { testPos = 0 } else { testPos = lv.ItemCount - 1; break }
			}
			if lv.IsSelectable == nil || lv.IsSelectable(testPos) {
				newPos = testPos
				found = true
				break
			}
			if !lv.Wrap && (testPos <= 0 || testPos >= lv.ItemCount-1) {
				break
			}
		}
		if !found {
			break
		}
	}

	lv.SetSelectPos(newPos)
	return lv.SelectPos != oldPos
}

func (lv *ListViewer) HandleNavKey(vk uint16) bool {
	switch vk {
	case vtinput.VK_UP:
		lv.MoveRelative(-1)
	case vtinput.VK_DOWN:
		lv.MoveRelative(1)
	case vtinput.VK_PRIOR:
		lv.MoveRelative(-lv.ViewHeight)
	case vtinput.VK_NEXT:
		lv.MoveRelative(lv.ViewHeight)
	case vtinput.VK_HOME:
		lv.SetSelectPos(0)
	case vtinput.VK_END:
		lv.SetSelectPos(lv.ItemCount - 1)
	default:
		return false
	}
	return true
}