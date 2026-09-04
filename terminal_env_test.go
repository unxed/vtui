package vtui

import (
	"bytes"
	"github.com/unxed/vtinput"
	"os"
	"strings"
	"testing"
)

type mockTermOut struct {
	builder strings.Builder
}

func (m *mockTermOut) WriteString(s string) (int, error) {
	return m.builder.WriteString(s)
}
func (m *mockTermOut) Sync() error { return nil }

func TestTerminalEnv_AltScreenManagement(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// Reset internal state
	isPrepared = true
	inAltScreen = true

	// 1. Test switching AltScreen OFF
	SetAltScreen(false)
	if inAltScreen {
		t.Error("inAltScreen should be false")
	}
	if !strings.Contains(mock.builder.String(), seqAltScreenOff) {
		t.Errorf("AltScreen OFF sequence missing, got %q", mock.builder.String())
	}

	mock.builder.Reset()

	// 2. Test switching AltScreen ON
	SetAltScreen(true)
	if !inAltScreen {
		t.Error("inAltScreen should be true")
	}
	if !strings.Contains(mock.builder.String(), seqAltScreenOn) {
		t.Errorf("AltScreen ON sequence missing, got %q", mock.builder.String())
	}
}

func TestTerminalEnv_Suspend(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// Simulate active TUI in AltScreen
	isPrepared = true
	inAltScreen = true
	inputRestore = func() {}

	Suspend()

	if isPrepared {
		t.Error("isPrepared should be false after Suspend")
	}
	if inAltScreen {
		t.Error("inAltScreen should be false after Suspend")
	}

	output := mock.builder.String()
	if !strings.Contains(output, seqAltScreenOff) {
		t.Error("Suspend did not exit AltScreen")
	}
	if !strings.Contains(output, seqDefaultCursor) {
		t.Error("Suspend did not restore default cursor")
	}
}

func TestTerminalEnv_ManageCursorDisabled(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// 1. Disable cursor management
	ManageCursorStyle = false
	isPrepared = false
	inAltScreen = false
	inputRestore = func() {}

	// 2. Resume
	Resume()
	if strings.Contains(mock.builder.String(), seqBlinkingUnderline) {
		t.Error("seqBlinkingUnderline sent even though ManageCursorStyle is false")
	}

	mock.builder.Reset()

	// 3. Suspend
	isPrepared = true
	Suspend()
	if strings.Contains(mock.builder.String(), seqDefaultCursor) {
		t.Error("seqDefaultCursor sent even though ManageCursorStyle is false")
	}

	// Reset global state for other tests
	ManageCursorStyle = true
}
func TestTerminalEnv_AutoWrap(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	// 1. Test Suspend restores AutoWrap (safely without calling vtinput.Enable)
	isPrepared = true
	inAltScreen = true
	inputRestore = func() {}

	Suspend()

	output := mock.builder.String()
	if !strings.Contains(output, seqAutoWrapOn) {
		t.Error("Suspend did not restore auto-wrap")
	}

	// 2. Test Resume writes AutoWrapOff (ignore ioctl error in headless test environment)
	isPrepared = false
	inAltScreen = false
	inputRestore = nil
	mock.builder.Reset()

	_ = Resume()

	output = mock.builder.String()
	if !strings.Contains(output, seqAutoWrapOff) {
		t.Error("Resume did not write seqAutoWrapOff")
	}
}
func TestAnsiRendererCursorStyle(t *testing.T) {
	oldTerm := os.Getenv("TERM")
	defer os.Setenv("TERM", oldTerm)

	tests := []struct {
		name       string
		term       string
		shape      CursorShape
		wantCursor string
	}{
		{
			name:       "Standard Underline",
			term:       "xterm-256color",
			shape:      CursorShapeUnderline,
			wantCursor: "\x1b[3 q",
		},
		{
			name:       "Standard Block",
			term:       "xterm-256color",
			shape:      CursorShapeBlock,
			wantCursor: "\x1b[1 q",
		},
		{
			name:       "Linux Console Underline",
			term:       "linux",
			shape:      CursorShapeUnderline,
			wantCursor: "\x1b[?3c",
		},
		{
			name:       "Linux Console Block",
			term:       "linux",
			shape:      CursorShapeBlock,
			wantCursor: "\x1b[?6c",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			os.Setenv("TERM", tc.term)

			scr := NewScreenBuf()
			var buf bytes.Buffer
			scr.Writer = &buf

			scr.AllocBuf(10, 10)
			scr.SetCursorPos(1, 1)
			scr.SetCursorVisible(true)
			scr.SetCursorShape(tc.shape)

			scr.Flush()

			out := buf.String()
			if !strings.Contains(out, tc.wantCursor) {
				t.Errorf("expected output to contain %q, got %q", tc.wantCursor, out)
			}
		})
	}
}

