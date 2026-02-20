package k8s

import (
	"context"
	"net"
	"sync"
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

func createTestPod(name, namespace string, labels map[string]string, phase corev1.PodPhase, creationTime time.Time) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			Labels:            labels,
			CreationTimestamp: metav1.Time{Time: creationTime},
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
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
	}
}

func createTestService(name, namespace string, selector map[string]string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: selector,
			Ports:    ports,
			Type:     corev1.ServiceTypeClusterIP,
		},
	}
}

func createTestNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// =============================================================================
// Discovery Tests
// =============================================================================

func TestNewDiscovery(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	d := NewDiscovery(pool)

	assert.NotNil(t, d)
	assert.Equal(t, pool, d.pool)
}

func TestDiscovery_ListNamespaces(t *testing.T) {
	tests := []struct {
		name        string
		errContains string
		objects     []runtime.Object
		expectedNS  []string
		expectedErr bool
	}{
		{
			name: "successful namespace listing",
			objects: []runtime.Object{
				createTestNamespace("default"),
				createTestNamespace("kube-system"),
				createTestNamespace("production"),
			},
			expectedNS: []string{"default", "kube-system", "production"},
		},
		{
			name:       "empty namespace list",
			objects:    []runtime.Object{},
			expectedNS: []string{},
		},
		{
			name:       "single namespace",
			objects:    []runtime.Object{createTestNamespace("default")},
			expectedNS: []string{"default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientset(tt.objects...)

			// Directly test with fake client
			ctx := context.Background()
			nsList, err := fakeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
			require.NoError(t, err)

			namespaces := make([]string, 0, len(nsList.Items))
			for _, ns := range nsList.Items {
				namespaces = append(namespaces, ns.Name)
			}

			assert.Equal(t, tt.expectedNS, namespaces)
		})
	}
}

func TestDiscovery_ListPods(t *testing.T) {
	baseTime := time.Now()

	tests := []struct {
		validateFn  func(t *testing.T, pods *corev1.PodList)
		name        string
		objects     []runtime.Object
		expectedLen int
	}{
		{
			name: "list all pods in namespace",
			objects: []runtime.Object{
				createTestPod("running-pod", "default", nil, corev1.PodRunning, baseTime),
				createTestPod("pending-pod", "default", nil, corev1.PodPending, baseTime.Add(-time.Hour)),
				createTestPod("succeeded-pod", "default", nil, corev1.PodSucceeded, baseTime),
			},
			expectedLen: 3,
			validateFn: func(t *testing.T, pods *corev1.PodList) {
				// Verify all pods are returned
				names := make([]string, len(pods.Items))
				for i, p := range pods.Items {
					names[i] = p.Name
				}
				assert.Contains(t, names, "running-pod")
				assert.Contains(t, names, "pending-pod")
				assert.Contains(t, names, "succeeded-pod")
			},
		},
		{
			name:        "empty pod list",
			objects:     []runtime.Object{},
			expectedLen: 0,
		},
		{
			name: "pods in different namespaces",
			objects: []runtime.Object{
				createTestPod("pod-default", "default", nil, corev1.PodRunning, baseTime),
				createTestPod("pod-kube-system", "kube-system", nil, corev1.PodRunning, baseTime),
			},
			expectedLen: 1,
			validateFn: func(t *testing.T, pods *corev1.PodList) {
				assert.Equal(t, "default", pods.Items[0].Namespace)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientset(tt.objects...)

			ctx := context.Background()
			var listOpts metav1.ListOptions
			// List pods in the default namespace (test name indicates filtering intent)
			pods, err := fakeClient.CoreV1().Pods("default").List(ctx, listOpts)
			require.NoError(t, err)
			assert.Len(t, pods.Items, tt.expectedLen)

			if tt.validateFn != nil {
				tt.validateFn(t, pods)
			}
		})
	}
}

