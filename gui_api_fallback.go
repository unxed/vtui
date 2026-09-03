//go:build !linux && !openbsd && !netbsd && !dragonfly && !darwin && !freebsd

package vtui

import (
	"fmt"
	"os"
	"runtime"
)

// RunInGUIWindow launches the TUI within a native graphical window.
// On platforms without X11/Wayland (like Windows), it defaults to gogpu.
func RunInGUIWindow(cols, rows int, backend string, fontName string, fontSize float64, setupApp func()) error {
	if backend == "win32" || backend == "winapi" || backend == "gdi" || backend == "win32gui" {
		return runInWin32Window(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "x11" {
		return runInX11Window(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "ebiten" {
		return runInEbitenWindow(cols, rows, fontName, fontSize, setupApp)
	}
	if backend == "gogpu" || backend == "" {
		if IsWine() && backend == "" {
			if err := runInWin32Window(cols, rows, fontName, fontSize, setupApp); err == nil {
				return nil
			}
		}
		if os.Getenv("DISPLAY") != "" && backend == "" {
			return runInX11Window(cols, rows, fontName, fontSize, setupApp)
		}
		return runInGogpuWindow(cols, rows, fontName, fontSize, setupApp)
	}
	return fmt.Errorf("GUI backend %q is not supported on %s", backend, runtime.GOOS)
}

func runInGogpuWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	// The gogpu backend reaches the GPU through the FFI layer. In a static
	// build, or on a universal build whose host loader goffi does not
	// recognise, that layer cannot load anything, so skip gogpu entirely and
	// use the native fallback (Win32 on Windows, X11 elsewhere).
	if !gogpuFFIAvailable() {
		DebugLog("GUI: FFI unavailable (static build or unrecognised host loader); falling back from gogpu")
		return guiFallbackFromGogpu(cols, rows, fontName, fontSize, setupApp, nil)
	}
	if err := RunGogpuHost(cols, rows, fontName, fontSize, setupApp); err != nil {
		DebugLog("GUI: gogpu host failed (%v); falling back", err)
		return guiFallbackFromGogpu(cols, rows, fontName, fontSize, setupApp, err)
	}
	return nil
}

// guiFallbackFromGogpu runs the best non-gogpu GUI backend for the current OS.
// Windows has no X server by default, so it prefers the native Win32 backend
// there; the other targets that reach this file (illumos, solaris) do ship
// X11. cause, when non-nil, is the original gogpu error and is wrapped into the
// result if the fallback also fails, so the reason gogpu was abandoned is not
// lost.
func guiFallbackFromGogpu(cols, rows int, fontName string, fontSize float64, setupApp func(), cause error) error {
	if runtime.GOOS == "windows" {
		if err := runInWin32Window(cols, rows, fontName, fontSize, setupApp); err != nil {
			if cause != nil {
				return fmt.Errorf("gogpu backend failed: %w (Win32 fallback also failed: %v)", cause, err)
			}
			return err
		}
		return nil
	}
	if err := runInX11Window(cols, rows, fontName, fontSize, setupApp); err != nil {
		if cause != nil {
			return fmt.Errorf("gogpu backend failed: %w (X11 fallback also failed: %v)", cause, err)
		}
		return err
	}
	return nil
}

func runInEbitenWindow(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return RunEbitenHost(cols, rows, fontName, fontSize, setupApp)
}
