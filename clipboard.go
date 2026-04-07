package vtui

import (
	"encoding/base64"
	"os"
	"os/exec"
	"strings"
)

var internalClipboard string

// SetClipboard copies text to the system clipboard.
func SetClipboard(text string) {
	DebugLog("CLIPBOARD: SetClipboard called, len: %d", len(text))
	// Global protection against terminal/IPC overload (2MB limit)
	const maxGlobalClipboardSize = 2 * 1024 * 1024
	if len(text) > maxGlobalClipboardSize {
		text = text[:maxGlobalClipboardSize]
		DebugLog("CLIPBOARD: Text truncated to %d bytes to prevent IPC lockup", maxGlobalClipboardSize)
	}
	internalClipboard = text
	if SetFar2lClipboard(text) {
		DebugLog("CLIPBOARD: SetFar2lClipboard SUCCESS")
		return
	}
	DebugLog("CLIPBOARD: SetFar2lClipboard FAILED or DISABLED")
	if setExternalClipboard(text) {
		DebugLog("CLIPBOARD: setExternalClipboard (X11/Wayland/pbcopy) SUCCESS")
		return
	}
	DebugLog("CLIPBOARD: setExternalClipboard FAILED, falling back to OSC 52")

	// Cap the OSC 52 payload to 1MB to prevent terminal hangs
	const maxClipboardSize = 1024 * 1024
	if len(text) > maxClipboardSize {
		text = text[:maxClipboardSize]
	}
	b64 := base64.StdEncoding.EncodeToString([]byte(text))
	// ANSI OSC 52: \x1b]52;c;<base64>\x07
	os.Stdout.WriteString("\x1b]52;c;" + b64 + "\x07")
}

// GetClipboard retrieves text from the system clipboard.
func GetClipboard() string {
	DebugLog("CLIPBOARD: GetClipboard called")
	if text, ok := GetFar2lClipboard(); ok {
		DebugLog("CLIPBOARD: GetFar2lClipboard SUCCESS, len: %d", len(text))
		return text
	}
	DebugLog("CLIPBOARD: GetFar2lClipboard FAILED or DISABLED")
	if text, ok := getExternalClipboard(); ok {
		DebugLog("CLIPBOARD: getExternalClipboard SUCCESS, len: %d", len(text))
		return text
	}
	DebugLog("CLIPBOARD: Returning internal buffer, len: %d", len(internalClipboard))
	return internalClipboard
}

func setExternalClipboard(text string) bool {
	if _, err := exec.LookPath("wl-copy"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		cmd := exec.Command("wl-copy")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil && os.Getenv("DISPLAY") != "" {
		cmd := exec.Command("xclip", "-selection", "clipboard", "-in")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	if _, err := exec.LookPath("xsel"); err == nil && os.Getenv("DISPLAY") != "" {
		cmd := exec.Command("xsel", "--clipboard", "--input")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	if _, err := exec.LookPath("pbcopy"); err == nil {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return true
		}
	}
	return false
}

func getExternalClipboard() (string, bool) {
	if _, err := exec.LookPath("wl-paste"); err == nil && os.Getenv("WAYLAND_DISPLAY") != "" {
		if out, err := exec.Command("wl-paste", "--no-newline").Output(); err == nil {
			return string(out), true
		}
	}
	if _, err := exec.LookPath("xclip"); err == nil && os.Getenv("DISPLAY") != "" {
		if out, err := exec.Command("xclip", "-selection", "clipboard", "-out").Output(); err == nil {
			return string(out), true
		}
	}
	if _, err := exec.LookPath("xsel"); err == nil && os.Getenv("DISPLAY") != "" {
		if out, err := exec.Command("xsel", "--clipboard", "--output").Output(); err == nil {
			return string(out), true
		}
	}
	if _, err := exec.LookPath("pbpaste"); err == nil {
		if out, err := exec.Command("pbpaste").Output(); err == nil {
			return string(out), true
		}
	}
	return "", false
}