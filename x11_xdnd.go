//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"strings"
	"sync"
	"time"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
	"github.com/unxed/vtinput"
)

// dragOutTimeout gives up on a target that never answers our drop. The
// pointer is already ungrabbed by then, so the user is not held hostage
// either way; this only decides when we stop waiting for a reply.
const dragOutTimeout = 30 * time.Second

// xdndVersion is the protocol version we announce. Version 5 is what every
// current toolkit speaks; older sources are handled by the same code, since
// the messages we care about have not changed shape.
const xdndVersion = 5

// xdndMaxTransfer bounds a single property read. A dropped selection of a
// few thousand files still fits; anything past it comes back in a second
// read, and an INCR transfer is refused outright (see readTransfer).
const xdndMaxTransfer = 1 << 18

type x11DndAtoms struct {
	aware     xproto.Atom
	selection xproto.Atom
	enter     xproto.Atom
	position  xproto.Atom
	status    xproto.Atom
	leave     xproto.Atom
	drop      xproto.Atom
	finished  xproto.Atom
	typeList  xproto.Atom
	actList   xproto.Atom

	actCopy    xproto.Atom
	actMove    xproto.Atom
	actLink    xproto.Atom
	actAsk     xproto.Atom
	actPrivate xproto.Atom

	uriList   xproto.Atom
	plainUTF8 xproto.Atom
	plain     xproto.Atom
	utf8      xproto.Atom

	transfer xproto.Atom
	incr     xproto.Atom
	targets  xproto.Atom
}

// x11Dnd is the receiving half of XDND for one window. Everything here runs
// on the X11 event loop goroutine; the hop to the UI thread happens inside
// DeliverDragEvent.
type x11Dnd struct {
	host *X11Host
	conn *xgb.Conn
	a    x11DndAtoms

	source     xproto.Window
	version    int
	chosen     xproto.Atom
	allowed    DropAction
	accepted   DropAction
	inside     bool
	waiting    bool
	lastX      int
	lastY      int
	lastMods   vtinput.ControlKeyState
	srcMu      sync.Mutex
	srcActive  bool
	srcData    []byte
	srcAllowed DropAction
	srcResult  chan DropAction

	srcTarget  xproto.Window
	srcVersion int
	srcStatus  DropAction
	srcDropped bool

	// srcWindow owns the selection and the pointer grab of a drag out,
	// and srcRoot is the root it walks looking for a target. Here they
	// are the host's window and screen, but the source half wants nothing
	// else from a host, so a backend with no X11Host of its own can drive
	// the same code with a window of its own making.
	srcWindow xproto.Window
	srcRoot   xproto.Window
	// srcReleased is called once the pointer has been handed back, for a
	// host that has to be told the button it never saw released is up. A
	// source that owns no UI leaves it nil.
	srcReleased func()
}

// newX11Dnd interns the atoms and marks the window as a drop target. It
// returns nil when the server does not give us the atoms we need, which
// leaves the backend reporting no drop support rather than half of it.
func newX11Dnd(h *X11Host) *x11Dnd {
	if h == nil || h.conn == nil {
		return nil
	}
	d := &x11Dnd{host: h, conn: h.conn, srcWindow: h.wid}
	if !d.internAtoms() {
		DebugLog("XDND: could not intern the required atoms, drops disabled")
		return nil
	}
	if h.screen != nil {
		d.srcRoot = h.screen.Root
	}
	d.srcReleased = func() {
		h.mu.Lock()
		h.mouseBtn = 0
		h.mu.Unlock()
		x, y, mods := d.pointer(0, 0)
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          int16(x),
			MouseY:          int16(y),
			ButtonState:     0,
			KeyDown:         false,
			ControlKeyState: mods,
		})
	}
	data := make([]byte, 4)
	xgb.Put32(data, xdndVersion)
	xproto.ChangeProperty(d.conn, xproto.PropModeReplace, h.wid, d.a.aware,
		xproto.AtomAtom, 32, 1, data)
	DebugLog("XDND: window %d announced as a drop target (version %d)", h.wid, xdndVersion)
	return d
}

func (d *x11Dnd) intern(name string) xproto.Atom {
	reply, err := xproto.InternAtom(d.conn, false, uint16(len(name)), name).Reply()
	if err != nil || reply == nil {
		return 0
	}
	return reply.Atom
}

