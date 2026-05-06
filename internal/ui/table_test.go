package ui

import (
	"testing"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewTableUI tests the constructor.
func TestNewTableUI(t *testing.T) {
	tui := NewTableUI(false)
	require.NotNil(t, tui)
	assert.NotNil(t, tui.forwards)
	assert.False(t, tui.verbose)

	tuiVerbose := NewTableUI(true)
	assert.True(t, tuiVerbose.verbose)
}

// TestTableUI_AddForward covers the happy path and resource-parsing branches.
func TestTableUI_AddForward(t *testing.T) {
	tests := []struct {
		name          string
		resource      string
		alias         string
		expectedType  string
		expectedName  string
		expectedAlias string
	}{
		{
			name:          "pod with prefix",
			resource:      "pod/my-app",
			alias:         "alias",
			expectedType:  "pod",
			expectedName:  "my-app",
			expectedAlias: "alias",
		},
		{
			name:          "service resource",
			resource:      "service/postgres",
			alias:         "",
			expectedType:  "service",
			expectedName:  "postgres",
			expectedAlias: "postgres", // Falls back to resource name
		},
		{
			name:          "no type prefix defaults to pod",
			resource:      "my-pod",
			alias:         "",
			expectedType:  "pod",
			expectedName:  "my-pod",
			expectedAlias: "my-pod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tui := NewTableUI(false)
			fwd := &config.Forward{
				Resource:  tt.resource,
				Port:      8080,
				LocalPort: 8080,
				Alias:     tt.alias,
			}
			tui.AddForward("id-1", fwd)

			tui.mu.RLock()
			defer tui.mu.RUnlock()

			require.Len(t, tui.forwards, 1)
			status := tui.forwards["id-1"]
			assert.Equal(t, tt.expectedType, status.Type)
			assert.Equal(t, tt.expectedName, status.Resource)
			assert.Equal(t, tt.expectedAlias, status.Alias)
			assert.Equal(t, "Starting", status.Status)
			assert.Equal(t, 8080, status.RemotePort)
			assert.Equal(t, 8080, status.LocalPort)
		})
	}
}

// TestTableUI_UpdateStatus verifies status mutation.
func TestTableUI_UpdateStatus(t *testing.T) {
	tui := NewTableUI(false)
	fwd := &config.Forward{Resource: "pod/app", Port: 80, LocalPort: 8080}
	tui.AddForward("id-1", fwd)

	tui.UpdateStatus("id-1", "Active")

	tui.mu.RLock()
	assert.Equal(t, "Active", tui.forwards["id-1"].Status)
	tui.mu.RUnlock()

	// Updating non-existent ID must not panic.
	tui.UpdateStatus("nonexistent", "Active")
}

// TestTableUI_GetForward covers the lookup path.
func TestTableUI_GetForward(t *testing.T) {
	tui := NewTableUI(false)
	fwd := &config.Forward{Resource: "pod/app", Port: 80, LocalPort: 8080}
	tui.AddForward("id-1", fwd)

	got := tui.GetForward("id-1")
	require.NotNil(t, got)
	assert.Equal(t, "app", got.Resource)

	missing := tui.GetForward("nonexistent")
	assert.Nil(t, missing)
}

// TestTableUI_Remove tests deletion.
func TestTableUI_Remove(t *testing.T) {
	tui := NewTableUI(false)
	fwd := &config.Forward{Resource: "pod/app", Port: 80, LocalPort: 8080}
	tui.AddForward("id-1", fwd)
	tui.AddForward("id-2", fwd)

	tui.Remove("id-1")

	tui.mu.RLock()
	defer tui.mu.RUnlock()
	assert.Len(t, tui.forwards, 1)
	assert.Nil(t, tui.forwards["id-1"])
	assert.NotNil(t, tui.forwards["id-2"])
}

// TestTruncate covers the truncation helper.
func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		maxLen   int
	}{
		{"hello", "hello", 10},
		{"hello world", "hello...", 8},
		{"hi", "hi", 2},
		{"hi!", "hi", 2},   // maxLen <= 3 branch: no ellipsis
		{"abcd", "abc", 3}, // maxLen <= 3 branch
		{"", "", 5},
	}

	for _, tt := range tests {
		t.Run(tt.input+"_"+string(rune('0'+tt.maxLen)), func(t *testing.T) {
			assert.Equal(t, tt.expected, truncate(tt.input, tt.maxLen))
		})
	}
}

// TestHyperlink verifies the OSC-8 escape sequence is produced.
func TestHyperlink(t *testing.T) {
	result := hyperlink("http://localhost:8080", "8080→")
	assert.Contains(t, result, "http://localhost:8080")
	assert.Contains(t, result, "8080→")
	// Must contain OSC-8 opener and closer
	assert.Contains(t, result, "\x1b]8;;")
	assert.Contains(t, result, "\x1b\\")
}

// TestFormatStatusWithIndicator covers all status branches.
func TestFormatStatusWithIndicator(t *testing.T) {
	statuses := []string{"Active", "Starting", "Reconnecting", "Error", "Failed", "Unknown"}
	for _, s := range statuses {
		t.Run(s, func(t *testing.T) {
			result := formatStatusWithIndicator(s)
			// Must contain the original status string.
			assert.Contains(t, result, s)
		})
	}
}
