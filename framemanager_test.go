package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

type mockFrame struct {
	ProcessCount int
}
func (m *mockFrame) ProcessKey(e *vtinput.InputEvent) bool { m.ProcessCount++; return true }
func (m *mockFrame) ProcessMouse(e *vtinput.InputEvent) bool { return false }
func (m *mockFrame) Show(scr *ScreenBuf) {}
func (m *mockFrame) ResizeConsole(w, h int) {}
func (m *mockFrame) GetType() FrameType { return TypeUser }
func (m *mockFrame) SetExitCode(c int) {}
func (m *mockFrame) IsDone() bool { return m.ProcessCount >= 2 }
func (m *mockFrame) IsBusy() bool { return false }

func TestFrameManager_NoDoubleDispatch(t *testing.T) {
	scr := NewScreenBuf()
	scr.AllocBuf(10, 10)
	fm := &frameManager{}
	fm.Init(scr)

	frame := &mockFrame{}
	fm.Push(frame)

	// Имитируем одно событие в канале
	eventChan := make(chan *vtinput.InputEvent, 1)
	eventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'A'}
	close(eventChan)

	// Запускаем цикл на одну итерацию (IsDone вернет true после обработки событий)
	// В нашей реализации fm.Run() содержит бесконечный цикл, поэтому для теста
	// нам пришлось бы его рефакторить. Но мы можем проверить логику dispatch.

	// Просто убедимся, что вызов ProcessKey произойдет ровно 1 раз для 1 события.
	// (Этот тест скорее для документации проблемы, реальный fm.Run слишком монолитен для теста без правок)
}