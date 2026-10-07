//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"
	"github.com/unxed/vtinput"
)

// fakeDPIConn stands in for an X server: interned atoms, one selection
// owner, window properties, and a record of the event masks selected.
type fakeDPIConn struct {
	atoms    map[string]xproto.Atom
	owner    xproto.Window
	props    map[[2]uint32][]byte
	selected map[xproto.Window]uint32
}

func newFakeDPIConn() *fakeDPIConn {
	return &fakeDPIConn{
		atoms:    map[string]xproto.Atom{},
		props:    map[[2]uint32][]byte{},
		selected: map[xproto.Window]uint32{},
	}
}

func (c *fakeDPIConn) internAtom(name string) xproto.Atom {
	if a, ok := c.atoms[name]; ok {
		return a
	}
	a := xproto.Atom(100 + len(c.atoms)) // #nosec G115 -- test fake: a few atoms
	c.atoms[name] = a
	return a
}

func (c *fakeDPIConn) selectionOwner(selection xproto.Atom) xproto.Window {
	if selection == c.atoms["_XSETTINGS_S0"] {
		return c.owner
	}
	return 0
}

func (c *fakeDPIConn) property(win xproto.Window, prop xproto.Atom) []byte {
	return c.props[[2]uint32{uint32(win), uint32(prop)}]
}

func (c *fakeDPIConn) selectEvents(win xproto.Window, mask uint32) {
	c.selected[win] = mask
}

func (c *fakeDPIConn) setProp(win xproto.Window, name string, value []byte) {
	c.props[[2]uint32{uint32(win), uint32(c.internAtom(name))}] = value
}

const (
	fakeRoot  = xproto.Window(1)
	fakeOwner = xproto.Window(50)
)

func xsettingsDPIBlob(dpi int32) []byte {
	return xsettingsBlob(binary.LittleEndian, []xsettingsEntry{
		{typ: xsettingsTypeInteger, name: "Xft/DPI", ival: dpi * 1024},
	})
}

func TestX11DPIWatch_FindsManagerAndSubscribes(t *testing.T) {
	conn := newFakeDPIConn()
	conn.owner = fakeOwner
	w := newX11DPIWatch(conn, fakeRoot, 0)

	if w.xsettingsOwner != fakeOwner {
		t.Fatalf("xsettingsOwner = %d, want %d", w.xsettingsOwner, fakeOwner)
	}
	if conn.selected[fakeOwner] != x11DPIEventMask {
		t.Errorf("manager window mask = %#x, want %#x", conn.selected[fakeOwner], x11DPIEventMask)
	}
	if _, ok := conn.selected[fakeRoot]; ok {
		t.Error("the root was subscribed before subscribe was called")
	}
	w.subscribe()
	if conn.selected[fakeRoot] != x11DPIEventMask {
		t.Errorf("root mask = %#x, want %#x", conn.selected[fakeRoot], x11DPIEventMask)
	}
}

func TestX11DPIWatch_NoManager(t *testing.T) {
	conn := newFakeDPIConn()
	w := newX11DPIWatch(conn, fakeRoot, 0)
	if w.xsettingsOwner != 0 {
		t.Fatalf("xsettingsOwner = %d without a manager, want 0", w.xsettingsOwner)
	}
	if len(conn.selected) != 0 {
		t.Errorf("events selected without a manager: %v", conn.selected)
	}
}

// XSETTINGS wins over the Xft.dpi resource, which wins over the default.
func TestX11DPIWatch_ReadDPIPrecedence(t *testing.T) {
	conn := newFakeDPIConn()
	conn.owner = fakeOwner
	w := newX11DPIWatch(conn, fakeRoot, 0)

	if got := w.readDPI(); got != 96 {
		t.Errorf("nothing published: readDPI = %v, want 96", got)
	}
	conn.setProp(fakeRoot, "RESOURCE_MANAGER", []byte("Xft.dpi:\t144\n"))
	if got := w.readDPI(); got != 144 {
		t.Errorf("resource only: readDPI = %v, want 144", got)
	}
	conn.setProp(fakeOwner, "_XSETTINGS_SETTINGS", xsettingsBlob(binary.LittleEndian,
		[]xsettingsEntry{{typ: xsettingsTypeString, name: "Net/ThemeName", sval: "Adwaita"}}))
	if got := w.readDPI(); got != 144 {
		t.Errorf("XSETTINGS without Xft/DPI: readDPI = %v, want the resource's 144", got)
	}
	conn.setProp(fakeOwner, "_XSETTINGS_SETTINGS", xsettingsDPIBlob(192))
	if got := w.readDPI(); got != 192 {
		t.Errorf("both: readDPI = %v, want XSETTINGS' 192", got)
	}
}

func newX11DPITestHost(t *testing.T, conn *fakeDPIConn) *X11Host {
	t.Helper()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 5)
	_, cellW, cellH := loadBestFont("", 16, 72)
	host := &X11Host{cols: 10, rows: 5, cellW: cellW, cellH: cellH, scale: 1,
		fontSize: 16, dpi: 72, scr: scr, wid: 7,
		width: uint16(10 * cellW), height: uint16(5 * cellH)} // #nosec G115 -- test fixture: a 10x5 grid of font-sized cells
	host.renderer = NewX11Renderer(host, nil)
	host.dpiWatch = newX11DPIWatch(conn, fakeRoot, 0)
	return host
}