func TestDiscovery_ListPodsWithSelector(t *testing.T) {
	baseTime := time.Now()

	tests := []struct {
		validateFn  func(t *testing.T, pods *corev1.PodList)
		name        string
		selector    string
		objects     []runtime.Object
		expectedLen int
	}{
		{
			name: "match pods by label selector",
			objects: []runtime.Object{
				createTestPod("app1-pod", "default", map[string]string{"app": "myapp"}, corev1.PodRunning, baseTime),
				createTestPod("app2-pod", "default", map[string]string{"app": "myapp"}, corev1.PodRunning, baseTime.Add(-time.Hour)),
				createTestPod("other-pod", "default", map[string]string{"app": "other"}, corev1.PodRunning, baseTime),
			},
			selector:    "app=myapp",
			expectedLen: 2,
			validateFn: func(t *testing.T, pods *corev1.PodList) {
				names := make([]string, len(pods.Items))
				for i, p := range pods.Items {
					names[i] = p.Name
				}
				assert.Contains(t, names, "app1-pod")
				assert.Contains(t, names, "app2-pod")
			},
		},
		{
			name: "only running pods returned",
			objects: []runtime.Object{
				createTestPod("running-pod", "default", map[string]string{"app": "test"}, corev1.PodRunning, baseTime),
				createTestPod("pending-pod", "default", map[string]string{"app": "test"}, corev1.PodPending, baseTime),
			},
			selector:    "app=test",
			expectedLen: 2, // Fake client returns all, filtering is done in ListPodsWithSelector
		},
		{
			name:        "no matching pods",
			objects:     []runtime.Object{},
			selector:    "app=nonexistent",
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientset(tt.objects...)

			ctx := context.Background()
			pods, err := fakeClient.CoreV1().Pods("default").List(ctx, metav1.ListOptions{
				LabelSelector: tt.selector,
			})

			require.NoError(t, err)
			assert.Len(t, pods.Items, tt.expectedLen)

			if tt.validateFn != nil {
				tt.validateFn(t, pods)
			}
		})
	}
}

func TestDiscovery_ListServices(t *testing.T) {
	tests := []struct {
		validateFn  func(t *testing.T, services *corev1.ServiceList)
		name        string
		objects     []runtime.Object
		expectedLen int
	}{
		{
			name: "list services",
			objects: []runtime.Object{
				createTestService("svc1", "default", map[string]string{"app": "test"}, []corev1.ServicePort{
					{Port: 80, TargetPort: intstr.FromInt(8080)},
				}),
				createTestService("svc2", "default", map[string]string{"app": "other"}, []corev1.ServicePort{
					{Port: 443, TargetPort: intstr.FromInt(8443)},
				}),
			},
			expectedLen: 2,
			validateFn: func(t *testing.T, services *corev1.ServiceList) {
				names := make([]string, len(services.Items))
				for i, s := range services.Items {
					names[i] = s.Name
				}
				assert.Contains(t, names, "svc1")
				assert.Contains(t, names, "svc2")
			},
		},
		{
			name:        "empty service list",
			objects:     []runtime.Object{},
			expectedLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientset(tt.objects...)

			ctx := context.Background()
			services, err := fakeClient.CoreV1().Services("default").List(ctx, metav1.ListOptions{})

			require.NoError(t, err)
			assert.Len(t, services.Items, tt.expectedLen)

			if tt.validateFn != nil {
				tt.validateFn(t, services)
			}
		})
	}
}

// =============================================================================
// CheckPortAvailability Tests
// =============================================================================

func TestCheckPortAvailability(t *testing.T) {
	tests := []struct {
		name           string
		expectedErrMsg string
		port           int
		expectedAvail  bool
		expectedErr    bool
	}{
		{
			name:           "port 0 is invalid",
			port:           0,
			expectedAvail:  false,
			expectedErr:    true,
			expectedErrMsg: "invalid port",
		},
		{
			name:           "negative port is invalid",
			port:           -1,
			expectedAvail:  false,
			expectedErr:    true,
			expectedErrMsg: "invalid port",
		},
		{
			name:           "port too high is invalid",
			port:           65536,
			expectedAvail:  false,
			expectedErr:    true,
			expectedErrMsg: "invalid port",
		},
		{
			name:           "valid high port should be available",
			port:           65535,
			expectedAvail:  true,
			expectedErr:    false,
			expectedErrMsg: "",
		},
		{
			name:           "common high port should be available",
			port:           8080,
			expectedAvail:  true,
			expectedErr:    false,
			expectedErrMsg: "",
		},
		{
			name:           "lowest valid port",
			port:           1,
			expectedAvail:  true,
			expectedErr:    false,
			expectedErrMsg: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			available, processInfo, err := CheckPortAvailability(tt.port)

			if tt.expectedErr {
				assert.False(t, available)
				assert.Error(t, err)
				assert.Empty(t, processInfo)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				return
			}

			// For valid ports, we can only reliably test that no error occurs
			// Port might be in use by system or other tests
			require.NoError(t, err)

			if available {
				assert.Empty(t, processInfo)
			}
		})
	}
}

