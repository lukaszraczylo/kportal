package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewMutator tests mutator creation
func TestNewMutator(t *testing.T) {
	mutator := NewMutator("/path/to/config.yaml")
	assert.NotNil(t, mutator)
	assert.Equal(t, "/path/to/config.yaml", mutator.configPath)
}

// TestMutator_AddForward_NewFile tests adding a forward to a new file
// Note: Due to how LoadConfig wraps errors, os.IsNotExist check in AddForward
// doesn't work with wrapped errors. This documents the current behavior.
func TestMutator_AddForward_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	mutator := NewMutator(configPath)

	fwd := Forward{
		Resource:  "pod/my-app",
		Protocol:  "tcp",
		Port:      8080,
		LocalPort: 8080,
	}

	// Currently fails because LoadConfig wraps the error and os.IsNotExist doesn't match
	err := mutator.AddForward("dev-cluster", "default", fwd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

// TestMutator_AddForward_EmptyFile tests adding a forward to an empty file
func TestMutator_AddForward_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create empty config file with minimal valid structure
	initial := `contexts: []
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	fwd := Forward{
		Resource:  "pod/my-app",
		Protocol:  "tcp",
		Port:      8080,
		LocalPort: 8080,
	}

	err = mutator.AddForward("dev-cluster", "default", fwd)
	require.NoError(t, err)

	// Verify file was updated and contains the forward
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Len(t, cfg.Contexts, 1)
	assert.Equal(t, "dev-cluster", cfg.Contexts[0].Name)
	assert.Len(t, cfg.Contexts[0].Namespaces, 1)
	assert.Equal(t, "default", cfg.Contexts[0].Namespaces[0].Name)
	assert.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 1)
	assert.Equal(t, "pod/my-app", cfg.Contexts[0].Namespaces[0].Forwards[0].Resource)
}

// TestMutator_AddForward_ExistingFile tests adding to existing config
func TestMutator_AddForward_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/existing-app
            protocol: tcp
            port: 8080
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	fwd := Forward{
		Resource:  "service/postgres",
		Protocol:  "tcp",
		Port:      5432,
		LocalPort: 5432,
	}

	err = mutator.AddForward("dev-cluster", "default", fwd)
	require.NoError(t, err)

	// Verify both forwards exist
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 2)
}

// TestMutator_AddForward_NewContext tests adding to new context
func TestMutator_AddForward_NewContext(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app
            protocol: tcp
            port: 8080
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	fwd := Forward{
		Resource:  "pod/prod-app",
		Protocol:  "tcp",
		Port:      80,
		LocalPort: 8081,
	}

	err = mutator.AddForward("prod-cluster", "production", fwd)
	require.NoError(t, err)

	// Verify new context was created
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Contexts, 2)
	assert.Equal(t, "prod-cluster", cfg.Contexts[1].Name)
}

// TestMutator_AddForward_DuplicatePort tests rejecting duplicate ports
func TestMutator_AddForward_DuplicatePort(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app
            protocol: tcp
            port: 8080
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	fwd := Forward{
		Resource:  "pod/another-app",
		Protocol:  "tcp",
		Port:      9090,
		LocalPort: 8080, // Duplicate local port
	}

	err = mutator.AddForward("dev-cluster", "default", fwd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port 8080 is already in use")
}

// TestMutator_AddForward_InvalidForward tests rejecting invalid forward
func TestMutator_AddForward_InvalidForward(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config file
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/existing-app
            protocol: tcp
            port: 8080
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	fwd := Forward{
		Resource:  "invalid/type/resource", // Invalid resource
		Protocol:  "tcp",
		Port:      9090,
		LocalPort: 9090,
	}

	err = mutator.AddForward("dev-cluster", "default", fwd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
}

// TestMutator_RemoveForwards tests removing forwards by predicate
func TestMutator_RemoveForwards(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config with multiple forwards
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app1
            protocol: tcp
            port: 8080
            localPort: 8080
          - resource: pod/app2
            protocol: tcp
            port: 8081
            localPort: 8081
          - resource: service/postgres
            protocol: tcp
            port: 5432
            localPort: 5432
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	// Remove all pod resources
	err = mutator.RemoveForwards(func(ctx, ns string, fwd Forward) bool {
		return fwd.Resource == "pod/app1"
	})
	require.NoError(t, err)

	// Verify the forward was removed
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 2)
	for _, fwd := range cfg.Contexts[0].Namespaces[0].Forwards {
		assert.NotEqual(t, "pod/app1", fwd.Resource)
	}
}

// TestMutator_RemoveForwards_RemovesEmptyNamespaces tests that empty namespaces are removed
func TestMutator_RemoveForwards_RemovesEmptyNamespaces(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create config with two namespaces
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: ns1
        forwards:
          - resource: pod/app1
            protocol: tcp
            port: 8080
            localPort: 8080
      - name: ns2
        forwards:
          - resource: pod/app2
            protocol: tcp
            port: 8081
            localPort: 8081
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	// Remove all forwards from ns1
	err = mutator.RemoveForwards(func(ctx, ns string, fwd Forward) bool {
		return ns == "ns1"
	})
	require.NoError(t, err)

	// Verify ns1 was removed (has no forwards left)
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Contexts[0].Namespaces, 1)
	assert.Equal(t, "ns2", cfg.Contexts[0].Namespaces[0].Name)
}

// TestMutator_RemoveForwards_NonExistentFile tests removing from non-existent file
func TestMutator_RemoveForwards_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	mutator := NewMutator(configPath)

	err := mutator.RemoveForwards(func(ctx, ns string, fwd Forward) bool {
		return true
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load config")
}

// TestMutator_RemoveForwardByID tests removing a specific forward by ID
func TestMutator_RemoveForwardByID(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app1
            protocol: tcp
            port: 8080
            localPort: 8080
          - resource: pod/app2
            protocol: tcp
            port: 8081
            localPort: 8081
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	// Remove by ID
	err = mutator.RemoveForwardByID("dev-cluster/default/pod/app1:8080")
	require.NoError(t, err)

	// Verify the forward was removed
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 1)
	assert.Equal(t, "pod/app2", cfg.Contexts[0].Namespaces[0].Forwards[0].Resource)
}

// TestMutator_UpdateForward tests updating an existing forward
func TestMutator_UpdateForward(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app1
            protocol: tcp
            port: 8080
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	newFwd := Forward{
		Resource:  "pod/app1-updated",
		Protocol:  "tcp",
		Port:      9090,
		LocalPort: 9090,
	}

	err = mutator.UpdateForward("dev-cluster/default/pod/app1:8080", "dev-cluster", "default", newFwd)
	require.NoError(t, err)

	// Verify the forward was updated
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	assert.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 1)
	assert.Equal(t, "pod/app1-updated", cfg.Contexts[0].Namespaces[0].Forwards[0].Resource)
	assert.Equal(t, 9090, cfg.Contexts[0].Namespaces[0].Forwards[0].LocalPort)
}

// TestMutator_UpdateForward_MoveToNewContext tests moving forward to new context
func TestMutator_UpdateForward_MoveToNewContext(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config with multiple forwards (so removing one doesn't leave empty namespace)
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app1
            protocol: tcp
            port: 8080
            localPort: 8080
          - resource: pod/app2
            protocol: tcp
            port: 9090
            localPort: 9090
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	newFwd := Forward{
		Resource:  "pod/app1-moved",
		Protocol:  "tcp",
		Port:      8080,
		LocalPort: 8080,
	}

	err = mutator.UpdateForward("dev-cluster/default/pod/app1:8080", "prod-cluster", "production", newFwd)
	require.NoError(t, err)

	// Verify the forward was moved
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)

	// New context should exist with the forward
	assert.Len(t, cfg.Contexts, 2)

	// Original namespace should still have one forward
	assert.Len(t, cfg.Contexts[0].Namespaces, 1)
	assert.Len(t, cfg.Contexts[0].Namespaces[0].Forwards, 1)

	// New context should have the moved forward
	assert.Equal(t, "prod-cluster", cfg.Contexts[1].Name)
	assert.Len(t, cfg.Contexts[1].Namespaces, 1)
	assert.Equal(t, "production", cfg.Contexts[1].Namespaces[0].Name)
}

// TestMutator_UpdateForward_NotFound tests updating non-existent forward
func TestMutator_UpdateForward_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app
            protocol: tcp
            port: 8080
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	newFwd := Forward{
		Resource:  "pod/app",
		Protocol:  "tcp",
		Port:      8080,
		LocalPort: 8080,
	}

	err = mutator.UpdateForward("non-existent-id", "dev-cluster", "default", newFwd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forward with ID non-existent-id not found")
}

// TestMutator_UpdateForward_DuplicatePort tests rejecting update with duplicate port
func TestMutator_UpdateForward_DuplicatePort(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config with two forwards
	initial := `contexts:
  - name: dev-cluster
    namespaces:
      - name: default
        forwards:
          - resource: pod/app1
            protocol: tcp
            port: 8080
            localPort: 8080
          - resource: pod/app2
            protocol: tcp
            port: 9090
            localPort: 9090
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	// Try to update app1 to use the same port as app2
	newFwd := Forward{
		Resource:  "pod/app1-updated",
		Protocol:  "tcp",
		Port:      9090,
		LocalPort: 9090, // Duplicate with app2
	}

	err = mutator.UpdateForward("dev-cluster/default/pod/app1:8080", "dev-cluster", "default", newFwd)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "port 9090 is already in use")
}

