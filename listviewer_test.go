package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestListViewer_Logic(t *testing.T) {
	sv := &ScrollView{ItemCount: 10, ViewHeight: 3}

	// 1. Initial
	sv.SetSelectPos(0)
	if sv.TopPos != 0 { t.Error("TopPos should be 0") }

	// 2. Navigation
	sv.HandleNavKey(vtinput.VK_DOWN) // pos 1
	sv.HandleNavKey(vtinput.VK_DOWN) // pos 2
	if sv.TopPos != 0 { t.Error("TopPos should still be 0 at pos 2") }

	sv.HandleNavKey(vtinput.VK_DOWN) // pos 3 (scrolls!)
	if sv.TopPos != 1 { t.Errorf("Expected TopPos 1, got %d", sv.TopPos) }

	// 3. Page Nav
	sv.HandleNavKey(vtinput.VK_NEXT) // 3 + 3 = 6
	if sv.SelectPos != 6 { t.Errorf("PgDn failed: got %d", sv.SelectPos) }
	if sv.TopPos != 4 { t.Errorf("PgDn TopPos failed: got %d", sv.TopPos) }

	// 4. Boundaries
	sv.SetSelectPos(9)
	sv.HandleNavKey(vtinput.VK_DOWN)
	if sv.SelectPos != 9 { t.Error("Should not exceed ItemCount") }

	// 5. Empty list
	sv.ItemCount = 0
	sv.SetSelectPos(5)
	if sv.SelectPos != 0 { t.Error("Empty list SelectPos should be 0 (compat)") }
}

func TestListViewer_MoveRelative(t *testing.T) {
	sv := &ScrollView{ItemCount: 10, ViewHeight: 5}
	sv.SetSelectPos(2)

	sv.MoveRelative(2)
	if sv.SelectPos != 4 { t.Errorf("MoveRelative(2) failed, got %d", sv.SelectPos) }

	sv.MoveRelative(-10)
	if sv.SelectPos != 0 { t.Errorf("MoveRelative underflow failed, got %d", sv.SelectPos) }
}