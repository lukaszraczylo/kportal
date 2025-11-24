package ui

import (
	"testing"

	"github.com/nvm/kportal/internal/k8s"
	"github.com/stretchr/testify/assert"
)

func TestFilterStrings(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		filter   string
		expected []string
	}{
		{
			name:     "empty filter returns all items",
			items:    []string{"namespace-1", "namespace-2", "namespace-3"},
			filter:   "",
			expected: []string{"namespace-1", "namespace-2", "namespace-3"},
		},
		{
			name:     "filter matches multiple items",
			items:    []string{"prod-api", "prod-db", "staging-api", "dev-api"},
			filter:   "prod",
			expected: []string{"prod-api", "prod-db"},
		},
		{
			name:     "filter matches single item",
			items:    []string{"namespace-1", "namespace-2", "namespace-3"},
			filter:   "2",
			expected: []string{"namespace-2"},
		},
		{
			name:     "filter matches no items",
			items:    []string{"namespace-1", "namespace-2", "namespace-3"},
			filter:   "xyz",
			expected: []string{},
		},
		{
			name:     "case insensitive matching",
			items:    []string{"Production", "Staging", "Development"},
			filter:   "prod",
			expected: []string{"Production"},
		},
		{
			name:     "partial string matching",
			items:    []string{"my-app-frontend", "my-app-backend", "other-service"},
			filter:   "app",
			expected: []string{"my-app-frontend", "my-app-backend"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterStrings(tt.items, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesFilter(t *testing.T) {
	tests := []struct {
		name     string
		item     string
		filter   string
		expected bool
	}{
		{
			name:     "empty filter matches everything",
			item:     "namespace-1",
			filter:   "",
			expected: true,
		},
		{
			name:     "exact match",
			item:     "namespace-1",
			filter:   "namespace-1",
			expected: true,
		},
		{
			name:     "partial match",
			item:     "production-api",
			filter:   "prod",
			expected: true,
		},
		{
			name:     "no match",
			item:     "namespace-1",
			filter:   "xyz",
			expected: false,
		},
		{
			name:     "case insensitive match",
			item:     "Production",
			filter:   "prod",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesFilter(tt.item, tt.filter)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFilteredContexts(t *testing.T) {
	wizard := &AddWizardState{
		contexts: []string{"prod-cluster", "staging-cluster", "dev-cluster", "test-cluster"},
	}

	tests := []struct {
		name     string
		filter   string
		expected []string
	}{
		{
			name:     "no filter returns all",
			filter:   "",
			expected: []string{"prod-cluster", "staging-cluster", "dev-cluster", "test-cluster"},
		},
		{
			name:     "filter by 'prod'",
			filter:   "prod",
			expected: []string{"prod-cluster"},
		},
		{
			name:     "filter by 'cluster'",
			filter:   "cluster",
			expected: []string{"prod-cluster", "staging-cluster", "dev-cluster", "test-cluster"},
		},
		{
			name:     "filter by 'staging'",
			filter:   "staging",
			expected: []string{"staging-cluster"},
		},
		{
			name:     "filter with no matches",
			filter:   "xyz",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wizard.searchFilter = tt.filter
			result := wizard.getFilteredContexts()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFilteredNamespaces(t *testing.T) {
	wizard := &AddWizardState{
		namespaces: []string{
			"kube-system", "kube-public", "default",
			"prod-api", "prod-db", "staging-api", "staging-db",
			"monitoring", "logging",
		},
	}

	tests := []struct {
		name     string
		filter   string
		expected []string
	}{
		{
			name:   "no filter returns all",
			filter: "",
			expected: []string{
				"kube-system", "kube-public", "default",
				"prod-api", "prod-db", "staging-api", "staging-db",
				"monitoring", "logging",
			},
		},
		{
			name:     "filter by 'prod'",
			filter:   "prod",
			expected: []string{"prod-api", "prod-db"},
		},
		{
			name:     "filter by 'kube'",
			filter:   "kube",
			expected: []string{"kube-system", "kube-public"},
		},
		{
			name:     "filter by 'api'",
			filter:   "api",
			expected: []string{"prod-api", "staging-api"},
		},
		{
			name:     "filter by 'ing' (partial match)",
			filter:   "ing",
			expected: []string{"staging-api", "staging-db", "monitoring", "logging"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wizard.searchFilter = tt.filter
			result := wizard.getFilteredNamespaces()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetFilteredServices(t *testing.T) {
	wizard := &AddWizardState{
		services: []k8s.ServiceInfo{
			{Name: "api-gateway"},
			{Name: "api-backend"},
			{Name: "database"},
			{Name: "redis-cache"},
			{Name: "postgres-db"},
		},
	}

	tests := []struct {
		name     string
		filter   string
		expected []string
	}{
		{
			name:     "no filter returns all",
			filter:   "",
			expected: []string{"api-gateway", "api-backend", "database", "redis-cache", "postgres-db"},
		},
		{
			name:     "filter by 'api'",
			filter:   "api",
			expected: []string{"api-gateway", "api-backend"},
		},
		{
			name:     "filter by 'db'",
			filter:   "db",
			expected: []string{"postgres-db"},
		},
		{
			name:     "filter by 'base'",
			filter:   "base",
			expected: []string{"database"},
		},
		{
			name:     "filter by 'redis'",
			filter:   "redis",
			expected: []string{"redis-cache"},
		},
		{
			name:     "filter with no matches",
			filter:   "xyz",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wizard.searchFilter = tt.filter
			result := wizard.getFilteredServices()
			resultNames := make([]string, len(result))
			for i, svc := range result {
				resultNames[i] = svc.Name
			}
			assert.Equal(t, tt.expected, resultNames)
		})
	}
}

func TestClearSearchFilter(t *testing.T) {
	wizard := &AddWizardState{
		searchFilter: "test",
		cursor:       5,
		scrollOffset: 10,
	}

	wizard.clearSearchFilter()

	assert.Equal(t, "", wizard.searchFilter, "searchFilter should be cleared")
	assert.Equal(t, 0, wizard.cursor, "cursor should be reset to 0")
	assert.Equal(t, 0, wizard.scrollOffset, "scrollOffset should be reset to 0")
}

func TestMoveCursorWithFilteredLists(t *testing.T) {
	tests := []struct {
		name           string
		step           AddWizardStep
		contexts       []string
		namespaces     []string
		searchFilter   string
		initialCursor  int
		delta          int
		expectedCursor int
	}{
		{
			name:           "move down in filtered contexts",
			step:           StepSelectContext,
			contexts:       []string{"prod-1", "prod-2", "staging-1", "dev-1"},
			searchFilter:   "prod",
			initialCursor:  0,
			delta:          1,
			expectedCursor: 1,
		},
		{
			name:           "cannot move beyond filtered list",
			step:           StepSelectContext,
			contexts:       []string{"prod-1", "prod-2", "staging-1", "dev-1"},
			searchFilter:   "prod",
			initialCursor:  1,
			delta:          1,
			expectedCursor: 1, // Should stay at 1 (last item in filtered list)
		},
		{
			name:           "move up in filtered list",
			step:           StepSelectNamespace,
			namespaces:     []string{"ns-1", "ns-2", "ns-3", "other"},
			searchFilter:   "ns",
			initialCursor:  2,
			delta:          -1,
			expectedCursor: 1,
		},
		{
			name:           "cannot move above 0",
			step:           StepSelectNamespace,
			namespaces:     []string{"ns-1", "ns-2", "ns-3"},
			searchFilter:   "ns",
			initialCursor:  0,
			delta:          -1,
			expectedCursor: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wizard := &AddWizardState{
				step:         tt.step,
				inputMode:    InputModeList,
				cursor:       tt.initialCursor,
				contexts:     tt.contexts,
				namespaces:   tt.namespaces,
				searchFilter: tt.searchFilter,
			}

			wizard.moveCursor(tt.delta)

			assert.Equal(t, tt.expectedCursor, wizard.cursor)
		})
	}
}
