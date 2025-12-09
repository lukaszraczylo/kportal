package k8s

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// Discovery provides cluster introspection capabilities for the UI wizards.
// It queries the Kubernetes API to list contexts, namespaces, pods, and services.
type Discovery struct {
	pool *ClientPool
}

// NewDiscovery creates a new Discovery instance using the provided client pool.
func NewDiscovery(pool *ClientPool) *Discovery {
	return &Discovery{
		pool: pool,
	}
}

// PodInfo contains information about a pod relevant for port forwarding.
type PodInfo struct {
	Name       string
	Namespace  string
	Containers []ContainerInfo
	Status     string
	Created    metav1.Time
}

// ContainerInfo contains information about a container within a pod.
type ContainerInfo struct {
	Name  string
	Ports []PortInfo
}

// PortInfo describes a port exposed by a container or service.
type PortInfo struct {
	Name       string
	Port       int32
	TargetPort int32 // For services: the actual pod port to forward to
	Protocol   string
}

// ServiceInfo contains information about a service.
type ServiceInfo struct {
	Name      string
	Namespace string
	Ports     []PortInfo
	Type      string
}

// ListContexts returns all available Kubernetes contexts from kubeconfig.
func (d *Discovery) ListContexts() ([]string, error) {
	return d.pool.ListContexts()
}

// GetCurrentContext returns the name of the current context from kubeconfig.
func (d *Discovery) GetCurrentContext() (string, error) {
	return d.pool.GetCurrentContext()
}

// ListNamespaces returns all namespaces in the given context.
// Returns an error if the context is invalid or unreachable.
func (d *Discovery) ListNamespaces(ctx context.Context, contextName string) ([]string, error) {
	client, err := d.pool.GetClient(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	nsList, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	namespaces := make([]string, 0, len(nsList.Items))
	for _, ns := range nsList.Items {
		namespaces = append(namespaces, ns.Name)
	}

	// Sort alphabetically
	sort.Strings(namespaces)

	return namespaces, nil
}

// ListPods returns all running pods in the given namespace with their port information.
// Only returns pods in Running or Pending state.
func (d *Discovery) ListPods(ctx context.Context, contextName, namespace string) ([]PodInfo, error) {
	client, err := d.pool.GetClient(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	pods := make([]PodInfo, 0)
	for _, pod := range podList.Items {
		// Only include Running or Pending pods
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}

		containers := make([]ContainerInfo, 0, len(pod.Spec.Containers))
		for _, container := range pod.Spec.Containers {
			ports := make([]PortInfo, 0, len(container.Ports))
			for _, port := range container.Ports {
				ports = append(ports, PortInfo{
					Name:     port.Name,
					Port:     port.ContainerPort,
					Protocol: string(port.Protocol),
				})
			}

			containers = append(containers, ContainerInfo{
				Name:  container.Name,
				Ports: ports,
			})
		}

		pods = append(pods, PodInfo{
			Name:       pod.Name,
			Namespace:  pod.Namespace,
			Containers: containers,
			Status:     string(pod.Status.Phase),
			Created:    pod.CreationTimestamp,
		})
	}

	// Sort by creation time (newest first)
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Created.After(pods[j].Created.Time)
	})

	return pods, nil
}

// ListPodsWithSelector returns pods matching the given label selector.
// Selector format: "key=value,key2=value2"
// Returns an error if the selector is invalid.
func (d *Discovery) ListPodsWithSelector(ctx context.Context, contextName, namespace, selector string) ([]PodInfo, error) {
	client, err := d.pool.GetClient(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	// Validate selector format
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, fmt.Errorf("selector cannot be empty")
	}

	podList, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list pods with selector: %w", err)
	}

	pods := make([]PodInfo, 0)
	for _, pod := range podList.Items {
		// Only include Running pods for selector-based forwards
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		containers := make([]ContainerInfo, 0, len(pod.Spec.Containers))
		for _, container := range pod.Spec.Containers {
			ports := make([]PortInfo, 0, len(container.Ports))
			for _, port := range container.Ports {
				ports = append(ports, PortInfo{
					Name:     port.Name,
					Port:     port.ContainerPort,
					Protocol: string(port.Protocol),
				})
			}

			containers = append(containers, ContainerInfo{
				Name:  container.Name,
				Ports: ports,
			})
		}

		pods = append(pods, PodInfo{
			Name:       pod.Name,
			Namespace:  pod.Namespace,
			Containers: containers,
			Status:     string(pod.Status.Phase),
			Created:    pod.CreationTimestamp,
		})
	}

	// Sort by creation time (newest first)
	sort.Slice(pods, func(i, j int) bool {
		return pods[i].Created.After(pods[j].Created.Time)
	})

	return pods, nil
}

