//go:build windows

package vtui

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

// osClipMu serializes every raw Win32 clipboard transaction. OpenClipboard
// ties ownership to the calling thread, so the sequence also pins its
// goroutine to one OS thread: without both, two concurrent writers (or a
// goroutine migrating threads mid-transaction) corrupt handle ownership --
// SetClipboardData transfers the HGLOBAL to the system, and a racing failure
// path then GlobalFrees memory the system already owns, killing the process
// with no Go-visible error.
var osClipMu sync.Mutex

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procSetClipboardData = user32.NewProc("SetClipboardData")

	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
)

const (
	CF_UNICODETEXT = 13
	GMEM_MOVEABLE  = 0x0002
)

// osClipboardAvailable reports whether the Win32 clipboard can be reached.
// Only a system without user32 answers no, and there the OSC 52 fallback is
// no use either, since a Windows console does not act on it.
func osClipboardAvailable() bool { return procOpenClipboard.Find() == nil }

func setOSClipboard(text string) bool {
	osClipMu.Lock()
	defer osClipMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := procOpenClipboard.Find(); err != nil {
		return false
	}
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return false
	}
	defer procCloseClipboard.Call()

	procEmptyClipboard.Call()

	u16, err := syscall.UTF16FromString(toWindowsLineBreaks(text))
	if err != nil {
		return false
	}

	size := uintptr(len(u16) * 2)
	hMem, _, _ := procGlobalAlloc.Call(GMEM_MOVEABLE, size)
	if hMem == 0 {
		return false
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		procGlobalFree.Call(hMem)
		return false
	}

	// Copy memory safely without CGO. ptr is GlobalLock's view of the
	// HGLOBAL, memory outside the Go heap, and holds exactly len(u16) units.
	copy(unsafe.Slice((*uint16)(winPtr(ptr)), len(u16)), u16)
	procGlobalUnlock.Call(hMem)

	rSet, _, _ := procSetClipboardData.Call(CF_UNICODETEXT, hMem)
	if rSet == 0 {
		procGlobalFree.Call(hMem)
		return false
	}
	return true
}

func getOSClipboard() (string, bool) {
	osClipMu.Lock()
	defer osClipMu.Unlock()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := procOpenClipboard.Find(); err != nil {
		return "", false
	}
	r, _, _ := procOpenClipboard.Call(0)
	if r == 0 {
		return "", false
	}
	defer procCloseClipboard.Call()

	hMem, _, _ := procGetClipboardData.Call(CF_UNICODETEXT)
	if hMem == 0 {
		return "", false
	}

	ptr, _, _ := procGlobalLock.Call(hMem)
	if ptr == 0 {
		return "", false
	}
	defer procGlobalUnlock.Call(hMem)

	// Read UTF-16 string until null terminator. base is GlobalLock's view of
	// the system-owned HGLOBAL.
	base := winPtr(ptr)
	var text []uint16
	for i := 0; ; i++ {
		val := *(*uint16)(unsafe.Add(base, i*2))
		if val == 0 {
			break
		}
		text = append(text, val)
	}

	return fromWindowsLineBreaks(syscall.UTF16ToString(text)), true
}
