package ui

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to create a model for testing
func newTestModel() model {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	return model{ui: ui, termWidth: 120, termHeight: 40}
}

// Helper to create a model with a forward
func newTestModelWithForward() model {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
		Alias:     "my-app",
	}
	ui.AddForward("test-id", fwd)
	return model{ui: ui, termWidth: 120, termHeight: 40}
}

// TestHandleMainViewKeys_Quit tests quit key handling
func TestHandleMainViewKeys_Quit(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"q", true},
		{"ctrl+c", true},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			m := newTestModel()
			_, cmd := m.handleMainViewKeys(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)})

			if tt.key == "ctrl+c" {
				keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
				_, cmd = m.handleMainViewKeys(keyMsg)
			}

			// tea.Quit returns a special command
			if tt.expected {
				assert.NotNil(t, cmd)
			}
		})
	}
}

// TestHandleMainViewKeys_Navigation tests cursor navigation
func TestHandleMainViewKeys_Navigation(t *testing.T) {
	m := newTestModelWithForward()

	// Add more forwards for navigation testing
	for i := 0; i < 5; i++ {
		fwd := &config.Forward{
			Resource:  "pod/app",
			Port:      8080 + i,
			LocalPort: 8080 + i,
		}
		m.ui.AddForward(string(rune('a'+i)), fwd)
	}

	tests := []struct {
		name          string
		key           string
		keyType       tea.KeyType
		initialIndex  int
		expectedIndex int
	}{
		{"down arrow", "down", tea.KeyDown, 0, 1},
		{"j key", "j", tea.KeyRunes, 0, 1},
		{"up arrow", "up", tea.KeyUp, 2, 1},
		{"k key", "k", tea.KeyRunes, 2, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m.ui.mu.Lock()
			m.ui.selectedIndex = tt.initialIndex
			m.ui.mu.Unlock()

			var keyMsg tea.KeyMsg
			if tt.keyType == tea.KeyRunes {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			} else {
				keyMsg = tea.KeyMsg{Type: tt.keyType}
			}

			m.handleMainViewKeys(keyMsg)

			m.ui.mu.RLock()
			assert.Equal(t, tt.expectedIndex, m.ui.selectedIndex)
			m.ui.mu.RUnlock()
		})
	}
}

// TestHandleMainViewKeys_Toggle tests space/enter toggle
func TestHandleMainViewKeys_Toggle(t *testing.T) {
	toggleCallback := NewMockToggleCallback()
	ui := NewBubbleTeaUI(toggleCallback.GetFunc(), "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Toggle with space
	keyMsg := tea.KeyMsg{Type: tea.KeySpace}
	m.handleMainViewKeys(keyMsg)

	// Check disabled state changed
	m.ui.mu.RLock()
	isDisabled := m.ui.disabledMap["test-id"]
	m.ui.mu.RUnlock()

	assert.True(t, isDisabled)

	// Give callback goroutine time to execute
	time.Sleep(10 * time.Millisecond)

	// Verify callback was called
	assert.GreaterOrEqual(t, toggleCallback.CallCount(), 1)
}

// TestHandleMainViewKeys_NewWizard tests 'n' key with dependencies
func TestHandleMainViewKeys_NewWizard(t *testing.T) {
	mockDiscovery := NewMockDiscovery()
	mockMutator := NewMockMutator()

	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.SetWizardDependencies(nil, nil, "/path/to/config") // Real Discovery/Mutator needed

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Without dependencies, 'n' should do nothing
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	m.handleMainViewKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Nil(t, m.ui.addWizard, "Wizard should not be created without dependencies")
	m.ui.mu.RUnlock()

	// With mock (but we can't inject easily due to concrete types)
	// This test documents the expected behavior
	_ = mockDiscovery
	_ = mockMutator
}

// TestHandleMainViewKeys_DeleteConfirmation tests 'd' key
func TestHandleMainViewKeys_DeleteConfirmation(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.SetWizardDependencies(nil, &config.Mutator{}, "/path/to/config")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
		Alias:     "my-app",
	}
	ui.AddForward("test-id", fwd)

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press 'd' to show delete confirmation
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}
	m.handleMainViewKeys(keyMsg)

	m.ui.mu.RLock()
	assert.True(t, m.ui.deleteConfirming)
	assert.Equal(t, "test-id", m.ui.deleteConfirmID)
	assert.Equal(t, "my-app", m.ui.deleteConfirmAlias)
	assert.Equal(t, 1, m.ui.deleteConfirmCursor) // Default to "No"
	m.ui.mu.RUnlock()
}