// resolveTargetPort resolves a service's targetPort to an actual port number.
// If targetPort is numeric, it returns that number directly.
// If targetPort is a named port, it looks up the port number from the backing pods.
// Falls back to the service port if resolution fails.
func (d *Discovery) resolveTargetPort(ctx context.Context, client kubernetes.Interface, namespace string, svc *corev1.Service, port *corev1.ServicePort) int32 {
	// If targetPort is not set, Kubernetes defaults to the service port
	if port.TargetPort.Type == intstr.Int && port.TargetPort.IntVal == 0 {
		return port.Port
	}

	// If targetPort is numeric, use it directly
	if port.TargetPort.Type == intstr.Int {
		return port.TargetPort.IntVal
	}

	// targetPort is a named port - need to look up from pods
	namedPort := port.TargetPort.StrVal
	if namedPort == "" {
		return port.Port
	}

	// Get a backing pod to resolve the named port
	if len(svc.Spec.Selector) == 0 {
		// No selector, can't resolve - fall back to service port
		return port.Port
	}

	selector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: svc.Spec.Selector})
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
		Limit:         1, // We only need one pod to resolve the port name
	})
	if err != nil || len(pods.Items) == 0 {
		// Can't get pods - fall back to service port
		return port.Port
	}

	// Look up the named port in the pod's containers
	pod := &pods.Items[0]
	for _, container := range pod.Spec.Containers {
		for _, containerPort := range container.Ports {
			if containerPort.Name == namedPort {
				return containerPort.ContainerPort
			}
		}
	}

	// Named port not found - fall back to service port
	return port.Port
}

// ListServices returns all services in the given namespace.
// For each service port, it resolves the targetPort to an actual port number
// by looking up the backing pods when the targetPort is a named port.
func (d *Discovery) ListServices(ctx context.Context, contextName, namespace string) ([]ServiceInfo, error) {
	client, err := d.pool.GetClient(contextName)
	if err != nil {
		return nil, fmt.Errorf("failed to get client: %w", err)
	}

	svcList, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	services := make([]ServiceInfo, 0, len(svcList.Items))
	for _, svc := range svcList.Items {
		ports := make([]PortInfo, 0, len(svc.Spec.Ports))
		for _, port := range svc.Spec.Ports {
			targetPort := d.resolveTargetPort(ctx, client, namespace, &svc, &port)

			ports = append(ports, PortInfo{
				Name:       port.Name,
				Port:       port.Port,
				TargetPort: targetPort,
				Protocol:   string(port.Protocol),
			})
		}

		services = append(services, ServiceInfo{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Ports:     ports,
			Type:      string(svc.Spec.Type),
		})
	}

	// Sort alphabetically
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	return services, nil
}

// GetUniquePorts extracts unique ports from a list of pods.
// Returns a sorted list of port numbers with their names (if available).
func GetUniquePorts(pods []PodInfo) []PortInfo {
	portMap := make(map[int32]string)

	for _, pod := range pods {
		for _, container := range pod.Containers {
			for _, port := range container.Ports {
				// Prefer named ports
				if _, ok := portMap[port.Port]; !ok || port.Name != "" {
					if port.Name != "" {
						portMap[port.Port] = port.Name
					} else if !ok {
						portMap[port.Port] = fmt.Sprintf("port-%d", port.Port)
					}
				}
			}
		}
	}

	// Convert to slice
	ports := make([]PortInfo, 0, len(portMap))
	for port, name := range portMap {
		ports = append(ports, PortInfo{
			Name: name,
			Port: port,
		})
	}

	// Sort by port number
	sort.Slice(ports, func(i, j int) bool {
		return ports[i].Port < ports[j].Port
	})

	return ports
}

// CheckPortAvailability checks if a local port is available.
// Returns: available (bool), processInfo (string), error
func CheckPortAvailability(port int) (bool, string, error) {
	if port < 1 || port > 65535 {
		return false, "", fmt.Errorf("invalid port: %d", port)
	}

	// Try to listen on the port
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		// Port is in use - return error details
		return false, err.Error(), nil
	}

	// Port is available, close the listener
	_ = listener.Close()
	return true, "", nil
}
