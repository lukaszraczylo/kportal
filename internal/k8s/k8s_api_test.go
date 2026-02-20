package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

// =============================================================================
// Test Helpers
// =============================================================================

func setupTestPool(t *testing.T, contextName string, objects ...runtime.Object) *ClientPool {
	t.Helper()

	pool, err := NewClientPool()
	require.NoError(t, err)

	fakeClient := fake.NewClientset(objects...)
	// Type assertion to convert fake client to *kubernetes.Clientset
	// Note: This works because fake.Clientset embeds *kubernetes.Clientset
	pool.setTestClient(contextName, fakeClient)

	return pool
}

// =============================================================================
// Discovery API Tests
// =============================================================================

func TestDiscovery_ListNamespaces_WithClient(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "kube-system"},
		},
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "production"},
		},
	)

	d := NewDiscovery(pool)

	namespaces, err := d.ListNamespaces(t.Context(), "test-context")

	require.NoError(t, err)
	assert.Len(t, namespaces, 3)
	assert.Contains(t, namespaces, "default")
	assert.Contains(t, namespaces, "kube-system")
	assert.Contains(t, namespaces, "production")
}

func TestDiscovery_ListNamespaces_Error(t *testing.T) {
	// Pool without test client - should fail
	pool, err := NewClientPool()
	require.NoError(t, err)

	d := NewDiscovery(pool)

	_, err = d.ListNamespaces(t.Context(), "non-existent-context")

	assert.Error(t, err)
}

func TestDiscovery_ListPods_WithClient(t *testing.T) {
	baseTime := time.Now()

	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "running-pod",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "main",
						Ports: []corev1.ContainerPort{
							{Name: "http", ContainerPort: 8080},
							{Name: "metrics", ContainerPort: 9090},
						},
					},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "pending-pod",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime.Add(-time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "succeeded-pod",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
		},
	)

	d := NewDiscovery(pool)

	pods, err := d.ListPods(t.Context(), "test-context", "default")

	require.NoError(t, err)
	// Only Running and Pending pods
	assert.Len(t, pods, 2)

	// Should be sorted by creation time (newest first)
	assert.Equal(t, "running-pod", pods[0].Name)
	assert.Equal(t, "pending-pod", pods[1].Name)

	// Check container info
	assert.Len(t, pods[0].Containers, 1)
	assert.Len(t, pods[0].Containers[0].Ports, 2)
	assert.Equal(t, "http", pods[0].Containers[0].Ports[0].Name)
	assert.Equal(t, int32(8080), pods[0].Containers[0].Ports[0].Port)
}

func TestDiscovery_ListPods_EmptyNamespace(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	d := NewDiscovery(pool)

	pods, err := d.ListPods(t.Context(), "test-context", "default")

	require.NoError(t, err)
	assert.Empty(t, pods)
}

func TestDiscovery_ListPodsWithSelector_WithClient(t *testing.T) {
	baseTime := time.Now()

	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "app-pod-1",
				Namespace:         "default",
				Labels:            map[string]string{"app": "myapp"},
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "app-pod-2",
				Namespace:         "default",
				Labels:            map[string]string{"app": "myapp"},
				CreationTimestamp: metav1.Time{Time: baseTime.Add(-time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "other-pod",
				Namespace:         "default",
				Labels:            map[string]string{"app": "other"},
				CreationTimestamp: metav1.Time{Time: baseTime},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	d := NewDiscovery(pool)

	pods, err := d.ListPodsWithSelector(t.Context(), "test-context", "default", "app=myapp")

	require.NoError(t, err)
	// Only Running pods with matching selector
	assert.Len(t, pods, 2)

	names := []string{pods[0].Name, pods[1].Name}
	assert.Contains(t, names, "app-pod-1")
	assert.Contains(t, names, "app-pod-2")
}

func TestDiscovery_ListPodsWithSelector_EmptySelector(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	d := NewDiscovery(pool)

	_, err := d.ListPodsWithSelector(t.Context(), "test-context", "default", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "selector cannot be empty")
}

func TestDiscovery_ListPodsWithSelector_NoRunningPods(t *testing.T) {
	pool := setupTestPool(t, "test-context",
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
	assert.Empty(t, pods)
}

func TestDiscovery_ListServices_WithClient(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "web"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "main",
						Ports: []corev1.ContainerPort{
							{Name: "http", ContainerPort: 8080},
						},
					},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "web-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": "web"},
				Ports: []corev1.ServicePort{
					{Name: "http", Port: 80, TargetPort: intstr.FromString("http")},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: map[string]string{"app": "api"},
				Ports: []corev1.ServicePort{
					{Port: 8080, TargetPort: intstr.FromInt(8080)},
				},
			},
		},
	)

	d := NewDiscovery(pool)

	services, err := d.ListServices(t.Context(), "test-context", "default")

	require.NoError(t, err)
	assert.Len(t, services, 2)

	// Should be sorted alphabetically
	assert.Equal(t, "api-svc", services[0].Name)
	assert.Equal(t, "web-svc", services[1].Name)

	// Check port resolution for named port
	assert.Len(t, services[1].Ports, 1)
	assert.Equal(t, int32(8080), services[1].Ports[0].TargetPort) // Resolved from pod
}

