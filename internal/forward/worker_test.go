package forward

import (
	"context"
	"testing"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestNewForwardWorker tests worker creation
func TestNewForwardWorker(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	assert.NotNil(t, worker)
	assert.Equal(t, fwd, worker.forward)
	assert.False(t, worker.verbose)
	assert.NotNil(t, worker.ctx)
	assert.NotNil(t, worker.stopChan)
	assert.NotNil(t, worker.doneChan)
	assert.NotNil(t, worker.reconnectChan)
	assert.NotNil(t, worker.successChan)
}

// TestNewForwardWorker_Verbose tests verbose mode worker creation
func TestNewForwardWorker_Verbose(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, true, nil, nil, nil)

	assert.True(t, worker.verbose)
}

// TestWorker_GetForwardConfig tests getting forward config
func TestWorker_GetForwardConfig(t *testing.T) {
	fwd := config.Forward{
		Resource:  "service/postgres",
		LocalPort: 5432,
		Port:      5432,
		Alias:     "db",
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)
	result := worker.GetForward()

	assert.Equal(t, fwd, result)
	assert.Equal(t, "service/postgres", result.Resource)
	assert.Equal(t, 5432, result.LocalPort)
	assert.Equal(t, "db", result.Alias)
}

// TestForwardWorker_GetForwardID tests GetForwardID implementation
func TestForwardWorker_GetForwardID(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)
	id := worker.GetForwardID()

	assert.NotEmpty(t, id)
	assert.Equal(t, fwd.ID(), id)
}

// TestForwardWorker_IsAlive tests IsAlive implementation
func TestForwardWorker_IsAlive(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Before starting, worker should be "alive" (context not cancelled)
	assert.True(t, worker.IsAlive())

	// Cancel context
	worker.cancel()

	// After cancel, IsAlive should return false
	assert.False(t, worker.IsAlive())
}

// TestWorker_IsRunningState tests IsRunning method
func TestWorker_IsRunningState(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Before done channel is closed, worker is "running"
	assert.True(t, worker.IsRunning())

	// Close done channel to simulate worker completion
	close(worker.doneChan)

	// After done channel closed, worker is not running
	assert.False(t, worker.IsRunning())
}

// TestForwardWorker_SignalConnectionSuccess tests success signaling
func TestForwardWorker_SignalConnectionSuccess(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Signal success
	worker.signalConnectionSuccess()

	// Should be able to receive from success channel
	select {
	case <-worker.successChan:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected signal on success channel")
	}

	// Second signal should not block (buffer of 1)
	worker.signalConnectionSuccess()
	worker.signalConnectionSuccess() // Should not block

	// Channel should have at most 1 pending signal
	select {
	case <-worker.successChan:
		// Got the signal
	default:
		// No signal (also acceptable - channel already had one)
	}
}

// TestForwardWorker_TriggerReconnect tests reconnect triggering
func TestForwardWorker_TriggerReconnect(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Trigger reconnect
	worker.TriggerReconnect("test reason")

	// Should be able to receive from reconnect channel
	select {
	case reason := <-worker.reconnectChan:
		assert.Equal(t, "test reason", reason)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected signal on reconnect channel")
	}
}

// TestForwardWorker_TriggerReconnect_WithForwardCancel tests reconnect with active forward
func TestForwardWorker_TriggerReconnect_WithForwardCancel(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Set up a forward cancel function
	ctx, cancel := context.WithCancel(context.Background())
	worker.forwardCancelMu.Lock()
	worker.forwardCancel = cancel
	worker.forwardCancelMu.Unlock()

	// Trigger reconnect
	worker.TriggerReconnect("stale connection")

	// Context should be cancelled
	select {
	case <-ctx.Done():
		// Success - context was cancelled
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected forward context to be cancelled")
	}
}

// TestForwardWorker_TriggerReconnect_NonBlocking tests non-blocking behavior
func TestForwardWorker_TriggerReconnect_NonBlocking(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Fill the channel
	worker.reconnectChan <- "first"

	// Second trigger should not block
	done := make(chan bool)
	go func() {
		worker.TriggerReconnect("second")
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("TriggerReconnect blocked when channel was full")
	}
}

// TestForwardWorker_Stop tests graceful stop
func TestForwardWorker_Stop(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Close done channel to simulate worker has finished
	close(worker.doneChan)

	// Stop should complete quickly since worker is "done"
	done := make(chan bool)
	go func() {
		worker.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop timed out")
	}
}

// TestForwardWorker_Stop_Timeout tests stop timeout behavior
func TestForwardWorker_Stop_Timeout(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)
	// Don't close doneChan - simulate hanging worker

	// Stop should timeout after ~3 seconds
	start := time.Now()
	done := make(chan bool)
	go func() {
		worker.Stop()
		done <- true
	}()

	select {
	case <-done:
		elapsed := time.Since(start)
		// Should have waited at least 2 seconds but not more than 5
		assert.True(t, elapsed >= 2*time.Second, "Should wait for timeout")
		assert.True(t, elapsed < 5*time.Second, "Should not wait too long")
	case <-time.After(10 * time.Second):
		t.Fatal("Stop never completed")
	}
}

// TestForwardWorker_GetHTTPProxy tests HTTP proxy getter
func TestForwardWorker_GetHTTPProxy(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Initially nil
	proxy := worker.GetHTTPProxy()
	assert.Nil(t, proxy)
}

// TestForwardWorker_HeartbeatResponder tests HeartbeatResponder interface
func TestForwardWorker_HeartbeatResponder(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 8080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Worker should implement HeartbeatResponder
	var responder HeartbeatResponder = worker
	assert.NotNil(t, responder)

	// Test interface methods
	assert.True(t, responder.IsAlive())
	assert.NotEmpty(t, responder.GetForwardID())
}

// TestLogWriter tests the logWriter implementation
func TestLogWriter(t *testing.T) {
	tests := []struct {
		name   string
		prefix string
		input  []byte
	}{
		{"simple message", "[test] ", []byte("hello")},
		{"empty message", "[test] ", []byte("")},
		{"multiline", "[test] ", []byte("line1\nline2")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lw := &logWriter{prefix: tt.prefix}
			n, err := lw.Write(tt.input)

			assert.NoError(t, err)
			assert.Equal(t, len(tt.input), n)
		})
	}
}

// TestHTTPLogPortOffset tests the port offset constant
func TestHTTPLogPortOffset(t *testing.T) {
	assert.Equal(t, 10000, httpLogPortOffset)
}

// TestPortForwardReadyTimeout tests the ready timeout constant
func TestPortForwardReadyTimeout(t *testing.T) {
	assert.Equal(t, 30*time.Second, portForwardReadyTimeout)
}
