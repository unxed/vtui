# Images

`vtui` can paint bitmaps on top of the character grid. The design keeps the
pixels and the transport strictly apart, because the same picture has to reach
a kitty terminal, an iTerm2 window, a sixel console and a native X11 / Wayland
/ gogpu window, and only the last of those wants raw memory.

## Pieces

* `ImageSurface` — a top-down RGBA8 buffer. It knows nothing about file
  formats: decoding belongs to the application, `vtui` only ships bytes.
  A cheap content hash lets backends notice a surface they already sent.
* `ImagePlacement` — one surface (or a crop of it) shown over a rectangle of
  **cells**. Cell geometry is what makes a placement portable: the terminal
  backends hand `c=`/`r=` to the terminal, the GUI backends multiply by their
  own cell metrics.
* `GraphicsLayer` — the set of placements owned by a `ScreenBuf`, reachable
  through `scr.Graphics()`. It is mutex protected, because decoding usually
  finishes on a worker goroutine while the UI thread is flushing.
* `GraphicsRenderer` — implemented by renderers that can display images.
  Renderers that cannot simply do not implement it.

## Protocol selection

`DetectGraphicsProtocol` reads the environment; `VTUI_GRAPHICS` overrides it
with `kitty`, `iterm2`, `sixel`, `native` or `none`. Inside `tmux` or `screen`
detection returns `none`, since a multiplexer that swallows half of an image
leaves the session in a mess. An application that can probe the terminal
should call `scr.Graphics().SetProtocol(...)` with the result.

## Redraw rules

Terminal graphics live above the cell grid, so an image has to be sent again
whenever the text below it was repainted — `GraphicsLayer.DirtyUnder` reports
exactly that, and the generation counter covers everything else. A forced
redraw (resize, `HardReset`, reattach) additionally tells the terminal to drop
every image it holds for us, otherwise stale placements would float over the
freshly painted screen.## Native backends

The X11, Wayland and gogpu backends own an RGBA framebuffer, so they take the
`GraphicsNative` path: the surface is resampled once to its on screen pixel
size, cached, and then alpha blended into the framebuffer after the text has
been drawn. No encoding, no base64, no terminal support required.

Two details matter for correctness:

* These backends repaint a whole text row whenever any cell in it changed, so
  a change far away from the image still wipes its pixels. `DirtyRowsUnder`
  is the row-wide dirty test they use instead of `DirtyUnder`.
* When a placement moves or disappears, the pixels it left behind are still
  in the framebuffer and no text cell is marked dirty for them. The layer
  therefore raises a repaint request, which `ScreenBuf.Flush` turns into a
  full redraw on the following frame.

Resampling area-averages when shrinking and interpolates when growing, and it
filters premultiplied values so that transparent pixels do not bleed their
colour into their neighbours.

## Video

There is no video type in this layer, and there does not need to be: a video
is a placement whose surface is replaced on every frame. Push a new
`ImageSurface` through `GraphicsLayer.Update`, and the content hash change
makes each backend do the right thing on its own.

Getting the frames is the application's job. On the current platform the
portable route is to run **ffmpeg as a child process** and read raw frames
from its standard output, asking it for `rawvideo` output in the `rgba` pixel
format at a fixed size. One frame is then exactly width times height times
four bytes, which drops straight into an `ImageSurface` with no conversion.
This keeps the pure Go, statically linked property of the project: there is no
cgo, no libc coupling and no build time dependency, and a missing ffmpeg
simply means video is unavailable rather than a broken build. For the few
formats the standard library already understands (animated GIF, MJPEG
streams) no external process is needed at all.

Practical notes for whoever implements it:

* Decode on a worker goroutine and pace the frames with a ticker. Drop late
  frames instead of queueing them; a TUI must never block on I/O.
* Reuse one placement id for the whole clip. Adding and removing a placement
  per frame would ask for a full text repaint every frame.
* Cost per backend differs a lot. A native blit is close to a memcpy. The
  kitty protocol has to re-upload every frame, so keep the displayed size
  small and reuse one image id. Sixel is the most expensive of the three and
  is best limited to short clips or a low frame rate.
* Audio is out of scope for the rendering layer.

## Sixel colour: three encoders and how one is chosen

Sixel carries 256 colour registers, and a photograph does not fit in 256
colours. There are two ways past that, and which one is available is a
property of the terminal rather than of the picture.

**Per-band palettes, for known-compatible terminals.** A register is redefined
between bands, so a decoder that resolves registers as it reads them paints an
unlimited number of colours through 256 of them. vtui selects this stream by
default for WezTerm and foot, where it has been verified. Set
`VTUI_SIXEL_PALETTE=truecolor` to opt in explicitly elsewhere.

**Layers, in Windows Terminal and native OpenConsole.** Their parser keeps a
raster indexed until it flushes, so a redefinition can recolour bands it has
already decoded — the per-band form is not available there. Instead the
picture goes several times to the same cell, each time as a complete DCS image
with `P2=1` and a palette of its own, and each lands on top of the last. The
first layer covers every pixel and each further one repaints only the pixels
the layer before got wrong, so the stack is cheap where the picture is flat,
and a stack cut short by the byte budget is still a whole picture rather than
a holed one. See `graphics_sixel_layered.go`, and microsoft/terminal#20020,
where the one report of layering breaking turned out to be a missing `P2=1`
in the reporter's own encoder.

Two things the layered path relies on. Every layer restates the cursor
position, because a sixel dump leaves the text cursor at the sixel active
position rather than where it started. And the whole stack goes out in one
frame: vtui wraps a frame in synchronised output (mode 2026), which is what
keeps a half-built stack off the screen, and layers split across two frames
would be in two different synchronisation blocks and would visibly assemble
themselves.

`VTUI_SIXEL_PALETTE` overrides the choice — `fixed`, `adaptive`, `layered`,
`truecolor`/`per-band`, or anything else for the per-band escape hatch — and an
explicit value always wins over the terminal. Unknown terminals use the
adaptive single-palette encoder by default.
