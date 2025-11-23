package k8s

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwarder handles Kubernetes port-forwarding operations.
type PortForwarder struct {
	clientPool *ClientPool
	resolver   *ResourceResolver
}

// NewPortForwarder creates a new PortForwarder instance.
func NewPortForwarder(clientPool *ClientPool, resolver *ResourceResolver) *PortForwarder {
	return &PortForwarder{
		clientPool: clientPool,
		resolver:   resolver,
	}
}

// ForwardRequest contains the parameters for a port-forward request.
type ForwardRequest struct {
	ContextName string // Kubernetes context name
	Namespace   string // Namespace
	Resource    string // Resource (pod/name or service/name)
	Selector    string // Label selector (for pod resolution)
	LocalPort   int    // Local port
	RemotePort  int    // Remote port
	StopChan    chan struct{}
	ReadyChan   chan struct{}
	Out         io.Writer // Output writer for logs
	ErrOut      io.Writer // Error output writer
}

// Forward establishes a port-forward connection to a Kubernetes resource.
// It supports both pod and service forwarding.
// The connection runs until StopChan is closed or an error occurs.
func (pf *PortForwarder) Forward(ctx context.Context, req *ForwardRequest) error {
	// Resolve the resource to an actual pod name
	resolvedResource, err := pf.resolver.Resolve(ctx, req.ContextName, req.Namespace, req.Resource, req.Selector)
	if err != nil {
		return fmt.Errorf("failed to resolve resource: %w", err)
	}

	// Parse the resolved resource
	parts := strings.SplitN(resolvedResource, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid resolved resource format: %s", resolvedResource)
	}

	resourceType := parts[0]
	resourceName := parts[1]

	// Handle different resource types
	switch resourceType {
	case "pod":
		return pf.forwardToPod(ctx, req, resourceName)
	case "service":
		return pf.forwardToService(ctx, req, resourceName)
	default:
		return fmt.Errorf("unsupported resource type: %s", resourceType)
	}
}

// forwardToPod establishes a port-forward to a specific pod.
func (pf *PortForwarder) forwardToPod(ctx context.Context, req *ForwardRequest, podName string) error {
	// Get Kubernetes client and config
	client, err := pf.clientPool.GetClient(req.ContextName)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	config, err := pf.clientPool.GetRestConfig(req.ContextName)
	if err != nil {
		return fmt.Errorf("failed to get rest config: %w", err)
	}

	// Verify pod exists and is running
	pod, err := client.CoreV1().Pods(req.Namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod: %w", err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		return fmt.Errorf("pod is not running (current phase: %s)", pod.Status.Phase)
	}

	// Build the port-forward URL
	reqURL := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(req.Namespace).
		Name(podName).
		SubResource("portforward").
		URL()

	// Create the port-forward
	return pf.executePortForward(config, reqURL, req)
}

// forwardToService establishes a port-forward to a service.
// This resolves the service to its backing pods and forwards to one of them.
func (pf *PortForwarder) forwardToService(ctx context.Context, req *ForwardRequest, serviceName string) error {
	// Get Kubernetes client
	client, err := pf.clientPool.GetClient(req.ContextName)
	if err != nil {
		return fmt.Errorf("failed to get client: %w", err)
	}

	// Get the service
	service, err := client.CoreV1().Services(req.Namespace).Get(ctx, serviceName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	// Get pods backing the service using label selector
	selector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: service.Spec.Selector})
	pods, err := client.CoreV1().Pods(req.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods for service: %w", err)
	}

	// Find first running pod
	var targetPod *corev1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodRunning {
			targetPod = pod
			break
		}
	}

	if targetPod == nil {
		return fmt.Errorf("no running pods found for service %s", serviceName)
	}

	// Forward to the pod
	config, err := pf.clientPool.GetRestConfig(req.ContextName)
	if err != nil {
		return fmt.Errorf("failed to get rest config: %w", err)
	}

	reqURL := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(req.Namespace).
		Name(targetPod.Name).
		SubResource("portforward").
		URL()

	return pf.executePortForward(config, reqURL, req)
}

// executePortForward performs the actual port-forward operation.
func (pf *PortForwarder) executePortForward(config *rest.Config, url *url.URL, req *ForwardRequest) error {
	// Create SPDY roundtripper
	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return fmt.Errorf("failed to create round tripper: %w", err)
	}

	// Create dialer
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	// Set up port forwarding
	ports := []string{fmt.Sprintf("%d:%d", req.LocalPort, req.RemotePort)}

	// Create output writers
	out := req.Out
	errOut := req.ErrOut
	if out == nil {
		out = io.Discard
	}
	if errOut == nil {
		errOut = io.Discard
	}

	// Create port forwarder
	fw, err := portforward.New(dialer, ports, req.StopChan, req.ReadyChan, out, errOut)
	if err != nil {
		return fmt.Errorf("failed to create port forwarder: %w", err)
	}

	// Start forwarding (blocks until stopped or error)
	if err := fw.ForwardPorts(); err != nil {
		return fmt.Errorf("port forward failed: %w", err)
	}

	return nil
}

// GetPodForResource returns the pod name that would be used for forwarding.
// This is useful for logging and debugging.
func (pf *PortForwarder) GetPodForResource(ctx context.Context, contextName, namespace, resource, selector string) (string, error) {
	resolvedResource, err := pf.resolver.Resolve(ctx, contextName, namespace, resource, selector)
	if err != nil {
		return "", err
	}

	parts := strings.SplitN(resolvedResource, "/", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid resolved resource format: %s", resolvedResource)
	}

	resourceType := parts[0]
	resourceName := parts[1]

	if resourceType == "service" {
		// For services, need to resolve to backing pod
		client, err := pf.clientPool.GetClient(contextName)
		if err != nil {
			return "", fmt.Errorf("failed to get client: %w", err)
		}

		service, err := client.CoreV1().Services(namespace).Get(ctx, resourceName, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("failed to get service: %w", err)
		}

		selector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: service.Spec.Selector})
		pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: selector,
		})
		if err != nil {
			return "", fmt.Errorf("failed to list pods: %w", err)
		}

		for i := range pods.Items {
			if pods.Items[i].Status.Phase == corev1.PodRunning {
				return pods.Items[i].Name, nil
			}
		}

		return "", fmt.Errorf("no running pods found for service")
	}

	return resourceName, nil
}
