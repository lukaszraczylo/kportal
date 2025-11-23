package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClientPool(t *testing.T) {
	pool, err := NewClientPool()

	assert.NoError(t, err, "NewClientPool should not return error")
	assert.NotNil(t, pool, "pool should not be nil")
	assert.NotNil(t, pool.clients, "clients map should be initialized")
	assert.NotNil(t, pool.configs, "configs map should be initialized")
	assert.Empty(t, pool.clients, "clients map should be empty initially")
	assert.Empty(t, pool.configs, "configs map should be empty initially")
}

func TestClientPool_ClearCache(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Initially empty
	assert.Empty(t, pool.clients)
	assert.Empty(t, pool.configs)

	// Call ClearCache on empty pool (should not panic)
	pool.ClearCache()

	// Should still be empty
	assert.Empty(t, pool.clients)
	assert.Empty(t, pool.configs)
}

func TestClientPool_RemoveContext(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Remove from empty pool (should not panic)
	pool.RemoveContext("non-existent-context")

	// Should still be empty
	assert.Empty(t, pool.clients)
	assert.Empty(t, pool.configs)
}

func TestClientPool_Structure(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Verify structure - just check maps are initialized
	assert.NotNil(t, pool.clients, "clients map should exist")
	assert.NotNil(t, pool.configs, "configs map should exist")
	assert.NotNil(t, pool.loader, "loader should exist")
}

func TestClientPool_GetCurrentContext(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Try to get current context
	// This may fail if kubeconfig is not available, which is fine for unit tests
	context, err := pool.GetCurrentContext()

	if err == nil {
		// If successful, context should be a string
		assert.IsType(t, "", context)
	} else {
		// If failed, error should mention kubeconfig
		assert.Contains(t, err.Error(), "kubeconfig")
	}
}

func TestClientPool_ListContexts(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Try to list contexts
	// This may fail if kubeconfig is not available, which is fine for unit tests
	contexts, err := pool.ListContexts()

	if err == nil {
		// If successful, contexts should be a slice
		assert.NotNil(t, contexts)
		assert.IsType(t, []string{}, contexts)
	} else {
		// If failed, error should mention kubeconfig
		assert.Contains(t, err.Error(), "kubeconfig")
	}
}

func TestClientPool_GetNamespace(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Try to get namespace for a non-existent context
	namespace, err := pool.GetNamespace("non-existent-context")

	// This should fail with context not found error
	if err != nil {
		// Error is expected, check it mentions the context or kubeconfig
		errMsg := err.Error()
		containsContext := assert.Contains(t, errMsg, "context", "error should mention context") ||
			assert.Contains(t, errMsg, "kubeconfig", "error should mention kubeconfig")
		assert.True(t, containsContext)
	} else {
		// If no error (unlikely), namespace should be a string
		assert.IsType(t, "", namespace)
	}
}

func TestClientPool_GetClient_NonExistentContext(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Try to get a client for a non-existent context
	client, err := pool.GetClient("non-existent-context")

	// This should fail
	assert.Error(t, err, "should return error for non-existent context")
	assert.Nil(t, client, "client should be nil on error")
}

func TestClientPool_GetRestConfig_NonExistentContext(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Try to get a rest config for a non-existent context
	config, err := pool.GetRestConfig("non-existent-context")

	// This should fail
	assert.Error(t, err, "should return error for non-existent context")
	assert.Nil(t, config, "config should be nil on error")
}

func TestClientPool_ThreadSafety(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Test that concurrent operations don't panic
	// We don't check for errors as the context may not exist
	done := make(chan bool)

	for i := 0; i < 10; i++ {
		go func() {
			pool.ClearCache()
			pool.RemoveContext("test-context")
			pool.GetCurrentContext()
			pool.ListContexts()
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we get here without panic, thread safety is working
	assert.True(t, true, "concurrent operations should not panic")
}

func TestClientPool_CacheBehavior(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Initially empty
	assert.Empty(t, pool.clients)
	assert.Empty(t, pool.configs)

	// Add a context to the internal cache manually for testing
	// Note: This is a simplified test that doesn't require actual kubeconfig
	pool.mu.Lock()
	testContext := "test-context"
	pool.clients[testContext] = nil // Just mark as cached
	pool.configs[testContext] = nil // Just mark as cached
	pool.mu.Unlock()

	// Verify it's cached
	pool.mu.RLock()
	_, clientExists := pool.clients[testContext]
	_, configExists := pool.configs[testContext]
	pool.mu.RUnlock()

	assert.True(t, clientExists, "client should be in cache")
	assert.True(t, configExists, "config should be in cache")

	// Remove context
	pool.RemoveContext(testContext)

	// Verify it's removed
	pool.mu.RLock()
	_, clientExists = pool.clients[testContext]
	_, configExists = pool.configs[testContext]
	pool.mu.RUnlock()

	assert.False(t, clientExists, "client should be removed from cache")
	assert.False(t, configExists, "config should be removed from cache")

	// Add back and clear cache
	pool.mu.Lock()
	pool.clients[testContext] = nil
	pool.configs[testContext] = nil
	pool.mu.Unlock()

	pool.ClearCache()

	// Verify cache is cleared
	assert.Empty(t, pool.clients, "clients should be cleared")
	assert.Empty(t, pool.configs, "configs should be cleared")
}

func TestClientPool_EmptyPoolOperations(t *testing.T) {
	pool, err := NewClientPool()
	assert.NoError(t, err)

	// Test various operations on empty pool (should not panic)
	pool.ClearCache()
	pool.RemoveContext("any-context")

	// All these operations should complete without panic
	assert.NotNil(t, pool, "pool should still be valid")
}
