package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// =============================================================================
// PortForwarder Extended Tests
// =============================================================================

func TestPortForwarder_Forward_ServiceResolutionError(t *testing.T) {
	// Create pool without any pods/services
	pool := setupTestPool(t, "test-context")

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	stopChan := make(chan struct{})
	req := &ForwardRequest{
		StopChan:    stopChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "service/nonexistent-svc",
		LocalPort:   8080,
		RemotePort:  80,
	}

	err := pf.Forward(t.Context(), req)
	assert.Error(t, err)
	// Should fail trying to get the service
	assert.Contains(t, err.Error(), "failed to get service")
}

func TestPortForwarder_Forward_PodNotRunning(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pending-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	stopChan := make(chan struct{})
	req := &ForwardRequest{
		StopChan:    stopChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "pod/pending-pod",
		LocalPort:   8080,
		RemotePort:  80,
	}

	err := pf.Forward(t.Context(), req)
	assert.Error(t, err)
	// Since pod is not running, it won't be found during resolution
	assert.Contains(t, err.Error(), "no running pods found")
}

func TestPortForwarder_Forward_PodPhaseCheck(t *testing.T) {
	// Create a running pod for resolution
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	stopChan := make(chan struct{})
	req := &ForwardRequest{
		StopChan:    stopChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "pod/test-pod",
		LocalPort:   8080,
		RemotePort:  80,
	}

	err := pf.Forward(t.Context(), req)
	// Will fail on port-forward since we can't actually forward
	// but the pod phase check should have passed
	assert.Error(t, err)
	// Error should not be about pod not running
	assert.NotContains(t, err.Error(), "pod is not running")
}

func TestPortForwarder_Forward_UnsupportedResourceType(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	stopChan := make(chan struct{})
	req := &ForwardRequest{
		StopChan:    stopChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "deployment/my-deploy",
		LocalPort:   8080,
		RemotePort:  80,
	}

	err := pf.Forward(t.Context(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported resource type")
}

func TestPortForwarder_Forward_GetClientError(t *testing.T) {
	// Create pool without setting test client
	pool, _ := NewClientPool()
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	stopChan := make(chan struct{})
	req := &ForwardRequest{
		StopChan:    stopChan,
		ContextName: "non-existent-context",
		Namespace:   "default",
		Resource:    "service/my-service",
		LocalPort:   8080,
		RemotePort:  80,
	}

	err := pf.Forward(t.Context(), req)
	assert.Error(t, err)
	// Will fail trying to get client (via resolver)
	assert.Contains(t, err.Error(), "failed to get client")
}

func TestPortForwarder_GetPodForResource_ServiceNotFound(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	_, err := pf.GetPodForResource(t.Context(), "test-context", "default", "service/nonexistent", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get service")
}

func TestPortForwarder_GetPodForResource_UnsupportedType(t *testing.T) {
	pool := setupTestPool(t, "test-context")

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	_, err := pf.GetPodForResource(t.Context(), "test-context", "default", "deployment/my-deploy", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported resource type")
}

func TestPortForwarder_GetPodForResource_DirectPod(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pod",
				Namespace: "default",
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	// For pod resources, GetPodForResource returns the pod name directly
	podName, err := pf.GetPodForResource(t.Context(), "test-context", "default", "pod/test-pod", "")
	require.NoError(t, err)
	assert.Equal(t, "test-pod", podName)
}

func TestPortForwarder_ForwardRequest_DefaultChannels(t *testing.T) {
	// Test that ForwardRequest can be created without channels
	req := &ForwardRequest{
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "pod/my-pod",
		LocalPort:   8080,
		RemotePort:  80,
		// StopChan and ReadyChan not set
	}

	assert.Nil(t, req.StopChan)
	assert.Nil(t, req.ReadyChan)
	assert.Nil(t, req.Out)
	assert.Nil(t, req.ErrOut)
}

func TestPortForwarder_Settings(t *testing.T) {
	pool := setupTestPool(t, "test-context")
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	// Test TCP keepalive setting
	pf.SetTCPKeepalive(30 * 1000000000) // 30 seconds in nanoseconds

	// Test dial timeout setting
	pf.SetDialTimeout(10 * 1000000000) // 10 seconds in nanoseconds

	// Just verify they don't panic
	assert.NotNil(t, pf)
}

func TestPortForwarder_Forward_GetPodError(t *testing.T) {
	pool := setupTestPool(t, "test-context")
	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	stopChan := make(chan struct{})
	req := &ForwardRequest{
		StopChan:    stopChan,
		ContextName: "test-context",
		Namespace:   "default",
		Resource:    "pod/nonexistent-prefix-xyz",
		LocalPort:   8080,
		RemotePort:  80,
	}

	err := pf.Forward(t.Context(), req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to resolve resource")
}

func TestPortForwarder_ForwardToService_NoRunningPods(t *testing.T) {
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
					{Port: 80},
				},
			},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

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
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pods found for service")
}

func TestPortForwarder_GetPodForResource_ServiceWithRunningPod(t *testing.T) {
	pool := setupTestPool(t, "test-context",
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "running-pod",
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
					{Port: 80},
				},
			},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	podName, err := pf.GetPodForResource(t.Context(), "test-context", "default", "service/backend-svc", "")
	require.NoError(t, err)
	assert.Equal(t, "running-pod", podName)
}

func TestPortForwarder_GetPodForResource_ServicePendingPod(t *testing.T) {
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
					{Port: 80},
				},
			},
		},
	)

	r := NewResourceResolver(pool)
	pf := NewPortForwarder(pool, r)

	_, err := pf.GetPodForResource(t.Context(), "test-context", "default", "service/backend-svc", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no running pods found for service")
}
