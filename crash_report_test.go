package vtui

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestCrashReporter_MemoryLog(t *testing.T) {
	crashMu.Lock()
	// Save original and restore later
	origMax := maxLogLines
	origRing := logRing
	origIdx := logIdx
	origFull := logFull

	maxLogLines = 5
	logRing = nil // Force re-init in recordLogMemory
	logIdx = 0
	logFull = false
	crashMu.Unlock()

	defer func() {
		crashMu.Lock()
		maxLogLines = origMax
		logRing = origRing
		logIdx = origIdx
		logFull = origFull
		crashMu.Unlock()
	}()

	for i := 0; i < 7; i++ {
		recordLogMemory(fmt.Sprintf("L%d", i))
	}

	logs := getMemLogs()
	if len(logs) != 5 {
		t.Fatalf("Expected exactly 5 lines, got %d", len(logs))
	}
	if logs[0] != "L2" || logs[4] != "L6" {
		t.Errorf("Ring buffer wrapping failed: %v", logs)
	}
}

func TestRecordCrash(t *testing.T) {
	tmpDir := t.TempDir()
	CrashDirBase = tmpDir
	defer func() { CrashDirBase = "" }()

	recordLogMemory("Test line before crash")

	path := RecordCrash("Test Panic", []byte("goroutine 1 [running]:\nstack info"))
	if path == "" {
		t.Fatal("RecordCrash returned empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read crash file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "=== F4 CRASH REPORT ===") {
		t.Error("Missing crash report header")
	}
	if !strings.Contains(content, "Test Panic") {
		t.Error("Missing panic value")
	}
	if !strings.Contains(content, "[running]") {
		t.Error("Missing stack trace")
	}
	if !strings.Contains(content, "Test line before crash") {
		t.Error("Missing log history")
	}
	if !strings.Contains(content, "Go Version:") {
		t.Error("Missing Go version info")
	}
}