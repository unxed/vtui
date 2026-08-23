//go:build !windows

package vtui

import (
	"github.com/unxed/goclip"
)

// The clipboard of a graphical session, through goclip, which talks to X and
// to Wayland directly and only shells out to xclip, xsel or wl-copy when it
// has to. That is the point: none of those are installed by default on any
// distribution, so a clipboard built on them is a clipboard that does not
// work on a fresh machine. See f4#599.
//
// The file backed driver goclip falls back to last is deliberately skipped.
// It always succeeds, and a success here would stop SetClipboard falling
// through to OSC 52 — which is the only thing that works in a terminal with
// no graphical session behind it at all. vtui already keeps an internal
// buffer for that case, so the file driver would only be a second and worse
// one.

func graphicalClipboard() goclip.Driver {
	for _, name := range goclip.RegisteredDrivers() {
		if name == "file" {
			continue
		}
		if d, ok := goclip.GetDriver(name); ok && d.Available() {
			return d
		}
	}
	return nil
}

func setOSClipboard(text string) bool {
	d := graphicalClipboard()
	if d == nil {
		return false
	}
	return d.WriteText(text) == nil
}

func getOSClipboard() (string, bool) {
	d := graphicalClipboard()
	if d == nil {
		return "", false
	}
	text, err := d.ReadText()
	switch err {
	case nil:
		return text, true
	case goclip.ErrEmpty:
		// An empty clipboard is an answer, and a definite one: falling
		// through would hand back a stale internal buffer instead.
		return "", true
	}
	return "", false
}
