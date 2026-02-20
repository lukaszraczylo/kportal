package ui

import (
	"fmt"
	"sync"
	"testing"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestConcurrent_AddAndRemove tests concurrent add and remove operations
// Run with: go test -race ./internal/ui/...
func TestConcurrent_AddAndRemove(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent adds
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fwd := &config.Forward{
				Resource:  fmt.Sprintf("pod/app-%d", idx),
				Port:      8080 + idx,
				LocalPort: 8080 + idx,
			}
			ui.AddForward(fmt.Sprintf("id-%d", idx), fwd)
		}(i)
	}

	wg.Wait()

	// Verify all adds succeeded
	ui.mu.RLock()
	assert.Len(t, ui.forwards, numGoroutines)
	ui.mu.RUnlock()

	// Concurrent removes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ui.Remove(fmt.Sprintf("id-%d", idx))
		}(i)
	}

	wg.Wait()

	// Verify all removes succeeded
	ui.mu.RLock()
	assert.Len(t, ui.forwards, 0)
	ui.mu.RUnlock()
}

// TestConcurrent_StatusUpdates tests concurrent status updates
func TestConcurrent_StatusUpdates(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Add forwards first
	for i := 0; i < 10; i++ {
		fwd := &config.Forward{
			Resource:  fmt.Sprintf("pod/app-%d", i),
			Port:      8080 + i,
			LocalPort: 8080 + i,
		}
		ui.AddForward(fmt.Sprintf("id-%d", i), fwd)
	}

	var wg sync.WaitGroup
	numUpdates := 1000
	statuses := []string{"Active", "Starting", "Reconnecting", "Error"}

	// Concurrent status updates
	for i := 0; i < numUpdates; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			forwardID := fmt.Sprintf("id-%d", idx%10)
			status := statuses[idx%len(statuses)]
			ui.UpdateStatus(forwardID, status)
		}(i)
	}

	wg.Wait()

	// Just verify no panics occurred - final state is non-deterministic
	ui.mu.RLock()
	assert.Len(t, ui.forwards, 10)
	ui.mu.RUnlock()
}

// TestConcurrent_SetErrors tests concurrent error setting
func TestConcurrent_SetErrors(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Add forwards
	for i := 0; i < 10; i++ {
		fwd := &config.Forward{
			Resource:  fmt.Sprintf("pod/app-%d", i),
			Port:      8080 + i,
			LocalPort: 8080 + i,
		}
		ui.AddForward(fmt.Sprintf("id-%d", i), fwd)
	}

	var wg sync.WaitGroup
	numErrors := 500

	// Concurrent error setting
	for i := 0; i < numErrors; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			forwardID := fmt.Sprintf("id-%d", idx%10)
			ui.SetError(forwardID, fmt.Sprintf("error-%d", idx))
		}(i)
	}

	wg.Wait()

	// Verify no panics
	ui.mu.RLock()
	assert.NotEmpty(t, ui.errors)
	ui.mu.RUnlock()
}

// TestConcurrent_MoveSelection tests concurrent selection movement
func TestConcurrent_MoveSelection(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Add forwards
	for i := 0; i < 20; i++ {
		fwd := &config.Forward{
			Resource:  fmt.Sprintf("pod/app-%d", i),
			Port:      8080 + i,
			LocalPort: 8080 + i,
		}
		ui.AddForward(fmt.Sprintf("id-%d", i), fwd)
	}

	var wg sync.WaitGroup
	numMoves := 1000

	// Concurrent moves
	for i := 0; i < numMoves; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			delta := 1
			if idx%2 == 0 {
				delta = -1
			}
			ui.moveSelection(delta)
		}(i)
	}

	wg.Wait()

	// Verify selection is within bounds
	ui.mu.RLock()
	assert.GreaterOrEqual(t, ui.selectedIndex, 0)
	assert.Less(t, ui.selectedIndex, len(ui.forwardOrder))
	ui.mu.RUnlock()
}

