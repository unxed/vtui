package vtui

import (
	"golang.org/x/term"
	"os"
)

// MouseTerminalIntegration provides utilities to coordinate mouse event handling
// with golang.org/x/term's terminal state management. This ensures that mouse
// support is properly integrated with terminal raw mode and keyboard protocol enablement.
type MouseTerminalIntegration struct {
	// StdInFd is the file descriptor for standard input (typically os.Stdin.Fd()).
	StdInFd int
	// StdOutFd is the file descriptor for standard output (typically os.Stdout.Fd()).
	StdOutFd int
	// MouseHandler is the handler for mouse events.
	MouseHandler *MouseHandler
	// RawTerminalState holds the original terminal state before entering raw mode.
	// This is used by term.Restore to revert the terminal to its original state.
	RawTerminalState *term.State
	// IsMouseEnabled indicates whether the terminal mouse protocols are active.
	IsMouseEnabled bool
}

// NewMouseTerminalIntegration creates a new integration coordinator with standard I/O descriptors.
func NewMouseTerminalIntegration() *MouseTerminalIntegration {
	return &MouseTerminalIntegration{
		StdInFd:      int(os.Stdin.Fd()),
		StdOutFd:     int(os.Stdout.Fd()),
		MouseHandler: NewMouseHandler(),
		IsMouseEnabled: false,
	}
}

// IsTerminal checks whether stdin is connected to a terminal.
// This determines whether mouse support can be enabled.
func (mti *MouseTerminalIntegration) IsTerminal() bool {
	return term.IsTerminal(mti.StdInFd)
}

// EnableRawMode puts the terminal into raw mode and enables mouse support.
// This should be called before any mouse event processing begins.
// Returns the original terminal state for restoration later.
func (mti *MouseTerminalIntegration) EnableRawMode() error {
	if !term.IsTerminal(mti.StdInFd) {
		return nil // Not a terminal, mouse support not available
	}

	// Make the terminal raw to ensure mouse events are read properly.
	// In raw mode, the terminal does not process input locally and sends
	// all bytes directly to the application.
	state, err := term.MakeRaw(mti.StdInFd)
	if err != nil {
		return err
	}

	mti.RawTerminalState = state
	// Note: Mouse protocol enablement is handled by vtinput.EnableProtocols()
	// which is called in the terminal initialization sequence (see terminal_env.go).
	// This integration layer documents the relationship.
	mti.IsMouseEnabled = true
	return nil
}

// DisableRawMode restores the terminal to its original state.
// This should be called when shutting down mouse event processing.
func (mti *MouseTerminalIntegration) DisableRawMode() error {
	if mti.RawTerminalState != nil && mti.IsMouseEnabled {
		err := term.Restore(mti.StdInFd, mti.RawTerminalState)
		mti.IsMouseEnabled = false
		return err
	}
	return nil
}

// GetMouseHandler returns the MouseHandler instance for direct use.
func (mti *MouseTerminalIntegration) GetMouseHandler() *MouseHandler {
	return mti.MouseHandler
}

// TerminalSupportsMouseReporting reports whether the terminal supports mouse event reporting.
// On Unix systems with ANSI terminal support, this is typically true.
// The actual mouse protocol negotiation is handled by vtinput.EnableProtocols().
func (mti *MouseTerminalIntegration) TerminalSupportsMouseReporting() bool {
	return mti.IsTerminal()
}

// Example usage (documentation):
// ===========================
//
// The typical flow for a mouse-enabled TUI application is:
//
//   1. Create a MouseTerminalIntegration instance
//   2. Enable raw mode (which coordinates with term.MakeRaw)
//   3. Initialize vtui's FrameManager (which handles vtinput protocol setup)
//   4. Process mouse events through the MouseHandler
//   5. Disable raw mode and restore terminal state on shutdown
//
// Example:
//
//   mti := NewMouseTerminalIntegration()
//   if !mti.IsTerminal() {
//       return errors.New("not running in a terminal")
//   }
//
//   if err := mti.EnableRawMode(); err != nil {
//       return err
//   }
//   defer mti.DisableRawMode()
//
//   // Now FrameManager and mouse handling can proceed
//   // Mouse events are available through mti.GetMouseHandler()
