//go:build windows

package vtui

import "testing"

// TestUseWinescapeProbe covers the backend-selection logic that decides
// between the libwinescape native-tty path and the existing Windows Console
// API path, without touching a real Wine install: winescapeAvailable is
// swapped out for a fake answer, the way f4's own conditional-backend code
// (e.g. hostmode.Posix()) is tested against an injected probe.
func TestUseWinescapeProbe(t *testing.T) {
	origEnabled := WinescapeGraphicsProbeEnabled
	origAvailable := winescapeAvailable
	t.Cleanup(func() {
		WinescapeGraphicsProbeEnabled = origEnabled
		winescapeAvailable = origAvailable
	})

	cases := []struct {
		name      string
		enabled   bool
		available bool
		want      bool
	}{
		{"off by default, even if winescape would be available", false, true, false},
		{"opted in, but winescape.Available() says no", true, false, false},
		{"opted in and available: takes the native path", true, true, true},
		{"neither opted in nor available", false, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			WinescapeGraphicsProbeEnabled = c.enabled
			winescapeAvailable = func() bool { return c.available }
			if got := useWinescapeProbe(); got != c.want {
				t.Errorf("useWinescapeProbe() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestWinescapeGraphicsProbeDefaultsOff guards the "off by default" promise
// itself: a build that never touches WinescapeGraphicsProbeEnabled must keep
// today's Windows Console API behavior.
func TestWinescapeGraphicsProbeDefaultsOff(t *testing.T) {
	if WinescapeGraphicsProbeEnabled {
		t.Fatal("WinescapeGraphicsProbeEnabled must default to false")
	}
}
