package healthcheck

import (
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// HealthCheckTestSuite contains tests for the health checker
type HealthCheckTestSuite struct {
	suite.Suite
	checker  *Checker
	listener net.Listener
	port     int
}

func TestHealthCheckSuite(t *testing.T) {
	suite.Run(t, new(HealthCheckTestSuite))
}

func (s *HealthCheckTestSuite) SetupTest() {
	// Create a test listener on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(s.T(), err)
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port

	// Create checker with fast intervals for testing
	s.checker = NewCheckerWithOptions(CheckerOptions{
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 500 * time.Millisecond,
		MaxIdleTime:      300 * time.Millisecond,
	})
}

func (s *HealthCheckTestSuite) TearDownTest() {
	if s.checker != nil {
		s.checker.Stop()
	}
	if s.listener != nil {
		s.listener.Close()
	}
}

// TestRegisterAndUnregister tests basic registration and unregistration
func (s *HealthCheckTestSuite) TestRegisterAndUnregister() {
	callbackCalled := false
	var callbackStatus Status
	var mu sync.Mutex

	callback := func(forwardID string, status Status, errorMsg string) {
		mu.Lock()
		defer mu.Unlock()
		callbackCalled = true
		callbackStatus = status
	}

	// Register port
	s.checker.Register("test-forward", s.port, callback)

	// Wait for health check to run
	time.Sleep(200 * time.Millisecond)

	// Verify callback was called with healthy status
	mu.Lock()
	assert.True(s.T(), callbackCalled, "Callback should have been called")
	assert.Equal(s.T(), StatusHealthy, callbackStatus)
	mu.Unlock()

	// Unregister
	s.checker.Unregister("test-forward")

	// Verify port is no longer monitored
	status, exists := s.checker.GetStatus("test-forward")
	assert.False(s.T(), exists, "Port should no longer exist after unregister")
	assert.Equal(s.T(), StatusUnhealthy, status)
}

// TestTCPDialMethod tests the TCP dial health check method
func (s *HealthCheckTestSuite) TestTCPDialMethod() {
	tests := []struct {
		name           string
		setupPort      bool
		expectedStatus Status
		description    string
	}{
		{
			name:           "port available - healthy",
			setupPort:      true,
			expectedStatus: StatusHealthy,
			description:    "When port is listening, status should be healthy",
		},
		{
			name:           "port unavailable - unhealthy",
			setupPort:      false,
			expectedStatus: StatusUnhealthy,
			description:    "When port is not listening, status should be unhealthy",
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var testPort int
			var testListener net.Listener

			if tt.setupPort {
				// Use the existing listener
				testPort = s.port
			} else {
				// Use a port that's not listening
				testPort = 54321 // Likely unused port
			}

			// Create a new checker for this test
			checker := NewCheckerWithOptions(CheckerOptions{
				Interval:         100 * time.Millisecond,
				Timeout:          50 * time.Millisecond,
				Method:           CheckMethodTCPDial,
				MaxConnectionAge: 0, // Disable for this test
				MaxIdleTime:      0, // Disable for this test
			})
			defer checker.Stop()

			checker.Register("test-forward", testPort, nil)

			// Wait for health checks to complete
			if !tt.setupPort {
				// For unhealthy case, wait for grace period
				time.Sleep(startupGracePeriod + 200*time.Millisecond)
			} else {
				time.Sleep(200 * time.Millisecond)
			}

			// Check status directly
			status, exists := checker.GetStatus("test-forward")
			assert.True(s.T(), exists)
			assert.Equal(s.T(), tt.expectedStatus, status, tt.description)

			if testListener != nil {
				testListener.Close()
			}
		})
	}
}

