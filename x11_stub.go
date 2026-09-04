//go:build android

package vtui

import "errors"

// runInX11Window is a stub for Android (Termux), where there is no X server
// and the X11 backend (github.com/jezek/xgb) is compiled out. Android builds
// are terminal-only.
func runInX11Window(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return errors.New("X11 backend is not supported on Android. Please use Terminal mode.")
}
