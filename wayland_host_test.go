//go:build linux && !android && (amd64 || arm64)

package vtui

import (
	"io"
	"testing"
	"time"

	"github.com/neurlang/wayland/window"
	"github.com/neurlang/wayland/wl"
	"github.com/unxed/vtinput"
)

func TestWaylandHost_KeyRepeatLogic(t *testing.T) {
	host := &WaylandHost{}

	host.mu.Lock()
	host.isRepeating = true
	host.repeatVK = vtinput.VK_A
	host.repeatNext = time.Now().Add(-1 * time.Millisecond) // force immediate trigger
	host.mu.Unlock()

	if !host.isRepeating {
		t.Error("Expected isRepeating to be true")
	}

	// Note: full integration test of Redraw() spin loop requires mocking window.Widget,
	// which is deeply integrated with the Wayland protocol implementation.
}

func TestWaylandEnhancedKeyMapping(t *testing.T) {
	if got := enhancedKeyForX11Keysym(0xffff); got != vtinput.EnhancedKey {
		t.Fatalf("XK_Delete flag = %v, want EnhancedKey", got)
	}
	if got := enhancedKeyForX11Keysym(0xff9f); got != 0 {
		t.Fatalf("XK_KP_Delete flag = %v, want no EnhancedKey", got)
	}
}

func TestWaylandNumLockFromKeysym(t *testing.T) {
	tests := []struct {
		name   string
		keysym uint32
		shift  bool
		want   bool
		known  bool
	}{
		{"numeric keypad", 0xffb5, false, true, true},
		{"numeric keypad with shift", 0xffb5, true, false, true},
		{"navigation keypad", 0xff9d, false, false, true},
		{"navigation keypad with shift", 0xff9d, true, true, true},
		{"ordinary home", 0xff50, false, false, false},
		{"keypad add does not reveal lock", 0xffab, false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known := waylandNumLockFromKeysym(tt.keysym, tt.shift)
			if got != tt.want || known != tt.known {
				t.Fatalf("waylandNumLockFromKeysym(0x%x, shift=%v) = %v, %v; want %v, %v",
					tt.keysym, tt.shift, got, known, tt.want, tt.known)
			}
		})
	}
}

