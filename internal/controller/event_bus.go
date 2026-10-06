package controller

import (
	"sync"
	"time"
)

// BridgeEvent encapsulates a typed event within the bridge runtime.
type BridgeEvent struct {
	Type      string    `json:"type"` // "device_state", "device_join", "binding_change", "permit_join", "ai_recommendation", "frame"
	Payload   any       `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

// EventBus provides high-performance, non-blocking fan-out event routing.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[chan BridgeEvent]struct{}
	closed      bool
}

// NewEventBus creates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[chan BridgeEvent]struct{}),
	}
}

// Subscribe registers a new subscriber channel.
func (b *EventBus) Subscribe(bufferSize int) chan BridgeEvent {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan BridgeEvent, bufferSize)
	b.subscribers[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a channel and closes it.
func (b *EventBus) Unsubscribe(ch chan BridgeEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.subscribers[ch]; ok {
		delete(b.subscribers, ch)
		close(ch)
	}
}

// Publish dispatches an event to all subscribers without blocking the publisher.
func (b *EventBus) Publish(evtType string, payload any) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	event := BridgeEvent{
		Type:      evtType,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	}

	for ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			// Non-blocking drop if consumer buffer is full to prevent pipeline stalling
		}
	}
}

// Close unsubscribes all listeners and terminates the bus.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	for ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = make(map[chan BridgeEvent]struct{})
}
