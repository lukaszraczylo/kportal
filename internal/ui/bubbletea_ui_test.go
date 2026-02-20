package ui

import (
	"testing"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestNewBubbleTeaUI tests the constructor
func TestNewBubbleTeaUI(t *testing.T) {
	callback := func(id string, enable bool) {}

	ui := NewBubbleTeaUI(callback, "1.0.0")

	assert.NotNil(t, ui)
	assert.NotNil(t, ui.forwards)
	assert.NotNil(t, ui.forwardOrder)
	assert.NotNil(t, ui.disabledMap)
	assert.NotNil(t, ui.errors)
	assert.Equal(t, "1.0.0", ui.version)
	assert.Equal(t, ViewModeMain, ui.viewMode)
	assert.Equal(t, 0, ui.selectedIndex)
}

// TestBubbleTeaUI_AddForward tests adding forwards
func TestBubbleTeaUI_AddForward(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
		Alias:     "my-app",
	}

	ui.AddForward("test-id", fwd)

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.Len(t, ui.forwards, 1)
	assert.Len(t, ui.forwardOrder, 1)
	assert.Equal(t, "test-id", ui.forwardOrder[0])

	status := ui.forwards["test-id"]
	assert.Equal(t, "my-app", status.Alias)
	assert.Equal(t, "my-app", status.Resource)
	assert.Equal(t, "pod", status.Type)
	assert.Equal(t, 8080, status.LocalPort)
	assert.Equal(t, 8080, status.RemotePort)
	assert.Equal(t, "Starting", status.Status)
}

// TestBubbleTeaUI_AddForward_ServiceResource tests adding a service forward
func TestBubbleTeaUI_AddForward_ServiceResource(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "service/postgres",
		Port:      5432,
		LocalPort: 5432,
	}

	ui.AddForward("svc-id", fwd)

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	status := ui.forwards["svc-id"]
	assert.Equal(t, "postgres", status.Alias) // Uses resource name when no alias
	assert.Equal(t, "postgres", status.Resource)
	assert.Equal(t, "service", status.Type)
}

// TestBubbleTeaUI_AddForward_ReEnable tests re-enabling a disabled forward
func TestBubbleTeaUI_AddForward_ReEnable(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}

	// Add forward
	ui.AddForward("test-id", fwd)

	// Disable it
	ui.mu.Lock()
	ui.disabledMap["test-id"] = true
	ui.forwards["test-id"].Status = "Disabled"
	ui.mu.Unlock()

	// Re-add (re-enable)
	ui.AddForward("test-id", fwd)

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.False(t, ui.disabledMap["test-id"])
	assert.Equal(t, "Starting", ui.forwards["test-id"].Status)
	assert.Len(t, ui.forwardOrder, 1) // Should not duplicate
}

// TestBubbleTeaUI_UpdateStatus tests status updates
func TestBubbleTeaUI_UpdateStatus(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Update to Active
	ui.UpdateStatus("test-id", "Active")

	ui.mu.RLock()
	assert.Equal(t, "Active", ui.forwards["test-id"].Status)
	ui.mu.RUnlock()

	// Update to Error
	ui.UpdateStatus("test-id", "Error")

	ui.mu.RLock()
	assert.Equal(t, "Error", ui.forwards["test-id"].Status)
	ui.mu.RUnlock()
}

// TestBubbleTeaUI_UpdateStatus_ClearsErrorOnActive tests that errors are cleared when status becomes Active
func TestBubbleTeaUI_UpdateStatus_ClearsErrorOnActive(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Set an error
	ui.SetError("test-id", "connection refused")

	ui.mu.RLock()
	assert.Equal(t, "connection refused", ui.errors["test-id"])
	ui.mu.RUnlock()

	// Update to Active - should clear error
	ui.UpdateStatus("test-id", "Active")

	ui.mu.RLock()
	_, hasError := ui.errors["test-id"]
	ui.mu.RUnlock()

	assert.False(t, hasError, "Error should be cleared when status becomes Active")
}

// TestBubbleTeaUI_UpdateStatus_KeepsErrorOnReconnecting tests that errors persist during reconnection
func TestBubbleTeaUI_UpdateStatus_KeepsErrorOnReconnecting(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Set an error
	ui.SetError("test-id", "connection refused")

	// Update to Reconnecting - should keep error
	ui.UpdateStatus("test-id", "Reconnecting")

	ui.mu.RLock()
	assert.Equal(t, "connection refused", ui.errors["test-id"])
	ui.mu.RUnlock()
}

// TestBubbleTeaUI_SetError tests error setting
func TestBubbleTeaUI_SetError(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	ui.SetError("test-id", "connection timeout")

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.Equal(t, "connection timeout", ui.errors["test-id"])
}

