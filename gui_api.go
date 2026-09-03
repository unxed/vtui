//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd

package vtui

import (
	"fmt"
	"os"
)

// RunInGUIWindow detects the available display server (Wayland or X11)
// and launches the TUI within a native graphical window.
func RunInGUIWindow(cols, rows int, backend string, fontName string, fontSize float64, setupApp func()) error {
	if backend == "win32" || backend == "winapi" || backend == "gdi" || backend == "win32gui" {
		return runInWin32Window(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "wayland" {
		return runInWaylandWindow(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "x11" {
		return runInX11Window(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "gogpu" {
		return runInGogpuWindow(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "ebiten" {
		return runInEbitenWindow(cols, rows, fontName, fontSize, setupApp)
	}

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		DebugLog("GUI: WAYLAND_DISPLAY detected, starting Wayland Host (default)")
		return runInWaylandWindow(cols, rows, fontName, fontSize, setupApp)
	}

	if os.Getenv("DISPLAY") != "" {
		DebugLog("GUI: DISPLAY detected, starting X11 Host")
		return runInX11Window(cols, rows, fontName, fontSize, setupApp)
	}

	return fmt.Errorf("no GUI display found (neither DISPLAY nor WAYLAND_DISPLAY are set)")
}

func runInGogpuWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	// The gogpu backend reaches the GPU through the FFI layer. In a static
	// build, or on a universal build whose host loader goffi does not
	// recognise, that layer cannot load anything, so don't even try to open a
	// gogpu window — fall back to X11 (macOS has no X server by default, so
	// this fallback is best-effort there and reports an honest error if no
	// DISPLAY is available).
	if !gogpuFFIAvailable() {
		DebugLog("GUI: FFI unavailable (static build or unrecognised host loader); falling back from gogpu to X11")
		return runInX11Window(cols, rows, fontName, fontSize, setupApp)
	}
	if err := RunGogpuHost(cols, rows, fontName, fontSize, setupApp); err != nil {
		DebugLog("GUI: gogpu host failed (%v); falling back to X11", err)
		if xerr := runInX11Window(cols, rows, fontName, fontSize, setupApp); xerr != nil {
			return fmt.Errorf("gogpu backend failed: %w (X11 fallback also failed: %v)", err, xerr)
		}
		return nil
	}
	return nil
}

func runInEbitenWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return RunEbitenHost(cols, rows, fontName, fontSize, setupApp)
}
