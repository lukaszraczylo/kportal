package k8s

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	Name     string
	Port     int32
	Protocol string
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

// ListServices returns all services in the given namespace.
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
			ports = append(ports, PortInfo{
				Name:     port.Name,
				Port:     port.Port,
				Protocol: string(port.Protocol),
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
		// Port is in use
		// Try to get process info (best-effort)
		processInfo := "unknown process"
		// Note: Getting process info requires platform-specific code
		// For now, just return a generic message
		return false, processInfo, nil
	}

	// Port is available, close the listener
	listener.Close()
	return true, "", nil
}

// ValidatePort checks if a port number is valid.
func ValidatePort(portStr string) (int, error) {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0, fmt.Errorf("invalid port number: %s", portStr)
	}

	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}

	return port, nil
}
