package vreactive

import (
	"sync"
)

var evalDepth int
var evalMu sync.Mutex

// UpdateQueue interface allows posting tasks to the UI thread.
type UpdateQueue interface {
	PostTask(task func())
}

// GlobalUpdateQueue should be set to the UI framework's task queue (e.g. vtui.FrameManager).
var GlobalUpdateQueue UpdateQueue

// AnimationManager handles the ticking of animators.
type AnimationManager interface {
	AddAnimation(anim func(dt float64) bool)
}

// GlobalAnimationManager should be set by the UI framework.
var GlobalAnimationManager AnimationManager

type BehaviorDef[T any] interface {
	CreateAnimator(start, end T) Animator[T]
}

type Animator[T any] interface {
	Tick(dt float64) (val T, done bool)
}

// Watcher provides a value-agnostic notification subscription.
type Watcher interface {
	Watch(handler func()) func()
}

type Property[T any] interface {
	Get() T
	Set(val T)
	OnChange(handler func(newVal T)) func()
	SetBehavior(b BehaviorDef[T])
	Watch(handler func()) func()
}

type property[T any] struct {
	mu               sync.RWMutex
	val              T
	handlers         map[int]func(T)
	nextID           int
	behavior         BehaviorDef[T]
	updateQueue      UpdateQueue
	animationManager AnimationManager
}

func NewProperty[T any](initial T) Property[T] {
	return &property[T]{
		val:              initial,
		handlers:         make(map[int]func(T)),
		updateQueue:      GlobalUpdateQueue,
		animationManager: GlobalAnimationManager,
	}
}

func (p *property[T]) Get() T {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.val
}

func (p *property[T]) SetBehavior(b BehaviorDef[T]) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.behavior = b
}

func (p *property[T]) Set(val T) {
	p.mu.RLock()
	b := p.behavior
	p.mu.RUnlock()

	if b != nil && p.animationManager != nil {
		start := p.Get()
		anim := b.CreateAnimator(start, val)
		p.animationManager.AddAnimation(func(dt float64) bool {
			newVal, done := anim.Tick(dt)
			p.setInternal(newVal)
			return done
		})
	} else {
		p.setInternal(val)
	}
}

func (p *property[T]) setInternal(val T) {
	evalMu.Lock()
	if evalDepth > 100 {
		evalMu.Unlock()
		panic("vreactive: cycle detected in property notification chain")
	}
	evalDepth++
	evalMu.Unlock()

	defer func() {
		evalMu.Lock()
		evalDepth--
		evalMu.Unlock()
	}()

	p.mu.Lock()
	p.val = val
	handlers := make([]func(T), 0, len(p.handlers))
	for _, h := range p.handlers {
		handlers = append(handlers, h)
	}
	p.mu.Unlock()

	for _, h := range handlers {
		h(val)
	}
}

func (p *property[T]) OnChange(handler func(T)) func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	id := p.nextID
	p.nextID++
	p.handlers[id] = handler
	return func() {
		p.mu.Lock()
		defer p.mu.Unlock()
		delete(p.handlers, id)
	}
}

func (p *property[T]) Watch(handler func()) func() {
	return p.OnChange(func(T) {
		handler()
	})
}

// SafeSet updates the property value on the global update queue if configured.
// Extremely useful for thread-safe mutation from background goroutines.
func SafeSet[T any](p Property[T], val T) {
	if owned, ok := p.(interface{ getUpdateQueue() UpdateQueue }); ok && owned.getUpdateQueue() != nil {
		owned.getUpdateQueue().PostTask(func() {
			p.Set(val)
		})
	} else {
		p.Set(val)
	}
}

func (p *property[T]) getUpdateQueue() UpdateQueue {
	return p.updateQueue
}
