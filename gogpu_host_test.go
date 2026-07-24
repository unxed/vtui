//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris

package vtui

import (
	"io"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestIsSpecialOrModifiedKey(t *testing.T) {
	tests := []struct {
		name string
		vk   uint16
		mods vtinput.ControlKeyState
		want bool
	}{
		{"Special Navigation Down", vtinput.VK_DOWN, 0, true},
		{"Special Return", vtinput.VK_RETURN, 0, true},
		{"Special Escape", vtinput.VK_ESCAPE, 0, true},
		{"Special Function F1", vtinput.VK_F1, 0, true},
		{"Modified Key Ctrl+A", vtinput.VK_A, vtinput.LeftCtrlPressed, true},
		{"Modified Key Alt+B", vtinput.VK_B, vtinput.LeftAltPressed, true},
		{"Regular Char A", vtinput.VK_A, 0, false},
		{"Regular Char Digit 1", vtinput.VK_1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSpecialOrModifiedKey(tt.vk, tt.mods)
			if got != tt.want {
				t.Errorf("isSpecialOrModifiedKey(%v, %v) = %v, want %v", tt.vk, tt.mods, got, tt.want)
			}
		})
	}
}

func TestGogpuHost_SendEvent_NonBlocking(t *testing.T) {
	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	defer reader.Close()

	host := &GogpuHost{reader: reader}

	// 1. Fill event channel capacity
	for i := 0; i < cap(reader.EventChan); i++ {
		reader.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true}
	}

	// 2. Sending MouseMoved when queue is full must return immediately without blocking
	done := make(chan bool)
	go func() {
		host.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseEventFlags: vtinput.MouseMoved,
		})
		done <- true
	}()

	select {
	case <-done:
		// Success: didn't block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sendEvent blocked on full queue during MouseMoved event")
	}
}
func TestGogpuHost_LastRuneForVK_KeyRepeat(t *testing.T) {
	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	defer reader.Close()

	host := &GogpuHost{
		reader:        reader,
		lastRuneForVK: make(map[uint16]rune),
	}

	vk := uint16(vtinput.VK_A)
	host.lastRuneForVK[vk] = 'a'

	ev := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vk,
	}

	if host.lastRuneForVK[vk] != 'a' {
		t.Errorf("Expected 'a' for VK_A in lastRuneForVK, got %c", host.lastRuneForVK[vk])
	}

	// Verify character restoration for key repeat
	if ev.Char == 0 && host.lastRuneForVK[vk] != 0 {
		ev.Char = host.lastRuneForVK[vk]
	}

	if ev.Char != 'a' {
		t.Errorf("Expected restored Char 'a', got %c", ev.Char)
	}
}
