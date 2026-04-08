package vtui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	logMu      sync.Mutex
	logRotated bool
)

func rotateLogs(basePath string) {
	ext := filepath.Ext(basePath)
	prefix := strings.TrimSuffix(basePath, ext)

	// debug.1.log -> debug.2.log
	oldest := prefix + ".2" + ext
	middle := prefix + ".1" + ext
	_ = os.Remove(oldest)
	_ = os.Rename(middle, oldest)

	// debug.log -> debug.1.log
	_ = os.Rename(basePath, middle)
}

var diskLoggingEnabled = true

// ConfigDiskLogging allows enabling or disabling writing to debug.log on disk.
// If disabled, logs are still kept in the in-memory ring buffer for crash reports.
func ConfigDiskLogging(enabled bool) {
	logMu.Lock()
	diskLoggingEnabled = enabled
	logMu.Unlock()
}
// DebugLog writes a timestamped message to debug.log file.
// If the file exists at the start of the session, it is rotated
// (up to 3 files: debug.log, debug.1.log, debug.2.log).
func DebugLog(format string, a ...any) {
	msg := fmt.Sprintf(format, a...)
	timestamp := time.Now().Format("15:04:05.000")
	fullMsg := fmt.Sprintf("[%s] %s", timestamp, msg)

	recordLogMemory(fullMsg)

	env := os.Getenv("VTUI_DEBUG")
	if env == "" {
		return
	}

	logMu.Lock()
	enabled := diskLoggingEnabled
	if !enabled {
		logMu.Unlock()
		return
	}

	filename := "debug.log"
	if env != "1" && env != "true" {
		filename = env
	}

	if !logRotated {
		if _, err := os.Stat(filename); err == nil {
			rotateLogs(filename)
		}
		logRotated = true
	}
	logMu.Unlock()

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	fmt.Fprintln(f, fullMsg)
	f.Sync()
}

func GetCurrentLogs() []string {
	return getMemLogs()
}