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

### NetBSD, and why the shim is not optional

NetBSD needs the same `fakecgo` `-gcflags` shim FreeBSD does, and this cannot
be fixed in goffi or pureffi. fakecgo supplies `environ`, `__progname` and
`__ps_strings` in place of the crt0 a cgo-free build does not link, and those
symbols have to reach the *dynamic* symbol table for rtld to resolve libc's
undefined references at startup. `//go:cgo_export_dynamic` is the only
mechanism for that, and the compiler accepts it only in a package built as
`std` -- hence the flag. FreeBSD has lived with it for the same reason.

Both CIs now pass it for NetBSD as well as FreeBSD, so `ffibridge` enables its
FFI path there.

`keytrans` deliberately does **not**. It supports building against vanilla
`ebitengine/purego`, not only the pureffi fork, and vanilla purego has no
NetBSD support (`syscall15Args` and `isAllSameFloat` come out undefined).
Turning NetBSD on in its constraints would gain the xkbcommon and XIM backends
for consumers that replace purego with pureffi, at the cost of breaking
everyone who does not. The trade is not worth it.

### plan9

Excluded from the gogpu constraint, which used to admit `plan9/amd64` and
select a backend goffi cannot serve there.

That was necessary but is not sufficient for a working Plan 9 build: the next
blocker is `github.com/unxed/vtinput`, whose `reader_unix.go` reaches for
`golang.org/x/sys/unix` (`unix.Poll`, `unix.PollFd`, `unix.POLLIN`), none of
which exists on Plan 9. Plan 9 needs a reader implementation there before it
can build at all.

## Fixed

`android/arm64` builds. Ebitengine is now excluded there: `GOOS=android` also
satisfies the `linux` build tag, so the Ebitengine backend was being selected
on Android, where its own `internal/ui` package does not compile without the
gomobile/cgo path (`dipToNativePixels` and `graphicsDriverCreatorImpl` come out
undefined). That is an Ebitengine limitation, reported upstream; vtui simply
does not select the backend there. The gogpu backend still is.

`windows/386` used to fail with `undefined: isSpecialOrModifiedKey`: the helper
lived in `gogpu_host.go`, which is limited to `amd64`/`arm64`, while its caller
in the Win32 backend is built for every Windows architecture. It now lives in
`keys_special.go` with no build tag.
