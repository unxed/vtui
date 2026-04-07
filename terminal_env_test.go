package vtui

import (
	"strings"
	"testing"
	"os"
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
	termOut = mock
	defer func() { termOut = os.Stdout }()

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
	termOut = mock
	defer func() { termOut = os.Stdout }()

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