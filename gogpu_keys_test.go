//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"testing"

	"github.com/gogpu/gpucontext"
	"github.com/unxed/vtinput"
)

// TestGogpuSyncMods_LockStates pins the fix for the keypad being stuck in
// numeric mode: the lock bits have to survive the trip from gpucontext into
// vtinput, or isNumLockEffectiveGogpu never sees NumLock at all.
func TestGogpuSyncMods_LockStates(t *testing.T) {
	tests := []struct {
		name string
		mods gpucontext.Modifiers
		want vtinput.ControlKeyState
	}{
		{"none", 0, 0},
		{"numlock", gpucontext.ModNumLock, vtinput.NumLockOn},
		{"capslock", gpucontext.ModCapsLock, vtinput.CapsLockOn},
		{"both locks", gpucontext.ModNumLock | gpucontext.ModCapsLock,
			vtinput.NumLockOn | vtinput.CapsLockOn},
		{"numlock and shift", gpucontext.ModNumLock | gpucontext.ModShift,
			vtinput.NumLockOn | vtinput.ShiftPressed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &GogpuHost{}
			got := h.syncMods(0, tt.mods, true)
			if got != tt.want {
				t.Errorf("syncMods(mods=%d) = %d, want %d", tt.mods, got, tt.want)
			}
		})
	}
}

// TestGogpuKeyToVK_Numpad covers the split the backend used to skip entirely:
// gogpu reports the physical keypad key, so NumLock (inverted by Shift) is
// what decides between a digit and a navigation code.
func TestGogpuKeyToVK_Numpad(t *testing.T) {
	const (
		off    = vtinput.ControlKeyState(0)
		on     = vtinput.NumLockOn
		shift  = vtinput.ShiftPressed
		onShft = vtinput.NumLockOn | vtinput.ShiftPressed
	)

	tests := []struct {
		name string
		key  gpucontext.Key
		mods vtinput.ControlKeyState
		want uint16
	}{
		// NumLock on: digits.
		{"numlock on 0", gpucontext.KeyNumpad0, on, vtinput.VK_NUMPAD0},
		{"numlock on 1", gpucontext.KeyNumpad1, on, vtinput.VK_NUMPAD1},
		{"numlock on 5", gpucontext.KeyNumpad5, on, vtinput.VK_NUMPAD5},
		{"numlock on 9", gpucontext.KeyNumpad9, on, vtinput.VK_NUMPAD9},
		{"numlock on decimal", gpucontext.KeyNumpadDecimal, on, vtinput.VK_DECIMAL},

		// NumLock off: navigation. This is what the backend never produced.
		{"numlock off 0", gpucontext.KeyNumpad0, off, vtinput.VK_INSERT},
		{"numlock off 1", gpucontext.KeyNumpad1, off, vtinput.VK_END},
		{"numlock off 2", gpucontext.KeyNumpad2, off, vtinput.VK_DOWN},
		{"numlock off 3", gpucontext.KeyNumpad3, off, vtinput.VK_NEXT},
		{"numlock off 4", gpucontext.KeyNumpad4, off, vtinput.VK_LEFT},
		{"numlock off 5", gpucontext.KeyNumpad5, off, vtinput.VK_CLEAR},
		{"numlock off 6", gpucontext.KeyNumpad6, off, vtinput.VK_RIGHT},
		{"numlock off 7", gpucontext.KeyNumpad7, off, vtinput.VK_HOME},
		{"numlock off 8", gpucontext.KeyNumpad8, off, vtinput.VK_UP},
		{"numlock off 9", gpucontext.KeyNumpad9, off, vtinput.VK_PRIOR},
		{"numlock off decimal", gpucontext.KeyNumpadDecimal, off, vtinput.VK_DELETE},

		// Shift inverts the lock, both ways.
		{"shift inverts lock on", gpucontext.KeyNumpad7, onShft, vtinput.VK_HOME},
		{"shift inverts lock off", gpucontext.KeyNumpad7, shift, vtinput.VK_NUMPAD7},

		// Operators and Enter mean the same whatever the lock says.
		{"add", gpucontext.KeyNumpadAdd, off, vtinput.VK_ADD},
		{"subtract", gpucontext.KeyNumpadSubtract, on, vtinput.VK_SUBTRACT},
		{"multiply", gpucontext.KeyNumpadMultiply, off, vtinput.VK_MULTIPLY},
		{"divide", gpucontext.KeyNumpadDivide, on, vtinput.VK_DIVIDE},
		{"keypad enter", gpucontext.KeyNumpadEnter, off, vtinput.VK_RETURN},

		// Lock keys themselves were unmapped too.
		{"numlock key", gpucontext.KeyNumLock, off, vtinput.VK_NUMLOCK},
		{"capslock key", gpucontext.KeyCapsLock, off, vtinput.VK_CAPITAL},
		{"pause key", gpucontext.KeyPause, off, vtinput.VK_PAUSE},
		{"cancel key", gpucontext.KeyCancel, vtinput.LeftCtrlPressed, vtinput.VK_CANCEL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gogpuKeyToVK(tt.key, tt.mods)
			if got != tt.want {
				t.Errorf("gogpuKeyToVK(%v, %d) = %d, want %d", tt.key, tt.mods, got, tt.want)
			}
		})
	}
}