func TestDiscovery_ListServices_Empty(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	d := NewDiscovery(pool)

	services, err := d.ListServices(t.Context(), "test-context", "default")

	require.NoError(t, err)
	assert.Empty(t, services)
}

// =============================================================================
// ResourceResolver API Tests
// =============================================================================

func TestResourceResolver_ResolvePodPrefix_WithClient(t *testing.T) {
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
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "my-app-abc123",
				Namespace:         "default",
				CreationTimestamp: metav1.Time{Time: baseTime.Add(-time.Hour)},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-app",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	result, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")

	require.NoError(t, err)
	// Should return newest pod matching prefix
	assert.Equal(t, "pod/my-app-xyz789", result)
}

func TestResourceResolver_ResolvePodPrefix_NotFound(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-app",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	_, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pods found matching prefix")
}

func TestResourceResolver_ResolvePodSelector_WithClient(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "app-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "myapp"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "other"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	result, err := r.Resolve(t.Context(), "test-context", "default", "pod", "app=myapp")

	require.NoError(t, err)
	assert.Equal(t, "pod/app-pod", result)
}

func TestResourceResolver_ResolvePodSelector_NotFound(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "other"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)

	_, err := r.Resolve(t.Context(), "test-context", "default", "pod", "app=myapp")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pods found matching selector")
}

func TestResourceResolver_Resolve_Caching(t *testing.T) {
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
	r.SetCacheTTL(100 * time.Millisecond)

	// First call - hits API
	result1, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")
	require.NoError(t, err)

	// Second call - uses cache
	result2, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")
	require.NoError(t, err)
	assert.Equal(t, result1, result2)

	// Wait for expiry
	time.Sleep(150 * time.Millisecond)

	// Third call - hits API again
	result3, err := r.Resolve(t.Context(), "test-context", "default", "pod/my-app", "")
	require.NoError(t, err)
	assert.Equal(t, result1, result3)
}

// =============================================================================
// PortForwarder API Tests
// =============================================================================

func TestPortForwarder_GetPodForResource_Pod(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	podName, err := pf.GetPodForResource(t.Context(), "test-context", "default", "pod/my-pod", "")

	require.NoError(t, err)
	assert.Equal(t, "my-pod", podName)
}

func TestPortForwarder_GetPodForResource_Service(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "backend"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "backend"},
				Ports: []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(8080)},
				},
			},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	podName, err := pf.GetPodForResource(t.Context(), "test-context", "default", "service/backend-svc", "")

	require.NoError(t, err)
	assert.Equal(t, "backend-pod", podName)
}

func TestPortForwarder_GetPodForResource_ServiceNoSelector(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "headless-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				// No selector
				Ports: []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(8080)},
				},
			},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	_, err := pf.GetPodForResource(t.Context(), "test-context", "default", "service/headless-svc", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no selector")
}

func TestPortForwarder_GetPodForResource_ServiceNoRunningPods(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pending-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "backend"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "backend"},
				Ports: []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(8080)},
				},
			},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	_, err := pf.GetPodForResource(t.Context(), "test-context", "default", "service/backend-svc", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pods found")
}

func TestPortForwarder_Forward_ServiceResolution(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-pod",
				Namespace: "default",
				Labels:    map[string]string{"app": "backend"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "backend-svc",
				Namespace: "default",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "backend"},
				Ports: []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(8080)},
				},
			},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	// Test that service resolution works (Forward will fail on actual port-forward,
	// but we can test the resolution part)
	stopChan := make(chan struct{})
	req := &ForwardRequest{
		StopChan:    stopChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "service/backend-svc",
		LocalPort:   8080,
		RemotePort:  80,
	}

	err := pf.Forward(t.Context(), req)

	// Will fail on port-forward setup, but should have resolved the service
	assert.Error(t, err)
	// Error should not be about resource resolution
	assert.NotContains(t, err.Error(), "failed to resolve resource")
}
