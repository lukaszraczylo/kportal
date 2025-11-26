package events

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBus_Subscribe(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	var received bool
	bus.Subscribe(EventForwardStarting, func(e Event) {
		received = true
	})

	bus.Publish(Event{Type: EventForwardStarting})
	assert.True(t, received)
}

func TestBus_SubscribeMultipleHandlers(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	var count int32
	handler := func(e Event) {
		atomic.AddInt32(&count, 1)
	}

	bus.Subscribe(EventForwardStarting, handler)
	bus.Subscribe(EventForwardStarting, handler)
	bus.Subscribe(EventForwardStarting, handler)

	bus.Publish(Event{Type: EventForwardStarting})
	assert.Equal(t, int32(3), atomic.LoadInt32(&count))
}

func TestBus_SubscribeAll(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	var count int32
	bus.SubscribeAll(func(e Event) {
		atomic.AddInt32(&count, 1)
	})

	bus.Publish(Event{Type: EventForwardStarting})
	bus.Publish(Event{Type: EventForwardConnected})
	bus.Publish(Event{Type: EventHealthStatusChanged})

	assert.Equal(t, int32(3), atomic.LoadInt32(&count))
}

func TestBus_PublishWithData(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	var receivedEvent Event
	bus.Subscribe(EventHealthStatusChanged, func(e Event) {
		receivedEvent = e
	})

	bus.Publish(Event{
		Type:      EventHealthStatusChanged,
		ForwardID: "test-forward",
		Data: map[string]interface{}{
			"status": "Active",
		},
	})

	assert.Equal(t, EventHealthStatusChanged, receivedEvent.Type)
	assert.Equal(t, "test-forward", receivedEvent.ForwardID)
	assert.Equal(t, "Active", receivedEvent.Data["status"])
}

func TestBus_PublishAsync(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	var wg sync.WaitGroup
	wg.Add(1)

	bus.Subscribe(EventForwardStarting, func(e Event) {
		wg.Done()
	})

	bus.PublishAsync(Event{Type: EventForwardStarting})

	// Wait for async handler with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(time.Second):
		t.Fatal("Async handler not called within timeout")
	}
}

func TestBus_Close(t *testing.T) {
	bus := NewBus()

	var received bool
	bus.Subscribe(EventForwardStarting, func(e Event) {
		received = true
	})

	bus.Close()

	// After close, publish should not call handlers
	bus.Publish(Event{Type: EventForwardStarting})
	assert.False(t, received)

	// Subscribe after close should be a no-op
	bus.Subscribe(EventForwardConnected, func(e Event) {
		received = true
	})
	bus.Publish(Event{Type: EventForwardConnected})
	assert.False(t, received)
}

func TestBus_ConcurrentAccess(t *testing.T) {
	bus := NewBus()
	defer bus.Close()

	var count int64
	bus.Subscribe(EventForwardStarting, func(e Event) {
		atomic.AddInt64(&count, 1)
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(Event{Type: EventForwardStarting})
		}()
	}

	wg.Wait()
	assert.Equal(t, int64(100), atomic.LoadInt64(&count))
}

func TestNewForwardEvent(t *testing.T) {
	event := NewForwardEvent(EventForwardStarting, "test-id", map[string]interface{}{
		"pod": "my-pod",
	})

	assert.Equal(t, EventForwardStarting, event.Type)
	assert.Equal(t, "test-id", event.ForwardID)
	assert.Equal(t, "my-pod", event.Data["pod"])
}

func TestNewHealthEvent(t *testing.T) {
	event := NewHealthEvent("test-id", "Active", "")

	assert.Equal(t, EventHealthStatusChanged, event.Type)
	assert.Equal(t, "test-id", event.ForwardID)
	assert.Equal(t, "Active", event.Data["status"])
	assert.Equal(t, "", event.Data["error_msg"])
}

func TestNewStaleEvent(t *testing.T) {
	event := NewStaleEvent("test-id", "connection too old")

	assert.Equal(t, EventHealthStale, event.Type)
	assert.Equal(t, "test-id", event.ForwardID)
	assert.Equal(t, "connection too old", event.Data["reason"])
}

func TestNewWorkerHungEvent(t *testing.T) {
	event := NewWorkerHungEvent("test-id", "60s")

	assert.Equal(t, EventWorkerHung, event.Type)
	assert.Equal(t, "test-id", event.ForwardID)
	assert.Equal(t, "60s", event.Data["time_since_heartbeat"])
}
