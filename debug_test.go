package vtui

import (
	"os"
	"strings"
	"path/filepath"
	"testing"
)

func TestDebugLog_CustomFile(t *testing.T) {
	customLog := "custom_test.log"
	os.Remove(customLog)
	defer os.Remove(customLog)

	os.Setenv("VTUI_DEBUG", customLog)
	defer os.Setenv("VTUI_DEBUG", "")

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
	defer os.Setenv("VTUI_DEBUG", "")

	// Helper to reset internal state for testing
	resetRotation := func() {
		logMu.Lock()
		logRotated = false
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