// TestHandleMainViewKeys_DeleteConfirmation_PreventsDuplicate tests that 'd' doesn't overwrite
func TestHandleMainViewKeys_DeleteConfirmation_PreventsDuplicate(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.SetWizardDependencies(nil, &config.Mutator{}, "/path/to/config")

	fwd1 := &config.Forward{Resource: "pod/app1", Port: 8080, LocalPort: 8080, Alias: "app1"}
	fwd2 := &config.Forward{Resource: "pod/app2", Port: 8081, LocalPort: 8081, Alias: "app2"}
	ui.AddForward("id-1", fwd1)
	ui.AddForward("id-2", fwd2)

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press 'd' for first forward
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")}
	m.handleMainViewKeys(keyMsg)

	// Change selection
	m.ui.mu.Lock()
	m.ui.selectedIndex = 1
	m.ui.mu.Unlock()

	// Press 'd' again - should not change confirmation
	m.handleMainViewKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, "id-1", m.ui.deleteConfirmID, "Delete confirmation should not be overwritten")
	m.ui.mu.RUnlock()
}

// TestHandleDeleteConfirmation_Cancel tests Esc cancels delete
func TestHandleDeleteConfirmation_Cancel(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Set up delete confirmation
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "test-id"
	ui.deleteConfirmAlias = "test-alias"
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press Esc
	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleDeleteConfirmation(keyMsg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.deleteConfirming)
	m.ui.mu.RUnlock()
}

// TestHandleDeleteConfirmation_NavigateAndConfirm tests cursor navigation in delete dialog
func TestHandleDeleteConfirmation_NavigateAndConfirm(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	// Note: We use SetWizardDependencies with a real (nil) mutator since
	// the navigation test doesn't actually call mutator methods
	ui.SetWizardDependencies(nil, nil, "/path/to/config")
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "test-id"
	ui.deleteConfirmCursor = 1 // Start on "No"
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Navigate left to "Yes"
	keyMsg := tea.KeyMsg{Type: tea.KeyLeft}
	m.handleDeleteConfirmation(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, 0, m.ui.deleteConfirmCursor)
	m.ui.mu.RUnlock()

	// Navigate right back to "No"
	keyMsg = tea.KeyMsg{Type: tea.KeyRight}
	m.handleDeleteConfirmation(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, 1, m.ui.deleteConfirmCursor)
	m.ui.mu.RUnlock()
}

// TestHandleDeleteConfirmation_ConfirmYes tests confirming deletion
func TestHandleDeleteConfirmation_ConfirmYes(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	// Note: The mutator needs to be set for the command to be generated,
	// but we don't call the actual mutator method in this test (just generate the cmd)
	ui.SetWizardDependencies(nil, &config.Mutator{}, "/path/to/config")
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "test-id"
	ui.deleteConfirmCursor = 0 // On "Yes"
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press Enter on "Yes"
	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleDeleteConfirmation(keyMsg)

	// Should return a command to remove the forward
	assert.NotNil(t, cmd)

	// Dialog should be closed
	m.ui.mu.RLock()
	assert.False(t, m.ui.deleteConfirming)
	m.ui.mu.RUnlock()
}

// TestHandleDeleteConfirmation_QuickYKey tests 'y' key for quick confirm
func TestHandleDeleteConfirmation_QuickYKey(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	// Set up with a real mutator (empty but valid) since we're testing command generation
	ui.SetWizardDependencies(nil, &config.Mutator{}, "/path/to/config")
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "test-id"
	ui.deleteConfirmCursor = 1 // On "No"
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press 'y' - should confirm regardless of cursor position
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
	_, cmd := m.handleDeleteConfirmation(keyMsg)

	assert.NotNil(t, cmd)

	m.ui.mu.RLock()
	assert.False(t, m.ui.deleteConfirming)
	m.ui.mu.RUnlock()
}

// TestHandleDeleteConfirmation_QuickNKey tests 'n' key for quick cancel
func TestHandleDeleteConfirmation_QuickNKey(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "test-id"
	ui.deleteConfirmCursor = 0 // On "Yes"
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press 'n' - should cancel regardless of cursor position
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	m.handleDeleteConfirmation(keyMsg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.deleteConfirming)
	m.ui.mu.RUnlock()
}

