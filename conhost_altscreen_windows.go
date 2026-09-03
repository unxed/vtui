//go:build windows

package vtui

import (
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The alternate screen on a classic Windows console window.
//
// Everywhere else f4 asks the terminal for its alternate screen with DECSET
// 1049 and lets it do the rest. conhost understands the sequence too, but
// what it builds behind it is a second SCREEN_INFORMATION that follows the
// window through a resize path of its own, and on Windows 10 that path
// falls over when "Wrap text output on resize" is turned off in the
// shortcut: the window vanishes, conhost.exe is gone, and every console call
// f4 makes from then on answers ERROR_PIPE_NOT_CONNECTED. To the user that is
// f4 crashing -- with no crash log, because nothing in f4 crashed
// (microsoft/terminal#4308, f4 #397). No retry helps; the host is dead.
//
// Far never sees this because Far never sends 1049. It makes a console screen
// buffer of its own with CreateConsoleScreenBuffer, shows it with
// SetConsoleActiveScreenBuffer, and puts the original back on the way out --
// which is also exactly the behaviour the alternate screen is for. So on a
// classic console window that is what f4 does now, and the VT stream still
// draws into that buffer as before: VT processing is a per-buffer mode, and
// everything vtui writes goes through os.Stdout, which moves with the switch.
//
// This applies only to a console with a real window of its own. Under a
// pseudoconsole (Windows Terminal, and anything else on the far side of a
// pty) the terminal implements the alternate screen itself and 1049 is the
// right request; Wine's console frontends have a documented history with
// rapid active-buffer switches (f4's WINE.md) and keep 1049 too; and the
// Win32 console renderer already has a buffer of its own.

var (
	procGetConsoleWindowAlt = kernel32.NewProc("GetConsoleWindow")
	procGetClassNameWAlt    = user32.NewProc("GetClassNameW")
	procIsWindowVisibleAlt  = user32.NewProc("IsWindowVisible")
)

type conhostAltScreen struct {
	orig    syscall.Handle // the buffer f4 was started in
	own     syscall.Handle // f4's screen
	origOut *os.File
	ownOut  *os.File
	on      bool
}

var (
	conhostAltMu    sync.Mutex
	conhostAlt      *conhostAltScreen
	conhostAltTried bool
)

func init() {
	consoleOwnsAltScreen = func() bool {
		conhostAltMu.Lock()
		defer conhostAltMu.Unlock()
		return conhostAlt != nil
	}
}

// classicConsoleWindow reports whether this process draws into a console
// that has a real, visible window: conhost as cmd.exe runs in it. A
// pseudoconsole answers GetConsoleWindow with a PseudoConsoleWindow that is
// 0x0 and reported visible, and a headless host with nothing at all.
func classicConsoleWindow() bool {
	h, _, _ := procGetConsoleWindowAlt.Call()
	if h == 0 {
		return false
	}
	var buf [64]uint16
	n, _, _ := procGetClassNameWAlt.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 || syscall.UTF16ToString(buf[:n]) != "ConsoleWindowClass" {
		return false
	}
	visible, _, _ := procIsWindowVisibleAlt.Call(h)
	return visible != 0
}

// setupConhostAltScreen makes f4's own screen buffer, once, when the console
// is one where 1049 must not be used. It does not show it: that is
// setAltScreenOS's job, at the moment the caller asks for the alternate
// screen.
func setupConhostAltScreen() {
	conhostAltMu.Lock()
	defer conhostAltMu.Unlock()
	if conhostAltTried {
		return
	}
	conhostAltTried = true

	if isWineOS() || win32ConsoleActive() || !classicConsoleWindow() {
		return
	}
	orig, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || orig == 0 || orig == syscall.InvalidHandle {
		return
	}
	var info consoleScreenBufferInfo
	if ok, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(orig), uintptr(unsafe.Pointer(&info))); ok == 0 {
		return
	}

	r1, _, _ := procCreateConsoleScreenBuffer.Call(
		uintptr(0xC0000000), // GENERIC_READ | GENERIC_WRITE
		uintptr(3),          // FILE_SHARE_READ | FILE_SHARE_WRITE
		0,
		uintptr(1), // CONSOLE_TEXTMODE_BUFFER
		0,
	)
	if r1 == 0 || syscall.Handle(r1) == syscall.InvalidHandle {
		return
	}
	own := syscall.Handle(r1)

	// The output mode is per buffer: without VT processing on this one the
	// frames would come out as text.
	var mode uint32
	if err := windows.GetConsoleMode(windows.Handle(orig), &mode); err == nil {
		mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_WRAP_AT_EOL_OUTPUT
		_ = windows.SetConsoleMode(windows.Handle(own), mode)
	}

	c := &conhostAltScreen{
		orig:    orig,
		own:     own,
		origOut: os.Stdout,
		ownOut:  os.NewFile(uintptr(own), "CONOUT$"),
	}
	c.fit(viewportSize(info))
	conhostAlt = c

	// A fresh buffer keeps whatever size the console settings gave it, and
	// conhost never shrinks a buffer when the window gets smaller -- it only
	// grows one when the window gets bigger -- so after a shrink f4's screen
	// would sit in the corner of a larger buffer behind scroll bars. The
	// window size is polled anyway; keep the buffer exactly the size of the
	// viewport each time it is read, the way Far's window mode does.
	prev := GetTerminalSize
	GetTerminalSize = func() (int, int, error) {
		w, h, err := prev()
		if err == nil {
			conhostAltMu.Lock()
			if conhostAlt != nil && conhostAlt.on {
				conhostAlt.fit(w, h)
			}
			conhostAltMu.Unlock()
		}
		return w, h, err
	}
}

