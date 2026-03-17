package vtui

import (
	"testing"
)

func TestCheckbox_Toggle(t *testing.T) {
	// 1. Тест 2-х состояний
	cb2 := NewCheckbox(0, 0, "2-state", false)
	if cb2.State != 0 { t.Error("Should start unchecked") }
	cb2.Toggle()
	if cb2.State != 1 { t.Error("Should be checked (1)") }
	cb2.Toggle()
	if cb2.State != 0 { t.Error("Should be unchecked again (0)") }

	// 2. Тест 3-х состояний
	cb3 := NewCheckbox(0, 0, "3-state", true)
	cb3.Toggle() // 0 -> 1
	if cb3.State != 1 { t.Error("3-state: expected 1") }
	cb3.Toggle() // 1 -> 2
	if cb3.State != 2 { t.Error("3-state: expected 2") }
	cb3.Toggle() // 2 -> 0
	if cb3.State != 0 { t.Error("3-state: expected 0") }
}