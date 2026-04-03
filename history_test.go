package vtui

import (
	"testing"
)

type mockHistoryProvider struct {
	storage map[string][]string
}

func (m *mockHistoryProvider) LoadHistory(id string) []string {
	return m.storage[id]
}

func (m *mockHistoryProvider) SaveHistory(id string, history []string) {
	m.storage[id] = history
}

func TestEdit_GlobalHistoryBinding(t *testing.T) {
	mock := &mockHistoryProvider{
		storage: map[string][]string{
			"search": {"old_query", "prev_query"},
		},
	}
	GlobalHistoryProvider = mock
	defer func() { GlobalHistoryProvider = nil }()

	edit := NewEdit(0, 0, 20, "")
	edit.HistoryID = "search"

	// 1. Trigger history loading
	edit.OpenHistory()

	if len(edit.History) != 2 || edit.History[0] != "old_query" {
		t.Errorf("History not loaded from provider. Got: %v", edit.History)
	}

	// 2. Add new item and verify it's saved back to provider
	edit.AddHistory("new_query")

	saved := mock.storage["search"]
	if len(saved) != 3 || saved[0] != "new_query" {
		t.Errorf("History not saved to provider. Current storage: %v", saved)
	}
}

func TestEdit_HistorySyncBetweenControls(t *testing.T) {
	mock := &mockHistoryProvider{
		storage: map[string][]string{
			"shared": {"cmd1"},
		},
	}
	GlobalHistoryProvider = mock
	defer func() { GlobalHistoryProvider = nil }()

	e1 := NewEdit(0, 0, 10, "")
	e1.HistoryID = "shared"

	e2 := NewEdit(0, 1, 10, "")
	e2.HistoryID = "shared"

	// Load in e1
	e1.OpenHistory()

	// Add in e1
	e1.AddHistory("cmd2")

	// Open in e2 - should see the update from e1
	e2.OpenHistory()
	if len(e2.History) < 2 || e2.History[0] != "cmd2" {
		t.Errorf("Sync failed: e2 did not see history added by e1. History: %v", e2.History)
	}
}