func viewportSize(info consoleScreenBufferInfo) (int, int) {
	return int(info.srWindow.Right-info.srWindow.Left) + 1, int(info.srWindow.Bottom-info.srWindow.Top) + 1
}

// fit makes f4's buffer exactly w by h cells with the viewport at the
// origin. Order matters to conhost: a buffer may never be smaller than its
// viewport, so grow first, place the viewport, and only then shrink.
func (c *conhostAltScreen) fit(w, h int) {
	if w <= 0 || h <= 0 || w > 0x7fff || h > 0x7fff {
		return
	}
	var info consoleScreenBufferInfo
	if ok, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(c.own), uintptr(unsafe.Pointer(&info))); ok == 0 {
		return
	}
	cw, ch := viewportSize(info)
	if int(info.dwSize.X) == w && int(info.dwSize.Y) == h && cw == w && ch == h &&
		info.srWindow.Left == 0 && info.srWindow.Top == 0 {
		return
	}
	grown := Coord{X: max(info.dwSize.X, int16(w)), Y: max(info.dwSize.Y, int16(h))}
	if grown != info.dwSize {
		procSetConsoleScreenBufferSize.Call(uintptr(c.own), coordArg(grown))
	}
	rect := SmallRect{Left: 0, Top: 0, Right: int16(w) - 1, Bottom: int16(h) - 1}
	procSetConsoleWindowInfo.Call(uintptr(c.own), uintptr(1), uintptr(unsafe.Pointer(&rect)))
	procSetConsoleScreenBufferSize.Call(uintptr(c.own), coordArg(Coord{X: int16(w), Y: int16(h)}))
}

// coordArg packs a COORD the way the console API takes it by value.
func coordArg(c Coord) uintptr {
	return uintptr(uint16(c.X)) | uintptr(uint16(c.Y))<<16
}

// set shows f4's buffer or the original one, and points os.Stdout and the
// process's standard output handle at whichever is on screen, so that
// everything written from now on -- frames, passthrough, a child's output
// -- lands where the user is looking.
//
// os.Stdout is reassigned here just as SetupStderrLog reassigns os.Stderr;
// readers take the pointer as it is at the moment of the write.
func (c *conhostAltScreen) set(on bool) {
	if c.on == on {
		return
	}
	if on {
		// The window may have changed size while the original buffer was
		// showing; the inactive one did not follow. Match it before it
		// appears, or conhost resizes the window to the stale viewport.
		var info consoleScreenBufferInfo
		if ok, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(c.orig), uintptr(unsafe.Pointer(&info))); ok != 0 {
			c.fit(viewportSize(info))
		}
		procSetConsoleActiveScreenBuffer.Call(uintptr(c.own))
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(c.own))
		os.Stdout = c.ownOut
	} else {
		procSetConsoleActiveScreenBuffer.Call(uintptr(c.orig))
		_ = windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, windows.Handle(c.orig))
		os.Stdout = c.origOut
	}
	c.on = on
}

// setConhostAltScreen is setAltScreenOS's half for this buffer. It is a
// no-op until setupConhostAltScreen has decided the console needs one.
func setConhostAltScreen(enable bool) {
	conhostAltMu.Lock()
	defer conhostAltMu.Unlock()
	if conhostAlt != nil {
		conhostAlt.set(enable)
	}
}
