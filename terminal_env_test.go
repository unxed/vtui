package vtui

import (
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
