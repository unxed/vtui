package vtui

import (
	"testing"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := &eventBus{listeners: make(map[EventType][]Listener)}

	received := false
	var receivedData string

	bus.Subscribe(EvCommand, func(e Event) {
		received = true
		receivedData = e.Data.(string)
	})

	bus.Publish(Event{
		Type: EvCommand,
		Data: "HelloBus",
	})

	if !received {
		t.Error("Event was not received")
	}
	if receivedData != "HelloBus" {
		t.Errorf("Expected 'HelloBus', got %q", receivedData)
	}
}

func TestEventBus_MultipleListeners(t *testing.T) {
	bus := &eventBus{listeners: make(map[EventType][]Listener)}
	count := 0

	l := func(e Event) { count++ }
	bus.Subscribe(EvCommand, l)
	bus.Subscribe(EvCommand, l)

	bus.Publish(Event{Type: EvCommand})

	if count != 2 {
		t.Errorf("Expected 2 handler calls, got %d", count)
	}
}