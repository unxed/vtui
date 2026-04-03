package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestVMenu_BoundaryNavigation(t *testing.T) {
	m := NewVMenu("Standalone")
	m.AddItem(MenuItem{Text: "1"})
	m.AddItem(MenuItem{Text: "2"})
	
	// 1. Up at top should return false (exit focus)
	m.SetSelectPos(0)
	if m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) {
		t.Error("Up at index 0 should return false")
	}

	// 2. Down at bottom should return false
	m.SetSelectPos(1)
	if m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}) {
		t.Error("Down at last index should return false")
	}

	// 3. Left/Right in standalone menu should return false (boundary exit)
	if m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT}) {
		t.Error("Left in standalone menu should return false")
	}
}

func TestVMenu_FocusVisualization(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 5)
	m := NewVMenu("Menu")
	m.SetPosition(0, 0, 10, 4)

	// 1. Inactive state
	m.SetFocus(false)
	m.Show(scr)
	// Title " Menu " should use ColMenuTitle
	checkCell(t, scr, 3, 0, 'M', Palette[ColMenuTitle])

	// 2. Focused state
	m.SetFocus(true)
	m.Show(scr)
	// Title should now use ColDialogHighlightBoxTitle
	checkCell(t, scr, 3, 0, 'M', Palette[ColDialogHighlightBoxTitle])
}

