//go:build windows

package vtui

import (
	"os"

	"golang.org/x/sys/windows"
)

func watchResizeSignal(c chan os.Signal) {
	// Windows doesn't use signals for resizing.
	// FrameManager already polls terminal size on Windows.
}

func initTerminalOS() {
	// Ensure that Windows Console handles UTF-8 output properly,
	// preventing Box Drawing characters from appearing as gibberish.
	// 65001 is the ID for CP_UTF8
	windows.SetConsoleOutputCP(65001)

	// Enable VT processing for Windows Console (conhost)
	hOut, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err == nil {
		var mode uint32
		if err := windows.GetConsoleMode(hOut, &mode); err == nil {
			mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING | windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_WRAP_AT_EOL_OUTPUT
			windows.SetConsoleMode(hOut, mode)
		}
	}
}