// TestHandleBenchmarkKeys_Cancel tests benchmark cancellation
func TestHandleBenchmarkKeys_Cancel(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	cancelled := false
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("fwd-id", "alias", 8080)
	ui.benchmarkState.cancelFunc = func() { cancelled = true }
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press Esc
	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleBenchmarkKeys(keyMsg)

	assert.True(t, cancelled, "Cancel function should be called")

	m.ui.mu.RLock()
	assert.Nil(t, m.ui.benchmarkState)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

// TestHandleBenchmarkKeys_Navigation tests benchmark config navigation
func TestHandleBenchmarkKeys_Navigation(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("fwd-id", "alias", 8080)
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Initial cursor is 0
	m.ui.mu.RLock()
	assert.Equal(t, 0, m.ui.benchmarkState.cursor)
	m.ui.mu.RUnlock()

	// Move down
	keyMsg := tea.KeyMsg{Type: tea.KeyDown}
	m.handleBenchmarkKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, 1, m.ui.benchmarkState.cursor)
	m.ui.mu.RUnlock()

	// Move down again
	m.handleBenchmarkKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, 2, m.ui.benchmarkState.cursor)
	m.ui.mu.RUnlock()

	// Move up
	keyMsg = tea.KeyMsg{Type: tea.KeyUp}
	m.handleBenchmarkKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, 1, m.ui.benchmarkState.cursor)
	m.ui.mu.RUnlock()
}

// TestHandleHTTPLogKeys_Close tests HTTP log view closing
func TestHandleHTTPLogKeys_Close(t *testing.T) {
	mockSubscriber := NewMockHTTPLogSubscriber()

	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "alias")
	ui.httpLogCleanup = mockSubscriber.Subscribe("fwd-id", func(entry HTTPLogEntry) {})
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press Esc
	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleHTTPLogKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Nil(t, m.ui.httpLogState)
	assert.Nil(t, m.ui.httpLogCleanup)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()

	// Verify cleanup was called
	assert.Equal(t, 1, mockSubscriber.CleanupCalls)
}

// TestHandleHTTPLogKeys_FilterCycle tests filter mode cycling
func TestHandleHTTPLogKeys_FilterCycle(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "alias")
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Initial mode is None
	m.ui.mu.RLock()
	assert.Equal(t, HTTPLogFilterNone, m.ui.httpLogState.filterMode)
	m.ui.mu.RUnlock()

	// Press 'f' to cycle - should skip Text mode and go to Non200
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")}
	m.handleHTTPLogKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, HTTPLogFilterNon200, m.ui.httpLogState.filterMode)
	m.ui.mu.RUnlock()

	// Press 'f' again - should go to Errors
	m.handleHTTPLogKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, HTTPLogFilterErrors, m.ui.httpLogState.filterMode)
	m.ui.mu.RUnlock()

	// Press 'f' again - should go back to None
	m.handleHTTPLogKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, HTTPLogFilterNone, m.ui.httpLogState.filterMode)
	m.ui.mu.RUnlock()
}

// TestHandleHTTPLogKeys_TextFilter tests '/' for text filter
func TestHandleHTTPLogKeys_TextFilter(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "alias")
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press '/'
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}
	m.handleHTTPLogKeys(keyMsg)

	m.ui.mu.RLock()
	assert.True(t, m.ui.httpLogState.filterActive)
	m.ui.mu.RUnlock()
}

// TestHandleHTTPLogKeys_ClearFilters tests 'c' to clear filters
func TestHandleHTTPLogKeys_ClearFilters(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "alias")
	ui.httpLogState.filterMode = HTTPLogFilterErrors
	ui.httpLogState.filterText = "api"
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Press 'c'
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")}
	m.handleHTTPLogKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, HTTPLogFilterNone, m.ui.httpLogState.filterMode)
	assert.Empty(t, m.ui.httpLogState.filterText)
	m.ui.mu.RUnlock()
}

// TestHandleHTTPLogEntry tests HTTP log entry handling
func TestHandleHTTPLogEntry(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "alias")
	ui.httpLogState.autoScroll = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Send an entry
	msg := HTTPLogEntryMsg{
		Entry: HTTPLogEntry{
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: 200,
		},
	}
	m.handleHTTPLogEntry(msg)

	m.ui.mu.RLock()
	assert.Len(t, m.ui.httpLogState.entries, 1)
	assert.Equal(t, "/api/test", m.ui.httpLogState.entries[0].Path)
	m.ui.mu.RUnlock()
}

