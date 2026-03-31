package vtui

import (
	"sync"
)

// EventType определяет категорию события.
type EventType string

const (
	EvCommand     EventType = "command"      // Команды типа cmQuit, cmSave
	EvFocus       EventType = "focus"        // Изменение фокуса
	EvWindowState EventType = "window_state" // Изменение состояния окна (resize, move, zoom)
	EvFileChanged EventType = "file_changed"
)

// Event представляет структуру сообщения в шине.
type Event struct {
	Type   EventType
	Sender any
	Data   any
}

// Listener — функция, обрабатывающая событие.
type Listener func(Event)

// eventBus реализует глобальную шину обмена сообщениями.
type eventBus struct {
	mu        sync.RWMutex
	listeners map[EventType][]Listener
}

// GlobalEvents — глобальный экземпляр шины событий.
var GlobalEvents = &eventBus{
	listeners: make(map[EventType][]Listener),
}

// Subscribe регистрирует функцию-обработчик для определенного типа событий.
func (eb *eventBus) Subscribe(et EventType, l Listener) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	eb.listeners[et] = append(eb.listeners[et], l)
}

// Publish отправляет событие всем подписанным слушателям.
// Доставка выполняется синхронно в текущем потоке для обеспечения порядка в UI.
func (eb *eventBus) Publish(e Event) {
	eb.mu.RLock()
	// Делаем копию списка слушателей, чтобы избежать deadlock если слушатель решит отписаться в процессе
	handlers := make([]Listener, len(eb.listeners[e.Type]))
	copy(handlers, eb.listeners[e.Type])
	eb.mu.RUnlock()

	for _, handler := range handlers {
		handler(e)
	}
}