// TestBubbleTeaUI_Remove tests forward removal
func TestBubbleTeaUI_Remove(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	ui.Remove("test-id")

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.Len(t, ui.forwards, 0)
	assert.Len(t, ui.forwardOrder, 0)
}

// TestBubbleTeaUI_Remove_ClearsErrors tests that removal clears associated errors
func TestBubbleTeaUI_Remove_ClearsErrors(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)
	ui.SetError("test-id", "some error")

	ui.Remove("test-id")

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	_, hasError := ui.errors["test-id"]
	assert.False(t, hasError, "Error should be cleared on removal")
}

// TestBubbleTeaUI_Remove_AdjustsSelectedIndex tests index adjustment after removal
func TestBubbleTeaUI_Remove_AdjustsSelectedIndex(t *testing.T) {
	tests := []struct {
		name              string
		removeID          string
		forwards          []string
		selectedIndex     int
		expectedIndex     int
		expectedRemaining int
	}{
		{
			name:              "remove selected item (last in list)",
			forwards:          []string{"a", "b", "c"},
			selectedIndex:     2,
			removeID:          "c",
			expectedIndex:     1, // Should move to previous item
			expectedRemaining: 2,
		},
		{
			name:              "remove item before selected",
			forwards:          []string{"a", "b", "c"},
			selectedIndex:     2,
			removeID:          "a",
			expectedIndex:     1, // Index shifts down but points to same item
			expectedRemaining: 2,
		},
		{
			name:              "remove item after selected",
			forwards:          []string{"a", "b", "c"},
			selectedIndex:     0,
			removeID:          "c",
			expectedIndex:     0, // No change needed
			expectedRemaining: 2,
		},
		{
			name:              "remove only item",
			forwards:          []string{"a"},
			selectedIndex:     0,
			removeID:          "a",
			expectedIndex:     0, // Stays at 0 (clamped)
			expectedRemaining: 0,
		},
		{
			name:              "remove middle item when selected is after",
			forwards:          []string{"a", "b", "c", "d"},
			selectedIndex:     3,
			removeID:          "b",
			expectedIndex:     2, // Adjusts down
			expectedRemaining: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui := NewBubbleTeaUI(nil, "1.0.0")

			// Add forwards
			for _, id := range tt.forwards {
				fwd := &config.Forward{
					Resource:  "pod/" + id,
					Port:      8080,
					LocalPort: 8080,
				}
				ui.AddForward(id, fwd)
			}

			// Set selected index
			ui.mu.Lock()
			ui.selectedIndex = tt.selectedIndex
			ui.mu.Unlock()

			// Remove
			ui.Remove(tt.removeID)

			ui.mu.RLock()
			defer ui.mu.RUnlock()

			assert.Equal(t, tt.expectedIndex, ui.selectedIndex)
			assert.Len(t, ui.forwardOrder, tt.expectedRemaining)
		})
	}
}

// TestBubbleTeaUI_Remove_ClearsDeleteConfirmation tests that pending delete confirmation is cleared
func TestBubbleTeaUI_Remove_ClearsDeleteConfirmation(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Set up delete confirmation
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "test-id"
	ui.deleteConfirmAlias = "my-app"
	ui.mu.Unlock()

	// Remove the forward
	ui.Remove("test-id")

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.False(t, ui.deleteConfirming, "Delete confirmation should be cleared")
	assert.Empty(t, ui.deleteConfirmID)
}

// TestBubbleTeaUI_Remove_KeepsOtherDeleteConfirmation tests that unrelated delete confirmation persists
func TestBubbleTeaUI_Remove_KeepsOtherDeleteConfirmation(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd1 := &config.Forward{Resource: "pod/app1", Port: 8080, LocalPort: 8080}
	fwd2 := &config.Forward{Resource: "pod/app2", Port: 8081, LocalPort: 8081}
	ui.AddForward("id-1", fwd1)
	ui.AddForward("id-2", fwd2)

	// Set up delete confirmation for id-2
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "id-2"
	ui.deleteConfirmAlias = "app2"
	ui.mu.Unlock()

	// Remove id-1 (different forward)
	ui.Remove("id-1")

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.True(t, ui.deleteConfirming, "Delete confirmation for other forward should persist")
	assert.Equal(t, "id-2", ui.deleteConfirmID)
}

// TestBubbleTeaUI_MoveSelection tests cursor movement
func TestBubbleTeaUI_MoveSelection(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Add some forwards
	for i := 0; i < 5; i++ {
		fwd := &config.Forward{
			Resource:  "pod/app",
			Port:      8080 + i,
			LocalPort: 8080 + i,
		}
		ui.AddForward(string(rune('a'+i)), fwd)
	}

	tests := []struct {
		name          string
		initialIndex  int
		delta         int
		expectedIndex int
	}{
		{"move down from 0", 0, 1, 1},
		{"move down from middle", 2, 1, 3},
		{"move up from middle", 2, -1, 1},
		{"cannot move below 0", 0, -1, 0},
		{"cannot move above max", 4, 1, 4},
		{"large delta clamped to max", 0, 100, 4},
		{"large negative delta clamped to 0", 4, -100, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui.mu.Lock()
			ui.selectedIndex = tt.initialIndex
			ui.mu.Unlock()

			ui.moveSelection(tt.delta)

			ui.mu.RLock()
			assert.Equal(t, tt.expectedIndex, ui.selectedIndex)
			ui.mu.RUnlock()
		})
	}
}

