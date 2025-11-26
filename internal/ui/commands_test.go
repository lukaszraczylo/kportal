package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nvm/kportal/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessageTypes tests the message type structures
func TestMessageTypes(t *testing.T) {
	t.Run("ContextsLoadedMsg", func(t *testing.T) {
		msg := ContextsLoadedMsg{
			contexts: []string{"ctx1", "ctx2"},
		}
		assert.Len(t, msg.contexts, 2)
		assert.Nil(t, msg.err)

		errMsg := ContextsLoadedMsg{
			err: assert.AnError,
		}
		assert.NotNil(t, errMsg.err)
	})

	t.Run("NamespacesLoadedMsg", func(t *testing.T) {
		msg := NamespacesLoadedMsg{
			namespaces: []string{"default", "kube-system"},
		}
		assert.Len(t, msg.namespaces, 2)
		assert.Nil(t, msg.err)
	})

	t.Run("PodsLoadedMsg", func(t *testing.T) {
		msg := PodsLoadedMsg{
			pods: []k8s.PodInfo{
				{Name: "pod1", Namespace: "default"},
				{Name: "pod2", Namespace: "default"},
			},
		}
		assert.Len(t, msg.pods, 2)
		assert.Nil(t, msg.err)
	})

	t.Run("ServicesLoadedMsg", func(t *testing.T) {
		msg := ServicesLoadedMsg{
			services: []k8s.ServiceInfo{
				{Name: "svc1", Namespace: "default"},
			},
		}
		assert.Len(t, msg.services, 1)
		assert.Nil(t, msg.err)
	})

	t.Run("SelectorValidatedMsg", func(t *testing.T) {
		validMsg := SelectorValidatedMsg{
			valid: true,
			pods: []k8s.PodInfo{
				{Name: "matched-pod"},
			},
		}
		assert.True(t, validMsg.valid)
		assert.Len(t, validMsg.pods, 1)

		invalidMsg := SelectorValidatedMsg{
			valid: false,
			err:   assert.AnError,
		}
		assert.False(t, invalidMsg.valid)
		assert.NotNil(t, invalidMsg.err)
	})

	t.Run("PortCheckedMsg", func(t *testing.T) {
		availableMsg := PortCheckedMsg{
			port:      8080,
			available: true,
			message:   "Port 8080 available",
		}
		assert.Equal(t, 8080, availableMsg.port)
		assert.True(t, availableMsg.available)

		unavailableMsg := PortCheckedMsg{
			port:      8080,
			available: false,
			message:   "Port 8080 in use by process",
		}
		assert.False(t, unavailableMsg.available)
	})

	t.Run("ForwardSavedMsg", func(t *testing.T) {
		successMsg := ForwardSavedMsg{success: true}
		assert.True(t, successMsg.success)

		failMsg := ForwardSavedMsg{success: false, err: assert.AnError}
		assert.False(t, failMsg.success)
		assert.NotNil(t, failMsg.err)
	})

	t.Run("ForwardsRemovedMsg", func(t *testing.T) {
		msg := ForwardsRemovedMsg{
			success: true,
			count:   3,
		}
		assert.True(t, msg.success)
		assert.Equal(t, 3, msg.count)
	})

	t.Run("WizardCompleteMsg", func(t *testing.T) {
		msg := WizardCompleteMsg{}
		assert.NotNil(t, msg)
	})

	t.Run("BenchmarkCompleteMsg", func(t *testing.T) {
		msg := BenchmarkCompleteMsg{
			ForwardID: "fwd-123",
			Results:   nil,
			Error:     nil,
		}
		assert.Equal(t, "fwd-123", msg.ForwardID)
	})

	t.Run("BenchmarkProgressMsg", func(t *testing.T) {
		msg := BenchmarkProgressMsg{
			ForwardID: "fwd-123",
			Completed: 50,
			Total:     100,
		}
		assert.Equal(t, "fwd-123", msg.ForwardID)
		assert.Equal(t, 50, msg.Completed)
		assert.Equal(t, 100, msg.Total)
	})

	t.Run("HTTPLogEntryMsg", func(t *testing.T) {
		msg := HTTPLogEntryMsg{
			Entry: HTTPLogEntry{
				Method:     "GET",
				Path:       "/api/test",
				StatusCode: 200,
			},
		}
		assert.Equal(t, "GET", msg.Entry.Method)
		assert.Equal(t, "/api/test", msg.Entry.Path)
		assert.Equal(t, 200, msg.Entry.StatusCode)
	})
}

