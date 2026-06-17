package vtui

import (
	"encoding/base64"
	"os"
	"sync"
)

var (
	internalClipboard string
	internalClipMu    sync.Mutex
)

// SetClipboard copies text to the system clipboard.
func SetClipboard(text string) {
	DebugLog("CLIPBOARD: SetClipboard called, len: %d", len(text))
	// Global protection against terminal/IPC overload (2MB limit)
	const maxGlobalClipboardSize = 2 * 1024 * 1024
	if len(text) > maxGlobalClipboardSize {
		text = text[:maxGlobalClipboardSize]
		DebugLog("CLIPBOARD: Text truncated to %d bytes to prevent IPC lockup", maxGlobalClipboardSize)
	}
	internalClipMu.Lock()
	internalClipboard = text
	internalClipMu.Unlock()
	if SetFar2lClipboard(text) {
		DebugLog("CLIPBOARD: SetFar2lClipboard SUCCESS")
		return
	}
	DebugLog("CLIPBOARD: SetFar2lClipboard FAILED or DISABLED")
	if setOSClipboard(text) {
		DebugLog("CLIPBOARD: setOSClipboard SUCCESS")
		return
	}
	DebugLog("CLIPBOARD: setOSClipboard FAILED, falling back to OSC 52")

	// Cap the OSC 52 payload to 1MB to prevent terminal hangs
	const maxClipboardSize = 1024 * 1024
	if len(text) > maxClipboardSize {
		text = text[:maxClipboardSize]
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	// ANSI OSC 52: \x1b]52;c;<base64>\x07
	os.Stdout.WriteString("\x1b]52;c;" + b64 + "\x07")
}

// SetOSClipboard bypasses terminal extensions and writes directly to the OS clipboard.
func SetOSClipboard(text string) bool {
	return setOSClipboard(text)
}

// GetClipboard retrieves text from the system clipboard.
func GetClipboard() string {
	DebugLog("CLIPBOARD: GetClipboard called")
	if text, ok := GetFar2lClipboard(); ok {
		DebugLog("CLIPBOARD: GetFar2lClipboard SUCCESS, len: %d", len(text))
		return text
	}
	DebugLog("CLIPBOARD: GetFar2lClipboard FAILED or DISABLED")
	if text, ok := getOSClipboard(); ok {
		DebugLog("CLIPBOARD: getOSClipboard SUCCESS, len: %d", len(text))
		return text
	}
	internalClipMu.Lock()
	fallback := internalClipboard
	internalClipMu.Unlock()
	DebugLog("CLIPBOARD: Returning internal buffer, len: %d", len(fallback))
	return fallback
}

// GetOSClipboard bypasses terminal extensions and reads directly from the OS clipboard.
func GetOSClipboard() string {
	if text, ok := getOSClipboard(); ok {
		return text
	}
	internalClipMu.Lock()
	fallback := internalClipboard
	internalClipMu.Unlock()
	return fallback
}
