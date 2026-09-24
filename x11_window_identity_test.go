//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris

package vtui

import "testing"

func TestX11WindowClassProperty(t *testing.T) {
	oldName, oldID := AppName, AppID
	defer func() { AppName, AppID = oldName, oldID }()

	AppName = "f4"
	AppID = "org.unxed.f4"
	if got, want := string(x11WindowClassProperty()), "f4\x00org.unxed.f4\x00"; got != want {
		t.Fatalf("WM_CLASS property = %q, want %q", got, want)
	}
}