func (d *x11Dnd) internAtoms() bool {
	d.a.aware = d.intern("XdndAware")
	d.a.selection = d.intern("XdndSelection")
	d.a.enter = d.intern("XdndEnter")
	d.a.position = d.intern("XdndPosition")
	d.a.status = d.intern("XdndStatus")
	d.a.leave = d.intern("XdndLeave")
	d.a.drop = d.intern("XdndDrop")
	d.a.finished = d.intern("XdndFinished")
	d.a.typeList = d.intern("XdndTypeList")
	d.a.actList = d.intern("XdndActionList")

	d.a.actCopy = d.intern("XdndActionCopy")
	d.a.actMove = d.intern("XdndActionMove")
	d.a.actLink = d.intern("XdndActionLink")
	d.a.actAsk = d.intern("XdndActionAsk")
	d.a.actPrivate = d.intern("XdndActionPrivate")

	d.a.uriList = d.intern("text/uri-list")
	d.a.plainUTF8 = d.intern("text/plain;charset=utf-8")
	d.a.plain = d.intern("text/plain")
	d.a.utf8 = d.intern("UTF8_STRING")

	d.a.transfer = d.intern("VTUI_XDND_TRANSFER")
	d.a.incr = d.intern("INCR")
	d.a.targets = d.intern("TARGETS")

	return d.a.aware != 0 && d.a.selection != 0 && d.a.enter != 0 &&
		d.a.position != 0 && d.a.status != 0 && d.a.drop != 0 &&
		d.a.finished != 0 && d.a.transfer != 0 && d.a.uriList != 0
}

// handleClientMessage returns true when the message belonged to XDND.
func (d *x11Dnd) handleClientMessage(e *xproto.ClientMessageEvent) bool {
	if d == nil || e == nil || e.Format != 32 {
		return false
	}
	switch e.Type {
	case d.a.enter:
		d.onEnter(e)
	case d.a.position:
		d.onPosition(e)
	case d.a.leave:
		d.onLeave()
	case d.a.drop:
		d.onDrop(e)
	case d.a.status:
		d.onSourceStatus(e)
	case d.a.finished:
		d.onSourceFinished(e)
	default:
		return false
	}
	return true
}

func (d *x11Dnd) onEnter(e *xproto.ClientMessageEvent) {
	data := e.Data.Data32
	d.reset()
	d.source = xproto.Window(data[0])
	d.version = int(data[1] >> 24)
	d.chosen = d.pickType(d.enterTypes(data))
	d.allowed = d.sourceActions()
	DebugLog("XDND: enter from %d, version %d, type %d, actions %s",
		d.source, d.version, d.chosen, d.allowed)
}

// enterTypes returns the types the source offers: three of them travel in
// the message itself, and a longer list waits in a property on the source.
func (d *x11Dnd) enterTypes(data []uint32) []xproto.Atom {
	if len(data) < 5 {
		return nil
	}
	if data[1]&1 != 0 {
		if list := d.propertyAtoms(xproto.Window(data[0]), d.a.typeList); len(list) > 0 {
			return list
		}
	}
	var out []xproto.Atom
	for _, v := range data[2:5] {
		if v != 0 {
			out = append(out, xproto.Atom(v))
		}
	}
	return out
}

// pickType chooses what to ask the source for. A file list is what a file
// manager wants; plain text is accepted so that a dragged URL or snippet
// still reaches the application.
func (d *x11Dnd) pickType(types []xproto.Atom) xproto.Atom {
	for _, want := range []xproto.Atom{d.a.uriList, d.a.plainUTF8, d.a.utf8, d.a.plain} {
		if want == 0 {
			continue
		}
		for _, t := range types {
			if t == want {
				return want
			}
		}
	}
	return 0
}

// sourceActions reads XdndActionList. A source that does not publish one
// leaves us with only the action it proposes in each XdndPosition, which is
// the conservative reading: we never announce a move the source did not
// offer, since only the source can honour it by deleting the original.
func (d *x11Dnd) sourceActions() DropAction {
	var out DropAction
	for _, a := range d.propertyAtoms(d.source, d.a.actList) {
		out |= d.actionOf(a)
	}
	return out
}

func (d *x11Dnd) actionOf(a xproto.Atom) DropAction {
	if a == 0 {
		return DropNone
	}
	switch a {
	case d.a.actMove:
		return DropMove
	case d.a.actLink:
		return DropLink
	case d.a.actCopy, d.a.actAsk, d.a.actPrivate:
		return DropCopy
	}
	return DropNone
}

