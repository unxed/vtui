//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import "github.com/go-webgpu/goffi/ffi"

// gogpuFFIAvailable reports whether the FFI layer the gogpu/Ebitengine backend
// depends on can actually load shared libraries and call foreign functions on
// this build and host.
//
// It returns false for a static build (-tags goffi_static) or for a universal
// build whose host loader goffi does not recognise. In both cases gogpu cannot
// start, so callers gate on this before choosing the gogpu backend and fall
// back to X11 (or the Win32 backend on Windows) instead. See ffi.Available.
func gogpuFFIAvailable() bool {
	return ffi.Available()
}
