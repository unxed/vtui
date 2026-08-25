package vreactive

import (
	"testing"
)

func TestProperty_CycleDetection(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for cycle detection, got none")
		}
	}()

	p1 := NewProperty(0)
	p2 := NewProperty(0)

	// Create an expanding cycle (p1 -> p2+1 -> p1+2...) to test depth limit panic
	p1.OnChange(func(v int) { p2.Set(v + 1) })
	p2.OnChange(func(v int) { p1.Set(v + 1) })

	p1.Set(1)
}

func TestProperty_SafeSet(t *testing.T) {
	called := false

	GlobalUpdateQueue = &mockUpdateQueue{
		postTask: func(task func()) {
			called = true
			task()
		},
	}
	defer func() { GlobalUpdateQueue = nil }()
	p := NewProperty(0)

	SafeSet(p, 42)
	if !called {
		t.Error("expected SafeSet to use GlobalUpdateQueue")
	}
	if p.Get() != 42 {
		t.Errorf("expected property to be 42, got %d", p.Get())
	}
}

type mockUpdateQueue struct {
	postTask func(func())
}

func (m *mockUpdateQueue) PostTask(task func()) {
	m.postTask(task)
}

func TestComputed(t *testing.T) {
	p := NewProperty(2)
	c := Computed(p, func(val int) int { return val * 2 })
	if c.Get() != 4 {
		t.Errorf("expected 4, got %d", c.Get())
	}
	p.Set(3)
	if c.Get() != 6 {
		t.Errorf("expected 6, got %d", c.Get())
	}
}

func TestStateMachine(t *testing.T) {
	val := NewProperty(0)
	sm := NewStateMachine("idle")
	sm.AddState("active", SetProp(val, 10))
	sm.AddState("idle", SetProp(val, 0))

	sm.State.Set("active")
	if val.Get() != 10 {
		t.Errorf("expected 10, got %d", val.Get())
	}

	sm.State.Set("idle")
	if val.Get() != 0 {
		t.Errorf("expected 0, got %d", val.Get())
	}
}