func (d *x11Dnd) atomOf(a DropAction) xproto.Atom {
	switch {
	case a.Has(DropMove):
		return d.a.actMove
	case a.Has(DropLink):
		return d.a.actLink
	case a.Has(DropCopy):
		return d.a.actCopy
	}
	return 0
}

func (d *x11Dnd) kinds() []string {
	switch d.chosen {
	case d.a.uriList:
		return []string{"text/uri-list"}
	case d.a.plainUTF8, d.a.utf8:
		return []string{"text/plain;charset=utf-8"}
	case d.a.plain:
		return []string{"text/plain"}
	}
	return nil
}

func (d *x11Dnd) propertyAtoms(w xproto.Window, prop xproto.Atom) []xproto.Atom {
	if w == 0 || prop == 0 {
		return nil
	}
	reply, err := xproto.GetProperty(d.conn, false, w, prop, xproto.AtomAtom, 0, 1024).Reply()
	if err != nil || reply == nil || reply.Format != 32 {
		return nil
	}
	out := make([]xproto.Atom, 0, len(reply.Value)/4)
	for i := 0; i+4 <= len(reply.Value); i += 4 {
		if v := xgb.Get32(reply.Value[i:]); v != 0 {
			out = append(out, xproto.Atom(v))
		}
	}
	return out
}

func (d *x11Dnd) onPosition(e *xproto.ClientMessageEvent) {
	data := e.Data.Data32
	if d.source == 0 {
		d.source = xproto.Window(data[0])
	}
	suggested := d.actionOf(xproto.Atom(data[4]))
	allowed := d.allowed | suggested

	x, y, mods := d.pointer(int(int16(data[2]>>16)), int(int16(data[2]&0xFFFF)))
	phase := DragOver
	if !d.inside {
		phase = DragEnter
		d.inside = true
	}

	action := DropNone
	if d.chosen != 0 {
		action = DeliverDragEvent(&DragEvent{
			Phase:     phase,
			X:         x,
			Y:         y,
			Modifiers: mods,
			Allowed:   allowed,
			Suggested: suggested,
			Payload:   DragPayload{Kinds: d.kinds()},
		})
	}
	if !allowed.Has(action) {
		action = DropNone
	}

	d.accepted = action
	d.lastX, d.lastY, d.lastMods = x, y, mods
	d.sendStatus(action)
}

// pointer converts the root coordinates of a position message into cells.
// Asking the server where the pointer is answers both that and which
// modifiers are held, which XdndPosition itself does not carry.
func (d *x11Dnd) pointer(rootX, rootY int) (int, int, vtinput.ControlKeyState) {
	h := d.host
	if reply, err := xproto.QueryPointer(d.conn, h.wid).Reply(); err == nil && reply != nil {
		return dndCell(int(reply.WinX), h.cellW), dndCell(int(reply.WinY), h.cellH), h.translateModifiers(reply.Mask)
	}
	if h.screen != nil {
		reply, err := xproto.TranslateCoordinates(d.conn, h.screen.Root, h.wid,
			int16(rootX), int16(rootY)).Reply()
		if err == nil && reply != nil {
			return dndCell(int(reply.DstX), h.cellW), dndCell(int(reply.DstY), h.cellH), 0
		}
	}
	return 0, 0, 0
}

// dndCell turns a pixel offset into a cell index, guarding a cell size the
// host may not have computed yet.
func dndCell(px, cell int) int {
	if cell <= 0 || px <= 0 {
		return 0
	}
	return px / cell
}

func (d *x11Dnd) onLeave() {
	if d.inside {
		DeliverDragEvent(&DragEvent{Phase: DragLeave, X: d.lastX, Y: d.lastY})
	}
	DebugLog("XDND: leave")
	d.reset()
}

func (d *x11Dnd) onDrop(e *xproto.ClientMessageEvent) {
	data := e.Data.Data32
	if d.accepted == DropNone || d.chosen == 0 {
		d.sendFinished(false)
		d.reset()
		return
	}
	ts := xproto.Timestamp(data[2])
	if ts == 0 {
		ts = xproto.TimeCurrentTime
	}
	d.waiting = true
	xproto.ConvertSelection(d.conn, d.host.wid, d.a.selection, d.chosen, d.a.transfer, ts)
	DebugLog("XDND: drop accepted as %s, selection requested", d.accepted)
}

