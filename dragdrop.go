package vtui

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/unxed/vtinput"
)

// DropAction says what a drop would do with the payload. The values are bit
// flags, so a set of actions a source is willing to perform travels in a
// single value, while a decision taken by a target is a single flag.
type DropAction uint8

const (
	DropNone DropAction = 0
	DropCopy DropAction = 1 << 0
	DropMove DropAction = 1 << 1
	DropLink DropAction = 1 << 2
)

// Has reports whether every flag of other is present in a.
func (a DropAction) Has(other DropAction) bool {
	return other != DropNone && a&other == other
}

// String renders one action or a set of them, for logs and tests.
func (a DropAction) String() string {
	if a == DropNone {
		return "none"
	}
	var parts []string
	if a&DropCopy != 0 {
		parts = append(parts, "copy")
	}
	if a&DropMove != 0 {
		parts = append(parts, "move")
	}
	if a&DropLink != 0 {
		parts = append(parts, "link")
	}
	return strings.Join(parts, "|")
}

// DragPhase is where in the gesture an event arrives. A backend sends
// DragEnter once when the pointer carrying a payload appears over the
// window, DragOver while it moves, and then exactly one of DragLeave or
// DragDrop.
type DragPhase uint8

const (
	DragEnter DragPhase = iota
	DragOver
	DragLeave
	DragDrop
)

func (p DragPhase) String() string {
	switch p {
	case DragEnter:
		return "enter"
	case DragOver:
		return "over"
	case DragLeave:
		return "leave"
	case DragDrop:
		return "drop"
	}
	return "unknown"
}

// DragPayload is what is being dragged. Paths hold file names on this
// machine, already decoded from their URIs; URIs hold everything else the
// source offered (http:, smb:, a remote file: with a foreign host), so a
// target can still do something useful with them. Kinds lists the MIME
// types the source announced, for targets that want to look closer.
type DragPayload struct {
	Kinds []string
	Paths []string
	URIs  []string
	Text  string
}

// HasFiles reports whether the payload names files on this machine.
func (p DragPayload) HasFiles() bool { return len(p.Paths) > 0 }

// OffersFiles reports whether the payload either names files or announces
// that it will. A target has to answer "yes, drop here" while the pointer is
// still moving, but XDND (and Wayland after it) hand the data over only
// after the drop, so until then all a target has to go on is the type list.
func (p DragPayload) OffersFiles() bool {
	if p.HasFiles() {
		return true
	}
	for _, k := range p.Kinds {
		if strings.EqualFold(k, "text/uri-list") {
			return true
		}
	}
	return false
}

// IsEmpty reports whether there is nothing to drop.
func (p DragPayload) IsEmpty() bool {
	return len(p.Paths) == 0 && len(p.URIs) == 0 && p.Text == ""
}

// DragEvent is one step of a drag gesture over our window. X and Y are cell
// coordinates, converted by the backend from device pixels, so a target
// reasons in the same units as a mouse event. Allowed is what the source
// permits, Suggested is what it would do by default.
type DragEvent struct {
	Phase     DragPhase
	X, Y      int
	Modifiers vtinput.ControlKeyState
	Allowed   DropAction
	Suggested DropAction
	Payload   DragPayload
}

// DropTarget is implemented by the application to answer what a drop at a
// given place would do. The returned action is reported back to the source,
// which is how the pointer gets its copy / move cursor. Returning DropNone
// means "not here".
type DropTarget interface {
	HandleDrag(ev *DragEvent) DropAction
}

// DropTargetFunc adapts a plain function to DropTarget.
type DropTargetFunc func(ev *DragEvent) DropAction

func (f DropTargetFunc) HandleDrag(ev *DragEvent) DropAction { return f(ev) }

// DragBackend is implemented by a graphical backend that speaks the drag and
// drop protocol of its display server. Terminals do not have one, so on them
// no backend is registered and both directions simply stay unavailable.
type DragBackend interface {
	// AcceptsDrops reports whether payloads from other applications can
	// reach us at all.
	AcceptsDrops() bool
	// StartDrag hands a payload to the display server and blocks until the
	// gesture is over, returning what the receiver did with it.
	StartDrag(payload DragPayload, allowed DropAction) (DropAction, error)
}

// ErrDragUnsupported is returned when a drag is started on a backend that
// has no drag and drop protocol (every terminal, for now).
var ErrDragUnsupported = errors.New("drag and drop is not supported by this backend")

// ErrDragBusy is returned when a drag is started while one is in flight.
// There is one pointer, so there is one gesture.
var ErrDragBusy = errors.New("a drag is already in progress")

// ErrDragNoData is returned when the payload holds nothing we can offer.
var ErrDragNoData = errors.New("nothing to drag")

var (
	dragMu      sync.Mutex
	dropTarget  DropTarget
	dragBackend DragBackend
)

// DragDeliverTimeout bounds how long a backend waits for the UI thread to
// answer. A display server expects a status reply quickly; a UI busy with a
// modal dialog must not stall the whole desktop's drag.
var DragDeliverTimeout = 250 * time.Millisecond

// DragDeliverToUI routes drag events through the UI thread, which is what a
// real backend needs, since it runs its event loop in its own goroutine.
// Tests set it to false to call the target directly.
var DragDeliverToUI = true

// SetDropTarget installs the application's drop target, or removes it when
// t is nil.
func SetDropTarget(t DropTarget) {
	dragMu.Lock()
	dropTarget = t
	dragMu.Unlock()
	DebugLog("DND: drop target is now %T", t)
}

// CurrentDropTarget returns the installed drop target, if any.
func CurrentDropTarget() DropTarget {
	dragMu.Lock()
	defer dragMu.Unlock()
	return dropTarget
}

