package vtui

import (
	"github.com/unxed/vtinput"
	"math"
	"testing"
)

func gestureEvent(x, y int, down bool, buttons uint32, moved bool) *vtinput.InputEvent {
	if x < math.MinInt16 || x > math.MaxInt16 || y < math.MinInt16 || y > math.MaxInt16 {
		panic("test mouse coordinate exceeds the input protocol range")
	}
	e := &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: int16(x), MouseY: int16(y), KeyDown: down, ButtonState: buttons}
	if moved {
		e.MouseEventFlags = vtinput.MouseMoved
	}
	return e
}

func gestureManager(t *testing.T) *frameManager {
	t.Helper()
	old := FrameManager
	fm := &frameManager{}
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)
	FrameManager = fm
	fm.Push(NewDesktop())
	t.Cleanup(func() { FrameManager = old })
	return fm
}

func TestMouseGestureCheckboxAndScrollbarCapture(t *testing.T) {
	for _, releaseButton := range []uint32{0, vtinput.FromLeft1stButtonPressed} {
		fm := gestureManager(t)
		window := NewDialog(0, 1, 60, 23, "Gesture")
		bar := NewScrollBar(10, 3, 10)
		bar.SetParams(0, 0, 100)
		bar.SetVisible(true)
		box := NewCheckbox(15, 5, "Toggle", false)
		window.AddItem(bar)
		window.AddItem(box)
		fm.Push(window)
		fm.dispatchEvent(gestureEvent(10, 4, true, 1, false), false)
		if !bar.IsMouseCaptured() {
			t.Fatalf("initial bar press missed: pos=%d,%d,%d,%d visible=%v hit=%v root=%T", bar.X1, bar.Y1, bar.X2, bar.Y2, bar.IsVisible(), bar.HitTest(10, 4), window.rootGroup.mouseCapture)
		}
		fm.dispatchEvent(gestureEvent(16, 5, true, 1, true), false)
		fm.dispatchEvent(gestureEvent(16, 6, false, 1, true), false)
		if !bar.IsMouseCaptured() || box.State != 0 || bar.Value == 0 {
			t.Fatalf("scrollbar did not own held gesture: capture=%v value=%d checkbox=%d frame=%T", bar.IsMouseCaptured(), bar.Value, box.State, fm.capturedFrame)
		}
		fm.dispatchEvent(gestureEvent(90, 30, false, releaseButton, false), false)
		if bar.IsMouseCaptured() || fm.capturedFrame != nil {
			t.Fatal("outside release did not clear capture")
		}
		fm.dispatchEvent(gestureEvent(16, 5, true, 1, false), false)
		for _, x := range []int{17, 18, 70, 18} {
			fm.dispatchEvent(gestureEvent(x, 5, true, 1, true), false)
		}
		if box.State != 1 {
			t.Fatal("checkbox toggled more than once during drag")
		}
		fm.dispatchEvent(gestureEvent(18, 5, false, releaseButton, false), false)
		fm.dispatchEvent(gestureEvent(16, 5, true, 1, false), false)
		fm.dispatchEvent(gestureEvent(16, 5, false, releaseButton, false), false)
		if box.State != 0 {
			t.Fatal("second physical checkbox click was lost")
		}
	}
}

func TestMouseGestureComboDragReleaseAndFreshClick(t *testing.T) {
	for _, releaseButton := range []uint32{0, vtinput.FromLeft1stButtonPressed} {
		fm := gestureManager(t)
		window := NewDialog(0, 1, 60, 23, "Combo")
		combo := NewComboBox(5, 3, 20, []string{"One", "Two", "Three"})
		combo.DropdownOnly = true
		combo.Menu.SetSelectPos(0)
		combo.Edit.SetText("One")
		box := NewCheckbox(30, 3, "Toggle", false)
		window.AddItem(combo)
		window.AddItem(box)
		fm.Push(window)
		fm.dispatchEvent(gestureEvent(6, 3, true, 1, false), false)
		if fm.GetTopFrame() != combo.Menu {
			t.Fatal("press did not open dropdown")
		}
		fm.dispatchEvent(gestureEvent(combo.Menu.X1+1, combo.Menu.Y1+2, true, 1, true), false)
		if combo.Menu.SelectPos != 1 || combo.Edit.GetText() != "One" {
			t.Fatal("drag must highlight without selecting")
		}
		fm.dispatchEvent(gestureEvent(combo.Menu.X1+1, combo.Menu.Y1+2, false, releaseButton, false), false)
		if combo.Edit.GetText() != "Two" || fm.GetTopFrame() != window {
			t.Fatal("release did not activate highlighted item")
		}
		fm.dispatchEvent(gestureEvent(31, 3, true, 1, false), false)
		fm.dispatchEvent(gestureEvent(31, 3, false, releaseButton, false), false)
		if box.State != 1 {
			t.Fatal("dropdown left stale capture in its owner")
		}
	}
}

func TestMouseGestureMenuBarStaysOpenAndReleasesOnItem(t *testing.T) {
	for _, releaseButton := range []uint32{0, vtinput.FromLeft1stButtonPressed} {
		fm := gestureManager(t)
		invoked := 0
		bar := NewMenuBar(nil)
		bar.Items = []MenuBarItem{
			{Label: "File", SubItems: []MenuItem{{Text: "First", OnClick: func() { invoked = 1 }}}},
			{Label: "Edit", SubItems: []MenuItem{{Text: "Second", OnClick: func() { invoked = 2 }}}},
		}
		bar.SetPosition(0, 0, 79, 0)
		fm.MenuBar = bar
		fm.dispatchEvent(gestureEvent(bar.GetItemX(0)+1, 0, true, 1, false), false)
		menu := fm.GetTopFrame()
		fm.dispatchEvent(gestureEvent(75, 0, true, 1, true), false)
		fm.dispatchEvent(gestureEvent(75, 10, true, 1, true), false)
		if !bar.Active || fm.GetTopFrame() != menu || invoked != 0 {
			t.Fatal("leaving menu while held closed it or invoked an action")
		}
		fm.dispatchEvent(gestureEvent(bar.GetItemX(1)+1, 0, true, 1, true), false)
		sub := fm.GetTopFrame().(*VMenu)
		fm.dispatchEvent(gestureEvent(sub.X1+1, sub.Y1+1, true, 1, true), false)
		if invoked != 0 {
			t.Fatal("hover invoked menu action")
		}
		fm.dispatchEvent(gestureEvent(sub.X1+1, sub.Y1+1, false, releaseButton, false), false)
		if invoked != 2 || bar.Active || bar.mouseSelecting {
			t.Fatal("release did not invoke second menu and end gesture")
		}
	}
}

func TestMouseGestureANSIButtonReleaseAndPopupCancel(t *testing.T) {
	fm := gestureManager(t)
	button := NewButton(2, 2, "Apply")
	clicks := 0
	button.OnClick = func() { clicks++ }
	button.ProcessMouse(gestureEvent(3, 2, true, 1, false))
	button.ProcessMouse(gestureEvent(4, 2, true, 1, true))
	button.ProcessMouse(gestureEvent(4, 2, false, 1, false))
	if clicks != 1 || button.mouseArmed {
		t.Fatal("ANSI release did not activate and disarm button")
	}
	combo := NewComboBox(3, 3, 20, []string{"One", "Two"})
	combo.Menu.BeginMouseSelection()
	combo.Menu.SetExitCode(-1)
	combo.Open()
	if combo.Menu.mouseSelecting {
		t.Fatal("keyboard reopening inherited cancelled mouse gesture")
	}
	fm.RemoveFrame(combo.Menu)
}