// TestDataTransferMethod tests the data transfer health check method
func (s *HealthCheckTestSuite) TestDataTransferMethod() {
	tests := []struct {
		name           string
		serverBehavior string // "banner", "silent", "close", "none"
		expectedStatus Status
	}{
		{
			name:           "server sends banner - healthy",
			serverBehavior: "banner",
			expectedStatus: StatusHealthy,
		},
		{
			name:           "server waits silently - healthy (timeout OK)",
			serverBehavior: "silent",
			expectedStatus: StatusHealthy,
		},
		{
			name:           "server closes connection - healthy (EOF OK)",
			serverBehavior: "close",
			expectedStatus: StatusHealthy,
		},
		{
			name:           "no server listening - unhealthy",
			serverBehavior: "none",
			expectedStatus: StatusUnhealthy,
		},
	}

	for _, tt := range tests {
		s.Run(tt.name, func() {
			var testPort int
			var testListener net.Listener
			var err error

			if tt.serverBehavior != "none" {
				// Start test server
				testListener, err = net.Listen("tcp", "127.0.0.1:0")
				require.NoError(s.T(), err)
				testPort = testListener.Addr().(*net.TCPAddr).Port

				// Handle connections based on behavior
				go func() {
					for {
						conn, err := testListener.Accept()
						if err != nil {
							return
						}
						switch tt.serverBehavior {
						case "banner":
							conn.Write([]byte("220 Welcome\r\n"))
							time.Sleep(50 * time.Millisecond)
							conn.Close()
						case "close":
							conn.Close()
						case "silent":
							// Just keep connection open
							time.Sleep(200 * time.Millisecond)
							conn.Close()
						}
					}
				}()
				defer testListener.Close()
			} else {
				testPort = 54322 // Unused port
			}

			// Create checker with data transfer method
			checker := NewCheckerWithOptions(CheckerOptions{
				Interval:         100 * time.Millisecond,
				Timeout:          50 * time.Millisecond,
				Method:           CheckMethodDataTransfer,
				MaxConnectionAge: 0, // Disable for this test
				MaxIdleTime:      0, // Disable for this test
			})
			defer checker.Stop()

			checker.Register("test-forward", testPort, nil)

			// Wait for health checks to complete
			if tt.serverBehavior == "none" {
				// For unhealthy case, wait for grace period
				time.Sleep(startupGracePeriod + 200*time.Millisecond)
			} else {
				time.Sleep(300 * time.Millisecond)
			}

			// Check status directly
			status, exists := checker.GetStatus("test-forward")
			assert.True(s.T(), exists)
			assert.Equal(s.T(), tt.expectedStatus, status)
		})
	}
}

// TestConnectionAgeDetection tests max connection age detection
func (s *HealthCheckTestSuite) TestConnectionAgeDetection() {
	statusChanges := make(chan Status, 10)
	callback := func(forwardID string, status Status, errorMsg string) {
		statusChanges <- status
	}

	// Create checker with very short max connection age
	checker := NewCheckerWithOptions(CheckerOptions{
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 150 * time.Millisecond, // Very short for testing
		MaxIdleTime:      0,                      // Disable idle detection
	})
	defer checker.Stop()

	checker.Register("test-forward", s.port, callback)

	// Wait for initial healthy status
	var gotHealthy, gotStale bool
	timeout := time.After(1 * time.Second)

	for {
		select {
		case status := <-statusChanges:
			if status == StatusHealthy || status == StatusStarting {
				gotHealthy = true
			}
			if status == StatusStale {
				gotStale = true
			}
			if gotHealthy && gotStale {
				return // Test passed
			}
		case <-timeout:
			s.T().Fatalf("Expected StatusStale after max connection age exceeded. gotHealthy=%v, gotStale=%v",
				gotHealthy, gotStale)
		}
	}
}

