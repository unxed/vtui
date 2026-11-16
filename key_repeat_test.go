package vtui

import (
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestKeyRepeatState_FollowsFar2lRule(t *testing.T) {
	var s keyRepeatState
	key := func(vk uint16, down bool) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: down, VirtualKeyCode: vk}
	}
	steps := []struct {
		name     string
		ev       *vtinput.InputEvent
		injected bool
		want     bool
	}{
		{"first press", key(vtinput.VK_DOWN, true), false, false},
		{"same key again with no release", key(vtinput.VK_DOWN, true), false, true},
		{"held on", key(vtinput.VK_DOWN, true), false, true},
		{"release", key(vtinput.VK_DOWN, false), false, false},
		{"press after release", key(vtinput.VK_DOWN, true), false, false},
		{"another key", key(vtinput.VK_UP, true), false, false},
		{"that key held", key(vtinput.VK_UP, true), false, true},
		{"mouse event leaves the run alone", &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseEventFlags: vtinput.MouseMoved}, false, true},
		{"injected press is never a repeat", key(vtinput.VK_UP, true), true, false},
		{"injected press does not break the physical run", key(vtinput.VK_UP, true), false, true},
		{"legacy press is never a repeat", &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_UP, IsLegacy: true}, false, false},
		{"legacy press ends the run, as far2l's synthesized release does", key(vtinput.VK_UP, true), false, false},
	}
	for _, st := range steps {
		s.observe(st.ev, st.injected)
		if s.repeated != st.want {
			t.Fatalf("%s: repeated = %v, want %v", st.name, s.repeated, st.want)
		}
	}
}

func TestShouldDeferRenderForQueuedHeldKeyInput(t *testing.T) {
	now := time.Now()
	fm := &frameManager{
		EventChan:  make(chan *vtinput.InputEvent, 1),
		lastRender: now,
		keyRepeat:  keyRepeatState{lastDownKnown: true},
	}
	fm.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT}

	if !fm.shouldDeferRender(now.Add(time.Millisecond)) {
		t.Fatal("queued input during a held-key run should be handled before another render")
	}
	if fm.shouldDeferRender(now.Add(pasteRenderGap)) {
		t.Fatal("a held-key input burst must still render after the maximum redraw gap")
	}
	<-fm.EventChan
	if fm.shouldDeferRender(now.Add(time.Millisecond)) {
		t.Fatal("must not defer a render after the input queue drains")
	}
}
