package vtui

import (
	"testing"
)

func TestComboBox_Selection(t *testing.T) {
	items := []string{"One", "Two", "Three"}
	cb := NewComboBox(0, 0, 20, items)

	// Изначально текст пустой
	if cb.Edit.GetText() != "" {
		t.Errorf("Expected empty text, got %q", cb.Edit.GetText())
	}

	// Имитируем выбор второго элемента ("Two") в меню
	cb.Menu.OnSelect(1)

	if cb.Edit.GetText() != "Two" {
		t.Errorf("Expected 'Two', got %q", cb.Edit.GetText())
	}
}