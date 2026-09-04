//go:build !windows

package vtui

import (
	"os"
	"os/signal"
	"syscall"
)

func initTerminalOS()            {}
func setAltScreenOS(enable bool) {}

func watchResizeSignal(c chan os.Signal) {
	signal.Notify(c, syscall.SIGWINCH)
}
func SetCursorStyleOS(visible bool, shape CursorShape) {}
func cursorStyleViaConsoleAPIOS() bool                 { return false }
func restoreConsoleCursorOS()                          {}