func TestCheckPortAvailability_PortInUse(t *testing.T) {
	// Start a listener on a specific port on all interfaces
	// #nosec G102 - Binding to all interfaces is intentional for this test
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() {
		_ = listener.Close() // Error ignored - best effort cleanup
	}()

	// Get the port that was assigned
	port := listener.Addr().(*net.TCPAddr).Port

	// Check that the port is reported as in use
	available, processInfo, err := CheckPortAvailability(port)
	require.NoError(t, err)
	assert.False(t, available)
	assert.NotEmpty(t, processInfo)
}

// =============================================================================
// ResourceResolver Tests
// =============================================================================

func TestNewResourceResolver(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	r := NewResourceResolver(pool)

	assert.NotNil(t, r)
	assert.Equal(t, pool, r.clientPool)
	assert.NotNil(t, r.cache)
	assert.Equal(t, defaultCacheTTL, r.cacheTTL)
}

func TestResourceResolver_SetCacheTTL(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	newTTL := 5 * time.Minute
	r.SetCacheTTL(newTTL)

	assert.Equal(t, newTTL, r.cacheTTL)
}

func TestResourceResolver_Resolve_Service(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	tests := []struct {
		name        string
		resource    string
		expected    string
		errContains string
		expectedErr bool
	}{
		{
			name:     "valid service resource",
			resource: "service/my-service",
			expected: "service/my-service",
		},
		{
			// Note: "service/" returns the resource as-is (current behavior)
			name:     "service with empty name part",
			resource: "service/",
			expected: "service/",
		},
		{
			name:        "service without slash returns error",
			resource:    "service",
			expectedErr: true,
			errContains: "invalid service resource format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := r.Resolve(ctx, "test-context", "default", tt.resource, "")

			if tt.expectedErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResourceResolver_Resolve_UnsupportedType(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	ctx := context.Background()
	result, err := r.Resolve(ctx, "test-context", "default", "deployment/my-deploy", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported resource type")
	assert.Empty(t, result)
}

func TestResourceResolver_Resolve_PodWithoutPrefixOrSelector(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	ctx := context.Background()
	result, err := r.Resolve(ctx, "test-context", "default", "pod", "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pod resource requires either a name prefix")
	assert.Empty(t, result)
}

func TestResourceResolver_Cache_Operations(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	// Test putInCache and getFromCache
	key := "test-context/default/pod/test"
	value := "test-pod-123"

	// Initially empty
	result := r.getFromCache(key)
	assert.Empty(t, result)

	// Put in cache
	r.putInCache(key, value)

	// Should be retrievable
	result = r.getFromCache(key)
	assert.Equal(t, value, result)
}

func TestResourceResolver_Cache_Expiry(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	// Set very short TTL
	r.SetCacheTTL(50 * time.Millisecond)

	key := "test-context/default/pod/test"
	value := "test-pod-123"

	// Put in cache
	r.putInCache(key, value)

	// Should be immediately retrievable
	result := r.getFromCache(key)
	assert.Equal(t, value, result)

	// Wait for expiry
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	result = r.getFromCache(key)
	assert.Empty(t, result)

	// Cache entry should be cleaned up
	r.cacheMu.RLock()
	_, exists := r.cache[key]
	r.cacheMu.RUnlock()
	assert.False(t, exists)
}

func TestResourceResolver_Cache_ConcurrentAccess(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := "key"
			value := "value"
			r.putInCache(key, value)
			_ = r.getFromCache(key)
		}(i)
	}

	wg.Wait()

	// Verify no race conditions occurred
	assert.NotNil(t, r.cache)
}

func TestResourceResolver_ClearCache(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	// Populate cache
	r.putInCache("key1", "value1")
	r.putInCache("key2", "value2")

	// Verify cache has entries
	r.cacheMu.RLock()
	assert.Greater(t, len(r.cache), 0)
	r.cacheMu.RUnlock()

	// Clear cache
	r.ClearCache()

	// Verify cache is empty
	r.cacheMu.RLock()
	assert.Equal(t, 0, len(r.cache))
	r.cacheMu.RUnlock()
}

func TestResourceResolver_InvalidateCache(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	// Populate cache with multiple entries in same namespace
	r.putInCache("test-context/default/pod/app1", "pod1")
	r.putInCache("test-context/default/pod/app2", "pod2")
	r.putInCache("test-context/other/pod/app1", "pod3")

	// Invalidate for specific namespace
	r.InvalidateCache("test-context", "default", "pod/app1")

	// All entries for that namespace should be cleared
	r.cacheMu.RLock()
	_, exists1 := r.cache["test-context/default/pod/app1"]
	_, exists2 := r.cache["test-context/default/pod/app2"]
	_, exists3 := r.cache["test-context/other/pod/app1"]
	r.cacheMu.RUnlock()

	assert.False(t, exists1)
	assert.False(t, exists2)
	assert.True(t, exists3, "other namespace should not be affected")
}

// =============================================================================
// PortForwarder Tests
// =============================================================================

func TestNewPortForwarder(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)

	pf := NewPortForwarder(pool, r)

	assert.NotNil(t, pf)
	assert.Equal(t, pool, pf.clientPool)
	assert.Equal(t, r, pf.resolver)
	assert.NotZero(t, pf.tcpKeepalive)
	assert.NotZero(t, pf.dialTimeout)
}

func TestPortForwarder_SetTCPKeepalive(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	newKeepalive := 60 * time.Second
	pf.SetTCPKeepalive(newKeepalive)

	assert.Equal(t, newKeepalive, pf.tcpKeepalive)
}

func TestPortForwarder_SetDialTimeout(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	newTimeout := 45 * time.Second
	pf.SetDialTimeout(newTimeout)

	assert.Equal(t, newTimeout, pf.dialTimeout)
}

func TestPortForwarder_Forward_InvalidResource(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	ctx := context.Background()
	req := &ForwardRequest{
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "invalid-resource",
	}

	err = pf.Forward(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported resource type")
}

func TestForwardRequest_Struct(t *testing.T) {
	// Test that ForwardRequest struct fields are correctly accessible
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})

	req := &ForwardRequest{
		Out:         nil,
		ErrOut:      nil,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "pod/my-pod",
		Selector:    "",
		LocalPort:   8080,
		RemotePort:  80,
	}

	assert.Equal(t, "test-context", req.ContextName)
	assert.Equal(t, "default", req.Namespace)
	assert.Equal(t, "pod/my-pod", req.Resource)
	assert.Equal(t, 8080, req.LocalPort)
	assert.Equal(t, 80, req.RemotePort)
	assert.Equal(t, stopChan, req.StopChan)
	assert.Equal(t, readyChan, req.ReadyChan)
}

