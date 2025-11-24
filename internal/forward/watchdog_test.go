package forward

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// WatchdogTestSuite contains tests for the watchdog
type WatchdogTestSuite struct {
	suite.Suite
	watchdog *Watchdog
}

func TestWatchdogSuite(t *testing.T) {
	suite.Run(t, new(WatchdogTestSuite))
}

func (s *WatchdogTestSuite) SetupTest() {
	// Create watchdog with fast intervals for testing
	s.watchdog = NewWatchdog(100*time.Millisecond, 300*time.Millisecond)
	s.watchdog.Start()
}

func (s *WatchdogTestSuite) TearDownTest() {
	if s.watchdog != nil {
		s.watchdog.Stop()
	}
}

// TestRegisterUnregister tests basic registration and unregistration
func (s *WatchdogTestSuite) TestRegisterUnregister() {
	callbackCalled := false
	callback := func(forwardID string) {
		callbackCalled = true
	}

	// Register worker
	s.watchdog.RegisterWorker("test-forward", callback)

	// Verify worker is registered
	_, _, exists := s.watchdog.GetWorkerState("test-forward")
	assert.True(s.T(), exists, "Worker should be registered")

	// Unregister worker
	s.watchdog.UnregisterWorker("test-forward")

	// Verify worker is unregistered
	_, _, exists = s.watchdog.GetWorkerState("test-forward")
	assert.False(s.T(), exists, "Worker should be unregistered")
	assert.False(s.T(), callbackCalled, "Callback should not have been called")
}

// TestHeartbeat tests that heartbeats update worker state
func (s *WatchdogTestSuite) TestHeartbeat() {
	s.watchdog.RegisterWorker("test-forward", nil)

	// Send initial heartbeat
	s.watchdog.Heartbeat("test-forward")

	lastHeartbeat1, count1, exists := s.watchdog.GetWorkerState("test-forward")
	require.True(s.T(), exists)
	assert.Equal(s.T(), uint64(1), count1)

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Send another heartbeat
	s.watchdog.Heartbeat("test-forward")

	lastHeartbeat2, count2, exists := s.watchdog.GetWorkerState("test-forward")
	require.True(s.T(), exists)
	assert.Equal(s.T(), uint64(2), count2)
	assert.True(s.T(), lastHeartbeat2.After(lastHeartbeat1), "Second heartbeat should be after first")
}

// TestHungWorkerDetection tests that hung workers are detected
func (s *WatchdogTestSuite) TestHungWorkerDetection() {
	callbackCalled := make(chan string, 1)
	callback := func(forwardID string) {
		callbackCalled <- forwardID
	}

	s.watchdog.RegisterWorker("test-forward", callback)

	// Send initial heartbeat
	s.watchdog.Heartbeat("test-forward")

	// Wait for worker to be considered hung (300ms threshold + 100ms check interval)
	timeout := time.After(1 * time.Second)

	select {
	case forwardID := <-callbackCalled:
		assert.Equal(s.T(), "test-forward", forwardID)
	case <-timeout:
		s.T().Fatal("Timeout waiting for hung worker callback")
	}
}

// TestHealthyWorkerNotDetectedAsHung tests that workers sending heartbeats are not considered hung
func (s *WatchdogTestSuite) TestHealthyWorkerNotDetectedAsHung() {
	callbackCalled := false
	var mu sync.Mutex
	callback := func(forwardID string) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
	}

	s.watchdog.RegisterWorker("test-forward", callback)

	// Send periodic heartbeats (faster than hang threshold)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	done := make(chan bool)
	go func() {
		for i := 0; i < 10; i++ {
			<-ticker.C
			s.watchdog.Heartbeat("test-forward")
		}
		done <- true
	}()

	// Wait for all heartbeats to complete
	<-done

	// Check that callback was not called
	mu.Lock()
	assert.False(s.T(), callbackCalled, "Callback should not be called for healthy worker")
	mu.Unlock()
}

