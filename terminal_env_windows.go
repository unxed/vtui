//go:build windows

package vtui

import "golang.org/x/sys/windows"

func initTerminalOS() {
	// Ensure that Windows Console handles UTF-8 output properly,
	// preventing Box Drawing characters from appearing as gibberish.
	windows.SetConsoleOutputCP(windows.CP_UTF8)

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