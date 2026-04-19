//go:build !windows && !darwin

package vtui

import (
	"os"
	"golang.org/x/sys/unix"
)

func RedirectStderr(f *os.File) error {
	return unix.Dup2(int(f.Fd()), 2)
}

func countOpenFDs() int {
	files, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(files)
}