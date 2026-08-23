# Issue 283 solution review

## Symptom

After an X11 GUI window is resized to dimensions that are not an exact
multiple of the terminal cell size, stale pixels remain in the incomplete
rightmost column and/or bottom row. The X11 host receives the real pixel
dimensions, but the renderer replaces its image with a cell-aligned image and
the subsequent `PutImage` covers only that smaller rectangle.

## Candidate solutions

1. **Keep the X11 image at the configured pixel dimensions (selected).**
   The renderer will allocate an image matching `X11Host.width/height`, keep
   the host dimensions unchanged, and continue to paint the cell grid inside
   it. The unoccupied partial-cell margins stay cleared, and `flushImage`
   uploads the complete configured surface.
2. **Explicitly paint partial margins with a black rectangle.**
   This would require special edge handling in every render path and would
   still leave the X11 image/host dimensions inconsistent. Rejected as more
   fragile and less general than preserving the actual surface size.
3. **Resize the native X11 window down to the cell-aligned grid.**
   This avoids margins but changes the user-requested window size and causes
   resize feedback loops with the window manager. Rejected because incomplete
   cells are a valid native window size and are expected to remain visible.

## Three-pass review

### Pass 1 — correctness

The selected solution makes the image dimensions, dirty-line count, X11
`PutImage` dimensions, and native window dimensions agree. A newly allocated
image is zeroed, so pixels outside the cell grid cannot inherit data from the
previous size. The cell renderer remains unchanged.

### Pass 2 — regression surface

Exact cell-aligned sizes still follow the same path. Non-aligned sizes now
upload the partial margins as part of the same frame. Shared-memory uploads
continue using the existing fixed segment; non-shared-memory uploads resize
their conversion buffer with the image.

### Pass 3 — portability and testability

The change is isolated to the X11 backend and its platform-neutral unit tests.
Wayland and gogpu are not modified. The regression test uses a fake host and
checks both that the actual configured dimensions are preserved and that stale
pixels in the partial margins are cleared.

## Verification plan

- Run the X11 renderer tests repeatedly, plus the full `vtui` test package and
  `go vet`.
- Build f4 against the updated local vtui module.
- Launch f4 on this Linux ARM64 KDE Wayland machine through XWayland with
  `--gui=x11`, resize to non-cell-aligned dimensions, and inspect the partial
  right/bottom margins for stale pixels.
