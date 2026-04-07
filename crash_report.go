//go:build !nocrashreport

package vtui

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

var (
	crashMu     sync.Mutex
	logRing     []string
	logIdx      int
	logFull     bool
	maxLogLines = 10000

	eventRing     []string
	eventIdx      int
	maxEventLines = 20
)

func RecordEvent(ev string) {
	crashMu.Lock()
	defer crashMu.Unlock()
	if eventRing == nil {
		eventRing = make([]string, maxEventLines)
	}
	eventRing[eventIdx] = ev
	eventIdx = (eventIdx + 1) % maxEventLines
}

func getEventHistory() []string {
	crashMu.Lock()
	defer crashMu.Unlock()
	if eventRing == nil {
		return nil
	}
	var res []string
	// For events we always show them in chronological order
	for i := 0; i < maxEventLines; i++ {
		idx := (eventIdx + i) % maxEventLines
		if eventRing[idx] != "" {
			res = append(res, eventRing[idx])
		}
	}
	return res
}

func countOpenFDs() int {
	files, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(files)
}

var CrashDirBase string

func recordLogMemory(line string) {
	crashMu.Lock()
	defer crashMu.Unlock()

	// Re-initialize if size changed or not yet allocated
	if logRing == nil || len(logRing) != maxLogLines {
		logRing = make([]string, maxLogLines)
		logIdx = 0
		logFull = false
	}

	logRing[logIdx] = line
	logIdx = (logIdx + 1) % maxLogLines
	if logIdx == 0 {
		logFull = true
	}
}

func getMemLogs() []string {
	crashMu.Lock()
	defer crashMu.Unlock()
	if logRing == nil {
		return nil
	}

	//size := len(logRing)
	var res []string
	if logFull {
		res = append(res, logRing[logIdx:]...)
	}
	res = append(res, logRing[:logIdx]...)
	return res
}

func getCrashDir() string {
	if CrashDirBase != "" {
		return filepath.Join(CrashDirBase, "f4", "crashes")
	}
	cd, err := os.UserCacheDir()
	if err != nil {
		cd = os.TempDir()
	}
	return filepath.Join(cd, "f4", "crashes")
}

// GetVersionInfo returns a string containing Git revision and Go version.
func GetVersionInfo() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		var vcsRev, vcsDirty string
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				vcsRev = s.Value
				if len(vcsRev) > 8 {
					vcsRev = vcsRev[:8]
				}
			}
			if s.Key == "vcs.modified" {
				vcsDirty = s.Value
			}
		}
		if vcsRev != "" {
			return fmt.Sprintf("rev:%s(dirty:%s) go:%s", vcsRev, vcsDirty, info.GoVersion)
		}
		return "go:" + info.GoVersion
	}
	return "unknown version"
}
// RecordCrash writes the crash details and the in-memory log buffer to a file.
func RecordCrash(panicVal any, stack []byte) string {
	crashDir := getCrashDir()
	os.MkdirAll(crashDir, 0755)

	now := time.Now()
	timestamp := now.Format("20060102_150405")
	filename := filepath.Join(crashDir, fmt.Sprintf("crash_%s.log", timestamp))

	f, err := os.Create(filename)
	if err != nil {
		return ""
	}
	defer f.Close()

	fmt.Fprintf(f, "=== F4 CRASH REPORT ===\n")
	fmt.Fprintf(f, "Date, Time: %s\n", now.Format("2006-01-02 15:04:05"))

	if info, ok := debug.ReadBuildInfo(); ok {
		var vcsRev, vcsDirty string
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				vcsRev = s.Value
				if len(vcsRev) > 8 {
					vcsRev = vcsRev[:8]
				}
			}
			if s.Key == "vcs.modified" {
				vcsDirty = s.Value
			}
		}
		if vcsRev != "" {
			fmt.Fprintf(f, "Git Revision: %s (Dirty: %s)\n", vcsRev, vcsDirty)
		}
		fmt.Fprintf(f, "Go Version: %s\n", info.GoVersion)
	}

	fmt.Fprintf(f, "\n=== PANIC ===\n%v\n\n=== STACK TRACE ===\n%s\n", panicVal, stack)

	fmt.Fprintf(f, "\n=== RUNTIME INFO ===\n")
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(f, "Goroutines: %d\n", runtime.NumGoroutine())
	fmt.Fprintf(f, "Memory Alloc: %v MB (Total: %v MB, Sys: %v MB)\n", m.Alloc/1024/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024)
	fmt.Fprintf(f, "OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	if FrameManager != nil && FrameManager.scr != nil {
		fmt.Fprintf(f, "Terminal Size: %dx%d\n", FrameManager.scr.width, FrameManager.scr.height)
	}
	fmt.Fprintf(f, "Open FDs: %d\n", countOpenFDs())
	fmt.Fprintf(f, "Env TERM: %s\n", os.Getenv("TERM"))
	fmt.Fprintf(f, "Env LANG: %s\n", os.Getenv("LANG"))

	fmt.Fprintf(f, "\n=== RECENT INPUT EVENTS ===\n")
	for _, ev := range getEventHistory() {
		fmt.Fprintln(f, ev)
	}

	fmt.Fprintf(f, "\n=== UI FRAME STACK ===\n")
	if FrameManager != nil {
		for i, s := range FrameManager.Screens {
			activeMark := ""
			if i == FrameManager.ActiveIdx {
				activeMark = " (ACTIVE)"
			}
			fmt.Fprintf(f, "Screen %d%s: [%s]\n", i, activeMark, s.GetTitle())
			for j, frame := range s.Frames {
				fmt.Fprintf(f, "  [%d] Type:%d Title:%q\n", j, frame.GetType(), frame.GetTitle())
			}
		}
	}

	fmt.Fprintf(f, "\n=== LOG HISTORY BEFORE CRASH ===\n")
	for _, l := range getMemLogs() {
		fmt.Fprintln(f, l)
	}

	return filename
}