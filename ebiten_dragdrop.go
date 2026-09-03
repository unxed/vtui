//go:build (linux || windows || darwin) && !android && (amd64 || arm64)

package vtui

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/unxed/vtinput"

	"github.com/hajimehoshi/ebiten/v2"
)

// AcceptsDrops implements DragBackend. Ebitengine delivers files dropped onto
// the window on every desktop platform this backend builds for.
func (h *EbitenHost) AcceptsDrops() bool { return true }

// StartDrag implements DragBackend. Ebitengine exposes no way to begin a drag
// out of the window: its drop support is receive-only, so a drag started here
// has nowhere to go and the honest answer is to say so rather than to appear
// to start something that will never complete.
func (h *EbitenHost) StartDrag(payload DragPayload, allowed DropAction) (DropAction, error) {
	return DropNone, ErrDragUnsupported
}

// pollDroppedFiles turns a completed drop into a DragEvent.
//
// Ebitengine reports only the drop itself, with no hover events beforehand, so
// a target sees DragEnter, DragOver and DragDrop arrive together at the same
// point. That is a real limitation next to X11 and Wayland, where the pointer
// can be tracked while it carries a payload; sending the leading phases anyway
// means a target written for those backends still gets the sequence it expects
// rather than a drop out of nowhere.
func (g *ebitenGame) pollDroppedFiles(mods vtinput.ControlKeyState) {
	dropped := ebiten.DroppedFiles()
	if dropped == nil {
		return
	}
	entries, err := fs.ReadDir(dropped, ".")
	if err != nil || len(entries) == 0 {
		return
	}

	paths := make([]string, 0, len(entries))
	for _, e := range entries {
		if p, ok := realPathOfDropped(dropped, e.Name()); ok {
			paths = append(paths, p)
		} else {
			DebugLog("EBITEN_HOST: dropped entry %q has no recoverable path, skipping", e.Name())
		}
	}
	if len(paths) == 0 {
		// A base name on its own is not something a file manager can act on,
		// and guessing a directory for it would be worse than dropping it.
		return
	}

	h := g.host
	h.mu.Lock()
	cw, ch := h.cellW, h.cellH
	h.mu.Unlock()
	if cw <= 0 || ch <= 0 {
		return
	}
	px, py := ebiten.CursorPosition()
	cx, cy := px/cw, py/ch

	DebugLog("EBITEN_HOST: %d file(s) dropped at cell %d,%d", len(paths), cx, cy)

	payload := DragPayload{Paths: paths}
	for _, phase := range []DragPhase{DragEnter, DragOver, DragDrop} {
		DeliverDragEvent(&DragEvent{
			Phase:     phase,
			X:         cx,
			Y:         cy,
			Modifiers: mods,
			Allowed:   DropCopy | DropMove | DropLink,
			Suggested: DropCopy,
			Payload:   payload,
		})
	}
}

// realPathOfDropped recovers the filesystem path behind a dropped entry.
//
// Ebitengine hands over an fs.FS whose root lists only base names, since the
// dropped items need not share a directory. It does keep the real paths
// internally and opens them with os.Open, so the *os.File it returns still
// knows its own name. Going through the FS this way is the only route to a
// usable path; a file manager needs somewhere to copy from, not a name.
func realPathOfDropped(fsys fs.FS, name string) (string, bool) {
	f, err := fsys.Open(name)
	if err != nil {
		return "", false
	}
	defer f.Close()

	if osf, ok := f.(*os.File); ok {
		if p := osf.Name(); p != "" {
			if abs, err := filepath.Abs(p); err == nil {
				return abs, true
			}
			return p, true
		}
	}
	return "", false
}
