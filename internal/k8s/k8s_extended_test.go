package k8s

import (
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
// ForwardRequest Tests
// =============================================================================

func TestForwardRequest_Fields(t *testing.T) {
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})
	outWriter := &mockWriter{}
	errWriter := &mockWriter{}

	req := &ForwardRequest{
		Out:         outWriter,
		ErrOut:      errWriter,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		ContextName: "test-context",
		Namespace:   "test-namespace",
		Resource:    "pod/test-pod",
		Selector:    "app=test",
		LocalPort:   8080,
		RemotePort:  80,
	}

	assert.Equal(t, outWriter, req.Out)
	assert.Equal(t, errWriter, req.ErrOut)
	assert.Equal(t, stopChan, req.StopChan)
	assert.Equal(t, readyChan, req.ReadyChan)
	assert.Equal(t, "test-context", req.ContextName)
	assert.Equal(t, "test-namespace", req.Namespace)
	assert.Equal(t, "pod/test-pod", req.Resource)
	assert.Equal(t, "app=test", req.Selector)
	assert.Equal(t, 8080, req.LocalPort)
	assert.Equal(t, 80, req.RemotePort)
}

func TestForwardRequest_NilWriters(t *testing.T) {
	stopChan := make(chan struct{})
	readyChan := make(chan struct{})

	req := &ForwardRequest{
		Out:         nil,
		ErrOut:      nil,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "pod/test-pod",
		LocalPort:   8080,
		RemotePort:  80,
	}

	// nil writers should be acceptable
	assert.Nil(t, req.Out)
	assert.Nil(t, req.ErrOut)
}

// mockWriter is a test double for io.Writer
type mockWriter struct {
	written []byte
}

func (m *mockWriter) Write(p []byte) (n int, err error) {
	m.written = append(m.written, p...)
	return len(p), nil
}

// =============================================================================
// PortForwarder Extended Tests
// =============================================================================

