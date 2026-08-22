package vtui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDebugLog_CustomFile(t *testing.T) {
	customLog := "custom_test.log"
	os.Remove(customLog)
	defer os.Remove(customLog)

	os.Setenv("VTUI_DEBUG", customLog)
	defer func() {
		os.Setenv("VTUI_DEBUG", "")
		logMu.Lock()
		if logFile != nil {
			logFile.Close()
			logFile = nil
		}
		logMu.Unlock()
	}()

	DebugLog("Test message %d", 123)

	data, err := os.ReadFile(customLog)
	if err != nil {
		t.Fatalf("Failed to read custom log file: %v", err)
	}

	if !strings.Contains(string(data), "Test message 123") {
		t.Errorf("Log content mismatch. Got: %s", string(data))
	}
}

func TestDebugLog_Rotation(t *testing.T) {
	baseLog := filepath.Join(t.TempDir(), "rotation.log")
	log1 := strings.TrimSuffix(baseLog, ".log") + ".1.log"
	log2 := strings.TrimSuffix(baseLog, ".log") + ".2.log"

	os.Setenv("VTUI_DEBUG", baseLog)
	defer func() {
		os.Setenv("VTUI_DEBUG", "")
		// Close the handle opened by the last DebugLog before t.TempDir
		// cleanup unlinks the log: Windows cannot remove an open file.
		logMu.Lock()
		if logFile != nil {
			logFile.Close()
			logFile = nil
		}
		logMu.Unlock()
	}()

	// Helper to reset internal state for testing
	resetRotation := func() {
		logMu.Lock()
		logRotated = false
		if logFile != nil {
			logFile.Close()
			logFile = nil
		}
		logMu.Unlock()
	}

	// 1. Create initial log
	resetRotation()
	DebugLog("Session 1")
	if _, err := os.Stat(baseLog); err != nil {
		t.Fatal("Base log not created")
	}

	// 2. Second session - should rotate base -> log.1
	resetRotation()
	DebugLog("Session 2")
	if _, err := os.Stat(log1); err != nil {
		t.Error("Rotation 1 failed: log.1.log not found")
	}

	// 3. Third session - should rotate log.1 -> log.2 and base -> log.1
	resetRotation()
	DebugLog("Session 3")
	if _, err := os.Stat(log2); err != nil {
		t.Error("Rotation 2 failed: log.2.log not found")
	}

	// 4. Verify contents
	cBase, _ := os.ReadFile(baseLog)
	if !strings.Contains(string(cBase), "Session 3") {
		t.Error("Current log has wrong content")
	}
	c1, _ := os.ReadFile(log1)
	if !strings.Contains(string(c1), "Session 2") {
		t.Error("Rotated log.1 has wrong content")
	}
	c2, _ := os.ReadFile(log2)
	if !strings.Contains(string(c2), "Session 1") {
		t.Error("Rotated log.2 has wrong content")
	}
}

func TestDebugLog_StderrMode(t *testing.T) {
	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stderr pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
		_ = r.Close()
		_ = w.Close()
	})
	t.Setenv("VTUI_DEBUG", "stderr")

	DebugLog("stderr message %d", 42)
	_ = w.Close()
	os.Stderr = oldStderr

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	if !strings.Contains(string(data), "stderr message 42") {
		t.Fatalf("stderr output = %q, want debug message", data)
	}
}

func TestDebugLog_TestLogger(t *testing.T) {
	var messages []string
	restore := SetTestLogger(func(format string, args ...any) {
		messages = append(messages, fmt.Sprintf(format, args...))
	})
	defer restore()
	t.Setenv("VTUI_DEBUG", "test")

	DebugLog("test message %d", 7)

	if len(messages) != 1 || !strings.Contains(messages[0], "test message 7") {
		t.Fatalf("test logger messages = %#v, want one formatted message", messages)
	}
}

func TestDebugLog_SyncIntervalAndClose(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "sync.log")
	t.Setenv("VTUI_DEBUG", logPath)

	logMu.Lock()
	if logFile != nil {
		_ = logFile.Close()
		logFile = nil
	}
	logRotated = false
	lastLogSync = time.Time{}
	logMu.Unlock()

	oldSyncLogFile := syncLogFile
	syncCalls := 0
	syncLogFile = func(*os.File) error {
		syncCalls++
		return nil
	}
	t.Cleanup(func() {
		syncLogFile = oldSyncLogFile
		logMu.Lock()
		closeLogFileLocked()
		logMu.Unlock()
	})

	DebugLog("first")
	DebugLog("second")
	if syncCalls != 0 {
		t.Fatalf("sync calls before interval = %d, want 0", syncCalls)
	}

	logMu.Lock()
	lastLogSync = time.Now().Add(-debugLogSyncInterval)
	logMu.Unlock()
	DebugLog("third")
	DebugLog("fourth")
	if syncCalls != 1 {
		t.Fatalf("sync calls during interval = %d, want 1", syncCalls)
	}

	logMu.Lock()
	closeLogFileLocked()
	logMu.Unlock()
	if syncCalls != 2 {
		t.Fatalf("sync calls after close = %d, want 2", syncCalls)
	}
}
