//go:build windows

package vtui

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/unxed/vtinput"
	"golang.org/x/sys/windows"
)

func TestGUIKeyTranslationWindowsLayouts(t *testing.T) {
	if !textProbeSupported {
		t.Skip("non-mutating translation requires Windows 10 build 14393")
	}
	load := user32.NewProc("LoadKeyboardLayoutW")
	activate := user32.NewProc("ActivateKeyboardLayout")
	for _, tc := range []struct {
		name, layout string
		vk           uint16
		mods         vtinput.ControlKeyState
		want         int
	}{
		{"GermanAt", "00000407", vtinput.VK_Q, vtinput.LeftCtrlPressed | vtinput.RightAltPressed, 1},
		{"GermanEuro", "00000407", vtinput.VK_E, vtinput.LeftCtrlPressed | vtinput.RightAltPressed, 1},
		{"PolishUpperA", "00000415", vtinput.VK_A, vtinput.LeftCtrlPressed | vtinput.RightAltPressed | vtinput.ShiftPressed, 1},
		{"GermanTilde", "00000407", vtinput.VK_OEM_PLUS, vtinput.LeftCtrlPressed | vtinput.RightAltPressed, 1},
		{"InternationalTildeDeadKey", "00020409", vtinput.VK_OEM_3, vtinput.ShiftPressed, -1},
		{"OrdinaryAltCharacter", "00000407", vtinput.VK_Q, vtinput.LeftAltPressed, 1},
		{"CtrlAltNoText", "00000407", vtinput.VK_F, vtinput.LeftCtrlPressed | vtinput.RightAltPressed, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			original, _, _ := textGetKeyboardLayout.Call(0)
			defer activate.Call(original, 0)
			klid, err := windows.UTF16PtrFromString(tc.layout)
			if err != nil {
				t.Fatal(err)
			}
			layout, _, _ := load.Call(uintptr(unsafe.Pointer(klid)), 0)
			if layout == 0 {
				t.Skipf("layout %s unavailable", tc.layout)
			}
			if result, _, _ := activate.Call(layout, 0); result == 0 {
				t.Skip("cannot activate test thread layout")
			}
			// Repeating a non-mutating probe must not consume/advance a dead key.
			for range 2 {
				if got := guiKeyTranslation(tc.vk, 0, tc.mods); got != tc.want {
					t.Fatalf("translation = %d, want %d", got, tc.want)
				}
			}
		})
	}
}
