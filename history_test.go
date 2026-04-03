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