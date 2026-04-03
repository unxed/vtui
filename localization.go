package vtui

import (
	"fmt"
	"sync"
)

var (
	stringsMu  sync.RWMutex
	stringsMap = map[string]string{
	"vtui.Ok":      "&Ok",
	"vtui.Cancel":  "Cancel",
	"vtui.Save":    "&Save",
	"vtui.Delete":  "&Delete",
	"vtui.Path":    "Path:",
	"vtui.File":    "&File:",
	"vtui.History": "History",
})

// Msg retrieves a localized string by key.
// It looks into the global vtui strings map.
func Msg(key string) string {
	stringsMu.RLock()
	defer stringsMu.RUnlock()
	if val, ok := stringsMap[key]; ok {
		return val
	}
	return fmt.Sprintf("{%s}", key)
}

// AddStrings allows an application to add or override strings in the UI.
func AddStrings(m map[string]string) {
	stringsMu.Lock()
	defer stringsMu.Unlock()
	for k, v := range m {
		stringsMap[k] = v
	}
}
