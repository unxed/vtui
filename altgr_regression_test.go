//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import (
	"reflect"
	"testing"

	"github.com/gogpu/gogpu"
	"github.com/unxed/vtinput"
)

func TestGogpuNativeEventsPreserveSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		native gogpu.NativeKeyEvent
		char   rune
		down   bool
	}{
		{"unknown translation keeps command", gogpu.NativeKeyEvent{Kind: gogpu.NativeKeyDown, VirtualKeyCode: vtinput.VK_A}, 'a', true},
		{"translated physical character", gogpu.NativeKeyEvent{Kind: gogpu.NativeKeyDown, VirtualKeyCode: vtinput.VK_A, Char: 'ф'}, 'ф', true},
		{"keyless IME commit", gogpu.NativeKeyEvent{Kind: gogpu.NativeTextCommit, Char: '界', TextInput: true}, '界', true},
		{"release never synthesizes character", gogpu.NativeKeyEvent{Kind: gogpu.NativeKeyUp, VirtualKeyCode: vtinput.VK_A, TextExpected: gogpu.NativeTextExpected}, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 8)}
			host := &GogpuHost{reader: reader, currentMods: vtinput.LeftAltPressed}
			native := tc.native
			native.VirtualScanCode, native.RepeatCount = 30, 3
			native.ControlKeyState = uint32(vtinput.RightAltPressed | vtinput.LeftCtrlPressed | vtinput.NumLockOn)
			host.handleNativeKey(native)
			want := vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: tc.down,
				VirtualKeyCode: native.VirtualKeyCode, VirtualScanCode: 30, RepeatCount: 3,
				ControlKeyState: vtinput.ControlKeyState(native.ControlKeyState), Char: tc.char,
				TextInput: native.TextInput, InputSource: "gogpu"}
			if got := takeHostEvent(t, host); !reflect.DeepEqual(*got, want) {
				t.Fatalf("native event = %+v, want %+v", got, want)
			}
			noHostEvent(t, host)
		})
	}
}

func TestGogpuCommittedAltGrTextReachesEdit(t *testing.T) {
	reader := &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 8)}
	host := &GogpuHost{reader: reader, currentMods: vtinput.RightAltPressed | vtinput.LeftCtrlPressed}
	host.handleTextInput("€")
	edit := NewEdit(0, 0, 20, "")
	edit.ProcessKey(<-reader.EventChan)
	if got := edit.GetText(); got != "€" {
		t.Fatalf("committed AltGr text = %q, want €", got)
	}
}

func TestGogpuNativeTextUsesCapturedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name  string
		vk    uint16
		text  string
		shift bool
	}{
		{"GermanAt", vtinput.VK_Q, "@", false},
		{"GermanEuro", vtinput.VK_E, "€", false},
		{"PolishShift", vtinput.VK_A, "Ą", true},
		{"MultipleUnicodeRunes", vtinput.VK_E, "€😀", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 8)}
			host := &GogpuHost{reader: reader}
			mods := vtinput.LeftCtrlPressed | vtinput.RightAltPressed
			if tc.shift {
				mods |= vtinput.ShiftPressed
			}
			native := gogpu.NativeKeyEvent{Kind: gogpu.NativeKeyDown,
				VirtualKeyCode: tc.vk, VirtualScanCode: 18, RepeatCount: 2,
				ControlKeyState: uint32(mods), TextExpected: gogpu.NativeTextExpected}
			host.handleNativeKey(native)
			if len(reader.EventChan) != 0 {
				t.Fatal("preliminary shortcut escaped before text")
			}
			// Modifiers may be released and the layout changed before the queued
			// commit callback. Only the source-captured metadata is authoritative.
			host.currentMods = 0
			native.Kind = gogpu.NativeTextCommit
			native.TextInput = true
			edit := NewEdit(0, 0, 30, "")
			want := ""
			for _, char := range tc.text {
				native.Char = char
				host.handleNativeKey(native)
				event := <-reader.EventChan
				if !event.TextInput || event.VirtualKeyCode != tc.vk || event.VirtualScanCode != 18 || event.ControlKeyState != mods || event.RepeatCount != 2 || event.InputSource != "gogpu" {
					t.Fatalf("lost source text intent/physical metadata: %+v", event)
				}
				edit.ProcessKey(event)
				want += string(char) + string(char)
			}
			if edit.GetText() != want {
				t.Fatalf("got %q, want %q", edit.GetText(), want)
			}
			if len(reader.EventChan) != 0 {
				t.Fatal("text delivered twice")
			}
		})
	}
}

func TestGogpuNativeDeadKeyHasNoShortcutFallback(t *testing.T) {
	reader := &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 8)}
	host := &GogpuHost{reader: reader}
	host.handleNativeKey(gogpu.NativeKeyEvent{Kind: gogpu.NativeKeyDown,
		VirtualKeyCode: vtinput.VK_OEM_3, TextExpected: gogpu.NativeTextDeadKey})
	if host.pendingKeyTimer != nil {
		t.Fatal("dead key entered timed shortcut fallback")
	}
	if len(reader.EventChan) != 0 {
		t.Fatal("dead key emitted a shortcut")
	}
	host.handleNativeKey(gogpu.NativeKeyEvent{Kind: gogpu.NativeTextCommit,
		VirtualKeyCode: vtinput.VK_SPACE, Char: '~', TextInput: true, RepeatCount: 1})
	event := <-reader.EventChan
	if !event.TextInput || event.Char != '~' || event.VirtualKeyCode != vtinput.VK_SPACE {
		t.Fatalf("dead-key completion = %+v", event)
	}
	if len(reader.EventChan) != 0 {
		t.Fatal("dead key typed twice")
	}
}

func TestGogpuNativeNonTextAndPlainTypingStayUnmarked(t *testing.T) {
	reader := &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 8)}
	host := &GogpuHost{reader: reader}
	for _, mods := range []vtinput.ControlKeyState{vtinput.RightAltPressed, vtinput.LeftCtrlPressed | vtinput.LeftAltPressed} {
		host.handleNativeKey(gogpu.NativeKeyEvent{Kind: gogpu.NativeKeyDown, VirtualKeyCode: vtinput.VK_F,
			TextExpected: gogpu.NativeTextNone, ControlKeyState: uint32(mods)})
		event := <-reader.EventChan
		if event.TextInput || event.VirtualKeyCode != vtinput.VK_F || event.ControlKeyState != mods {
			t.Fatalf("shortcut changed: %+v", event)
		}
	}
	host.handleNativeKey(gogpu.NativeKeyEvent{Kind: gogpu.NativeTextCommit, VirtualKeyCode: vtinput.VK_A, Char: 'a'})
	if event := <-reader.EventChan; event.TextInput || event.Char != 'a' {
		t.Fatalf("ordinary character shortcut changed: %+v", event)
	}
	host.handleNativeKey(gogpu.NativeKeyEvent{Kind: gogpu.NativeKeyUp, VirtualKeyCode: vtinput.VK_RMENU})
	if event := <-reader.EventChan; event.KeyDown || event.VirtualKeyCode != vtinput.VK_RMENU {
		t.Fatalf("release lost: %+v", event)
	}
}
