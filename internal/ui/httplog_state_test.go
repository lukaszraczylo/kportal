package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNewHTTPLogState tests the constructor
func TestNewHTTPLogState(t *testing.T) {
	state := newHTTPLogState("forward-123", "my-service")

	assert.Equal(t, "forward-123", state.forwardID)
	assert.Equal(t, "my-service", state.forwardAlias)
	assert.NotNil(t, state.entries)
	assert.Empty(t, state.entries)
	assert.True(t, state.autoScroll)
	assert.Equal(t, HTTPLogFilterNone, state.filterMode)
	assert.Empty(t, state.filterText)
	assert.False(t, state.filterActive)
}

// TestHTTPLogState_GetFilteredEntries_NoFilter tests filtering with no filter
func TestHTTPLogState_GetFilteredEntries_NoFilter(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "POST", Path: "/api/orders", StatusCode: 201},
		{Method: "GET", Path: "/health", StatusCode: 200},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 3)
}

// TestHTTPLogState_GetFilteredEntries_FiltersZeroStatusCode tests that entries without status codes are filtered
func TestHTTPLogState_GetFilteredEntries_FiltersZeroStatusCode(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "GET", Path: "/streaming", StatusCode: 0}, // No status (in-progress or error)
		{Method: "POST", Path: "/api/orders", StatusCode: 201},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 2)
	assert.Equal(t, "/api/users", filtered[0].Path)
	assert.Equal(t, "/api/orders", filtered[1].Path)
}

// TestHTTPLogState_GetFilteredEntries_Non200Filter tests non-2xx filter
func TestHTTPLogState_GetFilteredEntries_Non200Filter(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterMode = HTTPLogFilterNon200
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "GET", Path: "/api/error", StatusCode: 500},
		{Method: "POST", Path: "/api/orders", StatusCode: 201},
		{Method: "GET", Path: "/api/notfound", StatusCode: 404},
		{Method: "PUT", Path: "/api/redirect", StatusCode: 301},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 3)
	assert.Equal(t, 500, filtered[0].StatusCode)
	assert.Equal(t, 404, filtered[1].StatusCode)
	assert.Equal(t, 301, filtered[2].StatusCode)
}

// TestHTTPLogState_GetFilteredEntries_ErrorsFilter tests 4xx/5xx filter
func TestHTTPLogState_GetFilteredEntries_ErrorsFilter(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterMode = HTTPLogFilterErrors
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "GET", Path: "/api/error", StatusCode: 500},
		{Method: "POST", Path: "/api/orders", StatusCode: 201},
		{Method: "GET", Path: "/api/notfound", StatusCode: 404},
		{Method: "PUT", Path: "/api/redirect", StatusCode: 301},
		{Method: "GET", Path: "/api/bad", StatusCode: 400},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 3)
	assert.Equal(t, 500, filtered[0].StatusCode)
	assert.Equal(t, 404, filtered[1].StatusCode)
	assert.Equal(t, 400, filtered[2].StatusCode)
}

// TestHTTPLogState_GetFilteredEntries_TextFilter tests text filtering
func TestHTTPLogState_GetFilteredEntries_TextFilter(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterText = "users"
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "GET", Path: "/api/users/123", StatusCode: 200},
		{Method: "POST", Path: "/api/orders", StatusCode: 201},
		{Method: "GET", Path: "/health", StatusCode: 200},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 2)
	assert.Equal(t, "/api/users", filtered[0].Path)
	assert.Equal(t, "/api/users/123", filtered[1].Path)
}

// TestHTTPLogState_GetFilteredEntries_TextFilterCaseInsensitive tests case-insensitive text filtering
func TestHTTPLogState_GetFilteredEntries_TextFilterCaseInsensitive(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterText = "API"
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "GET", Path: "/Api/Orders", StatusCode: 200},
		{Method: "GET", Path: "/health", StatusCode: 200},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 2)
}

// TestHTTPLogState_GetFilteredEntries_TextFilterByMethod tests filtering by HTTP method
func TestHTTPLogState_GetFilteredEntries_TextFilterByMethod(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterText = "POST"
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "POST", Path: "/api/orders", StatusCode: 201},
		{Method: "POST", Path: "/api/items", StatusCode: 201},
		{Method: "PUT", Path: "/api/update", StatusCode: 200},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 2)
	assert.Equal(t, "POST", filtered[0].Method)
	assert.Equal(t, "POST", filtered[1].Method)
}

// TestHTTPLogState_GetFilteredEntries_CombinedFilters tests combining mode and text filters
func TestHTTPLogState_GetFilteredEntries_CombinedFilters(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterMode = HTTPLogFilterErrors
	state.filterText = "api"
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "GET", Path: "/api/error", StatusCode: 500},
		{Method: "GET", Path: "/health", StatusCode: 500}, // Error but doesn't match text
		{Method: "GET", Path: "/api/notfound", StatusCode: 404},
	}

	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 2)
	assert.Equal(t, "/api/error", filtered[0].Path)
	assert.Equal(t, "/api/notfound", filtered[1].Path)
}

// TestHTTPLogState_GetFilteredEntries_EmptyResult tests when no entries match
func TestHTTPLogState_GetFilteredEntries_EmptyResult(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")
	state.filterText = "nonexistent"
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/users", StatusCode: 200},
		{Method: "POST", Path: "/api/orders", StatusCode: 201},
	}

	filtered := state.getFilteredEntries()

	assert.Empty(t, filtered)
}

// TestHTTPLogState_GetFilterModeLabel tests filter mode labels
func TestHTTPLogState_GetFilterModeLabel(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")

	tests := []struct {
		expected string
		mode     HTTPLogFilterMode
	}{
		{mode: HTTPLogFilterNone, expected: "All"},
		{mode: HTTPLogFilterText, expected: "Text"},
		{mode: HTTPLogFilterNon200, expected: "Non-2xx"},
		{mode: HTTPLogFilterErrors, expected: "Errors (4xx/5xx)"},
	}

	for _, tt := range tests {
		state.filterMode = tt.mode
		assert.Equal(t, tt.expected, state.getFilterModeLabel())
	}
}

// TestHTTPLogState_FilterModeValues tests filter mode constants are correct
func TestHTTPLogState_FilterModeValues(t *testing.T) {
	// Ensure the modes are sequential for cycling to work correctly
	assert.Equal(t, HTTPLogFilterMode(0), HTTPLogFilterNone)
	assert.Equal(t, HTTPLogFilterMode(1), HTTPLogFilterText)
	assert.Equal(t, HTTPLogFilterMode(2), HTTPLogFilterNon200)
	assert.Equal(t, HTTPLogFilterMode(3), HTTPLogFilterErrors)
}

// TestHTTPLogState_LargeEntrySet tests filtering performance with many entries
func TestHTTPLogState_LargeEntrySet(t *testing.T) {
	state := newHTTPLogState("fwd", "alias")

	// Add 1000 entries
	for i := 0; i < 1000; i++ {
		code := 200
		if i%10 == 0 {
			code = 500
		}
		state.entries = append(state.entries, HTTPLogEntry{
			Method:     "GET",
			Path:       "/api/test",
			StatusCode: code,
		})
	}

	// Filter should work correctly
	state.filterMode = HTTPLogFilterErrors
	filtered := state.getFilteredEntries()

	assert.Len(t, filtered, 100) // 10% are errors
}