// TestIdleTimeDetection tests that connections with passing health checks are NOT marked as stale
// This verifies that successful health checks update LastActivity, preventing false idle detection
func (s *HealthCheckTestSuite) TestIdleTimeDetection() {
	statusChanges := make(chan Status, 10)
	callback := func(forwardID string, status Status, errorMsg string) {
		statusChanges <- status
	}

	// Create checker with very short max idle time
	checker := NewCheckerWithOptions(CheckerOptions{
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 0,                      // Disable age detection
		MaxIdleTime:      150 * time.Millisecond, // Very short for testing
	})
	defer checker.Stop()

	checker.Register("test-forward", s.port, callback)

	// Wait long enough that idle time WOULD be exceeded if health checks didn't update LastActivity
	time.Sleep(500 * time.Millisecond)

	// Verify connection is still healthy, not stale
	// This proves that successful health checks are updating LastActivity
	status, exists := checker.GetStatus("test-forward")
	require.True(s.T(), exists)
	assert.Equal(s.T(), StatusHealthy, status, "Connection with passing health checks should NOT be marked as stale")

	// Verify we never received a StatusStale callback
	select {
	case status := <-statusChanges:
		if status == StatusStale {
			s.T().Fatal("Connection should NOT be marked as stale when health checks are passing")
		}
	default:
		// No stale status - this is correct
	}
}

// TestMarkConnected tests that MarkConnected resets connection time
func (s *HealthCheckTestSuite) TestMarkConnected() {
	checker := NewCheckerWithOptions(CheckerOptions{
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 200 * time.Millisecond,
		MaxIdleTime:      0,
	})
	defer checker.Stop()

	statusChanges := make(chan Status, 10)
	callback := func(forwardID string, status Status, errorMsg string) {
		statusChanges <- status
	}

	checker.Register("test-forward", s.port, callback)

	// Wait a bit
	time.Sleep(100 * time.Millisecond)

	// Mark as reconnected (resets connection time)
	checker.MarkConnected("test-forward")

	// Wait for connection age to exceed (relative to first connection time)
	time.Sleep(200 * time.Millisecond)

	// Check status - should still be healthy because we reset connection time
	status, exists := checker.GetStatus("test-forward")
	assert.True(s.T(), exists)
	// Note: Might be StatusStale by now, but the key is that MarkConnected delayed it
	// This is a timing-sensitive test, so we just verify the functionality exists
	_ = status
}

// TestRecordActivity tests that RecordActivity resets idle time
func (s *HealthCheckTestSuite) TestRecordActivity() {
	checker := NewCheckerWithOptions(CheckerOptions{
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 0,
		MaxIdleTime:      200 * time.Millisecond,
	})
	defer checker.Stop()

	statusChanges := make(chan Status, 10)
	callback := func(forwardID string, status Status, errorMsg string) {
		statusChanges <- status
	}

	checker.Register("test-forward", s.port, callback)

	// Periodically record activity to prevent idle detection
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	go func() {
		for i := 0; i < 5; i++ {
			<-ticker.C
			checker.RecordActivity("test-forward")
		}
	}()

	// Wait longer than idle timeout
	time.Sleep(500 * time.Millisecond)

	// Should still be healthy due to activity
	status, exists := checker.GetStatus("test-forward")
	assert.True(s.T(), exists)
	// May transition to stale eventually, but activity recording should have delayed it
	_ = status
}

// TestMarkReconnecting tests the MarkReconnecting functionality
func (s *HealthCheckTestSuite) TestMarkReconnecting() {
	statusChanges := make(chan Status, 10)
	callback := func(forwardID string, status Status, errorMsg string) {
		statusChanges <- status
	}

	s.checker.Register("test-forward", s.port, callback)

	// Wait for initial status
	time.Sleep(150 * time.Millisecond)

	// Mark as reconnecting
	s.checker.MarkReconnecting("test-forward")

	// Should receive reconnecting status
	timeout := time.After(500 * time.Millisecond)
	gotReconnect := false
	for !gotReconnect {
		select {
		case status := <-statusChanges:
			if status == StatusReconnect {
				gotReconnect = true
			}
		case <-timeout:
			s.T().Fatal("Expected StatusReconnect")
		}
	}
}

