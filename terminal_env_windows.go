//go:build windows

package vtui

import (
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modMsvcrt   = syscall.NewLazyDLL("msvcrt.dll")
	procSetMode = modMsvcrt.NewProc("_setmode")
)

const _O_BINARY = 0x8000

func watchResizeSignal(c chan os.Signal) {
	// Windows doesn't use signals for resizing.
	// FrameManager already polls terminal size on Windows.
}

func isHandleValid(stdHandle uint32) bool {
	h, err := windows.GetStdHandle(stdHandle)
	if err != nil || h == windows.InvalidHandle || h == 0 {
		return false
	}
	fileType, err := windows.GetFileType(h)
	if err != nil || fileType == windows.FILE_TYPE_UNKNOWN {
		return false
	}
	return true
}

func initTerminalOS() {
	// Ensure that Windows Console handles UTF-8 output properly.
	// 65001 is the ID for CP_UTF8
	windows.SetConsoleOutputCP(65001)
	windows.SetConsoleCP(65001)

	// Set binary mode for Stdin and Stdout to prevent CRLF translation and improve speed.
	// This is the "secret trick" for high-performance console output in Windows.
	if isHandleValid(windows.STD_INPUT_HANDLE) {
		procSetMode.Call(uintptr(0), uintptr(_O_BINARY))
	}
	if isHandleValid(windows.STD_OUTPUT_HANDLE) {
		procSetMode.Call(uintptr(1), uintptr(_O_BINARY))
	}
	if isHandleValid(windows.STD_ERROR_HANDLE) {
		procSetMode.Call(uintptr(2), uintptr(_O_BINARY))
	}

	// Enable VT processing for Windows Console (conhost)
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err == nil && hOut != windows.InvalidHandle && hOut != 0 {
		var mode uint32
		if err := windows.GetConsoleMode(hOut, &mode); err == nil {
			mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_WRAP_AT_EOL_OUTPUT
			windows.SetConsoleMode(hOut, mode)
		}
	}

	// Отключаем режим QuickEdit на Windows, чтобы консоль отдавала нам правый и средний клики
	hIn, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err == nil && hIn != windows.InvalidHandle && hIn != 0 {
		var mode uint32
		if err := windows.GetConsoleMode(hIn, &mode); err == nil {
			const ENABLE_QUICK_EDIT_MODE = 0x0040
			const ENABLE_EXTENDED_FLAGS = 0x0080
			mode &^= ENABLE_QUICK_EDIT_MODE
			mode |= ENABLE_EXTENDED_FLAGS
			windows.SetConsoleMode(hIn, mode)
		}
	}

	// After the output mode is set, so the buffer inherits VT processing.
	setupConhostAltScreen()
}

func setAltScreenOS(enable bool) {
	setAltScreenWin32(enable)
	setConhostAltScreen(enable)
}

type consoleCursorInfo struct {
	size    uint32
	visible int32
}

var (
	kernel32DLL              = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleCursorInfo = kernel32DLL.NewProc("GetConsoleCursorInfo")
	procSetConsoleCursorInfo = kernel32DLL.NewProc("SetConsoleCursorInfo")

	// initialCursorSize is the dwSize the console already had before f4
	// ever touched it -- captured once, on the first SetCursorStyleOS
	// call. Real FAR2 for Windows does the same thing (far/interf.cpp:
	// SetInitialCursorType/InitialCursorInfo.dwSize) rather than assuming
	// a fixed percentage: the console's own default already matches
	// whatever that system/Wine build considers a normal thin cursor, and
	// a hardcoded guess (previously 30%) can look noticeably thicker than
	// it. 0 means "not captured yet".
	initialCursorSize uint32
)

// classicConsoleCursor is cursorStyleViaConsoleAPIOS's answer, decided
// once: a process does not move between a console window and a
// pseudoconsole. Wine's console frontends are not conhost and are left on
// the DECSCUSR path, as before.
var (
	classicConsoleCursorOnce sync.Once
	classicConsoleCursor     bool
)

func cursorStyleViaConsoleAPIOS() bool {
	classicConsoleCursorOnce.Do(func() {
		classicConsoleCursor = !isWineOS() && classicConsoleWindow()
	})
	return classicConsoleCursor
}

// restoreConsoleCursorOS is the console-API counterpart of seqDefaultCursor:
// on the way out (and before a child runs) put back the size the console
// had before f4 touched it, so a block cursor left by an overwrite-mode
// editor does not follow the user into the shell.
func restoreConsoleCursorOS() {
	SetCursorStyleOS(true, CursorShapeUnderline)
}

func SetCursorStyleOS(visible bool, shape CursorShape) {
	hOut, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil || hOut == syscall.InvalidHandle || hOut == 0 {
		return
	}

	var info consoleCursorInfo
	r1, _, _ := procGetConsoleCursorInfo.Call(uintptr(hOut), uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		return
	}
	current := info.size

	if initialCursorSize == 0 {
		if info.size >= 1 && info.size <= 100 {
			initialCursorSize = info.size
		} else {
			// GetConsoleCursorInfo can report 0 under some registry
			// quirks (see CONSOLE_CURSOR_INFO docs); 25 matches the
			// classic cmd.exe/conhost underline-cursor default.
			initialCursorSize = 25
		}
	}

	if visible {
		info.visible = 1
	} else {
		info.visible = 0
	}

	if shape == CursorShapeBlock {
		info.size = 100
	} else {
		info.size = initialCursorSize
	}

	// conhost keeps a cursor *type* next to the size, and dwSize only
	// matters for CursorType::Legacy. SetConsoleCursorInfo switches the
	// type back to Legacy solely when the size it is given differs from the
	// current one (microsoft/terminal#4124): asked for the size the buffer
	// already has, it changes nothing, and a DECSCUSR the shell or a child
	// sent earlier -- Underscore, the one-pixel line of f4 #219, or
	// PSReadLine's bar -- stays in force. So when the type may be stale and
	// the size would not move, go through a neighbouring size first.
	if cursorStyleViaConsoleAPIOS() && consoleCursorTypeStale && info.size == current {
		nudge := info
		if nudge.size > 1 {
			nudge.size--
		} else {
			nudge.size++
		}
		procSetConsoleCursorInfo.Call(uintptr(hOut), uintptr(unsafe.Pointer(&nudge)))
	}
	if r, _, _ := procSetConsoleCursorInfo.Call(uintptr(hOut), uintptr(unsafe.Pointer(&info))); r != 0 {
		consoleCursorTypeStale = false
	}
}
