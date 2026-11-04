//go:build windows

package vtui

import "testing"

// Both directions reach the HGLOBAL through GlobalLock's address: the copy
// writes the UTF-16 text into it and the paste walks it to the terminator.
// A session with no clipboard (a service without a window station) refuses
// the copy, and there is nothing to check then.
func TestWin32ClipboardRoundTrip(t *testing.T) {
	const text = `C:\каталог\файл с пробелом.txt`
	if !setOSClipboard(text) {
		t.Skip("the Win32 clipboard is not reachable from this session")
	}
	got, ok := getOSClipboard()
	if !ok {
		t.Fatal("the Win32 clipboard accepted a copy but refused the paste")
	}
	if got != text {
		t.Errorf("clipboard: got %q, want %q", got, text)
	}
}

// Text with LF line breaks goes to the clipboard as CRLF and comes back as
// LF, so a copy and a paste inside vtui leave the text as it was.
func TestWin32ClipboardLineBreaksRoundTrip(t *testing.T) {
	const text = "первая\nвторая\n\nthird"
	if !setOSClipboard(text) {
		t.Skip("the Win32 clipboard is not reachable from this session")
	}
	got, ok := getOSClipboard()
	if !ok {
		t.Fatal("the Win32 clipboard accepted a copy but refused the paste")
	}
	if got != text {
		t.Errorf("clipboard: got %q, want %q", got, text)
	}
}