// handleSelectionNotify picks up the data the source put on our window and
// finishes the gesture. Returns true when the notification was ours.
func (d *x11Dnd) handleSelectionNotify(e *xproto.SelectionNotifyEvent) bool {
	if d == nil || e == nil || !d.waiting || e.Selection != d.a.selection {
		return false
	}
	d.waiting = false

	payload := d.readTransfer(e.Property)
	action := DropNone
	if !payload.IsEmpty() {
		action = DeliverDragEvent(&DragEvent{
			Phase:     DragDrop,
			X:         d.lastX,
			Y:         d.lastY,
			Modifiers: d.lastMods,
			Allowed:   d.allowed | d.accepted,
			Suggested: d.accepted,
			Payload:   payload,
		})
	}
	if action != DropNone {
		d.accepted = action
	}
	DebugLog("XDND: drop delivered, files %d, action %s", len(payload.Paths), action)
	d.sendFinished(action != DropNone)
	d.reset()
	return true
}

func (d *x11Dnd) readTransfer(prop xproto.Atom) DragPayload {
	if prop == 0 {
		return DragPayload{}
	}
	reply, err := xproto.GetProperty(d.conn, true, d.host.wid, prop,
		xproto.AtomAny, 0, xdndMaxTransfer).Reply()
	if err != nil || reply == nil {
		return DragPayload{}
	}
	if d.a.incr != 0 && reply.Type == d.a.incr {
		// An incremental transfer needs a PropertyNotify pump we do not
		// have yet. Refusing is honest: the source is told the drop
		// failed instead of silently losing part of the list.
		DebugLog("XDND: INCR transfer is not supported yet")
		return DragPayload{}
	}
	body := reply.Value
	if reply.BytesAfter > 0 {
		more, err := xproto.GetProperty(d.conn, true, d.host.wid, prop,
			xproto.AtomAny, uint32(len(body)/4), reply.BytesAfter/4+1).Reply()
		if err == nil && more != nil {
			body = append(body, more.Value...)
		}
	}

	if d.chosen == d.a.uriList {
		return ParseURIList(string(body))
	}
	text := strings.TrimRight(string(body), "\x00")
	if text == "" {
		return DragPayload{}
	}
	return DragPayload{Kinds: d.kinds(), Text: text}
}

func (d *x11Dnd) sendStatus(action DropAction) {
	if d.source == 0 {
		return
	}
	var data [5]uint32
	data[0] = uint32(d.host.wid)
	if action != DropNone {
		data[1] = 1
	}
	// An empty rectangle asks the source to keep sending positions: our
	// answer changes from one cell to the next.
	data[4] = uint32(d.atomOf(action))
	d.send(d.source, d.a.status, data)
}

func (d *x11Dnd) sendFinished(accepted bool) {
	if d.source == 0 {
		return
	}
	var data [5]uint32
	data[0] = uint32(d.host.wid)
	if accepted {
		data[1] = 1
		data[2] = uint32(d.atomOf(d.accepted))
	}
	d.send(d.source, d.a.finished, data)
}

func (d *x11Dnd) send(target xproto.Window, typ xproto.Atom, data [5]uint32) {
	values := data
	ev := xproto.ClientMessageEvent{
		Format: 32,
		Window: target,
		Type:   typ,
		Data:   xproto.ClientMessageDataUnionData32New(values[:]),
	}
	xproto.SendEvent(d.conn, false, target, 0, string(ev.Bytes()))
}

func (d *x11Dnd) reset() {
	d.source = 0
	d.version = 0
	d.chosen = 0
	d.allowed = DropNone
	d.accepted = DropNone
	d.inside = false
	d.waiting = false
}

// AcceptsDrops implements DragBackend: the window is an XDND target.
func (h *X11Host) AcceptsDrops() bool { return h.dnd != nil }

// CanStartDrag implements DragSource.
func (h *X11Host) CanStartDrag() bool { return h.dnd != nil }

// StartDrag implements DragBackend.
func (h *X11Host) StartDrag(payload DragPayload, allowed DropAction) (DropAction, error) {
	if h.dnd == nil {
		return DropNone, ErrDragUnsupported
	}
	return h.dnd.startDrag(payload, allowed)
}

