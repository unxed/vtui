//go:build freebsd || dragonfly || openbsd || netbsd

package vtui

import "fmt"

func runInX11Window(cols, rows int, setupApp func()) error {
	return fmt.Errorf("X11 (purego) GUI mode is not supported on this platform. Use purex11 backend")
}