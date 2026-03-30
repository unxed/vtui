package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestListViewer_Logic(t *testing.T) {
	lv := &ListViewer{ItemCount: 10, ViewHeight: 3}

	// 1. Initial
	lv.SetSelectPos(0)
	if lv.TopPos != 0 { t.Error("TopPos should be 0") }

	// 2. Navigation
	lv.HandleNavKey(vtinput.VK_DOWN) // pos 1
	lv.HandleNavKey(vtinput.VK_DOWN) // pos 2
	if lv.TopPos != 0 { t.Error("TopPos should still be 0 at pos 2") }

	lv.HandleNavKey(vtinput.VK_DOWN) // pos 3 (scrolls!)
	if lv.TopPos != 1 { t.Errorf("Expected TopPos 1, got %d", lv.TopPos) }

	// 3. Page Nav
	lv.HandleNavKey(vtinput.VK_NEXT) // 3 + 3 = 6
	if lv.SelectPos != 6 { t.Errorf("PgDn failed: got %d", lv.SelectPos) }
	if lv.TopPos != 4 { t.Errorf("PgDn TopPos failed: got %d", lv.TopPos) }

	// 4. Boundaries
	lv.SetSelectPos(9)
	lv.HandleNavKey(vtinput.VK_DOWN)
	if lv.SelectPos != 9 { t.Error("Should not exceed ItemCount") }

	// 5. Empty list
	lv.ItemCount = 0
	lv.SetSelectPos(5)
	if lv.SelectPos != 0 { t.Error("Empty list SelectPos should be 0 (compat)") }
}

func TestListViewer_MoveRelative(t *testing.T) {
	lv := &ListViewer{ItemCount: 10, ViewHeight: 5}
	lv.SetSelectPos(2)

	lv.MoveRelative(2)
	if lv.SelectPos != 4 { t.Errorf("MoveRelative(2) failed, got %d", lv.SelectPos) }

	lv.MoveRelative(-10)
	if lv.SelectPos != 0 { t.Errorf("MoveRelative underflow failed, got %d", lv.SelectPos) }
}