func TestSetCursorStyleOS(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("SetCursorStyleOS panicked: %v", r)
		}
	}()
	SetCursorStyleOS(true, CursorShapeUnderline)
	SetCursorStyleOS(false, CursorShapeBlock)
}
func TestInitTerminalOS_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("initTerminalOS panicked: %v", r)
		}
	}()
	initTerminalOS()
}

// TestTerminalEnv_NoVTWhenWin32ConsoleOwnsScreen covers the fix for
// WINE.md §2j.4/P4: when the Win32ConsoleRenderer owns the visible screen
// through its own dedicated console buffer, getTermOut() is the *other*
// buffer (hStdOut) -- painted with WriteConsoleOutputW, not a VT stream --
// and Suspend/Resume/SetAltScreen must not write ANSI escape sequences into
// it. Not gating this was the leading suspect for f4's "console overlay
// renders at the top of the window" bug under Wine: literal escape bytes
// (or a second, VT-driven alt-screen toggle racing the Win32 one) landing in
// the buffer the user is actually looking at.
func TestTerminalEnv_NoVTWhenWin32ConsoleOwnsScreen(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	oldConsoleUsesVT := consoleUsesVT
	consoleUsesVT = func() bool { return false }
	defer func() {
		getTermOut = oldGetTermOut
		consoleUsesVT = oldConsoleUsesVT
	}()

	assertNoVTBytes := func(step string) {
		t.Helper()
		if mock.builder.Len() != 0 {
			t.Errorf("%s wrote %d bytes into the non-VT buffer: %q", step, mock.builder.Len(), mock.builder.String())
		}
		mock.builder.Reset()
	}

	isPrepared = false
	inAltScreen = false
	inputRestore = nil
	_ = Resume()
	assertNoVTBytes("Resume")
	// Resume leaves isPrepared=false if vtinput.Enable() errors in this
	// headless test environment (same caveat as TestTerminalEnv_AutoWrap);
	// force the state Suspend/SetAltScreen expect regardless of that.
	isPrepared = true
	inAltScreen = true
	inputRestore = func() {}

	SetAltScreen(false)
	assertNoVTBytes("SetAltScreen(false)")

	SetAltScreen(true)
	assertNoVTBytes("SetAltScreen(true)")

	Suspend()
	assertNoVTBytes("Suspend")

	// Sanity check: the same sequence WITH consoleUsesVT reporting true (the
	// normal, non-Win32-console case) must still produce output, so the test
	// above is verifying the gate and not a broken mock.
	consoleUsesVT = func() bool { return true }
	isPrepared = true
	inAltScreen = true
	SetAltScreen(false)
	if mock.builder.Len() == 0 {
		t.Error("SetAltScreen(false) wrote nothing even though consoleUsesVT() is true")
	}
}

func TestTerminalEnv_FreeBSDConsoleUsesOnlyRawInput(t *testing.T) {
	oldFreeBSDConsole := IsFreeBSDConsole
	defer func() { IsFreeBSDConsole = oldFreeBSDConsole }()

	IsFreeBSDConsole = true
	if got := terminalInputProtocols(); got != 0 {
		t.Fatalf("FreeBSD console protocols = %#x, want raw input only", got)
	}

	IsFreeBSDConsole = false
	if got := terminalInputProtocols(); got != vtinput.DefaultProtocols {
		t.Fatalf("regular terminal protocols = %#x, want %#x", got, vtinput.DefaultProtocols)
	}
}

