package vtui

import (
	"os"
	"sync"
	"github.com/unxed/vtinput"
)

const (
	seqAltScreenOn      = "\x1b[?1049h"
	seqAltScreenOff     = "\x1b[?1049l"
	seqBlinkingUnderline = "\x1b[3 q"
	seqDefaultCursor     = "\x1b[0 q"
	seqResetPalette      = "\x1b]104\x07"
	seqResetAttributes   = "\x1b[0m"
)

var (
	termMu       sync.Mutex
	inputRestore func()
	isPrepared   bool
	inAltScreen  bool
)

// PrepareTerminal puts the terminal into raw mode, enables advanced input,
// and switches to the alternate screen buffer. Returns a restore function.
func PrepareTerminal() (func(), error) {
	err := Resume()
	if err != nil {
		return nil, err
	}
	return Suspend, nil
}

// Suspend fully restores the terminal state (exits raw mode, alternate screen, etc.).
// Useful when temporarily returning control to the shell or an external program.
func Suspend() {
	termMu.Lock()
	defer termMu.Unlock()
	if isPrepared {
		if inAltScreen {
			os.Stdout.WriteString(seqAltScreenOff)
			inAltScreen = false
		}
		os.Stdout.WriteString(seqDefaultCursor + seqResetPalette + seqResetAttributes)
		os.Stdout.Sync()
		if inputRestore != nil {
			inputRestore()
			inputRestore = nil
		}
		isPrepared = false
	}
}

// Resume re-enables raw mode, advanced input, and returns to the alternate screen.
func Resume() error {
	termMu.Lock()
	defer termMu.Unlock()
	if !isPrepared {
		r, err := vtinput.Enable()
		if err != nil {
			return err
		}
		inputRestore = r

		os.Stdout.WriteString(seqBlinkingUnderline)
		if !inAltScreen {
			os.Stdout.WriteString(seqAltScreenOn)
			inAltScreen = true
		}
		os.Stdout.Sync()
		isPrepared = true

		// Force a full redraw if FrameManager is running
		if FrameManager != nil && FrameManager.scr != nil {
			FrameManager.scr.HardReset()
			FrameManager.Redraw()
		}
	}
	return nil
}

// SetAltScreen allows the application to temporarily switch between the
// alternate and main screen buffers without leaving raw mode.
func SetAltScreen(enable bool) {
	termMu.Lock()
	defer termMu.Unlock()

	if inAltScreen != enable {
		inAltScreen = enable
		if enable {
			os.Stdout.WriteString(seqAltScreenOn)
			// When returning to alt screen, it's usually empty, so force a redraw
			if FrameManager != nil && FrameManager.scr != nil {
				FrameManager.scr.HardReset()
				FrameManager.Redraw()
			}
		} else {
			os.Stdout.WriteString(seqAltScreenOff)
		}
		os.Stdout.Sync()
	}
}