// TestHandleContextsLoaded tests context loading handler
func TestHandleContextsLoaded(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	// Note: discovery is nil but the handler doesn't use it directly,
	// it uses the message data instead. The current context reordering
	// uses GetCurrentContext() which would fail with nil discovery,
	// but we test the basic loading behavior here.
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Simulate contexts loaded
	msg := ContextsLoadedMsg{
		contexts: []string{"default", "production", "staging"},
	}
	m.handleContextsLoaded(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.addWizard.loading)
	// Contexts should be loaded (order depends on GetCurrentContext which may fail with nil discovery)
	assert.Contains(t, m.ui.addWizard.contexts, "default")
	assert.Contains(t, m.ui.addWizard.contexts, "production")
	assert.Contains(t, m.ui.addWizard.contexts, "staging")
	m.ui.mu.RUnlock()
}

// TestHandleContextsLoaded_Error tests error handling
func TestHandleContextsLoaded_Error(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Simulate error
	expectedErr := errors.New("failed to list contexts")
	msg := ContextsLoadedMsg{
		err: expectedErr,
	}
	m.handleContextsLoaded(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.addWizard.loading)
	assert.Equal(t, expectedErr, m.ui.addWizard.error)
	m.ui.mu.RUnlock()
}

// TestHandleNamespacesLoaded tests namespace loading handler
func TestHandleNamespacesLoaded(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := NamespacesLoadedMsg{
		namespaces: []string{"default", "kube-system", "production"},
	}
	m.handleNamespacesLoaded(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.addWizard.loading)
	assert.Equal(t, []string{"default", "kube-system", "production"}, m.ui.addWizard.namespaces)
	m.ui.mu.RUnlock()
}

// TestHandlePodsLoaded tests pod loading handler
func TestHandlePodsLoaded(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	pods := []k8s.PodInfo{
		{Name: "app-1", Namespace: "default"},
		{Name: "app-2", Namespace: "default"},
	}
	msg := PodsLoadedMsg{pods: pods}
	m.handlePodsLoaded(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.addWizard.loading)
	assert.Len(t, m.ui.addWizard.pods, 2)
	m.ui.mu.RUnlock()
}

// TestHandleServicesLoaded tests service loading handler
func TestHandleServicesLoaded(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	services := []k8s.ServiceInfo{
		{Name: "api", Namespace: "default", Ports: []k8s.PortInfo{{Port: 80}}},
		{Name: "db", Namespace: "default", Ports: []k8s.PortInfo{{Port: 5432}}},
	}
	msg := ServicesLoadedMsg{services: services}
	m.handleServicesLoaded(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.addWizard.loading)
	assert.Len(t, m.ui.addWizard.services, 2)
	m.ui.mu.RUnlock()
}

// TestHandleSelectorValidated tests selector validation handler
func TestHandleSelectorValidated(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	pods := []k8s.PodInfo{
		{Name: "app-1", Namespace: "default"},
	}
	msg := SelectorValidatedMsg{
		valid: true,
		pods:  pods,
	}
	m.handleSelectorValidated(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.addWizard.loading)
	assert.Len(t, m.ui.addWizard.matchingPods, 1)
	m.ui.mu.RUnlock()
}

// TestHandlePortChecked tests port availability check handler
func TestHandlePortChecked(t *testing.T) {
	tests := []struct {
		name        string
		available   bool
		expectStep  AddWizardStep
		expectError bool
	}{
		{"port available", true, StepConfirmation, false},
		{"port in use", false, StepEnterLocalPort, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui := NewBubbleTeaUI(nil, "1.0.0")
			ui.mu.Lock()
			ui.viewMode = ViewModeAddWizard
			ui.addWizard = newAddWizardState()
			ui.addWizard.step = StepEnterLocalPort
			ui.addWizard.loading = true
			ui.addWizard.localPort = 8080
			ui.mu.Unlock()

			m := model{ui: ui, termWidth: 120, termHeight: 40}

			msg := PortCheckedMsg{
				port:      8080,
				available: tt.available,
				message:   "test message",
			}
			m.handlePortChecked(msg)

			m.ui.mu.RLock()
			assert.False(t, m.ui.addWizard.loading)
			assert.Equal(t, tt.available, m.ui.addWizard.portAvailable)
			if tt.expectError {
				assert.NotNil(t, m.ui.addWizard.error)
			} else {
				assert.Equal(t, tt.expectStep, m.ui.addWizard.step)
			}
			m.ui.mu.RUnlock()
		})
	}
}

