//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/gogpu/gpucontext"
	"github.com/unxed/vtinput"
)

func textTestHost(t *testing.T, composes, cmdIsCtrl bool) *GogpuHost {
	t.Helper()
	oldComposes, oldCmd := gogpuAltComposesText, gogpuCmdIsCtrl
	gogpuAltComposesText, gogpuCmdIsCtrl = composes, cmdIsCtrl
	t.Cleanup(func() { gogpuAltComposesText, gogpuCmdIsCtrl = oldComposes, oldCmd })
	return &GogpuHost{reader: &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 16)}}
}

func takeHostEvent(t *testing.T, host *GogpuHost) *vtinput.InputEvent {
	t.Helper()
	select {
	case event := <-host.reader.EventChan:
		return event
	default:
		t.Fatal("expected a host event")
		return nil
	}
}

func noHostEvent(t *testing.T, host *GogpuHost) {
	t.Helper()
	select {
	case event := <-host.reader.EventChan:
		t.Fatalf("unexpected host event: %+v", event)
	default:
	}
}

func TestGogpuHostTextCommitReplacesPendingKey(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		host := textTestHost(t, false, false)
		host.handleKeyPress(gpucontext.KeyA, gpucontext.ModShift)
		noHostEvent(t, host)
		host.handleTextInput("") // Empty callbacks must leave the press pending.
		noHostEvent(t, host)
		host.handleTextInput("Ą😀")
		first, second := takeHostEvent(t, host), takeHostEvent(t, host)
		if first.Char != 'Ą' || first.VirtualKeyCode != vtinput.VK_A || first.RepeatCount != 1 ||
			first.ControlKeyState != vtinput.ShiftPressed || first.TextInput || !first.KeyDown || first.InputSource != "gogpu" {
			t.Fatalf("first committed rune lost its key: %+v", first)
		}
		if second.Char != '😀' || second.VirtualKeyCode != 0 || second.ControlKeyState != vtinput.ShiftPressed ||
			second.TextInput || !second.KeyDown || second.InputSource != "gogpu" {
			t.Fatalf("remaining committed rune: %+v", second)
		}
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		noHostEvent(t, host) // The stopped fallback must not duplicate the commit.
		// A repeat without another text callback uses the learned layout rune.
		host.handleKeyPress(gpucontext.KeyA, 0)
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		if event := takeHostEvent(t, host); event.Char != 'Ą' || event.VirtualKeyCode != vtinput.VK_A {
			t.Fatalf("repeat forgot the committed layout: %+v", event)
		}
		noHostEvent(t, host)
	})
}

func TestGogpuHostPendingKeyFallback(t *testing.T) {
	for _, flush := range []string{"timer", "next press", "release", "unmapped press", "unmapped release"} {
		t.Run(flush, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				host := textTestHost(t, false, false)
				host.handleKeyPress(gpucontext.KeyA, 0)
				// Text arriving through another path can teach the layout while
				// this physical press is still waiting for its callback.
				host.lastRuneForVK = map[uint16]rune{vtinput.VK_A: 'ф'}
				switch flush {
				case "next press":
					host.handleKeyPress(gpucontext.KeyF1, 0)
				case "release":
					host.handleKeyRelease(gpucontext.KeyA, 0)
				case "unmapped press":
					host.handleKeyPress(gpucontext.Key(0), 0)
				case "unmapped release":
					host.handleKeyRelease(gpucontext.Key(0), 0)
				}
				time.Sleep(20 * time.Millisecond)
				synctest.Wait()
				first := takeHostEvent(t, host)
				if first.Char != 'ф' || first.VirtualKeyCode != vtinput.VK_A || !first.KeyDown || first.TextInput {
					t.Fatalf("fallback key must precede the flushing event: %+v", first)
				}
				if flush == "next press" {
					if event := takeHostEvent(t, host); event.VirtualKeyCode != vtinput.VK_F1 || !event.KeyDown {
						t.Fatalf("next command lost: %+v", event)
					}
				}
				if flush == "release" {
					if event := takeHostEvent(t, host); event.VirtualKeyCode != vtinput.VK_A || event.KeyDown {
						t.Fatalf("release lost: %+v", event)
					}
				}
				noHostEvent(t, host)
			})
		})
	}
}