// TestGogpuNumpadRune checks both halves of the helper's job: the character a
// keypad key types, and the zero that marks a keystroke as typing nothing.
func TestGogpuNumpadRune(t *testing.T) {
	tests := []struct {
		name string
		vk   uint16
		want rune
	}{
		{"numpad 0", vtinput.VK_NUMPAD0, '0'},
		{"numpad 7", vtinput.VK_NUMPAD7, '7'},
		{"numpad 9", vtinput.VK_NUMPAD9, '9'},
		{"decimal", vtinput.VK_DECIMAL, '.'},
		{"add", vtinput.VK_ADD, '+'},
		{"divide", vtinput.VK_DIVIDE, '/'},
		{"home types nothing", vtinput.VK_HOME, 0},
		{"clear types nothing", vtinput.VK_CLEAR, 0},
		{"letter is not a keypad key", vtinput.VK_A, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := gogpuNumpadRune(tt.vk); got != tt.want {
				t.Errorf("gogpuNumpadRune(%d) = %q, want %q", tt.vk, got, tt.want)
			}
		})
	}
}

// TestIsGogpuKeypadKey guards the suppression rule. Only keys whose meaning
// the lock changes may have their text dropped; the operators keep theirs.
func TestIsGogpuKeypadKey(t *testing.T) {
	yes := []gpucontext.Key{
		gpucontext.KeyNumpad0, gpucontext.KeyNumpad5, gpucontext.KeyNumpad9,
		gpucontext.KeyNumpadDecimal,
	}
	no := []gpucontext.Key{
		gpucontext.KeyNumpadAdd, gpucontext.KeyNumpadSubtract,
		gpucontext.KeyNumpadMultiply, gpucontext.KeyNumpadDivide,
		gpucontext.KeyNumpadEnter, gpucontext.KeyA, gpucontext.Key7,
		gpucontext.KeyHome,
	}

	for _, k := range yes {
		if !isGogpuKeypadKey(k) {
			t.Errorf("isGogpuKeypadKey(%v) = false, want true", k)
		}
	}
	for _, k := range no {
		if isGogpuKeypadKey(k) {
			t.Errorf("isGogpuKeypadKey(%v) = true, want false", k)
		}
	}
}

// TestIsGogpuEnhancedNavKey pins the enhanced-key split: the picture viewer
// pans with the four arrows on gogpu, so they stay plain, while the rest of
// the navigation cluster is flagged like the Windows console flags it.
func TestIsGogpuEnhancedNavKey(t *testing.T) {
	plain := []gpucontext.Key{
		gpucontext.KeyUp, gpucontext.KeyDown, gpucontext.KeyLeft, gpucontext.KeyRight,
		gpucontext.KeyNumpad2, gpucontext.KeyNumpad4, gpucontext.KeyNumpad6, gpucontext.KeyNumpad8,
		gpucontext.KeyA, gpucontext.KeyEnter,
	}
	enhanced := []gpucontext.Key{
		gpucontext.KeyHome, gpucontext.KeyEnd, gpucontext.KeyPageUp, gpucontext.KeyPageDown,
		gpucontext.KeyInsert, gpucontext.KeyDelete,
	}

	for _, k := range plain {
		if isGogpuEnhancedNavKey(k) {
			t.Errorf("isGogpuEnhancedNavKey(%v) = true, want false", k)
		}
	}
	for _, k := range enhanced {
		if !isGogpuEnhancedNavKey(k) {
			t.Errorf("isGogpuEnhancedNavKey(%v) = false, want true", k)
		}
	}
}

// TestIsSpecialOrModifiedKey_Clear covers keypad 5 with NumLock off: it types
// nothing, so pairing it with text that never arrives only delays it.
func TestIsSpecialOrModifiedKey_Clear(t *testing.T) {
	if !isSpecialOrModifiedKey(vtinput.VK_CLEAR, 0) {
		t.Error("isSpecialOrModifiedKey(VK_CLEAR, 0) = false, want true")
	}
}
