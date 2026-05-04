//go:build freebsd || (openbsd && arm64) || (netbsd && arm64)

package vtui

import "fmt"

var (
	shmId   int
	shmAddr uintptr
	shmData []byte
)

func RunInX11Window(cols, rows int, setupApp func()) error {
	return fmt.Errorf("GUI mode is not supported on this platform (BSD on arm64) due to upstream library limitations")
}