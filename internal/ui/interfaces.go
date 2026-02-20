package ui

import (
	"context"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/k8s"
)

// DiscoveryInterface defines the interface for Kubernetes discovery operations
// This allows for mocking in tests
type DiscoveryInterface interface {
	ListContexts() ([]string, error)
	GetCurrentContext() (string, error)
	ListNamespaces(ctx context.Context, contextName string) ([]string, error)
	ListPods(ctx context.Context, contextName, namespace string) ([]k8s.PodInfo, error)
	ListPodsWithSelector(ctx context.Context, contextName, namespace, selector string) ([]k8s.PodInfo, error)
	ListServices(ctx context.Context, contextName, namespace string) ([]k8s.ServiceInfo, error)
}

// MutatorInterface defines the interface for configuration mutation operations
// This allows for mocking in tests
type MutatorInterface interface {
	AddForward(contextName, namespaceName string, fwd config.Forward) error
	RemoveForwards(predicate func(ctx, ns string, fwd config.Forward) bool) error
	RemoveForwardByID(id string) error
	UpdateForward(oldID, newContextName, newNamespaceName string, newFwd config.Forward) error
}

// Compile-time checks to ensure real types implement interfaces
var _ DiscoveryInterface = (*k8s.Discovery)(nil)
var _ MutatorInterface = (*config.Mutator)(nil)