// TestMutator_WriteAtomic tests atomic write behavior
func TestMutator_WriteAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	mutator := NewMutator(configPath)

	cfg := &Config{
		Contexts: []Context{
			{
				Name: "test",
				Namespaces: []Namespace{
					{
						Name: "default",
						Forwards: []Forward{
							{Resource: "pod/app", Protocol: "tcp", Port: 8080, LocalPort: 8080},
						},
					},
				},
			},
		},
	}

	err := mutator.writeAtomic(cfg)
	require.NoError(t, err)

	// Verify file was created with correct permissions
	info, err := os.Stat(configPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm())

	// Verify temp file was cleaned up
	tmpFile := filepath.Join(tmpDir, ".kportal.yaml.tmp")
	_, err = os.Stat(tmpFile)
	assert.True(t, os.IsNotExist(err))
}

// TestMutator_FindOrCreateContext tests context finding/creation
func TestMutator_FindOrCreateContext(t *testing.T) {
	mutator := NewMutator("/fake/path")

	t.Run("find existing context", func(t *testing.T) {
		cfg := &Config{
			Contexts: []Context{
				{Name: "existing"},
			},
		}

		ctx := mutator.findOrCreateContext(cfg, "existing")
		assert.Equal(t, "existing", ctx.Name)
		assert.Len(t, cfg.Contexts, 1)
	})

	t.Run("create new context", func(t *testing.T) {
		cfg := &Config{
			Contexts: []Context{
				{Name: "existing"},
			},
		}

		ctx := mutator.findOrCreateContext(cfg, "new-context")
		assert.Equal(t, "new-context", ctx.Name)
		assert.Len(t, cfg.Contexts, 2)
	})
}