// TestHandleForwardSaved tests forward save handler
func TestHandleForwardSaved(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.step = StepConfirmation
	ui.addWizard.loading = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := ForwardSavedMsg{success: true}
	m.handleForwardSaved(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.addWizard.loading)
	assert.Equal(t, StepSuccess, m.ui.addWizard.step)
	m.ui.mu.RUnlock()
}

// TestHandleForwardsRemoved tests forward removal handler
func TestHandleForwardsRemoved(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{}
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := ForwardsRemovedMsg{success: true, count: 2}
	m.handleForwardsRemoved(msg)

	m.ui.mu.RLock()
	assert.Nil(t, m.ui.removeWizard)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

// TestHandleBenchmarkProgress tests benchmark progress handler
func TestHandleBenchmarkProgress(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("fwd-id", "alias", 8080)
	ui.benchmarkState.running = true
	ui.benchmarkState.progressCh = make(chan BenchmarkProgressMsg, 1)
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := BenchmarkProgressMsg{
		ForwardID: "fwd-id",
		Completed: 50,
		Total:     100,
	}
	m.handleBenchmarkProgress(msg)

	m.ui.mu.RLock()
	assert.Equal(t, 50, m.ui.benchmarkState.progress)
	assert.Equal(t, 100, m.ui.benchmarkState.total)
	m.ui.mu.RUnlock()
}

// TestHandleBenchmarkComplete tests benchmark completion handler
func TestHandleBenchmarkComplete(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("fwd-id", "alias", 8080)
	ui.benchmarkState.running = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Note: This test documents expected behavior
	// The actual BenchmarkCompleteMsg requires benchmark.Results which has CalculateStats
	msg := BenchmarkCompleteMsg{
		ForwardID: "fwd-id",
		Error:     errors.New("test error"),
	}
	m.handleBenchmarkComplete(msg)

	m.ui.mu.RLock()
	assert.False(t, m.ui.benchmarkState.running)
	assert.Equal(t, BenchmarkStepResults, m.ui.benchmarkState.step)
	assert.NotNil(t, m.ui.benchmarkState.error)
	m.ui.mu.RUnlock()
}

// TestModel_Update_MessageRouting tests message routing in Update
func TestModel_Update_MessageRouting(t *testing.T) {
	m := newTestModelWithForward()

	// Test window size message
	sizeMsg := tea.WindowSizeMsg{Width: 100, Height: 50}
	newModel, _ := m.Update(sizeMsg)
	updatedModel := newModel.(model)
	assert.Equal(t, 100, updatedModel.termWidth)
	assert.Equal(t, 50, updatedModel.termHeight)
}

// TestModel_Update_ViewModeRouting tests that key messages are routed based on view mode
func TestModel_Update_ViewModeRouting(t *testing.T) {
	tests := []struct {
		name     string
		viewMode ViewMode
	}{
		{"main view", ViewModeMain},
		{"add wizard", ViewModeAddWizard},
		{"benchmark", ViewModeBenchmark},
		{"http log", ViewModeHTTPLog},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui := NewBubbleTeaUI(nil, "1.0.0")
			ui.mu.Lock()
			ui.viewMode = tt.viewMode
			if tt.viewMode == ViewModeAddWizard {
				ui.addWizard = newAddWizardState()
			} else if tt.viewMode == ViewModeBenchmark {
				ui.benchmarkState = newBenchmarkState("id", "alias", 8080)
			} else if tt.viewMode == ViewModeHTTPLog {
				ui.httpLogState = newHTTPLogState("id", "alias")
			}
			ui.mu.Unlock()

			m := model{ui: ui, termWidth: 120, termHeight: 40}

			// Send a key message - should not panic
			keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
			_, _ = m.Update(keyMsg)
		})
	}
}

// TestWizardCompleteMsg tests wizard completion message handling
func TestWizardCompleteMsg(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := WizardCompleteMsg{}
	newModel, _ := m.Update(msg)
	updatedModel := newModel.(model)

	updatedModel.ui.mu.RLock()
	assert.Equal(t, ViewModeMain, updatedModel.ui.viewMode)
	assert.Nil(t, updatedModel.ui.addWizard)
	updatedModel.ui.mu.RUnlock()
}

// Helper to check that model implements tea.Model
func TestModel_ImplementsTeaModel(t *testing.T) {
	m := newTestModel()
	var _ tea.Model = m
	require.NotNil(t, m)
}
