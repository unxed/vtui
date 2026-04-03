package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestDesktop_ExitKeys(t *testing.T) {
	// Desktop uses global FrameManager.
	oldScreens := FrameManager.Screens
	defer func() { FrameManager.Screens = oldScreens }()

	// 1. Test F10
	FrameManager.Init(NewSilentScreenBuf())
	d1 := NewDesktop()
	FrameManager.Push(d1)

	d1.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F10})
	if !FrameManager.IsShutdown() {
		t.Error("Desktop should trigger Shutdown on F10 via CmQuit")
	}

	// 2. Test ESC (Re-init manager for fresh state)
	FrameManager.Init(NewSilentScreenBuf())
	d2 := NewDesktop()
	FrameManager.Push(d2)
	
	d2.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if !FrameManager.IsShutdown() {
		t.Error("Desktop should trigger Shutdown on ESC via CmQuit")
	}
}
