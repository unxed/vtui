# Drag and drop in vtui

`dragdrop.go` holds everything about drag and drop that is not tied to a
display server: the payload types, the two registration points, and the
`text/uri-list` codec every protocol needs. A backend adds the protocol; the
application adds the policy. Neither knows about the other.

## For the application

Install one drop target:

    vtui.SetDropTarget(myTarget)   // myTarget implements HandleDrag

`HandleDrag` is called for every step of a gesture over our window and
answers what a drop *would* do at `ev.X, ev.Y` (cell coordinates). That
answer is what gives the pointer its copy / move cursor, so it must be quick
and must not open dialogs. `DragDrop` is the phase where the work actually
starts; start it in the background and return.

To hand files to other applications:

    action, err := vtui.StartDrag(payload, vtui.DropCopy|vtui.DropMove)

`err` is `ErrDragUnsupported` on every backend without a protocol, which is
every terminal today. Check `vtui.DropSupported()` / `vtui.DragOutSupported()`
before offering the feature in the UI.

## For a backend

Implement `DragBackend` and register it once the window exists:

    vtui.SetDragBackend(host)

Then, for every step of an incoming gesture, build a `DragEvent` and call
`vtui.DeliverDragEvent(ev)`. Rules a backend has to follow:

- convert device pixels to cell coordinates before filling `X` and `Y`;
- decode dragged files into `Payload.Paths` (use `ParseURIList`), leave
  anything that is not a local file in `Payload.URIs`;
- send exactly one `DragEnter`, any number of `DragOver`, and then one
  `DragLeave` or one `DragDrop`;
- report the returned `DropAction` back to the source as the protocol
  requires.

`DeliverDragEvent` may be called from the backend's own goroutine: it hops to
the UI thread itself and gives up after `DragDeliverTimeout`, because a
display server will not wait for a UI stuck behind a modal dialog.

## Status

- core, uri-list codec: done (this file's package)
- X11 (XDND): both directions done, in x11_xdnd.go
- X11 limitation: only copy is announced when dragging out. A move would
  mean deleting the originals because a target said it took them, and no
  file is worth that much trust until the behaviour is tested widely.
- X11 limitation: XdndProxy is not followed when looking for a target, so a
  window that only accepts drops through a proxy is not seen
- X11 limitation: an INCR (incremental) selection transfer is refused
  rather than half read, so a drop of an enormous file list currently
  fails visibly instead of silently losing entries
- Wayland (wl_data_device): planned
- gogpu / Windows / macOS: planned
- terminals: no protocol exists; nothing is registered, so both directions
  are reported as unsupported rather than half working