package vtui

import (
	"strings"
	"testing"
)

// A console that owns the alternate screen (a classic conhost window, see
// conhost_altscreen_windows.go) must never be sent DECSET 1049: on Windows
// 10 with "Wrap text output on resize" off, a resize in the host's own VT
// alternate buffer crashes conhost (microsoft/terminal#4308, f4 #397). The
// screen is switched by setAltScreenOS instead, and since that switch moves
// os.Stdout, the clear that follows has to land in the buffer that is now on
// screen -- the one getTermOut() returns after the switch, not before it.
func TestTerminalEnv_ConsoleOwnedAltScreenNeverWrites1049(t *testing.T) {
	primary, alternate := &mockTermOut{}, &mockTermOut{}
	showingAlt := false

	oldGetTermOut, oldOwns := getTermOut, consoleOwnsAltScreen
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		if showingAlt {
			return alternate
		}
		return primary
	}
	consoleOwnsAltScreen = func() bool { return true }
	oldSetAlt := switchAltScreenOS
	switchAltScreenOS = func(enable bool) { showingAlt = enable }
	defer func() {
		getTermOut, consoleOwnsAltScreen, switchAltScreenOS = oldGetTermOut, oldOwns, oldSetAlt
		isPrepared, inAltScreen = false, false
	}()

	isPrepared, inAltScreen = true, false

	SetAltScreen(true)
	if !showingAlt {
		t.Fatal("SetAltScreen(true) did not switch the console buffer")
	}
	if got := primary.builder.String(); got != "" {
		t.Fatalf("the primary buffer was written to on the way in: %q", got)
	}
	if got := alternate.builder.String(); strings.Contains(got, "1049") || !strings.Contains(got, "\x1b[2J\x1b[H") {
		t.Fatalf("alternate buffer got %q, want a clear and no DECSET 1049", got)
	}

	alternate.builder.Reset()
	SetAltScreen(false)
	if showingAlt {
		t.Fatal("SetAltScreen(false) did not switch back")
	}
	if got := alternate.builder.String(); strings.Contains(got, "1049") {
		t.Fatalf("DECSET 1049 written on the way out: %q", got)
	}

	// Suspend writes the cursor and palette reset for the screen the user is
	// about to see, so those must reach the primary buffer.
	inAltScreen = true
	showingAlt = true
	primary.builder.Reset()
	Suspend()
	if showingAlt {
		t.Fatal("Suspend did not switch back to the primary buffer")
	}
	if got := primary.builder.String(); !strings.Contains(got, seqResetPalette) {
		t.Fatalf("Suspend wrote the palette reset elsewhere; primary got %q", got)
	}
	if got := strings.Join([]string{primary.builder.String(), alternate.builder.String()}, ""); strings.Contains(got, "1049") {
		t.Fatalf("DECSET 1049 written during Suspend: %q", got)
	}
}