// A changed Xft.dpi resource on the root rescales the window; unrelated
// properties and events are left to the event loop.
func TestX11Host_HandleDPIEventResource(t *testing.T) {
	conn := newFakeDPIConn()
	host := newX11DPITestHost(t, conn)

	unrelated := xproto.PropertyNotifyEvent{Window: fakeRoot, Atom: conn.internAtom("_NET_ACTIVE_WINDOW")}
	if host.handleDPIEvent(unrelated) {
		t.Error("an unrelated root property was taken as a DPI change")
	}
	if host.handleDPIEvent(xproto.ExposeEvent{Window: 7}) {
		t.Error("an Expose event was taken as a DPI change")
	}

	conn.setProp(fakeRoot, "RESOURCE_MANAGER", []byte("Xft.dpi: 192\n"))
	ev := xproto.PropertyNotifyEvent{Window: fakeRoot, Atom: conn.internAtom("RESOURCE_MANAGER")}
	if !host.handleDPIEvent(ev) {
		t.Fatal("a RESOURCE_MANAGER change was not handled")
	}
	if host.dpi != 144 || host.scale != 2 {
		t.Fatalf("dpi/scale = %v/%d, want 144/2", host.dpi, host.scale)
	}
	if host.cols != 10 || host.rows != 5 {
		t.Fatalf("grid = %dx%d, want unchanged 10x5", host.cols, host.rows)
	}
}

// A new XSETTINGS manager is found, subscribed to, and its DPI applied.
func TestX11Host_HandleDPIEventNewManager(t *testing.T) {
	conn := newFakeDPIConn()
	host := newX11DPITestHost(t, conn)
	if host.dpiWatch.xsettingsOwner != 0 {
		t.Fatal("setup: a manager exists before one was started")
	}

	conn.owner = fakeOwner
	conn.setProp(fakeOwner, "_XSETTINGS_SETTINGS", xsettingsDPIBlob(288))
	msg := xproto.ClientMessageEvent{Format: 32, Window: fakeRoot, Type: conn.internAtom("MANAGER")}
	msg.Data = xproto.ClientMessageDataUnionData32New([]uint32{0, uint32(conn.internAtom("_XSETTINGS_S0")), uint32(fakeOwner), 0, 0})
	if !host.handleDPIEvent(msg) {
		t.Fatal("the MANAGER announcement was not handled")
	}
	if host.dpiWatch.xsettingsOwner != fakeOwner || conn.selected[fakeOwner] == 0 {
		t.Fatal("the new manager was not found and subscribed to")
	}
	if host.dpi != 216 || host.scale != 3 {
		t.Fatalf("dpi/scale = %v/%d, want 216/3", host.dpi, host.scale)
	}

	// The manager's own settings changing is a DPI change too.
	conn.setProp(fakeOwner, "_XSETTINGS_SETTINGS", xsettingsDPIBlob(96))
	if !host.handleDPIEvent(xproto.PropertyNotifyEvent{Window: fakeOwner, Atom: conn.internAtom("_XSETTINGS_SETTINGS")}) {
		t.Fatal("an XSETTINGS change was not handled")
	}
	if host.dpi != 72 || host.scale != 1 {
		t.Fatalf("after going back to 96: dpi/scale = %v/%d, want 72/1", host.dpi, host.scale)
	}

	other := xproto.ClientMessageEvent{Format: 32, Window: 7, Type: conn.internAtom("WM_PROTOCOLS")}
	other.Data = xproto.ClientMessageDataUnionData32New([]uint32{0, 0, 0, 0, 0})
	if host.handleDPIEvent(other) {
		t.Error("a WM_PROTOCOLS message was taken as a DPI change")
	}
}

func TestX11Host_HandleDPIEventWithoutWatch(t *testing.T) {
	host := &X11Host{}
	if host.handleDPIEvent(xproto.PropertyNotifyEvent{Window: fakeRoot, Atom: 1}) {
		t.Error("a host without a DPI watch handled an event")
	}
	host.refreshDPI() // must not panic
}

// Configures of the root window, selected for the DPI watch, leave the grid
// alone; the host window's own resize reaches the application.
func TestX11Host_OnConfigureNotify(t *testing.T) {
	pr, pw := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	t.Cleanup(func() {
		reader.Close()
		_ = pw.Close()
	})
	host := &X11Host{wid: 7, cols: 10, rows: 5, cellW: 9, cellH: 17, width: 90, height: 85,
		reader: reader, closeChan: make(chan struct{})}

	host.onConfigureNotify(xproto.ConfigureNotifyEvent{Window: fakeRoot, Width: 1920, Height: 1080})
	if host.cols != 10 || host.rows != 5 || host.width != 90 {
		t.Fatalf("a root configure changed the grid to %dx%d", host.cols, host.rows)
	}

	go host.onConfigureNotify(xproto.ConfigureNotifyEvent{Window: 7, Width: 180, Height: 170})
	select {
	case ev := <-reader.EventChan:
		if ev.Type != vtinput.ResizeEventType {
			t.Fatalf("event = %+v, want a resize", ev)
		}
	case <-time.After(time.Second):
		t.Fatal("no resize event for the host window's configure")
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.cols != 20 || host.rows != 10 {
		t.Fatalf("grid = %dx%d, want 20x10", host.cols, host.rows)
	}
}