// TestMultipleWorkers tests monitoring multiple workers simultaneously
func (s *WatchdogTestSuite) TestMultipleWorkers() {
	callbacks := make(map[string]int)
	var mu sync.Mutex

	makeCallback := func(id string) func(string) {
		return func(forwardID string) {
			mu.Lock()
			defer mu.Unlock()
			callbacks[id]++
		}
	}

	// Register multiple workers
	s.watchdog.RegisterWorker("worker-1", makeCallback("worker-1"))
	s.watchdog.RegisterWorker("worker-2", makeCallback("worker-2"))
	s.watchdog.RegisterWorker("worker-3", makeCallback("worker-3"))

	// worker-1: Keep sending heartbeats (healthy)
	ticker1 := time.NewTicker(50 * time.Millisecond)
	defer ticker1.Stop()
	go func() {
		for i := 0; i < 10; i++ {
			<-ticker1.C
			s.watchdog.Heartbeat("worker-1")
		}
	}()

	// worker-2: Send initial heartbeat then stop (will become hung)
	s.watchdog.Heartbeat("worker-2")

	// worker-3: Send initial heartbeat then stop (will become hung)
	s.watchdog.Heartbeat("worker-3")

	// Wait for hung workers to be detected
	time.Sleep(600 * time.Millisecond)

	// Check results
	mu.Lock()
	defer mu.Unlock()

	assert.Equal(s.T(), 0, callbacks["worker-1"], "worker-1 should not trigger callback (healthy)")
	assert.Greater(s.T(), callbacks["worker-2"], 0, "worker-2 should trigger callback (hung)")
	assert.Greater(s.T(), callbacks["worker-3"], 0, "worker-3 should trigger callback (hung)")
}

// TestCallbackOnlyOnFirstDetection tests that callback is only called once when hung is first detected
func (s *WatchdogTestSuite) TestCallbackOnlyOnFirstDetection() {
	callbackCount := 0
	var mu sync.Mutex
	callback := func(forwardID string) {
		mu.Lock()
		defer mu.Unlock()
		callbackCount++
	}

	s.watchdog.RegisterWorker("test-forward", callback)

	// Send initial heartbeat
	s.watchdog.Heartbeat("test-forward")

	// Wait for multiple check cycles
	time.Sleep(1 * time.Second)

	// Check that callback was only called once
	mu.Lock()
	assert.Equal(s.T(), 1, callbackCount, "Callback should only be called once")
	mu.Unlock()
}

// TestHeartbeatResetsHungState tests that sending heartbeat after hung detection resets state
func (s *WatchdogTestSuite) TestHeartbeatResetsHungState() {
	callbackCount := 0
	var mu sync.Mutex
	callback := func(forwardID string) {
		mu.Lock()
		defer mu.Unlock()
		callbackCount++
	}

	s.watchdog.RegisterWorker("test-forward", callback)

	// Send initial heartbeat
	s.watchdog.Heartbeat("test-forward")

	// Wait for hung detection
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	firstCount := callbackCount
	mu.Unlock()

	assert.Equal(s.T(), 1, firstCount, "First hung detection should trigger callback")

	// Send heartbeat to reset hung state
	s.watchdog.Heartbeat("test-forward")

	// Wait for worker to become hung again
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	secondCount := callbackCount
	mu.Unlock()

	assert.Equal(s.T(), 2, secondCount, "Second hung detection should trigger callback again")
}

// TestConcurrentOperations tests thread safety
func (s *WatchdogTestSuite) TestConcurrentOperations() {
	var wg sync.WaitGroup
	numWorkers := 10

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			forwardID := string(rune('a' + id))
			s.watchdog.RegisterWorker(forwardID, nil)
			for j := 0; j < 10; j++ {
				s.watchdog.Heartbeat(forwardID)
				time.Sleep(10 * time.Millisecond)
			}
			s.watchdog.UnregisterWorker(forwardID)
		}(i)
	}

	wg.Wait()
	// If we get here without deadlocks or panics, test passes
}

// TestStopWatchdog tests that stopping watchdog cleans up properly
func TestStopWatchdog(t *testing.T) {
	watchdog := NewWatchdog(100*time.Millisecond, 300*time.Millisecond)
	watchdog.Start()

	callbackCalled := false
	callback := func(forwardID string) {
		callbackCalled = true
	}

	watchdog.RegisterWorker("test-forward", callback)
	watchdog.Heartbeat("test-forward")

	// Stop watchdog before hang detection
	time.Sleep(100 * time.Millisecond)
	watchdog.Stop()

	// Wait to ensure no more callbacks after stop
	time.Sleep(500 * time.Millisecond)

	assert.False(t, callbackCalled, "Callback should not be called after watchdog is stopped")
}

// TestWatchdogWithZeroHeartbeats tests detecting hung worker that never sends heartbeats
func (s *WatchdogTestSuite) TestWatchdogWithZeroHeartbeats() {
	callbackCalled := make(chan string, 1)
	callback := func(forwardID string) {
		callbackCalled <- forwardID
	}

	// Register worker but never send heartbeat
	s.watchdog.RegisterWorker("test-forward", callback)

	// Wait for hung detection
	timeout := time.After(1 * time.Second)

	select {
	case forwardID := <-callbackCalled:
		assert.Equal(s.T(), "test-forward", forwardID)
	case <-timeout:
		s.T().Fatal("Timeout waiting for hung worker callback")
	}
}
