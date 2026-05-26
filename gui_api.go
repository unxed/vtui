//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd

package vtui

import (
	"fmt"
	"os"
)

// RunInGUIWindow detects the available display server (Wayland or X11)
// and launches the TUI within a native graphical window.
func RunInGUIWindow(cols, rows int, backend string, setupApp func()) error {
	if backend == "wayland" {
		return runInWaylandWindow(cols, rows, setupApp)
	}
	if backend == "x11" {
		return runInX11Window(cols, rows, setupApp)
	}
	if backend == "gogpu" {
		return runInGogpuWindow(cols, rows, setupApp)
	}

	if os.Getenv("DISPLAY") != "" {
		DebugLog("GUI: DISPLAY detected, starting X11 Host (default)")
		return runInX11Window(cols, rows, setupApp)
	}

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		DebugLog("GUI: WAYLAND_DISPLAY detected, starting Wayland Host")
		return runInWaylandWindow(cols, rows, setupApp)
	}

	return fmt.Errorf("no GUI display found (neither DISPLAY nor WAYLAND_DISPLAY are set)")
}

func runInGogpuWindow(cols, rows int, setupApp func()) error {
	return RunGogpuHost(cols, rows, setupApp)
}
