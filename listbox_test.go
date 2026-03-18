package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestListBox_Scrolling(t *testing.T) {
	items := []string{"1", "2", "3", "4", "5"}
	// ListBox высотой 2 строки
	lb := NewListBox(0, 0, 10, 2, items)

	// 1. Изначально SelectPos 0, TopPos 0
	if lb.SelectPos != 0 || lb.TopPos != 0 {
		t.Errorf("Initial state error: SelectPos %d, TopPos %d", lb.SelectPos, lb.TopPos)
	}

	// 2. Листаем вниз 2 раза
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	// SelectPos должен быть 2, TopPos должен стать 1 (чтобы видеть индекс 2 в окне из 2 строк)
	if lb.SelectPos != 2 {
		t.Errorf("SelectPos error after Down: %d", lb.SelectPos)
	}
	if lb.TopPos != 1 {
		t.Errorf("TopPos error after Down (scrolling): %d", lb.TopPos)
	}

	// 3. Тест Home
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	if lb.SelectPos != 0 || lb.TopPos != 0 {
		t.Errorf("Home error: SelectPos %d, TopPos %d", lb.SelectPos, lb.TopPos)
	}
}

func TestListBox_MouseWheel(t *testing.T) {
	items := make([]string, 10)
	lb := NewListBox(0, 0, 10, 3, items)
	lb.TopPos = 2

	// Scroll Down (WheelDirection < 0)
	lb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	if lb.TopPos != 3 {
		t.Errorf("Mouse wheel down failed: TopPos %d", lb.TopPos)
	}

	// Scroll Up (WheelDirection > 0)
	lb.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1})
	if lb.TopPos != 2 {
		t.Errorf("Mouse wheel up failed: TopPos %d", lb.TopPos)
	}
}

func TestListBox_OnChange(t *testing.T) {
	called := false
	newIdx := -1
	lb := NewListBox(0, 0, 10, 5, []string{"A", "B", "C"})
	lb.OnChange = func(idx int) {
		called = true
		newIdx = idx
	}

	// Down to index 1
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	if !called {
		t.Error("OnChange callback was not called")
	}
	if newIdx != 1 {
		t.Errorf("Expected index 1 in callback, got %d", newIdx)
	}
}

func TestListBox_PageNavigation(t *testing.T) {
	items := make([]string, 20) // 20 элементов
	lb := NewListBox(0, 0, 10, 5, items) // Высота 5

	// 1. End
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END})
	if lb.SelectPos != 19 {
		t.Errorf("End failed: expected 19, got %d", lb.SelectPos)
	}

	// 2. Page Up (19 -> 14)
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR})
	if lb.SelectPos != 14 {
		t.Errorf("PageUp failed: expected 14, got %d", lb.SelectPos)
	}

	// 3. Page Down (14 -> 19)
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
	if lb.SelectPos != 19 {
		t.Errorf("PageDown failed: expected 19, got %d", lb.SelectPos)
	}
}

func TestListBox_MouseClickItem(t *testing.T) {
	lb := NewListBox(0, 0, 10, 5, []string{"0", "1", "2", "3", "4"})
	lb.TopPos = 1 // Смещено на 1 (видим 1, 2, 3, 4, 5)

	// Кликаем по Y=2 (в локальных координатах это вторая видимая строка)
	// Должен выбраться индекс TopPos (1) + (clickY (2) - lbY (0)) = 3.
	// Стоп. Формула: lb.TopPos + (my - lb.Y1). 1 + (2 - 0) = 3.
	lb.ProcessMouse(&vtinput.InputEvent{
		Type: vtinput.MouseEventType,
		KeyDown: true,
		MouseX: 2, MouseY: 2,
		ButtonState: vtinput.FromLeft1stButtonPressed,
	})

	if lb.SelectPos != 3 {
		t.Errorf("Mouse click selection failed: expected 3, got %d", lb.SelectPos)
	}
}

func TestListBox_EmptyList(t *testing.T) {
	lb := NewListBox(0, 0, 10, 5, []string{})

	// Попытка навигации не должна вызывать панику
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	if lb.SelectPos != 0 {
		t.Error("SelectPos should be 0 for empty list")
	}
}