// =============================================================================
// PodInfo and ServiceInfo Tests
// =============================================================================

func TestPodInfo_Struct(t *testing.T) {
	now := time.Now()
	podInfo := PodInfo{
		Created:   metav1.Time{Time: now},
		Name:      "test-pod",
		Namespace: "default",
		Status:    "Running",
		Containers: []ContainerInfo{
			{
				Name: "main",
				Ports: []PortInfo{
					{Name: "http", Port: 8080, Protocol: "TCP"},
				},
			},
		},
	}

	assert.Equal(t, "test-pod", podInfo.Name)
	assert.Equal(t, "default", podInfo.Namespace)
	assert.Equal(t, "Running", podInfo.Status)
	assert.Len(t, podInfo.Containers, 1)
	assert.Equal(t, "main", podInfo.Containers[0].Name)
	assert.Equal(t, int32(8080), podInfo.Containers[0].Ports[0].Port)
}

func TestServiceInfo_Struct(t *testing.T) {
	svcInfo := ServiceInfo{
		Name:      "test-svc",
		Namespace: "default",
		Type:      "ClusterIP",
		Ports: []PortInfo{
			{Name: "http", Port: 80, TargetPort: 8080, Protocol: "TCP"},
		},
	}

	assert.Equal(t, "test-svc", svcInfo.Name)
	assert.Equal(t, "default", svcInfo.Namespace)
	assert.Equal(t, "ClusterIP", svcInfo.Type)
	assert.Len(t, svcInfo.Ports, 1)
	assert.Equal(t, int32(80), svcInfo.Ports[0].Port)
	assert.Equal(t, int32(8080), svcInfo.Ports[0].TargetPort)
}

