package ui

import (
	"context"
	"sync"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/k8s"
)

// MockDiscovery is a mock implementation of DiscoveryInterface for testing
type MockDiscovery struct {
	ListPodsErr               error
	ListServicesErr           error
	ListPodsWithSelectorErr   error
	ListContextsErr           error
	GetCurrentContextErr      error
	ListNamespacesErr         error
	LastSelector              string
	CurrentContext            string
	LastNamespace             string
	LastContextName           string
	PodsWithSelector          []k8s.PodInfo
	Services                  []k8s.ServiceInfo
	Pods                      []k8s.PodInfo
	Namespaces                []string
	Contexts                  []string
	ListContextsCalls         int
	GetCurrentContextCalls    int
	ListNamespacesCalls       int
	ListPodsCalls             int
	ListPodsWithSelectorCalls int
	ListServicesCalls         int
	mu                        sync.Mutex
}

func NewMockDiscovery() *MockDiscovery {
	return &MockDiscovery{
		Contexts:   []string{"default", "production", "staging"},
		Namespaces: []string{"default", "kube-system"},
	}
}

func (m *MockDiscovery) ListContexts() ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListContextsCalls++
	return m.Contexts, m.ListContextsErr
}

func (m *MockDiscovery) GetCurrentContext() (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetCurrentContextCalls++
	if m.CurrentContext == "" {
		return "default", m.GetCurrentContextErr
	}
	return m.CurrentContext, m.GetCurrentContextErr
}

func (m *MockDiscovery) ListNamespaces(ctx context.Context, contextName string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListNamespacesCalls++
	m.LastContextName = contextName
	return m.Namespaces, m.ListNamespacesErr
}

func (m *MockDiscovery) ListPods(ctx context.Context, contextName, namespace string) ([]k8s.PodInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListPodsCalls++
	m.LastContextName = contextName
	m.LastNamespace = namespace
	return m.Pods, m.ListPodsErr
}

func (m *MockDiscovery) ListPodsWithSelector(ctx context.Context, contextName, namespace, selector string) ([]k8s.PodInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListPodsWithSelectorCalls++
	m.LastContextName = contextName
	m.LastNamespace = namespace
	m.LastSelector = selector
	return m.PodsWithSelector, m.ListPodsWithSelectorErr
}

func (m *MockDiscovery) ListServices(ctx context.Context, contextName, namespace string) ([]k8s.ServiceInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListServicesCalls++
	m.LastContextName = contextName
	m.LastNamespace = namespace
	return m.Services, m.ListServicesErr
}

// MockMutator is a mock implementation of MutatorInterface for testing
type MockMutator struct {
	RemoveForwardByIDErr error
	UpdateForwardErr     error
	AddForwardErr        error
	RemoveForwardsErr    error
	LastPredicate        func(ctx, ns string, fwd config.Forward) bool
	LastContextName      string
	LastOldID            string
	LastNamespaceName    string
	LastRemovedID        string
	Forwards             []struct {
		Context   string
		Namespace string
		Forward   config.Forward
	}
	LastForward            config.Forward
	RemoveForwardByIDCalls int
	UpdateForwardCalls     int
	RemoveForwardsCalls    int
	AddForwardCalls        int
	mu                     sync.Mutex
}

func NewMockMutator() *MockMutator {
	return &MockMutator{}
}

func (m *MockMutator) AddForward(contextName, namespaceName string, fwd config.Forward) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AddForwardCalls++
	m.LastContextName = contextName
	m.LastNamespaceName = namespaceName
	m.LastForward = fwd

	if m.AddForwardErr == nil {
		m.Forwards = append(m.Forwards, struct {
			Context   string
			Namespace string
			Forward   config.Forward
		}{contextName, namespaceName, fwd})
	}

	return m.AddForwardErr
}

func (m *MockMutator) RemoveForwards(predicate func(ctx, ns string, fwd config.Forward) bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RemoveForwardsCalls++
	m.LastPredicate = predicate
	return m.RemoveForwardsErr
}

func (m *MockMutator) RemoveForwardByID(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RemoveForwardByIDCalls++
	m.LastRemovedID = id
	return m.RemoveForwardByIDErr
}

func (m *MockMutator) UpdateForward(oldID, newContextName, newNamespaceName string, newFwd config.Forward) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpdateForwardCalls++
	m.LastOldID = oldID
	m.LastContextName = newContextName
	m.LastNamespaceName = newNamespaceName
	m.LastForward = newFwd
	return m.UpdateForwardErr
}

// MockHTTPLogSubscriber is a mock for HTTP log subscription
type MockHTTPLogSubscriber struct {
	Subscriptions map[string]func(HTTPLogEntry)
	CleanupCalls  int
	mu            sync.Mutex
	ShouldFail    bool
}

func NewMockHTTPLogSubscriber() *MockHTTPLogSubscriber {
	return &MockHTTPLogSubscriber{
		Subscriptions: make(map[string]func(HTTPLogEntry)),
	}
}

// Subscribe returns a cleanup function
func (m *MockHTTPLogSubscriber) Subscribe(forwardID string, callback func(HTTPLogEntry)) func() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Subscriptions[forwardID] = callback

	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		m.CleanupCalls++
		delete(m.Subscriptions, forwardID)
	}
}

// SendEntry sends an entry to a subscribed callback (for testing)
func (m *MockHTTPLogSubscriber) SendEntry(forwardID string, entry HTTPLogEntry) {
	m.mu.Lock()
	callback, exists := m.Subscriptions[forwardID]
	m.mu.Unlock()

	if exists && callback != nil {
		callback(entry)
	}
}

// GetSubscriberFunc returns the function signature expected by the UI
func (m *MockHTTPLogSubscriber) GetSubscriberFunc() HTTPLogSubscriber {
	return func(forwardID string, callback func(entry HTTPLogEntry)) func() {
		return m.Subscribe(forwardID, callback)
	}
}

// MockToggleCallback tracks toggle callback invocations
type MockToggleCallback struct {
	Calls []struct {
		ID     string
		Enable bool
	}
	mu sync.Mutex
}

func NewMockToggleCallback() *MockToggleCallback {
	return &MockToggleCallback{}
}

func (m *MockToggleCallback) Callback(id string, enable bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, struct {
		ID     string
		Enable bool
	}{id, enable})
}

func (m *MockToggleCallback) GetFunc() func(string, bool) {
	return m.Callback
}

func (m *MockToggleCallback) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

func (m *MockToggleCallback) LastCall() (string, bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Calls) == 0 {
		return "", false, false
	}
	last := m.Calls[len(m.Calls)-1]
	return last.ID, last.Enable, true
}
