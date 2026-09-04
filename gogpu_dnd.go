//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gogpu/gogpu"
)

// gogpuDropAllowed is what an incoming drop is allowed to do. Copy only:
// gogpu hands us a finished drop and nothing else - not the actions the
// source permits, not the modifiers held, and there is no way back to the
// source to say what we did. A move under those conditions would delete
// somebody's files on a guess. See DRAGDROP.md.
const gogpuDropAllowed = DropCopy

// gogpuDragOutActions is what we offer when dragging files out. Copy only,
// for the reason X11 gives in DRAGDROP.md: a move has us delete the
// originals because a receiver said it took them. gogpu's DragData carries
// no action either, so this is also all it could say.
const gogpuDragOutActions = DropCopy

// gogpuDragTimeout bounds the wait for a gesture that never comes back. The
// request has to reach the main loop, and the platform session has to end
// after it; a drag still alive past this is not one the user is having.
var gogpuDragTimeout = 30 * time.Second

// (The per-frame counter that used to live here answered one question -
// whether the main loop runs the update callback a drag out is handed
// over from - and the answer was yes.)

// logGogpuDragEnvironment records what the drag callbacks were registered
// on. Which display server gogpu picks decides which of its own backends
// answers, and a drop that never arrives is usually a question about that
// backend rather than about ours.
func logGogpuDragEnvironment() {
	DebugLog("GOGPU_DND: drag callbacks registered; DISPLAY=%q WAYLAND_DISPLAY=%q XDG_SESSION_TYPE=%q",
		os.Getenv("DISPLAY"), os.Getenv("WAYLAND_DISPLAY"), os.Getenv("XDG_SESSION_TYPE"))
}

// gogpuDragRequest is one drag out waiting for the main loop to start it.
type gogpuDragRequest struct {
	paths   []string
	started bool
	result  chan gogpuDragOutcome
}

// gogpuDragOutcome is how the gesture ended, whoever got there first.
type gogpuDragOutcome struct {
	action DropAction
	err    error
}

// AcceptsDrops implements DragBackend: a gogpu window is a drop target on
// every platform gogpu supports, as soon as the window exists.
func (h *GogpuHost) AcceptsDrops() bool { return h != nil && h.app != nil }

// CanStartDrag implements DragSource: gogpu's drag source needs the window,
// and nothing else. The protocol below it is a different one on every
// platform, which is precisely what gogpu is for.
func (h *GogpuHost) CanStartDrag() bool { return h != nil && h.app != nil }

// StartDrag implements DragBackend. It is called from the UI goroutine and
// blocks until the gesture is over, while the gesture itself runs on the
// main loop, the only thread gogpu's drag source may be used from.
func (h *GogpuHost) StartDrag(payload DragPayload, allowed DropAction) (DropAction, error) {
	DebugLog("GOGPU_DND: drag out asked for %d path(s), allowed=%s",
		len(payload.Paths), allowed)
	if h == nil || h.app == nil {
		DebugLog("GOGPU_DND: drag out refused: no window to drag from")
		return DropNone, ErrDragUnsupported
	}
	if allowed&gogpuDragOutActions == 0 {
		DebugLog("GOGPU_DND: drag out refused: %s asked for, only %s can be offered",
			allowed, gogpuDragOutActions)
		return DropNone, ErrDragUnsupported
	}
	req, err := h.queueDragOut(payload)
	if err != nil {
		DebugLog("GOGPU_DND: drag out not queued: %v", err)
		return DropNone, err
	}
	// The loop may be asleep waiting for input, and a redraw is what wakes
	// it; the frame it draws is also the one that starts the drag.
	h.app.RequestRedraw()
	return h.awaitDragOut(req)
}

// queueDragOut leaves a request for the main loop to find.
func (h *GogpuHost) queueDragOut(payload DragPayload) (*gogpuDragRequest, error) {
	paths := gogpuDragPaths(payload)
	if len(paths) == 0 {
		return nil, ErrDragNoData
	}
	req := &gogpuDragRequest{paths: paths, result: make(chan gogpuDragOutcome, 1)}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.dragOut != nil {
		return nil, ErrDragBusy
	}
	h.dragOut = req
	return req, nil
}

