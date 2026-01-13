package forward

import (
	"testing"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/events"
	"github.com/stretchr/testify/assert"
)

// TestNewManager tests manager creation
func TestNewManager(t *testing.T) {
	t.Run("creates manager with default settings", func(t *testing.T) {
		// Skip if no kubeconfig available (CI environment)
		manager, err := NewManager(false)
		if err != nil {
			t.Skip("Skipping test - no kubeconfig available")
		}
		defer manager.Stop()

		assert.NotNil(t, manager.workers)
		assert.NotNil(t, manager.portChecker)
		assert.NotNil(t, manager.healthChecker)
		assert.NotNil(t, manager.watchdog)
		assert.NotNil(t, manager.eventBus)
		assert.False(t, manager.verbose)
	})

	t.Run("creates manager in verbose mode", func(t *testing.T) {
		manager, err := NewManager(true)
		if err != nil {
			t.Skip("Skipping test - no kubeconfig available")
		}
		defer manager.Stop()

		assert.True(t, manager.verbose)
	})
}

// TestManager_SetStatusUI tests setting the status UI
func TestManager_SetStatusUI(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	mockUI := &MockStatusUpdater{}
	manager.SetStatusUI(mockUI)

	assert.Equal(t, mockUI, manager.statusUI)
}

// TestManager_GetWorker tests getting a worker by ID
func TestManager_GetWorker(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	// Non-existent worker
	worker := manager.GetWorker("non-existent")
	assert.Nil(t, worker)
}

// TestManager_Start_NilConfig tests starting with nil config
func TestManager_Start_NilConfig(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	err = manager.Start(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "configuration is nil")
}

// TestManager_Start_EmptyForwards tests starting with empty forwards
func TestManager_Start_EmptyForwards(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	cfg := &config.Config{}
	err = manager.Start(cfg)
	// Empty config is now valid - allows users to add forwards via TUI
	assert.NoError(t, err)
}

// TestManager_Reload_NilConfig tests reloading with nil config
func TestManager_Reload_NilConfig(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	err = manager.Reload(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "new configuration is nil")
}

// TestManager_EnableForward_NoConfig tests enabling without config
func TestManager_EnableForward_NoConfig(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	err = manager.EnableForward("some-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration available")
}

// TestManager_DisableForward_NotFound tests disabling non-existent forward
func TestManager_DisableForward_NotFound(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	err = manager.DisableForward("non-existent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker not found")
}

// TestManager_extractPorts tests port extraction
func TestManager_extractPorts(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	forwards := []config.Forward{
		{LocalPort: 8080},
		{LocalPort: 5432},
		{LocalPort: 3000},
	}

	ports := manager.extractPorts(forwards)
	assert.Equal(t, []int{8080, 5432, 3000}, ports)
}

// TestManager_getResourceForPort tests finding resource by port
func TestManager_getResourceForPort(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	forwards := []config.Forward{
		{Resource: "pod/app1", LocalPort: 8080, Port: 80},
		{Resource: "service/db", LocalPort: 5432, Port: 5432},
	}

	// Found
	resource := manager.getResourceForPort(forwards, 8080)
	assert.Contains(t, resource, "app1")

	// Not found
	resource = manager.getResourceForPort(forwards, 9999)
	assert.Equal(t, "unknown", resource)
}

// MockStatusUpdater is a mock implementation of StatusUpdater
type MockStatusUpdater struct {
	updates   []StatusUpdate
	adds      []ForwardAdd
	removes   []string
	errorSets []ErrorSet
}

type StatusUpdate struct {
	ID     string
	Status string
}

type ForwardAdd struct {
	Fwd *config.Forward
	ID  string
}

type ErrorSet struct {
	ID  string
	Msg string
}

func (m *MockStatusUpdater) UpdateStatus(id string, status string) {
	m.updates = append(m.updates, StatusUpdate{ID: id, Status: status})
}

func (m *MockStatusUpdater) AddForward(id string, fwd *config.Forward) {
	m.adds = append(m.adds, ForwardAdd{ID: id, Fwd: fwd})
}

func (m *MockStatusUpdater) Remove(id string) {
	m.removes = append(m.removes, id)
}

func (m *MockStatusUpdater) SetError(id, msg string) {
	m.errorSets = append(m.errorSets, ErrorSet{ID: id, Msg: msg})
}

// TestConfigureHealthChecker tests health checker configuration
func TestConfigureHealthChecker(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	tests := []struct {
		name   string
		method string
	}{
		{"tcp-dial method", "tcp-dial"},
		{"data-transfer method", "data-transfer"},
		{"unknown method defaults to data-transfer", "unknown"},
		{"empty method defaults to data-transfer", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				HealthCheck: &config.HealthCheckSpec{
					Method: tt.method,
				},
			}

			// Should not panic
			manager.configureHealthChecker(cfg)
			assert.NotNil(t, manager.healthChecker)
		})
	}
}

// TestManager_Stop tests graceful shutdown
func TestManager_Stop(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}

	// Stop should not panic even with no workers
	done := make(chan bool)
	go func() {
		manager.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Stop timed out")
	}
}

// TestManager_Reload_EmptyToEmpty tests reloading from empty to empty config
func TestManager_Reload_EmptyToEmpty(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	cfg := &config.Config{}
	err = manager.Reload(cfg)
	// Should handle gracefully (stop all workers if any)
	assert.NoError(t, err)
}

// TestPortConflict tests the PortConflict struct
func TestPortConflict(t *testing.T) {
	conflict := PortConflict{
		Port:     8080,
		Resource: "dev/default/pod/app:8080",
		UsedBy:   "nginx (PID 1234)",
	}

	assert.Equal(t, 8080, conflict.Port)
	assert.Equal(t, "dev/default/pod/app:8080", conflict.Resource)
	assert.Equal(t, "nginx (PID 1234)", conflict.UsedBy)
}

// TestStatusUpdater_Interface tests that MockStatusUpdater implements StatusUpdater
func TestStatusUpdater_Interface(t *testing.T) {
	var _ StatusUpdater = (*MockStatusUpdater)(nil)
}

// TestManager_WorkersMap tests workers map operations
func TestManager_WorkersMap(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	// Initial state
	assert.Empty(t, manager.workers)

	// Verify concurrent-safe access patterns
	manager.workersMu.RLock()
	count := len(manager.workers)
	manager.workersMu.RUnlock()
	assert.Equal(t, 0, count)
}

// TestManager_EventBusIntegration tests event bus wiring
func TestManager_EventBusIntegration(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	// Event bus should be wired to health checker and watchdog
	assert.NotNil(t, manager.eventBus)

	// SubscribeAll should work (no return value in this API)
	manager.eventBus.SubscribeAll(func(event events.Event) {
		// Handler
	})
}
