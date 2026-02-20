package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// =============================================================================
// ResourceResolver Extended Tests
// =============================================================================

func TestResourceResolver_ResolvePodPrefix_CacheHit(t *testing.T) {
	baseTime := time.Now()

	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-xyz789",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	// First call - hits API
	result1, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")
	require.NoError(t, err)
	assert.Equal(t, "pod/my-app-xyz789", result1)

	// Second call - should use cache (instant)
	start := time.Now()
	result2, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")
	require.NoError(t, err)
	assert.Equal(t, result1, result2)
	// Should be very fast since it's cached
	assert.Less(t, time.Since(start), 10*time.Millisecond)
}

func TestResourceResolver_ResolvePodSelector_CacheHit(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "myapp"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	// First call - hits API
	result1, err := r.Resolve(t.Context(), "test-context", "default", "pod", "app=myapp")
	require.NoError(t, err)
	assert.Equal(t, "pod/app-pod", result1)

	// Second call - should use cache
	result2, err := r.Resolve(t.Context(), "test-context", "default", "pod", "app=myapp")
	require.NoError(t, err)
	assert.Equal(t, result1, result2)
}

func TestResourceResolver_ResolvePodPrefix_ExcludesNonRunning(t *testing.T) {
	baseTime := time.Now()

	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-pending",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-succeeded",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-failed",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodFailed},
		},
	)

	r := NewResourceResolver(pool)

	_, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pods found matching prefix")
}

func TestResourceResolver_ResolvePodSelector_ExcludesNonRunning(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod-pending",
				Namespace: "default",
				Labels:    map[string]string{"app": "myapp"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	)

	r := NewResourceResolver(pool)

	_, err := r.Resolve(t.Context(), "test-context", "default", "pod", "app=myapp")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pods found matching selector")
}

func TestResourceResolver_getFromCache_NotFound(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)

	result := r.getFromCache("non-existent-key")
	assert.Empty(t, result)
}

func TestResourceResolver_getFromCache_ExpiredEntry(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)
	r.SetCacheTTL(1 * time.Millisecond)

	// Put entry in cache
	r.putInCache("test-key", "test-value")

	// Verify it's there
	result := r.getFromCache("test-key")
	assert.Equal(t, "test-value", result)

	// Wait for expiry
	time.Sleep(10 * time.Millisecond)

	// Should be expired and cleaned up
	result = r.getFromCache("test-key")
	assert.Empty(t, result)

	// Verify entry was deleted
	r.cacheMu.RLock()
	_, exists := r.cache["test-key"]
	r.cacheMu.RUnlock()
	assert.False(t, exists)
}

func TestResourceResolver_InvalidateCache_NoEntries(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)

	// Should not panic on empty cache
	r.InvalidateCache("test-context", "default", "pod/app")

	assert.NotNil(t, r.cache)
}

func TestResourceResolver_Resolve_GetClientError(t *testing.T) {
	// Create pool without test client - should fail when trying to get client
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)

	_, err := r.Resolve(t.Context(), "non-existent-context", "default", "pod/test", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get client")
}

func TestResourceResolver_ResolvePodPrefix_MultipleMatchesReturnsNewest(t *testing.T) {
	baseTime := time.Now()

	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-oldest",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime.Add(-2 * time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-middle",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime.Add(-1 * time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-newest",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	result, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")
	require.NoError(t, err)
	assert.Equal(t, "pod/my-app-newest", result)
}

func TestResourceResolver_ResolvePodSelector_FirstRunning(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod-1",
				Namespace: "default",
				Labels:    map[string]string{"app": "myapp"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod-2",
				Namespace: "default",
				Labels:    map[string]string{"app": "myapp"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	result, err := r.Resolve(t.Context(), "test-context", "default", "pod", "app=myapp")
	require.NoError(t, err)
	// Should return the first running pod found
	assert.Equal(t, "pod/app-pod-1", result)
}

// =============================================================================
// Discovery Extended Tests
// =============================================================================

func TestDiscovery_ListPods_FilteringAndSorting(t *testing.T) {
	baseTime := time.Now()

	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "newer-running-pod",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "main",
						Ports: []corev1.ContainerPort{
							{ContainerPort: 8080, Protocol: corev1.ProtocolTCP},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "older-pending-pod",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime.Add(-time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main"},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "older-running-pod",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime.Add(-2 * time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: "main"},
				},
			},
		},
		// Pods in other namespaces should not appear
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-namespace-pod",
				Namespace: "kube-system",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	d := NewDiscovery(pool)

	pods, err := d.ListPods(t.Context(), "test-context", "default")
	require.NoError(t, err)
	assert.Len(t, pods, 3) // 2 running + 1 pending

	// Should be sorted by creation time (newest first)
	assert.Equal(t, "newer-running-pod", pods[0].Name)
	assert.Equal(t, "older-pending-pod", pods[1].Name)
	assert.Equal(t, "older-running-pod", pods[2].Name)

	// Check protocol is set correctly
	assert.Equal(t, "TCP", pods[0].Containers[0].Ports[0].Protocol)
}

func TestDiscovery_ListPodsWithSelector_OnlyRunning(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "running-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "myapp"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pending-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "myapp"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	)

	d := NewDiscovery(pool)

	pods, err := d.ListPodsWithSelector(t.Context(), "test-context", "default", "app=myapp")
	require.NoError(t, err)
	// Only running pods should be returned for selector-based queries
	assert.Len(t, pods, 1)
	assert.Equal(t, "running-pod", pods[0].Name)
}

func TestDiscovery_ListServices_WithNamedPortResolution(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "backend"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "main",
						Ports: []corev1.ContainerPort{
							{Name: "http", ContainerPort: 8080},
							{Name: "grpc", ContainerPort: 50051},
						},
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": "backend"},
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, TargetPort: intstr.FromString("http")},
					{Name: "grpc", Port: 50051, TargetPort: intstr.FromString("grpc")},
				},
			},
		},
	)

	d := NewDiscovery(pool)

	services, err := d.ListServices(t.Context(), "test-context", "default")
	require.NoError(t, err)
	assert.Len(t, services, 1)

	// Named ports should be resolved
	assert.Len(t, services[0].Ports, 2)
	assert.Equal(t, int32(80), services[0].Ports[0].Port)
	assert.Equal(t, int32(8080), services[0].Ports[0].TargetPort) // Resolved from pod
	assert.Equal(t, int32(50051), services[0].Ports[1].Port)
	assert.Equal(t, int32(50051), services[0].Ports[1].TargetPort) // Resolved from pod
}

func TestDiscovery_ListServices_NoBackingPods(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": "nonexistent"},
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, TargetPort: intstr.FromString("http")},
				},
			},
		},
	)

	d := NewDiscovery(pool)

	services, err := d.ListServices(t.Context(), "test-context", "default")
	require.NoError(t, err)
	assert.Len(t, services, 1)

	// When no backing pods, falls back to service port
	assert.Equal(t, int32(80), services[0].Ports[0].TargetPort)
}
