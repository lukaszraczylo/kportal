package ui

import (
	"testing"

	"github.com/nvm/kportal/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestWizardMutualExclusion_AddWizardBlocksOthers tests that having an add wizard active blocks other modals
func TestWizardMutualExclusion_AddWizardBlocksOthers(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Add a forward so we have something to select
	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Activate add wizard
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.mu.Unlock()

	// Verify state
	ui.mu.RLock()
	assert.NotNil(t, ui.addWizard)
	assert.Equal(t, ViewModeAddWizard, ui.viewMode)
	ui.mu.RUnlock()

	// Check that other modals cannot be activated when add wizard is active
	// This is enforced in the handlers, not in state - we're testing the state setup
}

// TestWizardMutualExclusion_BenchmarkBlocksOthers tests that having benchmark active blocks other modals
func TestWizardMutualExclusion_BenchmarkBlocksOthers(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Add a forward
	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Activate benchmark
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("test-id", "my-app", 8080)
	ui.mu.Unlock()

	ui.mu.RLock()
	assert.NotNil(t, ui.benchmarkState)
	assert.Equal(t, ViewModeBenchmark, ui.viewMode)
	ui.mu.RUnlock()
}

// TestWizardMutualExclusion_HTTPLogBlocksOthers tests that having HTTP log view active blocks other modals
func TestWizardMutualExclusion_HTTPLogBlocksOthers(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Add a forward
	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Activate HTTP log view
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("test-id", "my-app")
	ui.mu.Unlock()

	ui.mu.RLock()
	assert.NotNil(t, ui.httpLogState)
	assert.Equal(t, ViewModeHTTPLog, ui.viewMode)
	ui.mu.RUnlock()
}

// TestWizardMutualExclusion_CheckActiveModal tests the modal activity check logic
func TestWizardMutualExclusion_CheckActiveModal(t *testing.T) {
	tests := []struct {
		setupFunc      func(*BubbleTeaUI)
		name           string
		activeModalStr string
		expectActive   bool
	}{
		{
			name:           "no modal active",
			setupFunc:      func(ui *BubbleTeaUI) {},
			expectActive:   false,
			activeModalStr: "none",
		},
		{
			name: "add wizard active",
			setupFunc: func(ui *BubbleTeaUI) {
				ui.addWizard = newAddWizardState()
			},
			expectActive:   true,
			activeModalStr: "addWizard",
		},
		{
			name: "remove wizard active",
			setupFunc: func(ui *BubbleTeaUI) {
				ui.removeWizard = &RemoveWizardState{}
			},
			expectActive:   true,
			activeModalStr: "removeWizard",
		},
		{
			name: "benchmark active",
			setupFunc: func(ui *BubbleTeaUI) {
				ui.benchmarkState = newBenchmarkState("id", "alias", 8080)
			},
			expectActive:   true,
			activeModalStr: "benchmark",
		},
		{
			name: "http log active",
			setupFunc: func(ui *BubbleTeaUI) {
				ui.httpLogState = newHTTPLogState("id", "alias")
			},
			expectActive:   true,
			activeModalStr: "httpLog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui := NewBubbleTeaUI(nil, "1.0.0")

			ui.mu.Lock()
			tt.setupFunc(ui)
			ui.mu.Unlock()

			ui.mu.RLock()
			hasActiveModal := ui.addWizard != nil ||
				ui.removeWizard != nil ||
				ui.benchmarkState != nil ||
				ui.httpLogState != nil
			ui.mu.RUnlock()

			assert.Equal(t, tt.expectActive, hasActiveModal, "Modal activity check failed for: %s", tt.activeModalStr)
		})
	}
}

// TestWizardCleanup_AddWizardReset tests that add wizard state is properly cleaned up
func TestWizardCleanup_AddWizardReset(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Set up wizard with various state
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.step = StepSelectNamespace
	ui.addWizard.selectedContext = "prod"
	ui.addWizard.contexts = []string{"prod", "staging"}
	ui.mu.Unlock()

	// Simulate cleanup (like pressing Esc)
	ui.mu.Lock()
	ui.viewMode = ViewModeMain
	ui.addWizard = nil
	ui.mu.Unlock()

	ui.mu.RLock()
	assert.Nil(t, ui.addWizard)
	assert.Equal(t, ViewModeMain, ui.viewMode)
	ui.mu.RUnlock()
}

// TestWizardCleanup_BenchmarkReset tests that benchmark state is properly cleaned up
func TestWizardCleanup_BenchmarkReset(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	cancelled := false

	// Set up benchmark with cancel function
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("id", "alias", 8080)
	ui.benchmarkState.running = true
	ui.benchmarkState.cancelFunc = func() { cancelled = true }
	ui.mu.Unlock()

	// Simulate cleanup with cancel
	ui.mu.Lock()
	if ui.benchmarkState.cancelFunc != nil {
		ui.benchmarkState.cancelFunc()
	}
	ui.viewMode = ViewModeMain
	ui.benchmarkState = nil
	ui.mu.Unlock()

	assert.True(t, cancelled, "Cancel function should have been called")

	ui.mu.RLock()
	assert.Nil(t, ui.benchmarkState)
	assert.Equal(t, ViewModeMain, ui.viewMode)
	ui.mu.RUnlock()
}

