package k8s

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// =============================================================================
// ClientPool Extended Tests
// =============================================================================

func TestClientPool_GetClient_Caching(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
		},
	)

	// First call - should create and cache
	client1, err := pool.GetClient("test-context")
	require.NoError(t, err)
	assert.NotNil(t, client1)

	// Second call - should return cached
	client2, err := pool.GetClient("test-context")
	require.NoError(t, err)
	assert.Equal(t, client1, client2)
}

func TestClientPool_GetRestConfig_Caching(t *testing.T) {
	// This test would require actual kubeconfig context
	// Skip it for unit testing - covered by integration tests
	t.Skip("Requires actual kubeconfig context - skipping in unit tests")
}

func TestClientPool_ClearCache_ThreadSafe(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	// Populate client cache
	_, err := pool.GetClient("test-context")
	require.NoError(t, err)

	// Manually populate configs for testing
	pool.mu.Lock()
	pool.configs["test-context"] = nil
	pool.mu.Unlock()

	// Clear cache multiple times concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.ClearCache()
		}()
	}
	wg.Wait()

	// Verify cache is empty
	pool.mu.RLock()
	assert.Empty(t, pool.clients)
	assert.Empty(t, pool.configs)
	pool.mu.RUnlock()
}

func TestClientPool_RemoveContext_ThreadSafe(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	// Populate cache
	_, err := pool.GetClient("test-context")
	require.NoError(t, err)

	// Remove from multiple goroutines
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.RemoveContext("test-context")
		}()
	}
	wg.Wait()

	// Verify removed
	pool.mu.RLock()
	_, exists := pool.clients["test-context"]
	pool.mu.RUnlock()
	assert.False(t, exists)
}

func TestClientPool_ConcurrentGetClient(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	var wg sync.WaitGroup

	// Concurrent reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pool.GetClient("test-context")
		}()
	}

	// Concurrent config reads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = pool.GetRestConfig("test-context")
		}()
	}

	// Concurrent cache operations
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.ClearCache()
		}()
	}

	wg.Wait()

	// If we got here without panic/deadlock, the test passed
	assert.NotNil(t, pool)
}

func TestClientPool_GetClient_MultipleContexts(t *testing.T) {
	fakeClient1 := fake.NewClientset()
	fakeClient2 := fake.NewClientset()

	pool, err := NewClientPool()
	require.NoError(t, err)

	pool.setTestClient("context-1", fakeClient1)
	pool.setTestClient("context-2", fakeClient2)

	client1, err := pool.GetClient("context-1")
	require.NoError(t, err)
	assert.Equal(t, fakeClient1, client1)

	client2, err := pool.GetClient("context-2")
	require.NoError(t, err)
	assert.Equal(t, fakeClient2, client2)

	// Verify they are different
	assert.NotEqual(t, client1, client2)
}

func TestClientPool_GetRestConfig_MultipleContexts(t *testing.T) {
	// This test would require actual kubeconfig contexts
	// Skip it for unit testing - covered by integration tests
	t.Skip("Requires actual kubeconfig contexts - skipping in unit tests")
}

func TestClientPool_RemoveContext_Specific(t *testing.T) {
	pool := setupTestPool(t, "context-1")
	pool.setTestClient("context-2", fake.NewClientset())

	// Populate both caches
	_, err := pool.GetClient("context-1")
	require.NoError(t, err)
	_, err = pool.GetClient("context-2")
	require.NoError(t, err)

	// Remove only context-1
	pool.RemoveContext("context-1")

	// Verify context-1 removed but context-2 still there
	pool.mu.RLock()
	_, exists1 := pool.clients["context-1"]
	_, exists2 := pool.clients["context-2"]
	pool.mu.RUnlock()

	assert.False(t, exists1)
	assert.True(t, exists2)
}

func TestClientPool_setTestClient_NilMap(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	// Clear the map manually to simulate nil case
	pool.mu.Lock()
	pool.clients = nil
	pool.mu.Unlock()

	// Should handle nil map
	pool.setTestClient("test-context", fake.NewClientset())

	// Verify it was set
	pool.mu.RLock()
	_, exists := pool.clients["test-context"]
	pool.mu.RUnlock()
	assert.True(t, exists)
}

func TestClientPool_GetNamespace_WithTestClient(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	// The GetNamespace method uses the loader to get namespace from kubeconfig context
	// Since we're using test client, this may fail depending on kubeconfig
	_, err := pool.GetNamespace("test-context")
	// May succeed or fail depending on environment
	// Just verify it doesn't panic
	_ = err
}

func TestClientPool_GetClient_NotFound(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	// Try to get client for non-existent context without setting test client
	_, err = pool.GetClient("non-existent-context")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in kubeconfig")
}

func TestClientPool_GetRestConfig_NotFound(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	// Try to get rest config for non-existent context
	_, err = pool.GetRestConfig("non-existent-context")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found in kubeconfig")
}

func TestClientPool_DoubleCheckCache(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	// Simulate race where two goroutines try to get the same client
	// One creates it, the other should get cached version

	var client1, client2 interface{}
	var err1, err2 error
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		client1, err1 = pool.GetClient("test-context")
	}()
	go func() {
		defer wg.Done()
		client2, err2 = pool.GetClient("test-context")
	}()

	wg.Wait()

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, client1, client2)
}

func TestClientPool_DoubleCheckRestConfig(t *testing.T) {
	// This test would require actual kubeconfig context
	// Skip it for unit testing - covered by integration tests
	t.Skip("Requires actual kubeconfig context - skipping in unit tests")
}