// awaitDragOut waits for the gesture to end, or gives up on it.
func (h *GogpuHost) awaitDragOut(req *gogpuDragRequest) (DropAction, error) {
	select {
	case out := <-req.result:
		return out.action, out.err
	case <-time.After(gogpuDragTimeout):
		DebugLog("GOGPU_DND: drag out gave up after %s", gogpuDragTimeout)
		h.clearDragOut(req)
		return DropNone, nil
	}
}

// pumpDragOut hands a waiting request to gogpu. It runs on the main loop,
// once per frame, and does nothing at all when there is nothing waiting.
func (h *GogpuHost) pumpDragOut() {
	h.mu.Lock()
	req := h.dragOut
	if req == nil || req.started {
		h.mu.Unlock()
		return
	}
	req.started = true
	app := h.app
	h.mu.Unlock()

	if app == nil {
		h.finishDragOut(req, gogpuDragOutcome{err: ErrDragUnsupported})
		return
	}

	DebugLog("GOGPU_DND: handing gogpu %d file(s): %q", len(req.paths), req.paths)
	startedAt := time.Now()
	err := app.StartDrag(gogpu.DragData{FilePaths: req.paths}, func(r gogpu.DragResult) {
		DebugLog("GOGPU_DND: gogpu reported the gesture as %s after %s",
			gogpuDragResultName(r), time.Since(startedAt).Round(time.Millisecond))
		h.finishDragOut(req, gogpuDragOutcome{action: gogpuDropActionOf(r)})
	})
	// On Windows and X11 the callback has already fired by now, and the
	// send below is dropped; on Wayland and macOS it is still to come.
	if err != nil {
		h.finishDragOut(req, gogpuDragOutcome{err: err})
		return
	}
	DebugLog("GOGPU_DND: gogpu took the drag out, waiting for the platform to end it")
}

// finishDragOut ends the gesture once, whoever gets there first: the
// platform callback, an error on the way in, or the wait giving up.
func (h *GogpuHost) finishDragOut(req *gogpuDragRequest, out gogpuDragOutcome) {
	h.clearDragOut(req)
	select {
	case req.result <- out:
		DebugLog("GOGPU_DND: drag out finished as %s, err=%v", out.action, out.err)
	default:
	}
}

// clearDragOut forgets the request and the button it was started with. The
// press that began the drag belongs to the platform's own grab from then
// on, so its release never reaches our handlers, and without this the host
// would go on believing the button is still down.
func (h *GogpuHost) clearDragOut(req *gogpuDragRequest) {
	h.mu.Lock()
	if h.dragOut == req {
		h.dragOut = nil
	}
	h.mouseBtn = 0
	h.mu.Unlock()
}

// gogpuDragPaths takes the local files out of a payload. gogpu wants
// absolute paths and refuses anything else, and a URI naming a file on
// another machine is not something we could hand over in any case.
func gogpuDragPaths(payload DragPayload) []string {
	paths := make([]string, 0, len(payload.Paths))
	for _, p := range payload.Paths {
		p = strings.TrimSpace(p)
		if p == "" || !filepath.IsAbs(p) {
			continue
		}
		paths = append(paths, p)
	}
	return paths
}

// gogpuDropActionOf translates what the receiver did into our own words.
func gogpuDropActionOf(r gogpu.DragResult) DropAction {
	switch r {
	case gogpu.DragCopied:
		return DropCopy
	case gogpu.DragMoved:
		return DropMove
	}
	return DropNone
}

// gogpuDragResultName renders what gogpu reported, including a value it has
// no name for. A gesture that ends as "none" says nothing about which of
// the two ways it got there, and the two want different questions asked.
func gogpuDragResultName(r gogpu.DragResult) string {
	switch r {
	case gogpu.DragCopied:
		return "copied"
	case gogpu.DragMoved:
		return "moved"
	case gogpu.DragCancelled:
		return "cancelled"
	}
	return fmt.Sprintf("a value it has no name for (%v)", r)
}

// pointerPixels reports where gogpu last saw the pointer, in the pixels
// its pointer callbacks use, or false when there is no window to ask.
func (h *GogpuHost) pointerPixels() (float64, float64, bool) {
	if h == nil || h.app == nil {
		return 0, 0, false
	}
	mx, my := h.app.Input().Mouse().Position()
	return float64(mx), float64(my), true
}