// TestConcurrent_AddRemoveAndUpdate tests mixed concurrent operations
func TestConcurrent_AddRemoveAndUpdate(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	var wg sync.WaitGroup

	// Concurrent adds
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fwd := &config.Forward{
				Resource:  fmt.Sprintf("pod/app-%d", idx),
				Port:      8080 + idx,
				LocalPort: 8080 + idx,
			}
			ui.AddForward(fmt.Sprintf("id-%d", idx), fwd)
		}(i)
	}

	// Concurrent updates (some will be for non-existent forwards)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			forwardID := fmt.Sprintf("id-%d", idx%60) // Some won't exist
			ui.UpdateStatus(forwardID, "Active")
		}(i)
	}

	// Concurrent removes (some will be for non-existent forwards)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ui.Remove(fmt.Sprintf("id-%d", idx))
		}(i)
	}

	wg.Wait()

	// Just verify no panics - final state depends on execution order
}

// TestConcurrent_HTTPLogEntries tests concurrent HTTP log entry additions
func TestConcurrent_HTTPLogEntries(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")

	var wg sync.WaitGroup
	var mu sync.Mutex // Simulate the UI lock for entries
	numEntries := 1000

	for i := 0; i < numEntries; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			entry := HTTPLogEntry{
				Method:     "GET",
				Path:       fmt.Sprintf("/api/test/%d", idx),
				StatusCode: 200,
			}
			mu.Lock()
			state.entries = append(state.entries, entry)
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	assert.Len(t, state.entries, numEntries)
}

// TestConcurrent_FilterWhileAdding tests filtering while entries are being added
func TestConcurrent_FilterWhileAdding(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterMode = HTTPLogFilterErrors

	var wg sync.WaitGroup
	var mu sync.Mutex

	// Add entries concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			code := 200
			if idx%5 == 0 {
				code = 500
			}
			entry := HTTPLogEntry{
				Method:     "GET",
				Path:       fmt.Sprintf("/api/test/%d", idx),
				StatusCode: code,
			}
			mu.Lock()
			state.entries = append(state.entries, entry)
			mu.Unlock()
		}(i)
	}

	// Filter concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mu.Lock()
			_ = state.getFilteredEntries()
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Verify filtering still works
	mu.Lock()
	filtered := state.getFilteredEntries()
	mu.Unlock()

	assert.Len(t, state.entries, 100)
	assert.Len(t, filtered, 20) // 20% are errors
}

// TestConcurrent_ToggleCallback tests that toggle callback is called safely
func TestConcurrent_ToggleCallback(t *testing.T) {
	var mu sync.Mutex
	callCount := 0

	callback := func(id string, enable bool) {
		mu.Lock()
		callCount++
		mu.Unlock()
	}

	ui := NewBubbleTeaUI(callback, "1.0.0")

	// Add a forward
	fwd := &config.Forward{
		Resource:  "pod/app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	var wg sync.WaitGroup

	// Toggle many times concurrently
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ui.toggleSelected()
		}()
	}

	wg.Wait()

	// Give callbacks time to complete (they run in goroutines)
	// This is a basic check - in real code you'd use proper synchronization
}

// TestConcurrent_WizardDependencies tests setting dependencies concurrently
func TestConcurrent_WizardDependencies(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ui.SetWizardDependencies(nil, nil, fmt.Sprintf("/path/%d", idx))
		}(i)
	}

	wg.Wait()

	// Just verify no panics
	ui.mu.RLock()
	assert.NotEmpty(t, ui.configPath)
	ui.mu.RUnlock()
}

// TestConcurrent_SetUpdateAvailable tests concurrent update availability setting
func TestConcurrent_SetUpdateAvailable(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ui.SetUpdateAvailable(fmt.Sprintf("2.0.%d", idx), "https://example.com")
		}(i)
	}

	wg.Wait()

	// Verify update is available
	ui.mu.RLock()
	assert.True(t, ui.updateAvailable)
	ui.mu.RUnlock()
}
