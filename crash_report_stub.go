//go:build nocrashreport

package vtui

func recordLogMemory(line string) {}
func RecordCrash(panicVal any, stack []byte) string { return "" }