package vtui

import (
	"github.com/unxed/vtinput"
	"os"
	"sync"
)

const (
	seqAltScreenOn       = "\x1b[?1049h\x1b[2J\x1b[H"
	seqAltScreenOff      = "\x1b[?1049l"
	seqAutoWrapOff       = "\x1b[?7l"
	seqAutoWrapOn        = "\x1b[?7h"
	seqBlinkingUnderline = "\x1b[3 q"
	seqDefaultCursor     = "\x1b[0 q"
	seqResetPalette      = "\x1b]104\x07"
	seqResetAttributes   = "\x1b[0m"
	// vtui hands the terminal rows in visual order (see bidi.go), so a
	// terminal that runs the bidi algorithm itself (VTE, and everything
	// following the terminal-wg BiDi recommendation) must be told not to
	// reorder them a second time: BDSM reset selects explicit mode, SCP the
	// left to right character path. Terminals without bidi support ignore
	// both. The pair is restored to the defaults (implicit, unset) on the
	// way out. Never do this with bidi control characters: they are text,
	// they take a cell in some terminals and none in others, and the
	// recommendation discards them in implicit mode anyway.
	seqBidiExplicitLTR = "\x1b[8l\x1b[1 k"
	seqBidiImplicit    = "\x1b[8h\x1b[0 k"
)

var (
	termMu            sync.Mutex
	inputRestore      func()
	isPrepared        bool
	inAltScreen       bool
	ManageCursorStyle bool = true

	// consoleCursorTypeStale is set whenever something other than vtui may
	// have changed the console's cursor *type* (Legacy, Underscore,
	// VerticalBar...): at start, and after every Resume, since a shell or
	// child that ran meanwhile may have sent DECSCUSR. Only the classic
	// Windows console path reads it; see SetCursorStyleOS in
	// terminal_env_windows.go.
	consoleCursorTypeStale = true
)

// cursorStyleViaConsoleAPI reports whether the cursor shape must be set
// through the Win32 console API alone, with no DECSCUSR (CSI Ps SP q) on the
// VT stream. That is the case on a classic conhost window (cmd.exe's, not a
// pseudoconsole): conhost does understand DECSCUSR, but what it draws for
// the "underline" styles 3/4 is CursorType::Underscore -- a single pixel row
// at the bottom of the cell, whatever the font size (renderer/gdi/paint.cpp)
// -- while its default CursorType::Legacy is the classic cursor whose height
// is the dwSize percentage from SetConsoleCursorInfo, the one Far and cmd.exe
// show. SetConsoleCursorInfo itself only puts the type back to Legacy when
// the size actually changes (microsoft/terminal#4124), so any DECSCUSR that
// reaches conhost after the API call wins and leaves the hairline cursor
// behind (f4 #219). Everywhere else -- pseudoconsoles like Windows Terminal,
// Wine, every Unix terminal -- DECSCUSR is the right request and this is
// false. A test can override it.
var cursorStyleViaConsoleAPI = func() bool {
	return cursorStyleViaConsoleAPIOS()
}

func terminalInputProtocols() vtinput.Protocol {
	// FreeBSD's direct console is not an xterm-compatible byte sink.  In
	// particular, the syscons parser treats the bytes after an unknown DEC
	// private-mode introducer as printable text.  Raw mode is still required,
	// but enabling mouse, focus, paste and keyboard protocols would paint their
	// numeric mode names onto the screen.
	if IsFreeBSDConsole {
		return 0
	}
	return vtinput.DefaultProtocols
}

var enableTerminalInput = func() (func(), error) {
	return vtinput.EnableProtocols(terminalInputProtocols())
}

var getTermOut = func() interface {
	WriteString(string) (int, error)
	Sync() error
} {
	return os.Stdout
}

// consoleUsesVT reports whether the buffer getTermOut() writes to is a VT
// stream that understands ANSI escape sequences. It is false exactly when
// the Win32ConsoleRenderer owns the visible screen through its own
// dedicated console buffer (see win32ConsoleActive): os.Stdout there is the
// *other* buffer -- the one the user sees after the renderer switches away,
// e.g. hStdOut under WINE.md's no-PTY console view -- and it is painted
// directly with WriteConsoleOutputW, not interpreted as text. Writing
// "\x1b[?1049h" into it does not toggle any alternate screen; it either
// prints seven junk characters or, if some other layer does interpret it,
// fights the Win32 Console API's own idea of which buffer is active and
// where the cursor sits. A test can override this to exercise the gated
// path without a real Win32 console.
var consoleUsesVT = func() bool {
	return !win32ConsoleActive()
}