func TestGogpuHostKeypadTextAndFallback(t *testing.T) {
	for _, tc := range []struct {
		name string
		text string
		want rune
	}{
		{"localized decimal", ",", ','},
		{"no platform text", "", '.'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				host := textTestHost(t, false, false)
				host.handleKeyPress(gpucontext.KeyNumpadDecimal, gpucontext.ModNumLock)
				host.handleTextInput(tc.text)
				time.Sleep(20 * time.Millisecond)
				synctest.Wait()
				if event := takeHostEvent(t, host); event.Char != tc.want || event.VirtualKeyCode != vtinput.VK_DECIMAL ||
					event.ControlKeyState != vtinput.NumLockOn || event.TextInput {
					t.Fatalf("keypad insertion: %+v", event)
				}
				noHostEvent(t, host)
			})
		})
	}
}

func TestGogpuHostCommandsStayCommands(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      gpucontext.Key
		mods     gpucontext.Modifiers
		composes bool
		vk       uint16
		char     rune
		state    vtinput.ControlKeyState
		text     string
	}{
		{"Ctrl C", gpucontext.KeyC, gpucontext.ModControl, false, vtinput.VK_C, 0, vtinput.LeftCtrlPressed, ""},
		{"arrow navigation", gpucontext.KeyLeft, 0, false, vtinput.VK_LEFT, 0, 0, ""},
		{"enhanced navigation", gpucontext.KeyHome, 0, false, vtinput.VK_HOME, 0, vtinput.EnhancedKey, ""},
		{"keypad navigation", gpucontext.KeyNumpad1, 0, false, vtinput.VK_END, 0, 0, "1"},
		{"Delete text artifact", gpucontext.KeyDelete, 0, false, vtinput.VK_DELETE, 0, vtinput.EnhancedKey, "\x7f"},
		{"Option accelerator", gpucontext.KeyT, gpucontext.ModAlt, true, vtinput.VK_T, 't', vtinput.LeftAltPressed, "†"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := textTestHost(t, tc.composes, false)
			host.handleKeyPress(tc.key, tc.mods)
			if event := takeHostEvent(t, host); event.VirtualKeyCode != tc.vk || event.Char != tc.char ||
				event.ControlKeyState != tc.state || !event.KeyDown || event.TextInput {
				t.Fatalf("command changed: %+v", event)
			}
			host.handleTextInput(tc.text)
			noHostEvent(t, host)
			// Suppression belongs to one callback, never to subsequent IME text.
			host.handleTextInput("界")
			if event := takeHostEvent(t, host); event.Char != '界' || event.VirtualKeyCode != 0 {
				t.Fatalf("keyless text swallowed: %+v", event)
			}
			noHostEvent(t, host)
		})
	}
}

func TestGogpuHostCommandTextAndCtrlRelease(t *testing.T) {
	for _, cmdFirst := range []bool{false, true} {
		name := "physical Ctrl released first"
		if cmdFirst {
			name = "Command released first"
		}
		t.Run(name, func(t *testing.T) {
			host := textTestHost(t, true, true)
			host.handleKeyPress(gpucontext.KeyLeftControl, gpucontext.ModControl)
			takeHostEvent(t, host)
			host.handleKeyPress(gpucontext.KeyLeftSuper, gpucontext.ModControl|gpucontext.ModSuper)
			takeHostEvent(t, host)
			host.handleKeyPress(gpucontext.KeyC, gpucontext.ModControl|gpucontext.ModSuper)
			if event := takeHostEvent(t, host); event.Char != 0 || event.TextInput || event.VirtualKeyCode != vtinput.VK_C {
				t.Fatalf("Command shortcut changed: %+v", event)
			}
			host.handleTextInput("c") // Consume chord suppression.
			host.handleTextInput("c") // Cmd-held guard must also suppress later text.
			noHostEvent(t, host)
			first, last, mods, vk := gpucontext.KeyLeftControl, gpucontext.KeyLeftSuper, gpucontext.ModSuper, uint16(vtinput.VK_LCONTROL)
			if cmdFirst {
				first, last, mods, vk = last, first, gpucontext.ModControl, vtinput.VK_RCONTROL
			}
			host.handleKeyRelease(first, mods)
			noHostEvent(t, host) // A partial release must not close the switcher.
			host.handleKeyRelease(last, 0)
			if event := takeHostEvent(t, host); event.KeyDown || event.VirtualKeyCode != vk || event.ControlKeyState != 0 {
				t.Fatalf("final Ctrl release: %+v", event)
			}
			noHostEvent(t, host)
		})
	}
}
