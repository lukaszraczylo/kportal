package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
)

func TestResolveTargetPort(t *testing.T) {
	tests := []struct {
		name         string
		servicePort  corev1.ServicePort
		service      *corev1.Service
		pods         []corev1.Pod
		expectedPort int32
		description  string
	}{
		{
			name: "numeric targetPort",
			servicePort: corev1.ServicePort{
				Port:       80,
				TargetPort: intstr.FromInt(8000),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
				},
			},
			pods:         nil, // No pods needed for numeric targetPort
			expectedPort: 8000,
			description:  "should use numeric targetPort directly",
		},
		{
			name: "named targetPort resolved from pod",
			servicePort: corev1.ServicePort{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromString("http"),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
								Ports: []corev1.ContainerPort{
									{Name: "http", ContainerPort: 8000},
								},
							},
						},
					},
				},
			},
			expectedPort: 8000,
			description:  "should resolve named port from pod container",
		},
		{
			name: "targetPort not set - defaults to service port",
			servicePort: corev1.ServicePort{
				Port:       80,
				TargetPort: intstr.FromInt(0), // Not set
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
				},
			},
			pods:         nil,
			expectedPort: 80,
			description:  "should fall back to service port when targetPort is not set",
		},
		{
			name: "named targetPort with no matching pod",
			servicePort: corev1.ServicePort{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromString("http"),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
				},
			},
			pods:         nil, // No pods available
			expectedPort: 80,
			description:  "should fall back to service port when no pods found",
		},
		{
			name: "service without selector",
			servicePort: corev1.ServicePort{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromString("http"),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: nil, // No selector
				},
			},
			pods:         nil,
			expectedPort: 80,
			description:  "should fall back to service port when service has no selector",
		},
		{
			name: "named targetPort not found in pod containers",
			servicePort: corev1.ServicePort{
				Name:       "http",
				Port:       80,
				TargetPort: intstr.FromString("nonexistent"),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
								Ports: []corev1.ContainerPort{
									{Name: "http", ContainerPort: 8000},
								},
							},
						},
					},
				},
			},
			expectedPort: 80,
			description:  "should fall back to service port when named port not found in pod",
		},
		{
			name: "multiple containers with named port in second container",
			servicePort: corev1.ServicePort{
				Name:       "metrics",
				Port:       9090,
				TargetPort: intstr.FromString("metrics"),
			},
			service: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
				},
				Spec: corev1.ServiceSpec{
					Selector: map[string]string{"app": "test"},
				},
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels:    map[string]string{"app": "test"},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name: "main",
								Ports: []corev1.ContainerPort{
									{Name: "http", ContainerPort: 8000},
								},
							},
							{
								Name: "sidecar",
								Ports: []corev1.ContainerPort{
									{Name: "metrics", ContainerPort: 9100},
								},
							},
						},
					},
				},
			},
			expectedPort: 9100,
			description:  "should find named port in any container",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fake client with pods
			var objects []runtime.Object
			for i := range tt.pods {
				objects = append(objects, &tt.pods[i])
			}
			fakeClient := fake.NewSimpleClientset(objects...)

			// Create discovery instance (we only need it to call resolveTargetPort)
			d := &Discovery{}

			// Call resolveTargetPort
			result := d.resolveTargetPort(
				context.Background(),
				fakeClient,
				"default",
				tt.service,
				&tt.servicePort,
			)

			assert.Equal(t, tt.expectedPort, result, tt.description)
		})
	}
}

func TestPortInfoTargetPort(t *testing.T) {
	// Test that PortInfo correctly stores TargetPort
	portInfo := PortInfo{
		Name:       "http",
		Port:       80,
		TargetPort: 8000,
		Protocol:   "TCP",
	}

	assert.Equal(t, int32(80), portInfo.Port)
	assert.Equal(t, int32(8000), portInfo.TargetPort)
	assert.Equal(t, "http", portInfo.Name)
	assert.Equal(t, "TCP", portInfo.Protocol)
}

func TestGetUniquePorts(t *testing.T) {
	// Test GetUniquePorts still works with the new PortInfo struct
	pods := []PodInfo{
		{
			Name: "pod1",
			Containers: []ContainerInfo{
				{
					Name: "main",
					Ports: []PortInfo{
						{Name: "http", Port: 8080},
						{Name: "metrics", Port: 9090},
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
						{Name: "http", Port: 8080}, // Duplicate
						{Name: "grpc", Port: 50051},
					},
				},
			},
		},
	}

	ports := GetUniquePorts(pods)

	// Should have 3 unique ports
	assert.Len(t, ports, 3)

	// Should be sorted by port number
	assert.Equal(t, int32(8080), ports[0].Port)
	assert.Equal(t, int32(9090), ports[1].Port)
	assert.Equal(t, int32(50051), ports[2].Port)

	// Names should be preserved
	assert.Equal(t, "http", ports[0].Name)
	assert.Equal(t, "metrics", ports[1].Name)
	assert.Equal(t, "grpc", ports[2].Name)
}
