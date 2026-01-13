// Package k8s provides Kubernetes client management, resource resolution,
// and port-forwarding capabilities for kportal.
//
// Key components:
//   - ClientPool: Thread-safe management of Kubernetes clients per context
//   - ResourceResolver: Resolves pod/service/selector targets to actual pods
//   - PortForwarder: Establishes and manages port-forward connections
//   - Discovery: Provides resource discovery for the UI wizards
//
// The package handles automatic pod restart detection through re-resolution,
// caching with 30-second TTL, and graceful connection management.
package k8s

import (
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ClientPool manages Kubernetes clients per context with thread-safe access.
type ClientPool struct {
	loader  clientcmd.ClientConfig
	clients map[string]*kubernetes.Clientset
	configs map[string]*rest.Config
	mu      sync.RWMutex
}

// NewClientPool creates a new ClientPool instance.
func NewClientPool() (*ClientPool, error) {
	// Load kubeconfig using default loading rules
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}

	loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	return &ClientPool{
		clients: make(map[string]*kubernetes.Clientset),
		configs: make(map[string]*rest.Config),
		loader:  loader,
	}, nil
}

// GetClient returns a Kubernetes client for the given context.
// Clients are cached and reused across multiple calls.
// This method is thread-safe.
func (p *ClientPool) GetClient(contextName string) (*kubernetes.Clientset, error) {
	// Try to get cached client (read lock)
	p.mu.RLock()
	client, exists := p.clients[contextName]
	p.mu.RUnlock()

	if exists {
		return client, nil
	}

	// Client doesn't exist, create it (write lock)
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check in case another goroutine created it while we waited
	if cachedClient, ok := p.clients[contextName]; ok {
		return cachedClient, nil
	}

	// Create new client
	config, err := p.getRestConfig(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get rest config for context %s: %w", contextName, err)
	}

	client, err = kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for context %s: %w", contextName, err)
	}

	// Cache the client and config
	p.clients[contextName] = client
	p.configs[contextName] = config

	return client, nil
}

// GetRestConfig returns the REST config for the given context.
// Configs are cached and reused.
// This method is thread-safe.
func (p *ClientPool) GetRestConfig(contextName string) (*rest.Config, error) {
	// Try to get cached config (read lock)
	p.mu.RLock()
	config, exists := p.configs[contextName]
	p.mu.RUnlock()

	if exists {
		return config, nil
	}

	// Config doesn't exist, create it (write lock)
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check in case another goroutine created it while we waited
	if cachedConfig, ok := p.configs[contextName]; ok {
		return cachedConfig, nil
	}

	// Create new config
	config, err := p.getRestConfig(contextName)
	if err != nil {
		return nil, err
	}

	// Cache the config
	p.configs[contextName] = config

	return config, nil
}

// getRestConfig creates a REST config for the given context.
// This is an internal method that should only be called with a lock held.
func (p *ClientPool) getRestConfig(contextName string) (*rest.Config, error) {
	// Load the raw kubeconfig
	rawConfig, err := p.loader.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Check if the context exists
	if _, exists := rawConfig.Contexts[contextName]; !exists {
		return nil, fmt.Errorf("context %s not found in kubeconfig", contextName)
	}

	// Create config overrides for the specific context
	overrides := &clientcmd.ConfigOverrides{
		CurrentContext: contextName,
	}

	// Build the config
	config, err := clientcmd.NewNonInteractiveClientConfig(
		rawConfig,
		contextName,
		overrides,
		clientcmd.NewDefaultClientConfigLoadingRules(),
	).ClientConfig()

	if err != nil {
		return nil, fmt.Errorf("failed to build client config for context %s: %w", contextName, err)
	}

	return config, nil
}

// GetCurrentContext returns the name of the current context from kubeconfig.
func (p *ClientPool) GetCurrentContext() (string, error) {
	rawConfig, err := p.loader.RawConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	return rawConfig.CurrentContext, nil
}

// ListContexts returns a list of all available contexts from kubeconfig.
func (p *ClientPool) ListContexts() ([]string, error) {
	rawConfig, err := p.loader.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	contexts := make([]string, 0, len(rawConfig.Contexts))
	for name := range rawConfig.Contexts {
		contexts = append(contexts, name)
	}

	return contexts, nil
}

// ClearCache removes all cached clients and configs.
// This is useful for testing or when kubeconfig has been updated.
func (p *ClientPool) ClearCache() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.clients = make(map[string]*kubernetes.Clientset)
	p.configs = make(map[string]*rest.Config)
}

// RemoveContext removes a specific context from the cache.
// This is useful when a context is removed or updated.
func (p *ClientPool) RemoveContext(contextName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.clients, contextName)
	delete(p.configs, contextName)
}

// GetNamespace returns the default namespace for the given context.
func (p *ClientPool) GetNamespace(contextName string) (string, error) {
	rawConfig, err := p.loader.RawConfig()
	if err != nil {
		return "", fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	context, exists := rawConfig.Contexts[contextName]
	if !exists {
		return "", fmt.Errorf("context %s not found", contextName)
	}

	// Return the namespace from the context, or "default" if not specified
	if context.Namespace == "" {
		return corev1.NamespaceDefault, nil
	}

	return context.Namespace, nil
}
