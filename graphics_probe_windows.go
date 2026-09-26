//go:build windows

package vtui

import (
	"os"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// da1Sixel sends a primary device attributes query and reads the answer,
// reporting whether the terminal declares sixel support (parameter 4).
func da1Sixel() bool {
	if useWinescapeProbe() {
		return winescapeDA1Sixel()
	}
	hIn := windows.Handle(os.Stdin.Fd())
	var oldIn uint32
	if err := windows.GetConsoleMode(hIn, &oldIn); err != nil {
		return false
	}
	// Terminal answers reach ReadFile only while VT input is enabled, and the
	// app's input reader deliberately clears it, so enable it for the query
	// and restore the original mode afterwards.
	if err := windows.SetConsoleMode(hIn, oldIn|windows.ENABLE_VIRTUAL_TERMINAL_INPUT); err != nil {
		return false
	}
	defer windows.SetConsoleMode(hIn, oldIn)

	if _, err := os.Stdout.WriteString("\x1b[c"); err != nil {
		return false
	}
	_ = os.Stdout.Sync()

	var sb strings.Builder
	buf := make([]byte, 128)
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		// The console input handle signals when bytes are waiting, so a
		// silent terminal costs the full budget instead of blocking forever.
		res, _ := windows.WaitForSingleObject(hIn, 60)
		if res != windows.WAIT_OBJECT_0 {
			continue
		}
		var done uint32
		err := windows.ReadFile(hIn, buf, &done, nil)
		if done > 0 {
			sb.Write(buf[:done])
			if da1ResponseComplete(sb.String()) {
				break
			}
		}
		if err != nil {
			break
		}
	}
	return parseDA1Sixel(sb.String())
}

// QueryCellSize asks the terminal (CSI 16 t) for the pixel size of one cell.
func QueryCellSize() (cw, ch int, ok bool) {
	if useWinescapeProbe() {
		return winescapeQueryCellSize()
	}
	hIn := windows.Handle(os.Stdin.Fd())
	var oldIn uint32
	if err := windows.GetConsoleMode(hIn, &oldIn); err != nil {
		return 0, 0, false
	}
	if err := windows.SetConsoleMode(hIn, oldIn|windows.ENABLE_VIRTUAL_TERMINAL_INPUT); err != nil {
		return 0, 0, false
	}
	defer windows.SetConsoleMode(hIn, oldIn)

	if _, err := os.Stdout.WriteString("\x1b[16t"); err != nil {
		return 0, 0, false
	}
	_ = os.Stdout.Sync()

	var sb strings.Builder
	buf := make([]byte, 128)
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		res, _ := windows.WaitForSingleObject(hIn, 60)
		if res != windows.WAIT_OBJECT_0 {
			continue
		}
		var done uint32
		err := windows.ReadFile(hIn, buf, &done, nil)
		if done > 0 {
			sb.Write(buf[:done])
			if cellSizeResponseComplete(sb.String()) {
				break
			}
		}
		if err != nil {
			break
		}
	}
	return parseCellSizeResponse(sb.String())
}
