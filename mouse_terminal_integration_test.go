package vtui

import (
	"testing"
	"os"
	"golang.org/x/term"
)

func TestMouseTerminalIntegration_NewInstance(t *testing.T) {
	mti := NewMouseTerminalIntegration()

	if mti.StdInFd != int(os.Stdin.Fd()) {
		t.Errorf("Expected StdInFd to be %d, got %d", os.Stdin.Fd(), mti.StdInFd)
	}

	if mti.MouseHandler == nil {
		t.Error("Expected MouseHandler to be initialized")
	}

	if mti.IsMouseEnabled {
		t.Error("Expected IsMouseEnabled to be false initially")
	}
}

func TestMouseTerminalIntegration_IsTerminal(t *testing.T) {
	mti := NewMouseTerminalIntegration()

	// This test may pass or fail depending on whether the test is run in a terminal.
	// We just verify that the method returns a boolean without error.
	_ = mti.IsTerminal()
}

func TestMouseTerminalIntegration_TerminalSupportsMouseReporting(t *testing.T) {
	mti := NewMouseTerminalIntegration()

	// Should return consistent results
	result1 := mti.TerminalSupportsMouseReporting()
	result2 := mti.TerminalSupportsMouseReporting()

	if result1 != result2 {
		t.Error("Expected consistent results from TerminalSupportsMouseReporting")
	}
}

func TestMouseTerminalIntegration_DisableRawMode_WithoutEnable(t *testing.T) {
	mti := NewMouseTerminalIntegration()

	// Calling DisableRawMode without EnableRawMode should be safe
	err := mti.DisableRawMode()
	if err != nil {
		t.Errorf("Expected no error from DisableRawMode without prior EnableRawMode, got %v", err)
	}
}

func TestMouseTerminalIntegration_GetMouseHandler(t *testing.T) {
	mti := NewMouseTerminalIntegration()

	handler := mti.GetMouseHandler()
	if handler != mti.MouseHandler {
		t.Error("Expected GetMouseHandler to return the same instance")
	}

	if handler == nil {
		t.Error("Expected GetMouseHandler to return a non-nil handler")
	}
}

func TestMouseTerminalIntegration_RawModeStateTracking(t *testing.T) {
	mti := NewMouseTerminalIntegration()

	// If we're in a terminal, test the raw mode state tracking
	if term.IsTerminal(int(os.Stdin.Fd())) {
		// Note: We skip actually enabling raw mode in tests to avoid side effects
		// but we can verify the state variables are initialized correctly
		if mti.RawTerminalState != nil {
			t.Error("Expected RawTerminalState to be nil before EnableRawMode")
		}

		if mti.IsMouseEnabled {
			t.Error("Expected IsMouseEnabled to be false before EnableRawMode")
		}
	}
}

func TestMouseTerminalIntegration_Integration_WithMouseHandler(t *testing.T) {
	mti := NewMouseTerminalIntegration()
	handler := mti.GetMouseHandler()

	// Verify the handler works through the integration layer
	if handler.WheelScrollLines != 3 {
		t.Errorf("Expected default WheelScrollLines to be 3, got %d", handler.WheelScrollLines)
	}

	// Verify we can configure the handler
	handler.DoubleClickThreshold = 200
	if mti.GetMouseHandler().DoubleClickThreshold != handler.DoubleClickThreshold {
		t.Error("Expected configuration to persist through integration layer")
	}
}