// startDrag runs the source half of the gesture. It is called from the UI
// goroutine and blocks until the pointer is released and the target has
// spoken, while the state machine below runs on the X11 event loop.
func (d *x11Dnd) startDrag(payload DragPayload, allowed DropAction) (DropAction, error) {
	data := []byte(FormatURIList(payload.Paths))
	if len(data) == 0 {
		return DropNone, ErrDragNoData
	}

	d.srcMu.Lock()
	if d.srcActive {
		d.srcMu.Unlock()
		return DropNone, ErrDragBusy
	}
	result := make(chan DropAction, 1)
	d.srcActive = true
	d.srcData = data
	d.srcAllowed = allowed
	d.srcResult = result
	d.srcMu.Unlock()

	d.srcTarget = 0
	d.srcVersion = 0
	d.srcStatus = DropNone
	d.srcDropped = false

	acts := make([]byte, 4)
	xgb.Put32(acts, uint32(d.a.actCopy))
	xproto.ChangeProperty(d.conn, xproto.PropModeReplace, d.srcWindow, d.a.actList,
		xproto.AtomAtom, 32, 1, acts)
	xproto.SetSelectionOwner(d.conn, d.srcWindow, d.a.selection, xproto.TimeCurrentTime)

	mask := uint16(xproto.EventMaskButtonRelease | xproto.EventMaskPointerMotion)
	if _, err := xproto.GrabPointer(d.conn, false, d.srcWindow, mask,
		xproto.GrabModeAsync, xproto.GrabModeAsync,
		xproto.Window(0), xproto.Cursor(0), xproto.TimeCurrentTime).Reply(); err != nil {
		d.finishSource(DropNone)
		return DropNone, err
	}
	DebugLog("XDND: drag out started, %d file(s)", len(payload.Paths))

	select {
	case action := <-result:
		return action, nil
	case <-time.After(dragOutTimeout):
		DebugLog("XDND: drag out timed out waiting for the target")
		d.abortSource()
		return DropNone, nil
	}
}

// draggingOut reports whether the pointer currently belongs to a drag of
// ours, which is when the host must stop feeding pointer events to the UI.
func (d *x11Dnd) draggingOut() bool {
	if d == nil {
		return false
	}
	d.srcMu.Lock()
	defer d.srcMu.Unlock()
	return d.srcActive
}

func (d *x11Dnd) srcDataCopy() []byte {
	d.srcMu.Lock()
	defer d.srcMu.Unlock()
	return d.srcData
}

// srcMotion follows the pointer: it finds who is under it and keeps that
// window told where we are.
func (d *x11Dnd) srcMotion(rootX, rootY int, t xproto.Timestamp) {
	target, version := d.findTarget(rootX, rootY)
	if target != d.srcTarget {
		if d.srcTarget != 0 {
			d.sendSourceLeave()
		}
		d.srcTarget, d.srcVersion = target, version
		d.srcStatus = DropNone
		if target != 0 {
			d.sendSourceEnter()
			DebugLog("XDND: drag out entered window %d (version %d)", target, version)
		}
	}
	if d.srcTarget == 0 {
		return
	}
	var data [5]uint32
	data[0] = uint32(d.srcWindow)
	data[2] = uint32(uint16(rootX))<<16 | uint32(uint16(rootY))
	data[3] = uint32(t)
	data[4] = uint32(d.a.actCopy)
	d.send(d.srcTarget, d.a.position, data)
}

// srcRelease ends the gesture. The pointer is handed back first, then the
// UI is told the button is up: it never saw the release, since every event
// of the drag was kept away from it.
func (d *x11Dnd) srcRelease(t xproto.Timestamp) {
	xproto.UngrabPointer(d.conn, t)

	if d.srcReleased != nil {
		d.srcReleased()
	}

	if d.srcTarget != 0 && d.srcStatus != DropNone {
		d.srcDropped = true
		var data [5]uint32
		data[0] = uint32(d.srcWindow)
		data[2] = uint32(t)
		d.send(d.srcTarget, d.a.drop, data)
		DebugLog("XDND: drop sent to %d as %s", d.srcTarget, d.srcStatus)
		return
	}
	if d.srcTarget != 0 {
		d.sendSourceLeave()
	}
	d.finishSource(DropNone)
}

func (d *x11Dnd) onSourceStatus(e *xproto.ClientMessageEvent) {
	if !d.draggingOut() {
		return
	}
	d.srcStatus = d.statusAction(e.Data.Data32)
}

// statusAction reads an XdndStatus. A target that accepts without naming an
// action means copy, which is the only thing we offer anyway.
func (d *x11Dnd) statusAction(data []uint32) DropAction {
	if len(data) < 5 || data[1]&1 == 0 {
		return DropNone
	}
	if a := d.actionOf(xproto.Atom(data[4])); a != DropNone {
		return a
	}
	return DropCopy
}

