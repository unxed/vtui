package vtui

import "github.com/unxed/vtinput"

// ListViewer is a mixin for handling list navigation logic (analogous to TListViewer).
type ListViewer struct {
	TopPos    int
	SelectPos int
	ItemCount int
	ViewHeight int
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
	oldPos := lv.SelectPos
	lv.SetSelectPos(lv.SelectPos + delta)
	return lv.SelectPos != oldPos
}

func (lv *ListViewer) HandleNavKey(vk uint16) bool {
	oldPos := lv.SelectPos
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
	return lv.SelectPos != oldPos
}