// TestBubbleTeaUI_MoveSelection_EmptyList tests movement with no forwards
func TestBubbleTeaUI_MoveSelection_EmptyList(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Should not panic with empty list
	ui.moveSelection(1)
	ui.moveSelection(-1)

	ui.mu.RLock()
	assert.Equal(t, 0, ui.selectedIndex)
	ui.mu.RUnlock()
}

// TestBubbleTeaUI_ToggleSelected tests toggling forward state
func TestBubbleTeaUI_ToggleSelected(t *testing.T) {
	callback := func(id string, enable bool) {
		// Callback is called in a goroutine
	}

	ui := NewBubbleTeaUI(callback, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}
	ui.AddForward("test-id", fwd)

	// Toggle to disabled
	ui.toggleSelected()

	// Wait for goroutine
	ui.mu.RLock()
	isDisabled := ui.disabledMap["test-id"]
	ui.mu.RUnlock()

	assert.True(t, isDisabled)

	// Toggle back to enabled
	ui.toggleSelected()

	ui.mu.RLock()
	isDisabled = ui.disabledMap["test-id"]
	ui.mu.RUnlock()

	assert.False(t, isDisabled)
}

// TestBubbleTeaUI_SetUpdateAvailable tests update notification
func TestBubbleTeaUI_SetUpdateAvailable(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	ui.SetUpdateAvailable("2.0.0", "https://example.com/update")

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.True(t, ui.updateAvailable)
	assert.Equal(t, "2.0.0", ui.updateVersion)
	assert.Equal(t, "https://example.com/update", ui.updateURL)
}

// TestBubbleTeaUI_SetWizardDependencies tests dependency injection
func TestBubbleTeaUI_SetWizardDependencies(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Initially nil
	ui.mu.RLock()
	assert.Nil(t, ui.discovery)
	assert.Nil(t, ui.mutator)
	assert.Empty(t, ui.configPath)
	ui.mu.RUnlock()

	// Set dependencies (using nil for simplicity - just testing the setter)
	ui.SetWizardDependencies(nil, nil, "/path/to/config.yaml")

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.Equal(t, "/path/to/config.yaml", ui.configPath)
}

// TestBubbleTeaUI_ResetDeleteConfirmation tests the reset helper
func TestBubbleTeaUI_ResetDeleteConfirmation(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	// Set up confirmation state
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "test-id"
	ui.deleteConfirmAlias = "test-alias"
	ui.deleteConfirmCursor = 1
	ui.mu.Unlock()

	// Reset
	ui.mu.Lock()
	ui.resetDeleteConfirmation()
	ui.mu.Unlock()

	ui.mu.RLock()
	defer ui.mu.RUnlock()

	assert.False(t, ui.deleteConfirming)
	assert.Empty(t, ui.deleteConfirmID)
	assert.Empty(t, ui.deleteConfirmAlias)
	assert.Equal(t, 0, ui.deleteConfirmCursor)
}

