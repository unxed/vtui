//go:build !windows

package vtui

import (
	"os"
	"os/signal"
	"syscall"
)

func initTerminalOS() {}

func watchResizeSignal(c chan os.Signal) {
	signal.Notify(c, syscall.SIGWINCH)
}