// dropPixels decides which position a drop happened at. Normally it is the
// one gogpu reports, but gogpu before 0.50.1 reported every drop at the
// origin, and that is also what any future loss of the position would look
// like: an exact 0,0 while the pointer is somewhere else entirely. When the
// two disagree that way the pointer is believed, because it is tracked
// through a foreign drag and is where the user was actually aiming. A real
// drop on the first cell reaches the same cell either way, so nothing is
// lost by not trusting the origin.
func (h *GogpuHost) dropPixels(x, y float64) (float64, float64) {
	if x != 0 || y != 0 {
		return x, y
	}
	px, py, ok := h.pointerPixels()
	if !ok || (px == 0 && py == 0) {
		return x, y
	}
	DebugLog("GOGPU_DND: the drop was reported at the origin, taking the pointer at %.1f,%.1f px instead", px, py)
	return px, py
}

// handleFileDrop is what gogpu calls when files are dropped on the window.
// It returns at once and delivers in the background: delivery waits for the
// UI thread, and this runs on the loop that draws the window, which the UI
// is about to need.
func (h *GogpuHost) handleFileDrop(paths []string, x, y float64) {
	DebugLog("GOGPU_DND: OnDragDrop fired at %.1f,%.1f with %d entry/entries: %q",
		x, y, len(paths), paths)
	go h.deliverFileDrop(paths, x, y)
}

// deliverFileDrop replays a finished gogpu drop as the gesture the core
// expects: exactly one enter, then the drop. There is nothing in between
// because gogpu reports nothing in between - no motion, no leave, and no
// word from the source about what it would allow.
func (h *GogpuHost) deliverFileDrop(paths []string, x, y float64) {
	payload := gogpuDragPayload(paths)
	if payload.IsEmpty() {
		DebugLog("GOGPU_DND: the drop carried nothing we could decode, ignoring it")
		return
	}

	h.mu.Lock()
	cellW, cellH, mods := h.cellW, h.cellH, h.currentMods
	cols, rows := h.cols, h.rows
	h.mu.Unlock()

	dx, dy := h.dropPixels(x, y)
	cx, cy := gogpuDropCell(dx, cellW), gogpuDropCell(dy, cellH)
	DebugLog("GOGPU_DND: the drop lands on cell %d,%d of a %dx%d screen, cell size %dx%d px",
		cx, cy, cols, rows, cellW, cellH)

	ev := DragEvent{
		Phase:     DragEnter,
		X:         cx,
		Y:         cy,
		Modifiers: mods,
		Allowed:   gogpuDropAllowed,
		Suggested: DropCopy,
		Payload:   payload,
	}
	DeliverDragEvent(&ev)

	ev.Phase = DragDrop
	action := DeliverDragEvent(&ev)
	DebugLog("GOGPU_DND: %d file(s) dropped at %d,%d, handled as %s",
		len(payload.Paths), ev.X, ev.Y, action)

	if h.app != nil {
		h.app.RequestRedraw()
	}
}

// gogpuDragPayload turns what gogpu hands over into a payload. Its platform
// backends produce plain local paths, but a file: URI is decoded as well,
// and anything else is kept as a URI so a target can still make sense of it.
func gogpuDragPayload(entries []string) DragPayload {
	var p DragPayload
	for _, e := range entries {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(e), "file:") {
			if path, ok := URIToLocalPath(e); ok {
				p.Paths = append(p.Paths, path)
				continue
			}
			p.URIs = append(p.URIs, e)
			continue
		}
		if strings.Contains(e, "://") {
			p.URIs = append(p.URIs, e)
			continue
		}
		p.Paths = append(p.Paths, e)
	}
	if len(p.Paths) > 0 || len(p.URIs) > 0 {
		p.Kinds = []string{"text/uri-list"}
	}
	return p
}

// gogpuDropCell turns the pixel offset of a drop into a cell index. gogpu
// reports it in the same pixels as its pointer callbacks, so it is divided
// by the cell size exactly as those are.
func gogpuDropCell(px float64, cell int) int {
	if cell <= 0 || px <= 0 {
		return 0
	}
	return int(px) / cell
}
