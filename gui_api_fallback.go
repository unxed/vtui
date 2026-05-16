//go:build !linux && !openbsd && !netbsd && !dragonfly && !darwin && !freebsd

package vtui

import (
	"fmt"
	"runtime"
)

// RunInGUIWindow launches the TUI within a native graphical window.
// On platforms without X11/Wayland (like Windows), it defaults to gogpu.
func RunInGUIWindow(cols, rows int, backend string, setupApp func()) error {
	if backend == "gogpu" || backend == "" {
		return runInGogpuWindow(cols, rows, setupApp)
	}
	return fmt.Errorf("GUI backend %q is not supported on %s", backend, runtime.GOOS)
}

func runInGogpuWindow(cols, rows int, setupApp func()) error {
	return RunGogpuHost(cols, rows, setupApp)
}