# Platform support

Where the GUI backends are built, and which targets are known not to build.

## The FFI layer

The gogpu and Ebitengine backends reach the system through goffi's `ffi`
package. It has an implementation for:

- Windows, on any architecture;
- Linux, macOS and FreeBSD on `amd64` and `arm64`.

FreeBSD additionally needs `-gcflags=github.com/go-webgpu/goffi/internal/fakecgo=-std`
when built without cgo.

Everywhere else there is no FFI, and the X11 backend (or the Win32 backend on
Windows) is the one that runs. `gogpuFFIAvailable` reports this at run time as
well, because a static build (`-tags goffi_static`) compiles the FFI layer but
cannot load anything through it.

## Known gaps

### NetBSD

`ffi` builds and works on `netbsd/amd64` and `netbsd/arm64`, but only with the
same `fakecgo` `-gcflags` shim FreeBSD needs. f4's CI passes that shim for
FreeBSD cells only, so enabling the FFI-backed backends for NetBSD means adding
the shim there first. Until then NetBSD deliberately stays on the non-FFI path;
turning the build tags on without the CI change would break the NetBSD build.

### android/arm64

Does not build, and not because of FFI. Ebitengine's own `internal/ui` package
fails to compile for Android without the gomobile/cgo build path
(`dipToNativePixels` and `graphicsDriverCreatorImpl` come out undefined). This
is upstream of vtui and cannot be fixed by build tags here.

### plan9

Does not build. goffi has no Plan 9 implementation, and the constraint the
gogpu backend uses admits `plan9/amd64`, so the backend is selected on a target
its dependency cannot serve. Fixing it means excluding `plan9` from the gogpu
constraint the way the BSDs and illumos/solaris already are.

## Notes

`windows/386` used to fail with `undefined: isSpecialOrModifiedKey`: the helper
lived in `gogpu_host.go`, which is limited to `amd64`/`arm64`, while its caller
in the Win32 backend is built for every Windows architecture. It now lives in
`keys_special.go` with no build tag.
