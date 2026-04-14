//go:build windows

package vtui

import (
	"os"
	"golang.org/x/sys/windows"
)

func RedirectStderr(f *os.File) error {
	err := windows.SetStdHandle(windows.STD_ERROR_HANDLE, windows.Handle(f.Fd()))
	os.Stderr = f
	return err
}
func countOpenFDs() int {
	// Not supported via simple VFS operations on Windows without CGO/Handle enumeration
	return -1
}