// termOut is what getTermOut returns: the VT stream f4 draws into.
type termOut = interface {
	WriteString(string) (int, error)
	Sync() error
}

// consoleOwnsAltScreen reports whether the alternate screen is a second
// console screen buffer of f4's own, switched by setAltScreenOS, rather than
// the terminal's alternate screen requested with DECSET 1049. It is true on
// a classic Windows console window (conhost with a window of its own, see
// conhost_altscreen_windows.go) and false everywhere else.
//
// When it is true, DECSET 1049 is never written: on a classic console with
// "Wrap text output on resize" turned off, a window resize while the host's
// own VT alternate buffer is up crashes conhost (microsoft/terminal#4308,
// f4 #397). The switch itself is what shows and hides f4's screen; the VT
// stream only gets a clear on the way in -- and since getTermOut() follows
// the switch, that clear has to be written after setAltScreenOS(true), into
// the buffer that is now on screen, not before it into the one that was.
var consoleOwnsAltScreen = func() bool { return false }

// switchAltScreenOS is setAltScreenOS behind a variable, so that a test can
// stand in for the platform's buffer switch without a console to switch.
var switchAltScreenOS = setAltScreenOS

// enterAltScreen shows f4's screen and returns the stream that now reaches
// it, which is not always the one the caller had: on a console that owns
// the alternate screen the switch moves os.Stdout to the other buffer.
func enterAltScreen(out termOut, vt, modernVT bool) termOut {
	if consoleOwnsAltScreen() {
		switchAltScreenOS(true)
		out = getTermOut()
		if vt {
			out.WriteString("\x1b[2J\x1b[H")
		}
		return out
	}
	if modernVT {
		out.WriteString(seqAltScreenOn)
	} else if vt {
		// A direct FreeBSD console has no alternate screen.  Clear it
		// with the plain ANSI operations its emulator implements.
		out.WriteString("\x1b[2J\x1b[H")
	}
	switchAltScreenOS(true)
	return out
}

// leaveAltScreen hides f4's screen and returns the stream that now reaches
// the one the user sees. What is written afterwards (cursor shape, palette
// reset) belongs there.
func leaveAltScreen(out termOut, modernVT bool) termOut {
	if consoleOwnsAltScreen() {
		switchAltScreenOS(false)
		return getTermOut()
	}
	if modernVT {
		out.WriteString(seqAltScreenOff)
	}
	switchAltScreenOS(false)
	return out
}

// PrepareTerminal puts the terminal into raw mode, enables advanced input,
// and switches to the alternate screen buffer. Returns a restore function.
func PrepareTerminal() (func(), error) {
	initTerminalOS()
	err := Resume()
	if err != nil {
		return nil, err
	}
	return Suspend, nil
}

// Suspend fully restores the terminal state (exits raw mode, alternate screen, etc.).
// Useful when temporarily returning control to the shell or an external program.
// IsPrepared reports whether the terminal is currently in the raw/alt-screen
// state (between Resume and Suspend). Frame flushes must not reach the host
// terminal outside that window: Suspend has already reset the palette and
// attributes, and a late frame would repaint the theme palette (OSC 4) right
// over the user's restored shell.
func IsPrepared() bool {
	termMu.Lock()
	defer termMu.Unlock()
	return isPrepared
}