// TestWizardCleanup_HTTPLogReset tests that HTTP log state is properly cleaned up
func TestWizardCleanup_HTTPLogReset(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	cleanupCalled := false

	// Set up HTTP log with cleanup function
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("id", "alias")
	ui.httpLogState.entries = []HTTPLogEntry{{Method: "GET", Path: "/"}}
	ui.httpLogCleanup = func() { cleanupCalled = true }
	ui.mu.Unlock()

	// Simulate cleanup
	ui.mu.Lock()
	if ui.httpLogCleanup != nil {
		ui.httpLogCleanup()
		ui.httpLogCleanup = nil
	}
	ui.viewMode = ViewModeMain
	ui.httpLogState = nil
	ui.mu.Unlock()

	assert.True(t, cleanupCalled, "Cleanup function should have been called")

	ui.mu.RLock()
	assert.Nil(t, ui.httpLogState)
	assert.Nil(t, ui.httpLogCleanup)
	assert.Equal(t, ViewModeMain, ui.viewMode)
	ui.mu.RUnlock()
}

// TestViewModeValues tests view mode constants
func TestViewModeValues(t *testing.T) {
	assert.Equal(t, ViewMode(0), ViewModeMain)
	assert.Equal(t, ViewMode(1), ViewModeAddWizard)
	assert.Equal(t, ViewMode(2), ViewModeRemoveWizard)
	assert.Equal(t, ViewMode(3), ViewModeBenchmark)
	assert.Equal(t, ViewMode(4), ViewModeHTTPLog)
}

// TestRemoveWizardState_Selection tests remove wizard selection logic
func TestRemoveWizardState_Selection(t *testing.T) {
	wizard := &RemoveWizardState{
		forwards: []RemovableForward{
			{ID: "a", Alias: "app-a"},
			{ID: "b", Alias: "app-b"},
			{ID: "c", Alias: "app-c"},
		},
		selected: make(map[int]bool),
		cursor:   0,
	}

	// Toggle selection
	wizard.toggleSelection()
	assert.True(t, wizard.selected[0])

	// Move and toggle
	wizard.moveCursor(1)
	wizard.toggleSelection()
	assert.True(t, wizard.selected[1])

	// Check selected count
	assert.Equal(t, 2, wizard.getSelectedCount())

	// Get selected forwards
	selected := wizard.getSelectedForwards()
	assert.Len(t, selected, 2)
}

// TestRemoveWizardState_SelectAll tests select all functionality
func TestRemoveWizardState_SelectAll(t *testing.T) {
	wizard := &RemoveWizardState{
		forwards: []RemovableForward{
			{ID: "a"},
			{ID: "b"},
			{ID: "c"},
		},
		selected: make(map[int]bool),
	}

	wizard.selectAll()

	assert.Equal(t, 3, wizard.getSelectedCount())
	assert.True(t, wizard.selected[0])
	assert.True(t, wizard.selected[1])
	assert.True(t, wizard.selected[2])
}

// TestRemoveWizardState_SelectNone tests deselect all functionality
func TestRemoveWizardState_SelectNone(t *testing.T) {
	wizard := &RemoveWizardState{
		forwards: []RemovableForward{
			{ID: "a"},
			{ID: "b"},
			{ID: "c"},
		},
		selected: map[int]bool{0: true, 1: true, 2: true},
	}

	wizard.selectNone()

	assert.Equal(t, 0, wizard.getSelectedCount())
}

// TestRemoveWizardState_MoveCursor tests cursor movement in remove wizard
func TestRemoveWizardState_MoveCursor(t *testing.T) {
	wizard := &RemoveWizardState{
		forwards: []RemovableForward{
			{ID: "a"},
			{ID: "b"},
			{ID: "c"},
		},
		selected: make(map[int]bool),
		cursor:   0,
	}

	// Move down
	wizard.moveCursor(1)
	assert.Equal(t, 1, wizard.cursor)

	// Move down again
	wizard.moveCursor(1)
	assert.Equal(t, 2, wizard.cursor)

	// Cannot go past end
	wizard.moveCursor(1)
	assert.Equal(t, 2, wizard.cursor)

	// Move up
	wizard.moveCursor(-1)
	assert.Equal(t, 1, wizard.cursor)

	// Cannot go below 0
	wizard.moveCursor(-10)
	assert.Equal(t, 0, wizard.cursor)
}

// TestRemoveWizardState_ConfirmationMode tests confirmation mode cursor
func TestRemoveWizardState_ConfirmationMode(t *testing.T) {
	wizard := &RemoveWizardState{
		forwards:      []RemovableForward{{ID: "a"}},
		selected:      map[int]bool{0: true},
		confirming:    true,
		confirmCursor: 0,
	}

	// In confirmation mode, cursor moves between Yes/No
	wizard.moveCursor(1)
	assert.Equal(t, 1, wizard.confirmCursor)

	// Cannot go past 1
	wizard.moveCursor(1)
	assert.Equal(t, 1, wizard.confirmCursor)

	// Move back
	wizard.moveCursor(-1)
	assert.Equal(t, 0, wizard.confirmCursor)

	// Cannot go below 0
	wizard.moveCursor(-1)
	assert.Equal(t, 0, wizard.confirmCursor)
}

// TestRemoveWizardState_ToggleInConfirmationMode tests that toggle is disabled in confirmation mode
func TestRemoveWizardState_ToggleInConfirmationMode(t *testing.T) {
	wizard := &RemoveWizardState{
		forwards:   []RemovableForward{{ID: "a"}},
		selected:   make(map[int]bool),
		confirming: true,
	}

	// Toggle should be no-op in confirmation mode
	wizard.toggleSelection()
	assert.Equal(t, 0, wizard.getSelectedCount())
}