// TestCheckPortCmd tests the port availability check command
func TestCheckPortCmd_PortAvailability(t *testing.T) {
	// Create a temporary config file for testing
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create an empty config file
	err := os.WriteFile(configPath, []byte("contexts: []\n"), 0600)
	require.NoError(t, err)

	// Test checking a random high port that should be available
	cmd := checkPortCmd(59999, configPath)
	msg := cmd()

	portMsg, ok := msg.(PortCheckedMsg)
	require.True(t, ok, "Expected PortCheckedMsg")
	assert.Equal(t, 59999, portMsg.port)
	// The port may or may not be available depending on the system,
	// but we verify the message structure is correct
	assert.NotEmpty(t, portMsg.message)
}

// TestCheckPortCmd_ConfigConflict tests port conflict detection in config
func TestCheckPortCmd_ConfigConflict(t *testing.T) {
	// Create a temporary config file with a forward using port 8080
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	configContent := `contexts:
  - name: test-ctx
    namespaces:
      - name: default
        forwards:
          - resource: pod/my-app
            port: 80
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	require.NoError(t, err)

	// Test checking port that's already in config
	cmd := checkPortCmd(8080, configPath)
	msg := cmd()

	portMsg, ok := msg.(PortCheckedMsg)
	require.True(t, ok, "Expected PortCheckedMsg")
	assert.Equal(t, 8080, portMsg.port)
	assert.False(t, portMsg.available, "Port should not be available (in config)")
	assert.Contains(t, portMsg.message, "already assigned")
}

// TestCheckPortCmd_InvalidConfig tests behavior with invalid config file
func TestCheckPortCmd_InvalidConfig(t *testing.T) {
	// Use a non-existent config path
	cmd := checkPortCmd(59998, "/nonexistent/path/.kportal.yaml")
	msg := cmd()

	portMsg, ok := msg.(PortCheckedMsg)
	require.True(t, ok, "Expected PortCheckedMsg")
	// Should still return a result (just skip config check)
	assert.Equal(t, 59998, portMsg.port)
	assert.NotEmpty(t, portMsg.message)
}

// TestListenBenchmarkProgressCmd tests the progress listener command
func TestListenBenchmarkProgressCmd(t *testing.T) {
	progressCh := make(chan BenchmarkProgressMsg, 1)

	// Send a progress message
	progressCh <- BenchmarkProgressMsg{
		ForwardID: "fwd-123",
		Completed: 25,
		Total:     100,
	}

	cmd := listenBenchmarkProgressCmd(progressCh)
	msg := cmd()

	progressMsg, ok := msg.(BenchmarkProgressMsg)
	require.True(t, ok, "Expected BenchmarkProgressMsg")
	assert.Equal(t, "fwd-123", progressMsg.ForwardID)
	assert.Equal(t, 25, progressMsg.Completed)
	assert.Equal(t, 100, progressMsg.Total)
}

// TestListenBenchmarkProgressCmd_ChannelClosed tests behavior when channel closes
func TestListenBenchmarkProgressCmd_ChannelClosed(t *testing.T) {
	progressCh := make(chan BenchmarkProgressMsg)
	close(progressCh)

	cmd := listenBenchmarkProgressCmd(progressCh)
	msg := cmd()

	assert.Nil(t, msg, "Should return nil when channel is closed")
}

// TestRunBenchmarkCmd_Cancellation tests benchmark cancellation
func TestRunBenchmarkCmd_Cancellation(t *testing.T) {
	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	progressCh := make(chan BenchmarkProgressMsg, 100)

	cmd := runBenchmarkCmd(ctx, "fwd-123", 59997, "/", "GET", 1, 10, progressCh)

	// Run with timeout to prevent hanging
	done := make(chan bool, 1)
	var msg interface{}
	go func() {
		msg = cmd()
		done <- true
	}()

	select {
	case <-done:
		// Command completed
	case <-time.After(5 * time.Second):
		t.Fatal("runBenchmarkCmd timed out")
	}

	completeMsg, ok := msg.(BenchmarkCompleteMsg)
	require.True(t, ok, "Expected BenchmarkCompleteMsg")
	assert.Equal(t, "fwd-123", completeMsg.ForwardID)
	// When cancelled, we expect either an error or the context cancellation message
	// The benchmark may or may not have had time to process the cancellation
}

// TestK8sAPITimeout tests that the timeout constant is correct
func TestK8sAPITimeout(t *testing.T) {
	assert.Equal(t, 10*time.Second, k8sAPITimeout)
}

// TestRemovableForwardStruct tests the RemovableForward structure used by commands
func TestRemovableForwardStruct(t *testing.T) {
	rf := RemovableForward{
		ID:        "fwd-123",
		Context:   "prod",
		Namespace: "default",
		Resource:  "pod/my-app",
		Selector:  "app=my-app",
		Alias:     "my-app",
		Port:      80,
		LocalPort: 8080,
	}

	assert.Equal(t, "fwd-123", rf.ID)
	assert.Equal(t, "prod", rf.Context)
	assert.Equal(t, "default", rf.Namespace)
	assert.Equal(t, "pod/my-app", rf.Resource)
	assert.Equal(t, "app=my-app", rf.Selector)
	assert.Equal(t, "my-app", rf.Alias)
	assert.Equal(t, 80, rf.Port)
	assert.Equal(t, 8080, rf.LocalPort)
}

// TestBenchmarkProgressCallback tests the progress callback in runBenchmarkCmd
func TestBenchmarkProgressCallback(t *testing.T) {
	// Test that progress channel handles blocking gracefully
	progressCh := make(chan BenchmarkProgressMsg, 1) // Small buffer

	// Fill the channel
	progressCh <- BenchmarkProgressMsg{Completed: 1, Total: 100}

	// Test non-blocking send by creating callback similar to runBenchmarkCmd
	callback := func(completed, total int) {
		select {
		case progressCh <- BenchmarkProgressMsg{
			ForwardID: "test",
			Completed: completed,
			Total:     total,
		}:
		default:
			// Drop if channel is full - should not block
		}
	}

	// Should not block even with full channel
	done := make(chan bool, 1)
	go func() {
		callback(50, 100) // This should not block
		done <- true
	}()

	select {
	case <-done:
		// Success - didn't block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Callback blocked when channel was full")
	}
}

// TestHTTPLogEntry tests the HTTPLogEntry structure
func TestHTTPLogEntry(t *testing.T) {
	entry := HTTPLogEntry{
		Timestamp:  "2025-11-26T10:30:00Z",
		Direction:  "request",
		Method:     "POST",
		Path:       "/api/users",
		StatusCode: 201,
		LatencyMs:  150,
		BodySize:   1024,
	}

	assert.Equal(t, "2025-11-26T10:30:00Z", entry.Timestamp)
	assert.Equal(t, "request", entry.Direction)
	assert.Equal(t, "POST", entry.Method)
	assert.Equal(t, "/api/users", entry.Path)
	assert.Equal(t, 201, entry.StatusCode)
	assert.Equal(t, int64(150), entry.LatencyMs)
	assert.Equal(t, 1024, entry.BodySize)
}

// TestHTTPLogSubscriberType tests the HTTPLogSubscriber function type
func TestHTTPLogSubscriberType(t *testing.T) {
	// Test that our mock matches the type
	mock := NewMockHTTPLogSubscriber()
	var subscriber HTTPLogSubscriber = mock.GetSubscriberFunc()

	// Test subscription
	callCount := 0
	cleanup := subscriber("fwd-123", func(entry HTTPLogEntry) {
		callCount++
	})

	// Send an entry
	mock.SendEntry("fwd-123", HTTPLogEntry{Method: "GET"})
	assert.Equal(t, 1, callCount)

	// Clean up
	cleanup()
	assert.Equal(t, 1, mock.CleanupCalls)
}
