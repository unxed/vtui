//go:build !windows

package vtui

// The clipboard of a graphical session now goes through goclip. These check
// the two things that switch has to get right: that a real session round
// trips, and that a session with no graphical shell behind it reports failure
// rather than success, because a false success would stop SetClipboard
// falling through to OSC 52.
//
// The first needs a display. Start one with
//
//	Xvfb :99 -screen 0 800x600x24 &
//	DISPLAY=:99 go test .
//
// and it runs; without one it skips.

import (
	"os"
	"strings"
	"testing"
)

func TestOSClipboardRoundTrip(t *testing.T) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no graphical session")
	}
	if graphicalClipboard() == nil {
		t.Skip("goclip found no graphical driver here")
	}

	const text = "путь/к/файлу с пробелом.txt"
	if !setOSClipboard(text) {
		t.Fatal("the graphical clipboard refused a copy")
	}
	got, ok := getOSClipboard()
	if !ok {
		t.Fatal("the graphical clipboard refused a paste")
	}
	if got != text {
		t.Errorf("clipboard: got %q, want %q", got, text)
	}
}

// Anything past a couple of hundred kilobytes is handed over in pieces, which
// is where the old xclip path and goclip's own native path both used to lose
// the text.
func TestOSClipboardLargeRoundTrip(t *testing.T) {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		t.Skip("no graphical session")
	}
	if graphicalClipboard() == nil {
		t.Skip("goclip found no graphical driver here")
	}

	want := strings.Repeat("сорок два.", 40000)
	if !setOSClipboard(want) {
		t.Fatal("the graphical clipboard refused a large copy")
	}
	got, ok := getOSClipboard()
	if !ok {
		t.Fatal("the graphical clipboard refused a large paste")
	}
	if got != want {
		t.Errorf("large clipboard came back as %d bytes, want %d", len(got), len(want))
	}
}

// With no display and no Wayland socket there is no graphical clipboard, and
// saying so is what lets SetClipboard reach OSC 52. The file backed driver
// goclip would otherwise fall to must not be allowed to answer here.
func TestOSClipboardWithoutASession(t *testing.T) {
	display, hadDisplay := os.LookupEnv("DISPLAY")
	wayland, hadWayland := os.LookupEnv("WAYLAND_DISPLAY")
	os.Unsetenv("DISPLAY")
	os.Unsetenv("WAYLAND_DISPLAY")
	t.Cleanup(func() {
		if hadDisplay {
			os.Setenv("DISPLAY", display)
		}
		if hadWayland {
			os.Setenv("WAYLAND_DISPLAY", wayland)
		}
	})

	// A driver that has already connected stays connected, so this only
	// means anything on a process that never had a session. Skipping is
	// honest; asserting on a cached connection would not be.
	if graphicalClipboard() != nil {
		t.Skip("a driver from an earlier test is still connected")
	}
	if setOSClipboard("anything") {
		t.Error("there is no graphical clipboard to copy to")
	}
	if _, ok := getOSClipboard(); ok {
		t.Error("there is no graphical clipboard to paste from")
	}
}

// The file driver is goclip's last resort and vtui must never pick it: it
// always succeeds, and a success is what stops the fall through to OSC 52.
func TestGraphicalClipboardSkipsTheFileDriver(t *testing.T) {
	if d := graphicalClipboard(); d != nil && d.Name() == "file" {
		t.Error("the file backed driver is not a graphical clipboard")
	}
}
