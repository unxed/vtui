//go:build !windows

package vtui

import (
	"os"
	"syscall"
)

func RedirectStderr(f *os.File) error {
	return syscall.Dup2(int(f.Fd()), 2)
}

func countOpenFDs() int {
	files, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(files)
}