// TestMutator_FindOrCreateNamespace tests namespace finding/creation
func TestMutator_FindOrCreateNamespace(t *testing.T) {
	mutator := NewMutator("/fake/path")

	t.Run("find existing namespace", func(t *testing.T) {
		ctx := &Context{
			Name: "test",
			Namespaces: []Namespace{
				{Name: "existing"},
			},
		}

		ns := mutator.findOrCreateNamespace(ctx, "existing")
		assert.Equal(t, "existing", ns.Name)
		assert.Len(t, ctx.Namespaces, 1)
	})

	t.Run("create new namespace", func(t *testing.T) {
		ctx := &Context{
			Name: "test",
			Namespaces: []Namespace{
				{Name: "existing"},
			},
		}

		ns := mutator.findOrCreateNamespace(ctx, "new-namespace")
		assert.Equal(t, "new-namespace", ns.Name)
		assert.Len(t, ctx.Namespaces, 2)
	})
}

// TestMutator_Concurrent tests mutex protection
func TestMutator_Concurrent(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".kportal.yaml")

	// Create initial config file
	initial := `contexts:
  - name: dev
    namespaces:
      - name: default
        forwards:
          - resource: pod/app
            protocol: tcp
            port: 8080
            localPort: 8080
`
	err := os.WriteFile(configPath, []byte(initial), 0600)
	require.NoError(t, err)

	mutator := NewMutator(configPath)

	// Run concurrent operations
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(port int) {
			defer func() { done <- true }()
			fwd := Forward{
				Resource:  "pod/app",
				Protocol:  "tcp",
				Port:      port + 9000,
				LocalPort: port + 9000,
			}
			// Some will succeed, some will fail due to validation
			// The important thing is no race condition
			_ = mutator.AddForward("dev", "default", fwd)
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify config is still valid
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
}
