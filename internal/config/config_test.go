package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig_ValidYAML(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	validYAML := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/my-app
            protocol: tcp
            port: 8080
            localPort: 8080
      - name: staging
        forwards:
          - resource: service/postgres
            protocol: tcp
            port: 5432
            localPort: 5433
  - name: prod-cluster
    namespaces:
      - name: production
        forwards:
          - resource: pod
            selector: app=nginx,env=prod
            protocol: tcp
            port: 80
            localPort: 8081
`

	err := os.WriteFile(configPath, []byte(validYAML), 0644)
	assert.NoError(t, err, "should write temp config file")

	// Load the config
	cfg, err := LoadConfig(configPath)
	assert.NoError(t, err, "LoadConfig should succeed")
	assert.NotNil(t, cfg, "config should not be nil")

	// Verify structure
	assert.Len(t, cfg.Contexts, 2, "should have 2 contexts")

	// Verify first context
	assert.Equal(t, "dev-cluster", cfg.Contexts[0].Name)
	assert.Len(t, cfg.Contexts[0].Namespaces, 2, "dev-cluster should have 2 namespaces")

	// Verify first namespace in first context
	assert.Equal(t, "default", cfg.Contexts[0].Namespaces[0].Name)
	assert.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 1)

	// Verify forward details
	fwd := cfg.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, "pod/my-app", fwd.Resource)
	assert.Equal(t, "tcp", fwd.Protocol)
	assert.Equal(t, 8080, fwd.Port)
	assert.Equal(t, 8080, fwd.LocalPort)
	assert.Equal(t, "", fwd.Selector)

	// Verify runtime fields are populated
	assert.Equal(t, "dev-cluster", fwd.GetContext())
	assert.Equal(t, "default", fwd.GetNamespace())
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	invalidYAML := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards: [this is invalid yaml syntax
`

	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	assert.NoError(t, err, "should write temp config file")

	// Load the config
	cfg, err := LoadConfig(configPath)
	assert.Error(t, err, "LoadConfig should fail with invalid YAML")
	assert.Nil(t, cfg, "config should be nil on error")
	assert.Contains(t, err.Error(), "failed to parse YAML", "error should mention YAML parsing")
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	// Try to load a non-existent file
	cfg, err := LoadConfig("/non/existent/path/.kportal.yaml")
	assert.Error(t, err, "LoadConfig should fail with non-existent file")
	assert.Nil(t, cfg, "config should be nil on error")
	assert.Contains(t, err.Error(), "failed to stat config file", "error should mention stat failure")
}

func TestForward_ID(t *testing.T) {
	tests := []struct {
		name       string
		forward    Forward
		expectedID string
	}{
		{
			name: "pod with explicit name",
			forward: Forward{
				Resource:      "pod/my-app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev-cluster",
				namespaceName: "default",
			},
			expectedID: "dev-cluster/default/pod/my-app:8080",
		},
		{
			name: "service resource",
			forward: Forward{
				Resource:      "service/postgres",
				Port:          5432,
				LocalPort:     5433,
				contextName:   "prod-cluster",
				namespaceName: "database",
			},
			expectedID: "prod-cluster/database/service/postgres:5433",
		},
		{
			name: "pod with selector",
			forward: Forward{
				Resource:      "pod",
				Selector:      "app=nginx",
				Port:          80,
				LocalPort:     8081,
				contextName:   "staging",
				namespaceName: "web",
			},
			expectedID: "staging/web/pod:8081",
		},
		{
			name: "forward with alias",
			forward: Forward{
				Resource:      "service/postgres",
				Port:          5432,
				LocalPort:     5432,
				Alias:         "shared-postgres",
				contextName:   "home",
				namespaceName: "shared-resources",
			},
			expectedID: "shared-postgres:5432",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := tt.forward.ID()
			assert.Equal(t, tt.expectedID, id, "ID() should return correct format")
		})
	}
}

