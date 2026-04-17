//go:build windows

package vtui

import (
	"os"
	"syscall"

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

func initTerminalOS() {
	// Ensure that Windows Console handles UTF-8 output properly.
	// 65001 is the ID for CP_UTF8
	windows.SetConsoleOutputCP(65001)
	windows.SetConsoleCP(65001)

	// Set binary mode for Stdin and Stdout to prevent CRLF translation and improve speed.
	// This is the "secret trick" for high-performance console output in Windows.
	procSetMode.Call(uintptr(0), uintptr(_O_BINARY))
	procSetMode.Call(uintptr(1), uintptr(_O_BINARY))
	procSetMode.Call(uintptr(2), uintptr(_O_BINARY))

	// Enable VT processing for Windows Console (conhost)
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err == nil {
		var mode uint32
		if err := windows.GetConsoleMode(hOut, &mode); err == nil {
			mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_WRAP_AT_EOL_OUTPUT

			// ENABLE_PROCESSED_OUTPUT (0x0001)
			if WindowsProcessedOutput {
				mode |= 0x0001
			} else {
				mode &^= 0x0001
			}

			windows.SetConsoleMode(hOut, mode)
		}
	}
}