// SetDragBackend is called by a graphical backend once its window exists.
func SetDragBackend(b DragBackend) {
	dragMu.Lock()
	dragBackend = b
	dragMu.Unlock()
	DebugLog("DND: drag backend is now %T", b)
}

// CurrentDragBackend returns the registered backend, if any.
func CurrentDragBackend() DragBackend {
	dragMu.Lock()
	defer dragMu.Unlock()
	return dragBackend
}

// DropSupported reports whether drops from other applications can arrive.
func DropSupported() bool {
	b := CurrentDragBackend()
	return b != nil && b.AcceptsDrops()
}

// DragOutSupported reports whether we can hand a payload to other
// applications. It is the same condition today, but the two directions are
// separate protocols and one may well arrive before the other.
func DragOutSupported() bool {
	b := CurrentDragBackend()
	if b == nil {
		return false
	}
	if s, ok := b.(DragSource); ok {
		return s.CanStartDrag()
	}
	return true
}

// DragSource is implemented by a backend that receives drops but cannot
// start them yet. The two directions are separate protocols and one of them
// usually lands first, so a backend has to be able to say so.
type DragSource interface {
	CanStartDrag() bool
}

// StartDrag offers payload to the rest of the desktop and blocks until the
// gesture ends.
func StartDrag(payload DragPayload, allowed DropAction) (DropAction, error) {
	b := CurrentDragBackend()
	if b == nil {
		DebugLog("DND: drag out asked for with no backend registered")
		return DropNone, ErrDragUnsupported
	}
	return b.StartDrag(payload, allowed)
}

// DeliverDragEvent is what a backend calls for every step of a gesture. It
// moves the call to the UI thread, since backends run their event loops in
// their own goroutines and a target inspects live UI state.
func DeliverDragEvent(ev *DragEvent) DropAction {
	if ev == nil {
		return DropNone
	}
	DebugLog("DND: %s at %d,%d files=%d allowed=%s", ev.Phase, ev.X, ev.Y, len(ev.Payload.Paths), ev.Allowed)
	t := CurrentDropTarget()
	if t == nil {
		DebugLog("DND: no drop target is installed, the payload has nowhere to go")
		return DropNone
	}

	// Backend delivery belongs to the manager and policy which were active
	// when this event arrived. Re-reading package globals while waiting lets a
	// later host/test lifecycle silently redirect an in-flight gesture.
	deliverToUI := DragDeliverToUI
	fm := FrameManager
	timeout := DragDeliverTimeout
	if !deliverToUI || fm == nil || !fm.hasTaskPump() {
		return safeHandleDrag(t, ev)
	}

	res := make(chan DropAction, 1)
	if !fm.enqueueTask(func() { res <- safeHandleDrag(t, ev) }) {
		return DropNone
	}
	select {
	case action := <-res:
		DebugLog("DND: %s handled as %s", ev.Phase, action)
		return action
	case <-time.After(timeout):
		DebugLog("DND: UI thread silent for %s, reporting no action", timeout)
		return DropNone
	}
}

// safeHandleDrag keeps a panicking target from taking the whole desktop's
// drag down with it.
func safeHandleDrag(t DropTarget, ev *DragEvent) (action DropAction) {
	defer func() {
		if r := recover(); r != nil {
			DebugLog("DND: drop target panicked: %v", r)
			action = DropNone
		}
	}()
	return t.HandleDrag(ev)
}

// ParseURIList decodes a text/uri-list body (RFC 2483): one URI per line,
// CRLF separated, lines starting with '#' are comments. This is the format
// every desktop uses for dragged files, so every backend needs it.
func ParseURIList(data string) DragPayload {
	var p DragPayload
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if path, ok := URIToLocalPath(line); ok {
			p.Paths = append(p.Paths, path)
			continue
		}
		p.URIs = append(p.URIs, line)
	}
	if len(p.Paths) > 0 || len(p.URIs) > 0 {
		p.Kinds = []string{"text/uri-list"}
	}
	return p
}

// FormatURIList encodes local paths back into a text/uri-list body.
func FormatURIList(paths []string) string {
	var b strings.Builder
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		b.WriteString(LocalPathToURI(p))
		b.WriteString("\r\n")
	}
	return b.String()
}

// URIToLocalPath converts a file: URI into a path on this machine. A URI
// with a foreign authority names a file somewhere else and is refused, so
// the caller can pass it on as a URI instead of pretending it is local.
func URIToLocalPath(uri string) (string, bool) {
	if !strings.HasPrefix(strings.ToLower(uri), "file:") {
		return "", false
	}
	rest := uri[len("file:"):]
	if strings.HasPrefix(rest, "//") {
		rest = rest[2:]
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return "", false
		}
		host := rest[:slash]
		if host != "" && !strings.EqualFold(host, "localhost") {
			return "", false
		}
		rest = rest[slash:]
	} else if !strings.HasPrefix(rest, "/") {
		return "", false
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil {
		return "", false
	}
	return normalizeURIPath(decoded), true
}

// LocalPathToURI is the other direction, escaping whatever needs escaping.
func LocalPathToURI(path string) string {
	slashed := filepath.ToSlash(path)
	if len(slashed) >= 2 && isDriveLetter(slashed[0]) && slashed[1] == ':' {
		slashed = "/" + slashed
	}
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	u := url.URL{Scheme: "file", Path: slashed}
	return u.String()
}

// normalizeURIPath turns the path part of a URI into a native one. A Windows
// drive letter arrives as "/C:/dir" and has to lose its leading slash before
// it means anything to the OS.
func normalizeURIPath(p string) string {
	if len(p) >= 3 && p[0] == '/' && isDriveLetter(p[1]) && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