func TestPortForwarder_ForwardRequestValidation(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	ctx := t.Context()

	tests := []struct {
		name        string
		resource    string
		errContains string
		expectedErr bool
	}{
		{
			name:        "invalid resource format - no slash",
			resource:    "invalid",
			expectedErr: true,
			errContains: "unsupported resource type",
		},
		{
			name:        "unsupported resource type",
			resource:    "deployment/my-deployment",
			expectedErr: true,
			errContains: "unsupported resource type",
		},
		{
			name:        "empty resource",
			resource:    "",
			expectedErr: true,
			errContains: "unsupported resource type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stopChan := make(chan struct{})
			req := &ForwardRequest{
				StopChan:    stopChan,
				ContextName: "test-context",
				Namespace:   "default",
				Resource:    tt.resource,
				LocalPort:   8080,
				RemotePort:  80,
			}

			err := pf.Forward(ctx, req)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// =============================================================================
// Discovery Method Tests (with fake client integration)
// =============================================================================

func TestDiscovery_ListNamespaces_WithFakeClient(t *testing.T) {
	objects := []runtime.Object{
		createTestNamespace("default"),
		createTestNamespace("kube-system"),
		createTestNamespace("production"),
	}

	fakeClient := fake.NewClientset(objects...)

	ctx := t.Context()
	nsList, err := fakeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	require.NoError(t, err)

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}

	assert.Len(t, namespaces, 3)
	assert.Contains(t, namespaces, "default")
	assert.Contains(t, namespaces, "kube-system")
	assert.Contains(t, namespaces, "production")
}

func TestDiscovery_ListServices_WithPorts(t *testing.T) {
	objects := []runtime.Object{
		createTestService("web-svc", "default", map[string]string{"app": "web"}, []corev1.ServicePort{
			{Name: "http", Port: 80, TargetPort: intstr.FromInt(8080)},
			{Name: "https", Port: 443, TargetPort: intstr.FromInt(8443)},
		}),
		createTestService("api-svc", "default", map[string]string{"app": "api"}, []corev1.ServicePort{
			{Port: 8080, TargetPort: intstr.FromInt(8080)},
		}),
	}

	fakeClient := fake.NewClientset(objects...)

	ctx := t.Context()
	svcList, err := fakeClient.CoreV1().Services("default").List(ctx, metav1.ListOptions{})

	require.NoError(t, err)
	assert.Len(t, svcList.Items, 2)

	// Verify service with multiple ports
	var webSvc *corev1.Service
	for i := range svcList.Items {
		if svcList.Items[i].Name == "web-svc" {
			webSvc = &svcList.Items[i]
			break
		}
	}
	require.NotNil(t, webSvc)
	assert.Len(t, webSvc.Spec.Ports, 2)

	// Verify port details
	foundHTTP := false
	foundHTTPS := false
	for _, port := range webSvc.Spec.Ports {
		if port.Name == "http" {
			foundHTTP = true
			assert.Equal(t, int32(80), port.Port)
			assert.Equal(t, int32(8080), port.TargetPort.IntVal)
		}
		if port.Name == "https" {
			foundHTTPS = true
			assert.Equal(t, int32(443), port.Port)
			assert.Equal(t, int32(8443), port.TargetPort.IntVal)
		}
	}
	assert.True(t, foundHTTP, "http port not found")
	assert.True(t, foundHTTPS, "https port not found")
}

// =============================================================================
// ContainerInfo and PortInfo Tests
// =============================================================================

func TestContainerInfo_Struct(t *testing.T) {
	container := ContainerInfo{
		Name: "test-container",
		Ports: []PortInfo{
			{Name: "http", Port: 8080, Protocol: "TCP"},
			{Name: "grpc", Port: 50051, Protocol: "TCP"},
		},
	}

	assert.Equal(t, "test-container", container.Name)
	assert.Len(t, container.Ports, 2)
	assert.Equal(t, "http", container.Ports[0].Name)
	assert.Equal(t, int32(8080), container.Ports[0].Port)
	assert.Equal(t, "TCP", container.Ports[0].Protocol)
}

func TestPortInfo_Struct(t *testing.T) {
	port := PortInfo{
		Name:       "test-port",
		Protocol:   "TCP",
		Port:       8080,
		TargetPort: 80,
	}

	assert.Equal(t, "test-port", port.Name)
	assert.Equal(t, "TCP", port.Protocol)
	assert.Equal(t, int32(8080), port.Port)
	assert.Equal(t, int32(80), port.TargetPort)
}

// =============================================================================
// GetUniquePorts Edge Cases
// =============================================================================

func TestGetUniquePorts_MultipleContainers(t *testing.T) {
	pods := []PodInfo{
		{
			Name: "pod1",
			Containers: []ContainerInfo{
				{
					Name: "app",
					Ports: []PortInfo{
						{Name: "http", Port: 8080},
					},
				},
				{
					Name: "sidecar",
					Ports: []PortInfo{
						{Name: "metrics", Port: 9090},
					},
				},
			},
		},
	}

	result := GetUniquePorts(pods)
	assert.Len(t, result, 2)

	ports := make([]int32, len(result))
	for i, p := range result {
		ports[i] = p.Port
	}
	assert.Contains(t, ports, int32(8080))
	assert.Contains(t, ports, int32(9090))
}

func TestGetUniquePorts_DuplicateAcrossPods(t *testing.T) {
	pods := []PodInfo{
		{
			Name: "pod1",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Name: "http", Port: 8080},
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
						{Name: "http", Port: 8080}, // Same port, same name
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

func TestGetUniquePorts_NamedVsUnnamedDuplicate(t *testing.T) {
	pods := []PodInfo{
		{
			Name: "pod1",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Port: 8080}, // Unnamed - generates "port-8080"
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
						{Name: "http", Port: 8080}, // Named - should take precedence
					},
				},
			},
		},
	}

	result := GetUniquePorts(pods)
	assert.Len(t, result, 1)
	assert.Equal(t, int32(8080), result[0].Port)
	assert.Equal(t, "http", result[0].Name, "named port should take precedence over generated name")
}

// =============================================================================
// Cache Entry Tests
// =============================================================================

func TestCacheEntry_Struct(t *testing.T) {
	now := time.Now()
	entry := cacheEntry{
		expiresAt: now.Add(30 * time.Second),
		resource: ResolvedResource{
			Timestamp: now,
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	assert.Equal(t, now.Add(30*time.Second), entry.expiresAt)
	assert.Equal(t, "test-pod", entry.resource.Name)
	assert.Equal(t, "default", entry.resource.Namespace)
	assert.Equal(t, now, entry.resource.Timestamp)
}

// =============================================================================
// ClientPool Extended Tests
// =============================================================================

func TestClientPool_ConcurrentAccess(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	var wg sync.WaitGroup

	// Concurrent reads and writes to cache
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			pool.ClearCache()
			pool.RemoveContext("context")
			_, _ = pool.GetCurrentContext()
			_, _ = pool.ListContexts()
		}(i)
	}

	wg.Wait()
	// If we get here without panic, concurrent access is safe
}