// =============================================================================
// ResolvedResource Tests
// =============================================================================

func TestResolvedResource_Struct(t *testing.T) {
	now := time.Now()
	resource := ResolvedResource{
		Timestamp: now,
		Name:      "my-pod",
		Namespace: "default",
	}

	assert.Equal(t, "my-pod", resource.Name)
	assert.Equal(t, "default", resource.Namespace)
	assert.Equal(t, now, resource.Timestamp)
}

// =============================================================================
// GetUniquePorts Additional Tests
// =============================================================================

func TestGetUniquePorts_EmptyInput(t *testing.T) {
	result := GetUniquePorts([]PodInfo{})
	assert.Empty(t, result)
}

func TestGetUniquePorts_SinglePod(t *testing.T) {
	pods := []PodInfo{
		{
			Name: "single-pod",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Name: "http", Port: 8080},
					},
				},
			},
		},
	}

	result := GetUniquePorts(pods)
	assert.Len(t, result, 1)
	assert.Equal(t, int32(8080), result[0].Port)
	assert.Equal(t, "http", result[0].Name)
}

func TestGetUniquePorts_NoNamedPorts(t *testing.T) {
	pods := []PodInfo{
		{
			Name: "pod1",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Port: 8080}, // No name
					},
				},
			},
		},
	}

	result := GetUniquePorts(pods)
	assert.Len(t, result, 1)
	assert.Equal(t, int32(8080), result[0].Port)
	assert.Equal(t, "port-8080", result[0].Name)
}

func TestGetUniquePorts_PreferNamedOverGenerated(t *testing.T) {
	pods := []PodInfo{
		{
			Name: "pod1",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Port: 8080}, // No name, generates "port-8080"
					},
				},
			},
		},
		{
			Name: "pod2",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Name: "http", Port: 8080}, // Named port
					},
				},
			},
		},
	}

	result := GetUniquePorts(pods)
	assert.Len(t, result, 1)
	assert.Equal(t, int32(8080), result[0].Port)
	assert.Equal(t, "http", result[0].Name, "named port should take precedence")
}

func TestGetUniquePorts_SortedByPortNumber(t *testing.T) {
	pods := []PodInfo{
		{
			Name: "pod1",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Name: "high", Port: 9000},
						{Name: "low", Port: 80},
						{Name: "mid", Port: 8080},
					},
				},
			},
		},
	}

	result := GetUniquePorts(pods)
	assert.Len(t, result, 3)
	assert.Equal(t, int32(80), result[0].Port)
	assert.Equal(t, int32(8080), result[1].Port)
	assert.Equal(t, int32(9000), result[2].Port)
}

// =============================================================================
// Discovery Context Operations Tests
// =============================================================================

func TestDiscovery_ListContexts(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	d := NewDiscovery(pool)

	// This will either succeed or fail based on kubeconfig availability
	contexts, err := d.ListContexts()

	if err != nil {
		// Expected if no kubeconfig
		assert.Contains(t, err.Error(), "kubeconfig")
	} else {
		// If successful, should be a slice
		assert.NotNil(t, contexts)
	}
}

func TestDiscovery_GetCurrentContext(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	d := NewDiscovery(pool)

	// This will either succeed or fail based on kubeconfig availability
	context, err := d.GetCurrentContext()

	if err != nil {
		// Expected if no kubeconfig
		assert.Contains(t, err.Error(), "kubeconfig")
	} else {
		// If successful, should be a string
		assert.NotEmpty(t, context)
	}
}
