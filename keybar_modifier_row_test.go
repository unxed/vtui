package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// newKeyBarRowManager builds a frame manager with a key bar and one ordinary
// frame, which is the minimum dispatchEvent needs to reach the modifier row.
func newKeyBarRowManager(t *testing.T) (*frameManager, *KeyBar) {
	t.Helper()

	old := FrameManager
	fm := NewFrameManager()
	FrameManager = fm
	t.Cleanup(func() {
		fm.Shutdown()
		FrameManager = old
	})

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	fm.Init(scr)

	kb := NewKeyBar()
	kb.SetPosition(0, 24, 79, 24)
	fm.KeyBar = kb

	f := &mockFrame{}
	f.SetPosition(0, 0, 79, 24)
	fm.Push(f)

	return fm, kb
}

// A terminal without key release reporting delivers Shift+F1 as a single F1
// keypress carrying the Shift bit and then says nothing at all when Shift is
// let go. Reading that bit as "Shift is held" parked the key bar on the Shift
// row until some unrelated keystroke came along -- a row that is mostly empty,
// and empty slots are drawn as filled blocks, so it looked for all the world
// like a window had opened over the panels. Reported as f4 issue #983.
func TestKeyBarChordDoesNotStickModifierRow(t *testing.T) {
	fm, kb := newKeyBarRowManager(t)

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_F1,
		ControlKeyState: vtinput.ShiftPressed,
	}, false)

	if kb.shiftState {
		t.Error("Shift+F1 from a press-only terminal left the key bar on the Shift row")
	}

	// The other half of #983: a chord carrying Ctrl and Alt, which is what a
	// keymap.ini rule such as CtrlAlt*=Ctrl* rewrites.
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  'A',
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed,
	}, false)

	if kb.ctrlState || kb.altState {
		t.Errorf("a Ctrl+Alt chord left the key bar on a modifier row (ctrl=%v alt=%v)", kb.ctrlState, kb.altState)
	}
}

// Backends that report a modifier key in its own right (X11, Wayland, the GUI
// renderers, Windows console, the kitty and far2l protocols) report its
// release too, so holding Shift must still bring up the Shift row and keep it
// there for the whole chord.
func TestKeyBarModifierKeyStillSwitchesRow(t *testing.T) {
	fm, kb := newKeyBarRowManager(t)

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_SHIFT,
	}, false)
	if !kb.shiftState {
		t.Fatal("pressing Shift did not bring up the Shift row")
	}

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_F1,
		ControlKeyState: vtinput.ShiftPressed,
	}, false)
	if !kb.shiftState {
		t.Error("pressing F1 with Shift held took the Shift row away")
	}

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         false,
		VirtualKeyCode:  vtinput.VK_F1,
		ControlKeyState: vtinput.ShiftPressed,
	}, false)
	if !kb.shiftState {
		t.Error("releasing F1 with Shift still held took the Shift row away")
	}

	// X11 and macOS report the state as it was *before* the transition, so
	// Shift's own release still carries the Shift bit: KeyDown is what counts.
	fm.dispatchEvent(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         false,
		VirtualKeyCode:  vtinput.VK_SHIFT,
		ControlKeyState: vtinput.ShiftPressed,
	}, false)
	if kb.shiftState {
		t.Error("releasing Shift left the key bar on the Shift row")
	}
}

// SetModifiers is the entry point callers outside the dispatcher use, f4's key
// remapping among them: it must be able to retire the row belonging to a chord
// it rewrote, and must not be able to raise one of its own.
func TestKeyBarSetModifiersOnlyClears(t *testing.T) {
	kb := NewKeyBar()

	kb.SetModifiers(true, true, true)
	if kb.shiftState || kb.ctrlState || kb.altState {
		t.Fatal("an ordinary event lit a modifier row by itself")
	}

	// Ctrl and Alt are both held and a rule has just rewritten Ctrl+Alt+A
	// into Ctrl+A: the Alt row goes, the Ctrl row stays.
	kb.LatchModifiers(false, true, true)
	kb.SetModifiers(false, true, false)
	if !kb.ctrlState {
		t.Error("the modifier the rewritten chord kept was dropped")
	}
	if kb.altState {
		t.Error("the modifier the rewrite dropped is still up")
	}
}
