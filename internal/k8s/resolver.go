package k8s

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// Default cache TTL for resolved resources
	defaultCacheTTL = 30 * time.Second
)

// ResolvedResource represents a resolved Kubernetes resource.
type ResolvedResource struct {
	Name      string    // The resolved pod or service name
	Namespace string    // The namespace
	Timestamp time.Time // When this was resolved
}

// cacheEntry stores a cached resolution result with expiry.
type cacheEntry struct {
	resource  ResolvedResource
	expiresAt time.Time
}

// ResourceResolver resolves Kubernetes resources with caching.
// It handles prefix matching for pods and label selector resolution.
type ResourceResolver struct {
	clientPool *ClientPool
	cache      map[string]cacheEntry // key: contextName/namespace/resource -> resolved name
	cacheMu    sync.RWMutex
	cacheTTL   time.Duration
}

// NewResourceResolver creates a new ResourceResolver instance.
func NewResourceResolver(clientPool *ClientPool) *ResourceResolver {
	return &ResourceResolver{
		clientPool: clientPool,
		cache:      make(map[string]cacheEntry),
		cacheTTL:   defaultCacheTTL,
	}
}

// SetCacheTTL sets the cache TTL for resolved resources.
func (r *ResourceResolver) SetCacheTTL(ttl time.Duration) {
	r.cacheTTL = ttl
}

// Resolve resolves a resource name to an actual pod or service name.
// It supports:
// - pod/prefix: Prefix matching (e.g., "pod/my-app" matches "my-app-xyz789")
// - pod + selector: Label selector matching (e.g., "pod" with selector "app=nginx")
// - service/name: Direct service name (no resolution needed)
func (r *ResourceResolver) Resolve(ctx context.Context, contextName, namespace, resource, selector string) (string, error) {
	// Parse resource type and name
	parts := strings.SplitN(resource, "/", 2)
	resourceType := parts[0]

	// Services don't need resolution
	if resourceType == "service" {
		if len(parts) < 2 {
			return "", fmt.Errorf("invalid service resource format: %s", resource)
		}
		return resource, nil
	}

	// Handle pod resolution
	if resourceType == "pod" {
		if len(parts) == 2 {
			// pod/prefix format - prefix matching
			prefix := parts[1]
			return r.resolvePodPrefix(ctx, contextName, namespace, prefix)
		}

		// pod with selector - label selector matching
		if selector != "" {
			return r.resolvePodSelector(ctx, contextName, namespace, selector)
		}

		return "", fmt.Errorf("pod resource requires either a name prefix (pod/name) or a selector")
	}

	return "", fmt.Errorf("unsupported resource type: %s", resourceType)
}

// resolvePodPrefix resolves a pod name using prefix matching.
// It returns the newest running pod that matches the prefix.
func (r *ResourceResolver) resolvePodPrefix(ctx context.Context, contextName, namespace, prefix string) (string, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s/%s/pod/%s", contextName, namespace, prefix)
	if cached := r.getFromCache(cacheKey); cached != "" {
		return fmt.Sprintf("pod/%s", cached), nil
	}

	// Get Kubernetes client
	client, err := r.clientPool.GetClient(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client: %w", err)
	}

	// List all pods in the namespace
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	// Find pods matching the prefix
	var matchingPods []*corev1.Pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if strings.HasPrefix(pod.Name, prefix) && pod.Status.Phase == corev1.PodRunning {
			matchingPods = append(matchingPods, pod)
		}
	}

	if len(matchingPods) == 0 {
		return "", fmt.Errorf("no running pods found matching prefix '%s' in namespace %s", prefix, namespace)
	}

	// Sort by creation timestamp (newest first)
	sort.Slice(matchingPods, func(i, j int) bool {
		return matchingPods[i].CreationTimestamp.After(matchingPods[j].CreationTimestamp.Time)
	})

	// Return the newest pod
	resolvedName := matchingPods[0].Name
	r.putInCache(cacheKey, resolvedName)

	return fmt.Sprintf("pod/%s", resolvedName), nil
}

// resolvePodSelector resolves a pod name using label selectors.
// It returns the first running pod matching the selector.
func (r *ResourceResolver) resolvePodSelector(ctx context.Context, contextName, namespace, selector string) (string, error) {
	// Check cache first
	cacheKey := fmt.Sprintf("%s/%s/pod?selector=%s", contextName, namespace, selector)
	if cached := r.getFromCache(cacheKey); cached != "" {
		return fmt.Sprintf("pod/%s", cached), nil
	}

	// Get Kubernetes client
	client, err := r.clientPool.GetClient(contextName)
	if err != nil {
		return "", fmt.Errorf("failed to get client: %w", err)
	}

	// List pods matching the selector
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list pods with selector '%s': %w", selector, err)
	}

	// Find first running pod
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodRunning {
			resolvedName := pod.Name
			r.putInCache(cacheKey, resolvedName)
			return fmt.Sprintf("pod/%s", resolvedName), nil
		}
	}

	return "", fmt.Errorf("no running pods found matching selector '%s' in namespace %s", selector, namespace)
}

// getFromCache retrieves a cached resolution result if it exists and hasn't expired.
func (r *ResourceResolver) getFromCache(key string) string {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	entry, exists := r.cache[key]
	if !exists {
		return ""
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		return ""
	}

	return entry.resource.Name
}

// putInCache stores a resolution result in the cache with TTL.
func (r *ResourceResolver) putInCache(key, value string) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	r.cache[key] = cacheEntry{
		resource: ResolvedResource{
			Name:      value,
			Timestamp: time.Now(),
		},
		expiresAt: time.Now().Add(r.cacheTTL),
	}
}

// ClearCache clears all cached resolution results.
func (r *ResourceResolver) ClearCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	r.cache = make(map[string]cacheEntry)
}

// InvalidateCache invalidates cache entries for a specific resource.
func (r *ResourceResolver) InvalidateCache(contextName, namespace, resource string) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	// Remove exact match
	delete(r.cache, fmt.Sprintf("%s/%s/%s", contextName, namespace, resource))

	// Remove prefix matches (for selector-based resources)
	prefix := fmt.Sprintf("%s/%s/", contextName, namespace)
	for key := range r.cache {
		if strings.HasPrefix(key, prefix) {
			delete(r.cache, key)
		}
	}
}