func Suspend() {
	termMu.Lock()
	defer termMu.Unlock()
	if isPrepared {
		out := getTermOut()
		vt := consoleUsesVT()
		modernVT := vt && !IsFreeBSDConsole
		if modernVT {
			if DefaultBidiMode != BidiOff {
				out.WriteString(seqBidiImplicit)
			}
			out.WriteString(seqAutoWrapOn) // Restore auto-wrap
		}
		if inAltScreen {
			out = leaveAltScreen(out, modernVT)
			inAltScreen = false
		} else {
			setAltScreenOS(false)
		}
		if modernVT {
			if ManageCursorStyle {
				if cursorStyleViaConsoleAPI() {
					restoreConsoleCursorOS()
				} else {
					out.WriteString(seqDefaultCursor)
				}
			}
			out.WriteString(seqResetPalette + seqResetAttributes)
		} else if vt {
			// syscons' native local-cursor command.  Unlike DECTCEM it is
			// understood by the sc terminal emulator and does not leak digits
			// onto the visible screen.
			out.WriteString("\x1b[=0S" + seqResetAttributes)
		}
		out.Sync()
		// seqResetPalette (OSC 104) throws away the colors we loaded into
		// the terminal. Unless the screen buffer is told, it keeps believing
		// the terminal still holds them and stays silent on the next Resume
		// or re-attach, leaving the session with default colors.
		if FrameManager != nil && FrameManager.scr != nil {
			FrameManager.scr.InvalidateHostPalette()
		}
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
	return resumeLocked(true)
}

// ResumeWithoutAltScreen re-enables raw mode and advanced input exactly like
// Resume, but never touches which screen buffer is active (no AltScreen
// enter, no setAltScreenOS call, no forced FrameManager redraw).
//
// Resume() unconditionally switches to f4's own alternate screen buffer as
// its first step, on the assumption that "resuming" means "going back to
// showing our own UI". That assumption is wrong for a caller who suspended
// only to hand the terminal to a child process while deliberately staying on
// the *other* buffer (e.g. f4's no-PTY console view, ConsoleMode=own /
// ConsoleViewFar: WriteConsoleOutputW painted its overlay directly onto the
// host buffer and wants to keep that buffer visible). Such a caller used to
// have to call Resume() anyway just to get vtinput re-enabled, then
// immediately call SetAltScreen(false) to undo the unwanted switch --
// producing two SetConsoleActiveScreenBuffer calls (host buffer -> f4's own
// buffer -> host buffer again) within a handful of milliseconds. Real
// Windows consoles handle that synchronously and it is invisible; Wine's
// console frontends have a documented history of mishandling rapid active-
// screen-buffer switches with an async/stale-snapshot repaint (see f4's
// WINE.md §2f-§2g and the "single-line command output vanishes after
// Ctrl+O under Wine" report this function was added for). Since the caller
// already knows the correct buffer is showing, skip the switch entirely
// instead of doing it and immediately undoing it.
func ResumeWithoutAltScreen() error {
	termMu.Lock()
	defer termMu.Unlock()
	return resumeLocked(false)
}

// resumeLocked is Resume()'s body, parameterized on whether to perform the
// AltScreen-enter step. Callers must hold termMu.
func resumeLocked(withAltScreen bool) error {
	if !isPrepared {
		out := getTermOut()
		vt := consoleUsesVT()
		modernVT := vt && !IsFreeBSDConsole

		if withAltScreen {
			// 1. Enter AltScreen FIRST. Many terminals (like Kitty) reset
			// their keyboard protocol state when switching screen buffers.
			if !inAltScreen {
				out = enterAltScreen(out, vt, modernVT)
				inAltScreen = true
			} else {
				setAltScreenOS(true)
			}
			if modernVT {
				out.WriteString(seqAutoWrapOff) // Disable auto-wrap for exact rendering
				if DefaultBidiMode != BidiOff {
					out.WriteString(seqBidiExplicitLTR)
				}
			}
			out.Sync()
		}

		// 2. Enable advanced input protocols AFTER entering AltScreen.
		r, err := enableTerminalInput()
		if err != nil && !vt {
			// On Windows without VT input support (Windows 7/8/8.1), ReadConsoleInput
			// works in standard console mode without ENABLE_VIRTUAL_TERMINAL_INPUT.
			err = nil
		}
		if err != nil {
			// Rollback AltScreen if input setup failed
			if withAltScreen {
				out = leaveAltScreen(out, modernVT)
				inAltScreen = false
				out.Sync()
			}
			return err
		}
		inputRestore = r

		// Whatever ran while f4 was suspended may have restyled the
		// console cursor; the next SetCursorStyleOS has to assume so.
		consoleCursorTypeStale = true
		if modernVT && ManageCursorStyle && !cursorStyleViaConsoleAPI() {
			out.WriteString(seqBlinkingUnderline)
		}
		out.Sync()
		isPrepared = true

		if withAltScreen {
			// Force a full redraw if FrameManager is running. Only relevant
			// when we actually switched back to f4's own buffer -- with
			// ResumeWithoutAltScreen the host buffer stays visible and
			// there is nothing of f4's own to redraw yet.
			if FrameManager != nil && FrameManager.scr != nil {
				FrameManager.scr.HardReset()
				FrameManager.Redraw()
			}
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
		out := getTermOut()
		vt := consoleUsesVT()
		modernVT := vt && !IsFreeBSDConsole
		if enable {
			out = enterAltScreen(out, vt, modernVT)
			// When returning to alt screen, it's usually empty, so force a redraw
			if FrameManager != nil && FrameManager.scr != nil {
				FrameManager.scr.HardReset()
				FrameManager.Redraw()
			}
		} else {
			out = leaveAltScreen(out, modernVT)
		}
		out.Sync()
	}
}
