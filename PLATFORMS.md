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

Does not build yet, but the target is reachable and CI for it is not the
obstacle: GitHub has no Plan 9 runners, but nothing here needs one. A Plan 9
row would cross-compile on `ubuntu-latest` exactly as the illumos, solaris and
dragonfly rows already do, so it would be build-and-vet coverage with no test
execution -- the same deal those targets get.

Two blockers are gone. `plan9/amd64` used to match the gogpu constraint and
select a backend goffi cannot serve; it is excluded now. And `vtinput` grew a
Plan 9 reader: its Unix one is built on poll(2) plus a self-pipe, neither of
which Plan 9 has, so reads there run in a pump goroutine that reports over a
channel instead.

What is left is in vtui, and it is five files rather than one idea:

| File | Missing on Plan 9 |
| --- | --- |
| `win32_gui_renderer.go` | `glyphKey`, `drawBoxGlyph` |
| `crash_report_pid_unix.go` | `syscall.Kill` |
| `sys_unix.go` | `unix.Dup2` |
| `terminal_env_unix.go` | `syscall.SIGWINCH` |
| `gui_api_fallback.go` | `runInX11Window` |

The first row is worth a look on its own account: the Win32 renderer should not
be compiled anywhere but Windows, and its appearing in a Plan 9 build says its
constraint is too wide regardless of what Plan 9 does. The others are the usual
Unix-isms that need a Plan 9 variant or a constraint that excludes it.

`gui_api_fallback.go` is the one that is not merely mechanical. Plan 9 has no
GUI backend at all -- rio is not X11, and vtui has no rio renderer -- so there
is nothing for the fallback to fall back *to*. The realistic goal on Plan 9 is
the terminal path with no GUI, which makes the question "is the ANSI path
self-sufficient here" rather than "which backend do we pick". That is a design
decision, not a build tag.

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