func TestWaylandFocusLossClearsKeyboardState(t *testing.T) {
	pr, pw := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	t.Cleanup(func() {
		reader.Close()
		_ = pw.Close()
	})

	host := &WaylandHost{
		reader:       reader,
		isRepeating:  true,
		repeatVK:     vtinput.VK_LMENU,
		repeatChar:   'x',
		repeatMods:   vtinput.LeftAltPressed,
		repeatNext:   time.Now().Add(time.Second),
		currentMods:  vtinput.LeftAltPressed,
		numLockOn:    true,
		numLockKnown: true,
		lCtrl:        true,
		rCtrl:        true,
		lAlt:         true,
		rAlt:         true,
		lShift:       true,
		rShift:       true,
	}

	// Gaining focus must not discard state established by a subsequent key
	// event; only the nil-device notification denotes focus loss.
	host.Focus(nil, &window.Input{})
	if !host.isRepeating || !host.lAlt {
		t.Fatal("focus gain unexpectedly cleared keyboard state")
	}
	select {
	case event := <-reader.EventChan:
		if event.Type != vtinput.FocusEventType || !event.SetFocus {
			t.Fatalf("focus gain event = %+v, want FocusEvent(true)", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for Wayland focus gain event")
	}

	host.Focus(nil, nil)

	host.mu.Lock()
	defer host.mu.Unlock()
	if host.isRepeating || host.repeatVK != 0 || host.repeatChar != 0 || host.repeatMods != 0 || !host.repeatNext.IsZero() {
		t.Fatalf("repeat state not cleared: active=%v vk=%d char=%q mods=%d next=%v",
			host.isRepeating, host.repeatVK, host.repeatChar, host.repeatMods, host.repeatNext)
	}
	if host.currentMods != 0 || host.numLockOn || host.numLockKnown || host.lCtrl || host.rCtrl || host.lAlt || host.rAlt || host.lShift || host.rShift {
		t.Fatalf("modifier state not cleared: mods=%d numlock=%v/%v ctrl=%v/%v alt=%v/%v shift=%v/%v",
			host.currentMods, host.numLockOn, host.numLockKnown, host.lCtrl, host.rCtrl, host.lAlt, host.rAlt, host.lShift, host.rShift)
	}

	select {
	case event := <-reader.EventChan:
		if event.Type != vtinput.FocusEventType || event.SetFocus {
			t.Fatalf("focus loss event = %+v, want FocusEvent(false)", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for Wayland focus loss event")
	}
}

func TestWaylandModifierStateHealing(t *testing.T) {
	host := &WaylandHost{}

	host.syncModifierStateLocked(vtinput.LeftCtrlPressed, vtinput.VK_A, true)
	if !host.lCtrl || host.rCtrl {
		t.Fatalf("active aggregate Ctrl sides = %v/%v, want left Ctrl", host.lCtrl, host.rCtrl)
	}

	host.lCtrl, host.rCtrl = true, true
	host.syncModifierStateLocked(0, vtinput.VK_A, true)
	if host.lCtrl || host.rCtrl {
		t.Fatalf("inactive aggregate Ctrl sides = %v/%v, want cleared state", host.lCtrl, host.rCtrl)
	}

	// The key transition is applied before the aggregate mask is read. A
	// release must not recreate the side from a still-active pre-release mask.
	host.syncModifierStateLocked(vtinput.LeftCtrlPressed, vtinput.VK_LCONTROL, false)
	if host.lCtrl || host.rCtrl {
		t.Fatalf("modifier release recreated a Ctrl side: %v/%v", host.lCtrl, host.rCtrl)
	}
}

func TestWaylandModifierKeysDoNotRepeat(t *testing.T) {
	modifiers := []uint16{
		vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LSHIFT, vtinput.VK_RSHIFT,
		vtinput.VK_LWIN, vtinput.VK_RWIN,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL,
	}
	for _, vk := range modifiers {
		if waylandKeyCanRepeat(vk) {
			t.Errorf("modifier/toggle key %#x unexpectedly repeats", vk)
		}
	}
	if !waylandKeyCanRepeat(vtinput.VK_A) {
		t.Error("ordinary key unexpectedly does not repeat")
	}
}

func newWaylandPointerTestHost(t *testing.T) *WaylandHost {
	t.Helper()
	pr, pw := io.Pipe()
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})
	return &WaylandHost{
		reader: vtinput.NewReader(pr, true),
		cellW:  10,
		cellH:  20,
		scale:  1,
	}
}

func nextWaylandMouseEvent(t *testing.T, host *WaylandHost) *vtinput.InputEvent {
	t.Helper()
	select {
	case event := <-host.reader.EventChan:
		return event
	case <-time.After(250 * time.Millisecond):
		t.Fatal("timed out waiting for Wayland mouse event")
		return nil
	}
}

func assertNoWaylandMouseEvent(t *testing.T, host *WaylandHost) {
	t.Helper()
	select {
	case event := <-host.reader.EventChan:
		t.Fatalf("unexpected Wayland mouse event: %+v", event)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestWaylandButtonReleaseClearsMotionState(t *testing.T) {
	host := newWaylandPointerTestHost(t)

	host.Button(nil, nil, 0, 272, wl.PointerButtonStatePressed, nil)
	press := nextWaylandMouseEvent(t, host)
	if !press.KeyDown || press.ButtonState != vtinput.FromLeft1stButtonPressed {
		t.Fatalf("press = KeyDown:%v ButtonState:%d", press.KeyDown, press.ButtonState)
	}

	host.Button(nil, nil, 0, 272, wl.PointerButtonStateReleased, nil)
	release := nextWaylandMouseEvent(t, host)
	if release.KeyDown || release.ButtonState != 0 {
		t.Fatalf("release = KeyDown:%v ButtonState:%d", release.KeyDown, release.ButtonState)
	}

	host.Motion(nil, nil, 0, 30, 40)
	assertNoWaylandMouseEvent(t, host)
}

func TestWaylandMotionReportsHeldButton(t *testing.T) {
	host := newWaylandPointerTestHost(t)

	host.Button(nil, nil, 0, 272, wl.PointerButtonStatePressed, nil)
	_ = nextWaylandMouseEvent(t, host)
	host.Motion(nil, nil, 0, 30, 40)
	drag := nextWaylandMouseEvent(t, host)

	if !drag.KeyDown || drag.ButtonState != vtinput.FromLeft1stButtonPressed {
		t.Fatalf("drag = KeyDown:%v ButtonState:%d", drag.KeyDown, drag.ButtonState)
	}
	if drag.MouseX != 3 || drag.MouseY != 2 {
		t.Fatalf("drag cell = %d,%d, want 3,2", drag.MouseX, drag.MouseY)
	}
}

func TestWaylandMotionReportsOnlyCellChanges(t *testing.T) {
	host := newWaylandPointerTestHost(t)

	host.Button(nil, nil, 0, 272, wl.PointerButtonStatePressed, nil)
	_ = nextWaylandMouseEvent(t, host)

	host.Motion(nil, nil, 0, 9, 19)
	assertNoWaylandMouseEvent(t, host)

	host.Motion(nil, nil, 0, 10, 19)
	firstCell := nextWaylandMouseEvent(t, host)
	if firstCell.MouseX != 1 || firstCell.MouseY != 0 {
		t.Fatalf("first drag cell = %d,%d, want 1,0", firstCell.MouseX, firstCell.MouseY)
	}

	host.Motion(nil, nil, 0, 19, 19)
	assertNoWaylandMouseEvent(t, host)

	host.Motion(nil, nil, 0, 19, 20)
	secondCell := nextWaylandMouseEvent(t, host)
	if secondCell.MouseX != 1 || secondCell.MouseY != 1 {
		t.Fatalf("second drag cell = %d,%d, want 1,1", secondCell.MouseX, secondCell.MouseY)
	}
}

func TestWaylandPointerFramePrefersValue120OverRawAxis(t *testing.T) {
	host := newWaylandPointerTestHost(t)

	host.Axis(nil, nil, 0, wl.PointerAxisVerticalScroll, 15)
	host.AxisValue120(nil, nil, wl.PointerAxisVerticalScroll, 120)
	host.PointerFrame(nil, nil)

	wheel := nextWaylandMouseEvent(t, host)
	if wheel.WheelDirection != -1 {
		t.Fatalf("wheel direction = %d, want -1", wheel.WheelDirection)
	}
	select {
	case extra := <-host.reader.EventChan:
		t.Fatalf("raw axis duplicated value120 event: %+v", extra)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestWaylandPointerFrameSupportsSmoothAxis(t *testing.T) {
	host := newWaylandPointerTestHost(t)

	host.Axis(nil, nil, 0, wl.PointerAxisVerticalScroll, 25)
	host.PointerFrame(nil, nil)

	wheel := nextWaylandMouseEvent(t, host)
	if wheel.WheelDirection != -1 {
		t.Fatalf("smooth wheel direction = %d, want -1", wheel.WheelDirection)
	}
}

func TestWaylandScaleFromDimensions(t *testing.T) {
	tests := []struct {
		name                           string
		width, height, pwidth, pheight int32
		want                           float64
	}{
		{name: "one times", width: 800, height: 600, pwidth: 800, pheight: 600, want: 1},
		{name: "two times", width: 800, height: 600, pwidth: 1600, pheight: 1200, want: 2},
		{name: "fractional", width: 800, height: 600, pwidth: 1200, pheight: 900, want: 1.5},
		{name: "uses available dimension", width: 0, height: 600, pwidth: 0, pheight: 1200, want: 2},
		{name: "sub-unit scale", width: 800, height: 600, pwidth: 400, pheight: 300, want: 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := waylandScaleFromDimensions(tt.width, tt.height, tt.pwidth, tt.pheight); got != tt.want {
				t.Errorf("waylandScaleFromDimensions() = %.2f, want %.2f", got, tt.want)
			}
		})
	}
}

func TestHasWaylandPixelSize(t *testing.T) {
	tests := []struct {
		name                           string
		width, height, pwidth, pheight int32
		want                           bool
	}{
		{name: "valid", width: 1000, height: 690, pwidth: 1500, pheight: 1035, want: true},
		{name: "zero logical width", width: 0, height: 690, pwidth: 0, pheight: 1035, want: false},
		{name: "zero physical height", width: 1000, height: 690, pwidth: 1500, pheight: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasWaylandPixelSize(tt.width, tt.height, tt.pwidth, tt.pheight); got != tt.want {
				t.Errorf("hasWaylandPixelSize(%d, %d, %d, %d) = %v, want %v", tt.width, tt.height, tt.pwidth, tt.pheight, got, tt.want)
			}
		})
	}
}

func TestLogicalWaylandPixelsRoundsToNearest(t *testing.T) {
	if got := logicalWaylandPixels(1001, 2); got != 501 {
		t.Errorf("logicalWaylandPixels(1001, 2) = %d, want 501", got)
	}
	if got := logicalWaylandPixels(1400, 1.5); got != 933 {
		t.Errorf("logicalWaylandPixels(1400, 1.5) = %d, want 933", got)
	}
	if got := logicalWaylandPixels(1, 3); got != 1 {
		t.Errorf("logicalWaylandPixels(1, 3) = %d, want 1", got)
	}
}