func TestClientPool_MultipleContexts(t *testing.T) {
	pool, err := NewClientPool()
	require.NoError(t, err)

	// Test that multiple contexts can be tracked
	pool.mu.Lock()
	pool.clients["context1"] = nil
	pool.clients["context2"] = nil
	pool.clients["context3"] = nil
	pool.mu.Unlock()

	// Remove one context
	pool.RemoveContext("context2")

	// Verify context2 is removed
	pool.mu.RLock()
	_, exists1 := pool.clients["context1"]
	_, exists2 := pool.clients["context2"]
	_, exists3 := pool.clients["context3"]
	pool.mu.RUnlock()

	assert.True(t, exists1)
	assert.False(t, exists2)
	assert.True(t, exists3)

	// Clear all
	pool.ClearCache()

	pool.mu.RLock()
	assert.Equal(t, 0, len(pool.clients))
	pool.mu.RUnlock()
}

// =============================================================================
// ResourceResolver Resolve Tests (using internal methods)
// =============================================================================

func TestResourceResolver_Resolve_InvalidFormat(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)

	ctx := t.Context()

	tests := []struct {
		name        string
		resource    string
		selector    string
		errContains string
	}{
		{
			name:        "unsupported resource type",
			resource:    "configmap/my-config",
			selector:    "",
			errContains: "unsupported resource type",
		},
		{
			name:        "pod without prefix or selector",
			resource:    "pod",
			selector:    "",
			errContains: "pod resource requires either a name prefix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := r.Resolve(ctx, "test-context", "default", tt.resource, tt.selector)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestResourceResolver_Resolve_ServiceVariations(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)

	ctx := t.Context()

	tests := []struct {
		name     string
		resource string
		expected string
	}{
		{
			name:     "simple service",
			resource: "service/my-service",
			expected: "service/my-service",
		},
		{
			name:     "service with namespace in name",
			resource: "service/my-service.namespace",
			expected: "service/my-service.namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := r.Resolve(ctx, "test-context", "default", tt.resource, "")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// resolveTargetPort Extended Tests
// =============================================================================

func TestResolveTargetPort_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		service     *corev1.Service
		servicePort corev1.ServicePort
		pods        []corev1.Pod
		expected    int32
	}{
		{
			name: "zero value targetPort returns service port",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
					Ports:    []corev1.ServicePort{{Port: 80}},
				},
			},
			servicePort: corev1.ServicePort{
				Port: 80,
				// TargetPort is zero value
			},
			pods:     nil,
			expected: 80,
		},
		{
			name: "empty named port returns service port",
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{Name: "svc", Namespace: "default"},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
				},
			},
			servicePort: corev1.ServicePort{
				Port:       80,
				TargetPort: intstr.FromString(""), // Empty string
			},
			pods:     nil,
			expected: 80,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []runtime.Object
			for i := range tt.pods {
				objects = append(objects, &tt.pods[i])
			}
			fakeClient := fake.NewClientset(objects...)
			d := &Discovery{}

			result := d.resolveTargetPort(t.Context(), fakeClient, "default", tt.service, &tt.servicePort)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// =============================================================================
// PortForwarder Settings Tests
// =============================================================================

func TestPortForwarder_DefaultSettings(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	// Verify defaults are set
	assert.NotZero(t, pf.tcpKeepalive)
	assert.NotZero(t, pf.dialTimeout)
}

func TestPortForwarder_SettingsChain(t *testing.T) {
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	// Chain multiple settings
	pf.SetTCPKeepalive(60 * time.Second)
	pf.SetDialTimeout(45 * time.Second)
	pf.SetTCPKeepalive(30 * time.Second) // Override

	assert.Equal(t, 30*time.Second, pf.tcpKeepalive)
	assert.Equal(t, 45*time.Second, pf.dialTimeout)
}
