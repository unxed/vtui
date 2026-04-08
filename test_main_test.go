package vtui

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	result := m.Run()
	if result != 0 {
		dumpLogsOnFailure()
	}
	os.Exit(result)
}

func dumpLogsOnFailure() {
	logs := GetCurrentLogs()
	if len(logs) == 0 {
		return
	}

	filename := "_failed_tests_vtui.log"
	f, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Failed to create failure log: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "=== TEST FAILURE LOG DUMP ===\n")
	for _, l := range logs {
		fmt.Fprintln(f, l)
	}
	fmt.Printf("\n[!] Tests failed. Log dump saved to: %s\n", filename)
}