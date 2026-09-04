//go:build freebsd || dragonfly || openbsd || netbsd || illumos || solaris || plan9 || android || !(amd64 || arm64)

package vtui

// gogpuFFIAvailable is the stub for platforms where the gogpu backend is not
// built at all. There is no FFI-backed GUI to run here, so it always reports
// false and callers fall back to X11 (or the Win32 backend on Windows).
func gogpuFFIAvailable() bool {
	return false
}