func TestTerminalEnv_FreeBSDConsoleAvoidsUnsupportedSequences(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	oldEnableInput := enableTerminalInput
	oldFreeBSDConsole := IsFreeBSDConsole
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	enableTerminalInput = func() (func(), error) { return func() {}, nil }
	IsFreeBSDConsole = true
	defer func() {
		getTermOut = oldGetTermOut
		enableTerminalInput = oldEnableInput
		IsFreeBSDConsole = oldFreeBSDConsole
		isPrepared = false
		inAltScreen = false
		inputRestore = nil
	}()

	isPrepared = false
	inAltScreen = false
	inputRestore = nil
	if err := Resume(); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	SetAltScreen(false)
	SetAltScreen(true)
	Suspend()

	output := mock.builder.String()
	for _, unsupported := range []string{"\x1b[?", "\x1b[>", "\x1b[<", "\x1b]"} {
		if strings.Contains(output, unsupported) {
			t.Errorf("FreeBSD console output contains unsupported sequence prefix %q: %q", unsupported, output)
		}
	}
}

// On a classic Windows console window the cursor shape is set with
// SetConsoleCursorInfo only: conhost draws DECSCUSR's underline styles as
// a one-pixel hairline (f4 #219), so the sequence must not reach it.
func TestAnsiRendererCursorStyle_ClassicConsoleSkipsDECSCUSR(t *testing.T) {
	oldTerm := os.Getenv("TERM")
	defer os.Setenv("TERM", oldTerm)
	os.Setenv("TERM", "xterm-256color")

	oldVia := cursorStyleViaConsoleAPI
	cursorStyleViaConsoleAPI = func() bool { return true }
	defer func() { cursorStyleViaConsoleAPI = oldVia }()

	for _, shape := range []CursorShape{CursorShapeUnderline, CursorShapeBlock} {
		scr := NewScreenBuf()
		var buf bytes.Buffer
		scr.Writer = &buf

		scr.AllocBuf(10, 10)
		scr.SetCursorPos(1, 1)
		scr.SetCursorVisible(true)
		scr.SetCursorShape(shape)
		scr.Flush()

		out := buf.String()
		if !strings.Contains(out, "\x1b[?25h") {
			t.Errorf("shape %d: cursor visibility (DECTCEM) not sent: %q", shape, out)
		}
		for _, seq := range []string{"\x1b[1 q", "\x1b[3 q", "\x1b]1337;CursorShape="} {
			if strings.Contains(out, seq) {
				t.Errorf("shape %d: %q sent to a classic console: %q", shape, seq, out)
			}
		}
	}
}

func TestTerminalEnv_ClassicConsoleSkipsDECSCUSR(t *testing.T) {
	mock := &mockTermOut{}
	oldGetTermOut := getTermOut
	getTermOut = func() interface {
		WriteString(string) (int, error)
		Sync() error
	} {
		return mock
	}
	defer func() { getTermOut = oldGetTermOut }()

	oldVia := cursorStyleViaConsoleAPI
	cursorStyleViaConsoleAPI = func() bool { return true }
	defer func() { cursorStyleViaConsoleAPI = oldVia }()

	oldEnable := enableTerminalInput
	enableTerminalInput = func() (func(), error) { return func() {}, nil }
	defer func() { enableTerminalInput = oldEnable }()

	ManageCursorStyle = true
	isPrepared = false
	inAltScreen = false
	inputRestore = func() {}
	consoleCursorTypeStale = false

	Resume()
	if strings.Contains(mock.builder.String(), seqBlinkingUnderline) {
		t.Error("seqBlinkingUnderline sent to a classic console")
	}
	if !consoleCursorTypeStale {
		t.Error("Resume did not mark the console cursor type as stale")
	}

	mock.builder.Reset()
	isPrepared = true
	Suspend()
	if strings.Contains(mock.builder.String(), seqDefaultCursor) {
		t.Error("seqDefaultCursor sent to a classic console")
	}
}
