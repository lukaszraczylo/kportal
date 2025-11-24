package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Mutator provides safe, atomic mutations to the kportal configuration file.
// All operations use atomic file writes (write to temp, then rename) to prevent
// corruption and ensure the file watcher picks up changes.
type Mutator struct {
	configPath string
	mu         sync.Mutex // Ensure only one mutation at a time
}

// NewMutator creates a new configuration mutator for the given config file path.
func NewMutator(configPath string) *Mutator {
	return &Mutator{
		configPath: configPath,
	}
}

// findOrCreateContext finds an existing context or creates a new one
func (m *Mutator) findOrCreateContext(cfg *Config, contextName string) *Context {
	for i := range cfg.Contexts {
		if cfg.Contexts[i].Name == contextName {
			return &cfg.Contexts[i]
		}
	}

	// Create new context
	cfg.Contexts = append(cfg.Contexts, Context{
		Name:       contextName,
		Namespaces: []Namespace{},
	})
	return &cfg.Contexts[len(cfg.Contexts)-1]
}

// findOrCreateNamespace finds an existing namespace or creates a new one
func (m *Mutator) findOrCreateNamespace(ctx *Context, namespaceName string) *Namespace {
	for i := range ctx.Namespaces {
		if ctx.Namespaces[i].Name == namespaceName {
			return &ctx.Namespaces[i]
		}
	}

	// Create new namespace
	ctx.Namespaces = append(ctx.Namespaces, Namespace{
		Name:     namespaceName,
		Forwards: []Forward{},
	})
	return &ctx.Namespaces[len(ctx.Namespaces)-1]
}

// AddForward adds a new port forward to the configuration.
// If the context or namespace doesn't exist, they will be created.
// The new configuration is validated before writing.
// Returns an error if the port is already in use or validation fails.
func (m *Mutator) AddForward(contextName, namespaceName string, fwd Forward) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load current config
	cfg, err := LoadConfig(m.configPath)
	if err != nil {
		// If file doesn't exist, create empty config
		if os.IsNotExist(err) {
			cfg = &Config{Contexts: []Context{}}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	// Find or create context and namespace
	targetContext := m.findOrCreateContext(cfg, contextName)
	targetNamespace := m.findOrCreateNamespace(targetContext, namespaceName)

	// Set context/namespace on the forward for validation
	fwd.SetContext(contextName, namespaceName)

	// Check for duplicate local port
	allForwards := cfg.GetAllForwards()
	for _, existing := range allForwards {
		if existing.LocalPort == fwd.LocalPort {
			return fmt.Errorf("port %d is already in use by %s", fwd.LocalPort, existing.String())
		}
	}

	// Add the forward
	targetNamespace.Forwards = append(targetNamespace.Forwards, fwd)

	// Validate the new configuration
	validator := NewValidator()
	if errs := validator.ValidateConfig(cfg); len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", FormatValidationErrors(errs))
	}

	// Write atomically
	return m.writeAtomic(cfg)
}

// RemoveForwards removes forwards matching the predicate function.
// The predicate receives the context, namespace, and forward, and should return true
// to remove that forward.
// Empty namespaces and contexts are preserved (not automatically removed).
func (m *Mutator) RemoveForwards(predicate func(ctx, ns string, fwd Forward) bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load current config
	cfg, err := LoadConfig(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Iterate and filter
	for i := range cfg.Contexts {
		ctx := &cfg.Contexts[i]
		filteredNamespaces := []Namespace{}

		for j := range ctx.Namespaces {
			ns := &ctx.Namespaces[j]

			// Filter forwards
			filtered := []Forward{}
			for _, fwd := range ns.Forwards {
				// CRITICAL: Set context/namespace so fwd.ID() generates correct ID
				fwd.SetContext(ctx.Name, ns.Name)

				if !predicate(ctx.Name, ns.Name, fwd) {
					// Keep this forward
					filtered = append(filtered, fwd)
				}
			}

			ns.Forwards = filtered

			// Only keep namespaces that have at least one forward
			if len(ns.Forwards) > 0 {
				filteredNamespaces = append(filteredNamespaces, *ns)
			}
		}

		ctx.Namespaces = filteredNamespaces
	}

	// Validate the new configuration
	validator := NewValidator()
	if errs := validator.ValidateConfig(cfg); len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", FormatValidationErrors(errs))
	}

	// Write atomically
	return m.writeAtomic(cfg)
}

// RemoveForwardByID removes a specific forward by its ID.
func (m *Mutator) RemoveForwardByID(id string) error {
	return m.RemoveForwards(func(ctx, ns string, fwd Forward) bool {
		return fwd.ID() == id
	})
}

// UpdateForward atomically replaces an existing forward with a new one.
// This is used for editing - it removes the old forward and adds the new one in a single transaction.
// If the old forward doesn't exist, returns an error.
// If the new forward validation fails, the operation is rolled back (old forward remains).
func (m *Mutator) UpdateForward(oldID, newContextName, newNamespaceName string, newFwd Forward) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load current config
	cfg, err := LoadConfig(m.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// First, verify the old forward exists and remove it
	oldForwardFound := false
	for i := range cfg.Contexts {
		ctx := &cfg.Contexts[i]
		for j := range ctx.Namespaces {
			ns := &ctx.Namespaces[j]

			// Filter forwards, removing the old one
			filtered := []Forward{}
			for _, fwd := range ns.Forwards {
				// CRITICAL: Set context/namespace so fwd.ID() generates correct ID
				fwd.SetContext(ctx.Name, ns.Name)

				if fwd.ID() == oldID {
					oldForwardFound = true
					// Skip this forward (remove it)
					continue
				}

				// Keep this forward
				filtered = append(filtered, fwd)
			}

			ns.Forwards = filtered
		}
	}

	if !oldForwardFound {
		return fmt.Errorf("forward with ID %s not found", oldID)
	}

	// Now add the new forward
	// Find or create context and namespace
	targetContext := m.findOrCreateContext(cfg, newContextName)
	targetNamespace := m.findOrCreateNamespace(targetContext, newNamespaceName)

	// Set context/namespace on the forward for validation
	newFwd.SetContext(newContextName, newNamespaceName)

	// Check for duplicate local port (excluding the one we just removed)
	allForwards := cfg.GetAllForwards()
	for _, existing := range allForwards {
		if existing.LocalPort == newFwd.LocalPort && existing.ID() != oldID {
			return fmt.Errorf("port %d is already in use by %s", newFwd.LocalPort, existing.String())
		}
	}

	// Add the new forward
	targetNamespace.Forwards = append(targetNamespace.Forwards, newFwd)

	// Validate the new configuration
	validator := NewValidator()
	if errs := validator.ValidateConfig(cfg); len(errs) > 0 {
		return fmt.Errorf("validation failed: %s", FormatValidationErrors(errs))
	}

	// Write atomically
	return m.writeAtomic(cfg)
}

// writeAtomic writes the configuration atomically to prevent corruption.
// Steps:
// 1. Marshal config to YAML
// 2. Write to temporary file (.kportal.yaml.tmp)
// 3. Atomic rename to actual config file
//
// This ensures the file watcher picks up a complete, valid file.
func (m *Mutator) writeAtomic(cfg *Config) error {
	// Marshal to YAML
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create temporary file in same directory as config
	dir := filepath.Dir(m.configPath)
	tmpFile := filepath.Join(dir, ".kportal.yaml.tmp")

	// Write to temp file
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpFile, m.configPath); err != nil {
		// Clean up temp file on failure
		os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}
