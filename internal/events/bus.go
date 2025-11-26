package events

import (
	"sync"
)

// EventType represents the type of event
type EventType string

const (
	// Forward lifecycle events
	EventForwardStarting     EventType = "forward.starting"
	EventForwardConnected    EventType = "forward.connected"
	EventForwardDisconnected EventType = "forward.disconnected"
	EventForwardReconnecting EventType = "forward.reconnecting"
	EventForwardStopped      EventType = "forward.stopped"
	EventForwardError        EventType = "forward.error"

	// Health events
	EventHealthStatusChanged EventType = "health.status_changed"
	EventHealthStale         EventType = "health.stale"

	// Watchdog events
	EventWorkerHung EventType = "watchdog.worker_hung"

	// Config events
	EventConfigReloaded EventType = "config.reloaded"
)

// Event represents a system event
type Event struct {
	Type      EventType
	ForwardID string
	Data      map[string]interface{}
}

// Handler is a function that handles events
type Handler func(event Event)

// Bus is a simple event bus for decoupled communication between components
type Bus struct {
	mu       sync.RWMutex
	handlers map[EventType][]Handler
	closed   bool
}

// NewBus creates a new event bus
func NewBus() *Bus {
	return &Bus{
		handlers: make(map[EventType][]Handler),
	}
}

// Subscribe registers a handler for a specific event type
func (b *Bus) Subscribe(eventType EventType, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// SubscribeAll registers a handler for all events
func (b *Bus) SubscribeAll(handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	// Subscribe to all known event types
	eventTypes := []EventType{
		EventForwardStarting,
		EventForwardConnected,
		EventForwardDisconnected,
		EventForwardReconnecting,
		EventForwardStopped,
		EventForwardError,
		EventHealthStatusChanged,
		EventHealthStale,
		EventWorkerHung,
		EventConfigReloaded,
	}

	for _, et := range eventTypes {
		b.handlers[et] = append(b.handlers[et], handler)
	}
}

// Publish sends an event to all registered handlers
// Handlers are called synchronously in the order they were registered
func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	handlers := make([]Handler, len(b.handlers[event.Type]))
	copy(handlers, b.handlers[event.Type])
	b.mu.RUnlock()

	for _, handler := range handlers {
		handler(event)
	}
}

// PublishAsync sends an event to all registered handlers asynchronously
func (b *Bus) PublishAsync(event Event) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	handlers := make([]Handler, len(b.handlers[event.Type]))
	copy(handlers, b.handlers[event.Type])
	b.mu.RUnlock()

	for _, handler := range handlers {
		go handler(event)
	}
}

// Close stops the event bus and prevents new subscriptions/publications
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	b.handlers = make(map[EventType][]Handler)
}

// Helper functions for creating common events

// NewForwardEvent creates a forward-related event
func NewForwardEvent(eventType EventType, forwardID string, data map[string]interface{}) Event {
	return Event{
		Type:      eventType,
		ForwardID: forwardID,
		Data:      data,
	}
}

// NewHealthEvent creates a health status change event
func NewHealthEvent(forwardID string, status string, errorMsg string) Event {
	return Event{
		Type:      EventHealthStatusChanged,
		ForwardID: forwardID,
		Data: map[string]interface{}{
			"status":    status,
			"error_msg": errorMsg,
		},
	}
}

// NewStaleEvent creates a stale connection event
func NewStaleEvent(forwardID string, reason string) Event {
	return Event{
		Type:      EventHealthStale,
		ForwardID: forwardID,
		Data: map[string]interface{}{
			"reason": reason,
		},
	}
}

// NewWorkerHungEvent creates a hung worker event
func NewWorkerHungEvent(forwardID string, timeSinceHeartbeat string) Event {
	return Event{
		Type:      EventWorkerHung,
		ForwardID: forwardID,
		Data: map[string]interface{}{
			"time_since_heartbeat": timeSinceHeartbeat,
		},
	}
}