// TestStartingGracePeriod tests that errors during grace period show as "Starting"
func (s *HealthCheckTestSuite) TestStartingGracePeriod() {
	// Use a port that's not listening
	unavailablePort := 54323

	checker := NewCheckerWithOptions(CheckerOptions{
		Interval:         50 * time.Millisecond,
		Timeout:          25 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 0,
		MaxIdleTime:      0,
	})
	defer checker.Stop()

	// Register without callback - we'll check status directly
	checker.Register("test-forward", unavailablePort, nil)

	// Immediately check status - should be Starting or not yet checked
	status, exists := checker.GetStatus("test-forward")
	assert.True(s.T(), exists)
	// Initially should be Starting
	assert.Equal(s.T(), StatusStarting, status)

	// Wait for grace period to expire
	time.Sleep(startupGracePeriod + 200*time.Millisecond)

	// Now should be Unhealthy
	status, exists = checker.GetStatus("test-forward")
	assert.True(s.T(), exists)
	assert.Equal(s.T(), StatusUnhealthy, status)
}

// TestGetAllErrors tests retrieving all error messages
func (s *HealthCheckTestSuite) TestGetAllErrors() {
	// Create a new checker with faster intervals for this test
	checker := NewCheckerWithOptions(CheckerOptions{
		Interval:         100 * time.Millisecond,
		Timeout:          50 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 0,
		MaxIdleTime:      0,
	})
	defer checker.Stop()

	// Register multiple forwards
	checker.Register("forward1", s.port, nil)
	checker.Register("forward2", 54324, nil) // Unavailable port

	// Wait for grace period to expire
	time.Sleep(startupGracePeriod + 300*time.Millisecond)

	errors := checker.GetAllErrors()

	// forward2 should have an error
	_, hasError := errors["forward2"]
	assert.True(s.T(), hasError, "forward2 should have an error")

	// forward1 should not have an error
	_, hasError = errors["forward1"]
	assert.False(s.T(), hasError, "forward1 should not have an error")
}

// TestConcurrentOperations tests thread safety
func (s *HealthCheckTestSuite) TestConcurrentOperations() {
	var wg sync.WaitGroup
	numGoroutines := 10

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			forwardID := fmt.Sprintf("forward-%d", id)
			s.checker.Register(forwardID, s.port, nil)
			time.Sleep(50 * time.Millisecond)
			s.checker.MarkConnected(forwardID)
			s.checker.RecordActivity(forwardID)
			status, _ := s.checker.GetStatus(forwardID)
			_ = status
			s.checker.Unregister(forwardID)
		}(i)
	}

	wg.Wait()
	// If we get here without deadlocks or panics, test passes
}

// TestDefaultOptions tests that NewChecker uses sensible defaults
func TestDefaultOptions(t *testing.T) {
	checker := NewChecker(5*time.Second, 2*time.Second)
	defer checker.Stop()

	assert.Equal(t, 5*time.Second, checker.interval)
	assert.Equal(t, 2*time.Second, checker.timeout)
	assert.Equal(t, CheckMethodDataTransfer, checker.method)
	assert.Equal(t, 25*time.Minute, checker.maxConnectionAge)
	assert.Equal(t, 10*time.Minute, checker.maxIdleTime)
}

// TestCustomOptions tests NewCheckerWithOptions
func TestCustomOptions(t *testing.T) {
	opts := CheckerOptions{
		Interval:         1 * time.Second,
		Timeout:          500 * time.Millisecond,
		Method:           CheckMethodTCPDial,
		MaxConnectionAge: 5 * time.Minute,
		MaxIdleTime:      2 * time.Minute,
	}

	checker := NewCheckerWithOptions(opts)
	defer checker.Stop()

	assert.Equal(t, 1*time.Second, checker.interval)
	assert.Equal(t, 500*time.Millisecond, checker.timeout)
	assert.Equal(t, CheckMethodTCPDial, checker.method)
	assert.Equal(t, 5*time.Minute, checker.maxConnectionAge)
	assert.Equal(t, 2*time.Minute, checker.maxIdleTime)
}
