package vtui

import (
	"fmt"
	"os"
	"time"
)

// DebugLog writes a timestamped message to debug.log file.
func DebugLog(format string, a ...any) {
	f, err := os.OpenFile("debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	msg := fmt.Sprintf(format, a...)
	timestamp := time.Now().Format("15:04:05.000")
	fmt.Fprintf(f, "[%s] %s\n", timestamp, msg)
}