func (d *x11Dnd) onSourceFinished(e *xproto.ClientMessageEvent) {
	if !d.draggingOut() || !d.srcDropped {
		return
	}
	action := d.srcStatus
	if len(e.Data.Data32) >= 2 && d.srcVersion >= 5 && e.Data.Data32[1]&1 == 0 {
		action = DropNone
	}
	DebugLog("XDND: target finished with %s", action)
	d.finishSource(action)
}

func (d *x11Dnd) sendSourceEnter() {
	var data [5]uint32
	data[0] = uint32(d.srcWindow)
	data[1] = uint32(xdndVersion) << 24
	data[2] = uint32(d.a.uriList)
	d.send(d.srcTarget, d.a.enter, data)
}

func (d *x11Dnd) sendSourceLeave() {
	var data [5]uint32
	data[0] = uint32(d.srcWindow)
	d.send(d.srcTarget, d.a.leave, data)
}

func (d *x11Dnd) abortSource() {
	xproto.UngrabPointer(d.conn, xproto.TimeCurrentTime)
	if d.srcTarget != 0 {
		d.sendSourceLeave()
	}
	d.finishSource(DropNone)
}

func (d *x11Dnd) finishSource(action DropAction) {
	d.srcMu.Lock()
	ch := d.srcResult
	d.srcActive = false
	d.srcResult = nil
	d.srcData = nil
	d.srcMu.Unlock()

	d.srcTarget = 0
	d.srcVersion = 0
	d.srcStatus = DropNone
	d.srcDropped = false

	if ch != nil {
		select {
		case ch <- action:
		default:
		}
	}
}

// findTarget walks down from the root looking for a window that announces
// XdndAware. The window manager's frame sits between the root and the real
// client window, so the first candidate is rarely the right one.
func (d *x11Dnd) findTarget(rootX, rootY int) (xproto.Window, int) {
	if d.srcRoot == 0 {
		return 0, 0
	}
	root := d.srcRoot
	w := root
	for i := 0; i < 16; i++ {
		reply, err := xproto.TranslateCoordinates(d.conn, root, w,
			int16(rootX), int16(rootY)).Reply()
		if err != nil || reply == nil || reply.Child == 0 {
			break
		}
		w = reply.Child
		if v := d.awareVersion(w); v > 0 {
			return w, v
		}
	}
	return 0, 0
}

func (d *x11Dnd) awareVersion(w xproto.Window) int {
	if w == 0 || d.a.aware == 0 {
		return 0
	}
	reply, err := xproto.GetProperty(d.conn, false, w, d.a.aware, xproto.AtomAny, 0, 1).Reply()
	if err != nil || reply == nil || reply.Format != 32 || len(reply.Value) < 4 {
		return 0
	}
	return int(xgb.Get32(reply.Value))
}

// sourceTargets is what we answer a TARGETS request with: the one type we
// publish, plus the TARGETS atom itself as the convention requires.
func (d *x11Dnd) sourceTargets() []xproto.Atom {
	return []xproto.Atom{d.a.targets, d.a.uriList}
}

// handleSelectionRequest serves the data itself. It arrives after the drop,
// from the window that accepted it. Returns true when it was ours.
func (d *x11Dnd) handleSelectionRequest(e *xproto.SelectionRequestEvent) bool {
	if d == nil || e == nil || e.Selection != d.a.selection {
		return false
	}
	prop := e.Property
	if prop == 0 {
		prop = e.Target
	}

	served := false
	switch {
	case e.Target == d.a.targets:
		list := d.sourceTargets()
		buf := make([]byte, 4*len(list))
		for i, a := range list {
			xgb.Put32(buf[i*4:], uint32(a))
		}
		xproto.ChangeProperty(d.conn, xproto.PropModeReplace, e.Requestor, prop,
			xproto.AtomAtom, 32, uint32(len(list)), buf)
		served = true
	case e.Target == d.a.uriList || e.Target == d.a.plain ||
		e.Target == d.a.plainUTF8 || e.Target == d.a.utf8:
		if data := d.srcDataCopy(); len(data) > 0 {
			xproto.ChangeProperty(d.conn, xproto.PropModeReplace, e.Requestor, prop,
				e.Target, 8, uint32(len(data)), data)
			served = true
		}
	}

	notify := xproto.SelectionNotifyEvent{
		Time:      e.Time,
		Requestor: e.Requestor,
		Selection: e.Selection,
		Target:    e.Target,
	}
	if served {
		notify.Property = prop
	}
	xproto.SendEvent(d.conn, false, e.Requestor, 0, string(notify.Bytes()))
	return true
}
