package vtui

import (
	"os"
	"strings"
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