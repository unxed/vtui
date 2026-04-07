//go:build !nocrashreport

package vtui

import (
	"fmt"
	"os"
	"path/filepath"
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
)

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

	timestamp := time.Now().Format("20060102_150405")
	filename := filepath.Join(crashDir, fmt.Sprintf("crash_%s.log", timestamp))

	f, err := os.Create(filename)
	if err != nil {
		return ""
	}
	defer f.Close()

	fmt.Fprintf(f, "=== F4 CRASH REPORT ===\n")
	fmt.Fprintf(f, "Time: %s\n", time.Now().Format(time.RFC3339))

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

	fmt.Fprintf(f, "\n=== LOG HISTORY BEFORE CRASH ===\n")
	for _, l := range getMemLogs() {
		fmt.Fprintln(f, l)
	}

	return filename
}