func TestForward_String(t *testing.T) {
	tests := []struct {
		name           string
		forward        Forward
		expectedString string
	}{
		{
			name: "pod without selector",
			forward: Forward{
				Resource:      "pod/my-app",
				Port:          8080,
				LocalPort:     8080,
				contextName:   "dev-cluster",
				namespaceName: "default",
			},
			expectedString: "dev-cluster/default/pod/my-app:8080→8080",
		},
		{
			name: "service resource",
			forward: Forward{
				Resource:      "service/postgres",
				Port:          5432,
				LocalPort:     5433,
				contextName:   "prod-cluster",
				namespaceName: "database",
			},
			expectedString: "prod-cluster/database/service/postgres:5432→5433",
		},
		{
			name: "pod with selector",
			forward: Forward{
				Resource:      "pod",
				Selector:      "app=nginx,env=prod",
				Port:          80,
				LocalPort:     8081,
				contextName:   "staging",
				namespaceName: "web",
			},
			expectedString: "staging/web/pod[app=nginx,env=prod]:80→8081",
		},
		{
			name: "forward with alias",
			forward: Forward{
				Resource:      "service/redis",
				Port:          6379,
				LocalPort:     6379,
				Alias:         "redis-at-home",
				contextName:   "home",
				namespaceName: "shared-resources",
			},
			expectedString: "redis-at-home:6379→6379",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			str := tt.forward.String()
			assert.Equal(t, tt.expectedString, str, "String() should return correct format")
		})
	}
}

func TestParseConfig_ValidYAML(t *testing.T) {
	yamlData := []byte(`contexts:
  - name: test-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app
            protocol: tcp
            port: 8080
            localPort: 8080
`)

	cfg, err := ParseConfig(yamlData)
	assert.NoError(t, err, "ParseConfig should succeed")
	assert.NotNil(t, cfg, "config should not be nil")
	assert.Len(t, cfg.Contexts, 1)
	assert.Equal(t, "test-cluster", cfg.Contexts[0].Name)
}

func TestParseConfig_PopulatesRuntimeFields(t *testing.T) {
	yamlData := []byte(`contexts:
  - name: my-cluster
    namespaces:
      - name: my-namespace
        forwards:
          - resource: pod/my-pod
            port: 8080
            localPort: 8080
`)

	cfg, err := ParseConfig(yamlData)
	assert.NoError(t, err)
	assert.NotNil(t, cfg)

	// Check that runtime fields are populated
	fwd := cfg.Contexts[0].Namespaces[0].Forwards[0]
	assert.Equal(t, "my-cluster", fwd.GetContext())
	assert.Equal(t, "my-namespace", fwd.GetNamespace())
	assert.Equal(t, "my-cluster/my-namespace/pod/my-pod:8080", fwd.ID())
}

func TestConfig_GetAllForwards(t *testing.T) {
	yamlData := []byte(`contexts:
  - name: cluster1
    namespaces:
      - name: ns1
        forwards:
          - resource: pod/app1
            port: 8080
            localPort: 8080
          - resource: pod/app2
            port: 8081
            localPort: 8081
      - name: ns2
        forwards:
          - resource: service/db
            port: 5432
            localPort: 5432
  - name: cluster2
    namespaces:
      - name: ns3
        forwards:
          - resource: pod/app3
            port: 9090
            localPort: 9090
`)

	cfg, err := ParseConfig(yamlData)
	assert.NoError(t, err)

	forwards := cfg.GetAllForwards()
	assert.Len(t, forwards, 4, "should return all forwards from all contexts and namespaces")
}

func TestForward_SetContext(t *testing.T) {
	fwd := Forward{
		Resource:  "pod/my-app",
		Port:      8080,
		LocalPort: 8080,
	}

	assert.Equal(t, "", fwd.GetContext(), "initial context should be empty")
	assert.Equal(t, "", fwd.GetNamespace(), "initial namespace should be empty")

	fwd.SetContext("my-cluster", "my-namespace")

	assert.Equal(t, "my-cluster", fwd.GetContext())
	assert.Equal(t, "my-namespace", fwd.GetNamespace())
}