// TestBubbleTeaUI_IsForwardDisabled tests the disabled state helper
func TestBubbleTeaUI_IsForwardDisabled(t *testing.T) {
	tests := []struct {
		name           string
		forwardStatus  string
		disabledMap    bool
		expectedResult bool
	}{
		{
			name:           "not disabled in map, Active status",
			disabledMap:    false,
			forwardStatus:  "Active",
			expectedResult: false,
		},
		{
			name:           "disabled in map, Active status",
			disabledMap:    true,
			forwardStatus:  "Active",
			expectedResult: true,
		},
		{
			name:           "not disabled in map, Disabled status",
			disabledMap:    false,
			forwardStatus:  "Disabled",
			expectedResult: true,
		},
		{
			name:           "both disabled in map and Disabled status",
			disabledMap:    true,
			forwardStatus:  "Disabled",
			expectedResult: true,
		},
		{
			name:           "not disabled in map, Error status",
			disabledMap:    false,
			forwardStatus:  "Error",
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui := NewBubbleTeaUI(nil, "1.0.0")

			fwd := &config.Forward{
				Resource:  "pod/my-app",
				Port:      8080,
				LocalPort: 8080,
			}
			ui.AddForward("test-id", fwd)

			ui.mu.Lock()
			ui.disabledMap["test-id"] = tt.disabledMap
			ui.forwards["test-id"].Status = tt.forwardStatus
			ui.mu.Unlock()

			ui.mu.RLock()
			result := ui.isForwardDisabled("test-id")
			ui.mu.RUnlock()

			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// TestBubbleTeaUI_IsForwardDisabled_NonExistent tests disabled check for non-existent forward
func TestBubbleTeaUI_IsForwardDisabled_NonExistent(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	ui.mu.RLock()
	result := ui.isForwardDisabled("non-existent")
	ui.mu.RUnlock()

	assert.False(t, result, "Non-existent forward should not be disabled")
}

// TestBubbleTeaUI_AddForward_ReEnableClearsError tests that re-enabling clears previous errors
func TestBubbleTeaUI_AddForward_ReEnableClearsError(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")

	fwd := &config.Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}

	// Add forward
	ui.AddForward("test-id", fwd)

	// Set error and disable
	ui.SetError("test-id", "connection refused")
	ui.mu.Lock()
	ui.disabledMap["test-id"] = true
	ui.forwards["test-id"].Status = "Disabled"
	ui.mu.Unlock()

	// Verify error exists
	ui.mu.RLock()
	_, hasError := ui.errors["test-id"]
	ui.mu.RUnlock()
	assert.True(t, hasError, "Error should exist before re-enable")

	// Re-enable (re-add)
	ui.AddForward("test-id", fwd)

	// Verify error is cleared
	ui.mu.RLock()
	_, hasError = ui.errors["test-id"]
	ui.mu.RUnlock()
	assert.False(t, hasError, "Error should be cleared after re-enable")
}

// TestWrapText tests the text wrapping function
func TestWrapText(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected string
		width    int
	}{
		{
			name:     "short text fits",
			text:     "hello world",
			width:    20,
			expected: "hello world",
		},
		{
			name:     "single long word",
			text:     "superlongwordthatexceedswidth",
			width:    10,
			expected: "superlongwordthatexceedswidth",
		},
		{
			name:     "wraps at word boundary",
			text:     "hello world this is a test",
			width:    15,
			expected: "hello world\nthis is a test",
		},
		{
			name:     "multiple wraps",
			text:     "one two three four five six",
			width:    10,
			expected: "one two\nthree four\nfive six",
		},
		{
			name:     "empty string",
			text:     "",
			width:    10,
			expected: "",
		},
		{
			name:     "single word",
			text:     "hello",
			width:    10,
			expected: "hello",
		},
		{
			name:     "exact width",
			text:     "hello wor",
			width:    9,
			expected: "hello wor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapText(tt.text, tt.width)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBubbleTeaUI_AddForward_ResourceParsing tests various resource format parsing
func TestBubbleTeaUI_AddForward_ResourceParsing(t *testing.T) {
	tests := []struct {
		name         string
		resource     string
		expectedType string
		expectedName string
	}{
		{
			name:         "pod with prefix",
			resource:     "pod/my-app",
			expectedType: "pod",
			expectedName: "my-app",
		},
		{
			name:         "service resource",
			resource:     "service/postgres",
			expectedType: "service",
			expectedName: "postgres",
		},
		{
			name:         "deployment resource",
			resource:     "deployment/api-server",
			expectedType: "deployment",
			expectedName: "api-server",
		},
		{
			name:         "no type prefix (pod default)",
			resource:     "my-pod",
			expectedType: "pod",
			expectedName: "my-pod",
		},
		{
			name:         "resource with multiple slashes",
			resource:     "custom/type/resource",
			expectedType: "custom",
			expectedName: "type/resource",
		},
		{
			name:         "empty resource",
			resource:     "",
			expectedType: "pod",
			expectedName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ui := NewBubbleTeaUI(nil, "1.0.0")

			fwd := &config.Forward{
				Resource:  tt.resource,
				Port:      8080,
				LocalPort: 8080,
			}
			ui.AddForward("test-id", fwd)

			ui.mu.RLock()
			status := ui.forwards["test-id"]
			ui.mu.RUnlock()

			assert.Equal(t, tt.expectedType, status.Type)
			assert.Equal(t, tt.expectedName, status.Resource)
		})
	}
}

// TestConstants tests that UI constants are properly defined
func TestConstants(t *testing.T) {
	assert.Equal(t, 120, DefaultTermWidth)
	assert.Equal(t, 40, DefaultTermHeight)
	assert.Equal(t, 7, ColumnStatus)
	assert.Equal(t, 14, ColumnWidthContext)
	assert.Equal(t, 16, ColumnWidthNamespace)
	assert.Equal(t, 18, ColumnWidthAlias)
	assert.Equal(t, 8, ColumnWidthType)
	assert.Equal(t, 20, ColumnWidthResource)
	assert.Equal(t, 118, ErrorDisplayWidth)
	assert.Equal(t, 20, ViewportHeight)
}
