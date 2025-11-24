package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/k8s"
)

const (
	k8sAPITimeout = 10 * time.Second
)

// Messages sent from async commands back to the update loop

// ContextsLoadedMsg is sent when contexts have been loaded
type ContextsLoadedMsg struct {
	contexts []string
	err      error
}

// NamespacesLoadedMsg is sent when namespaces have been loaded
type NamespacesLoadedMsg struct {
	namespaces []string
	err        error
}

// PodsLoadedMsg is sent when pods have been loaded
type PodsLoadedMsg struct {
	pods []k8s.PodInfo
	err  error
}

// ServicesLoadedMsg is sent when services have been loaded
type ServicesLoadedMsg struct {
	services []k8s.ServiceInfo
	err      error
}

// SelectorValidatedMsg is sent when a selector has been validated
type SelectorValidatedMsg struct {
	valid bool
	pods  []k8s.PodInfo
	err   error
}

// PortCheckedMsg is sent when a port's availability has been checked
type PortCheckedMsg struct {
	port      int
	available bool
	message   string
}

// ForwardSavedMsg is sent when a forward has been saved to config
type ForwardSavedMsg struct {
	success bool
	err     error
}

// ForwardsRemovedMsg is sent when forwards have been removed from config
type ForwardsRemovedMsg struct {
	success bool
	count   int
	err     error
}

// WizardCompleteMsg signals that the wizard has completed
type WizardCompleteMsg struct{}

// Command functions (return tea.Cmd)

// loadContextsCmd loads available Kubernetes contexts
func loadContextsCmd(discovery *k8s.Discovery) tea.Cmd {
	return func() tea.Msg {
		contexts, err := discovery.ListContexts()
		if err != nil {
			return ContextsLoadedMsg{err: err}
		}
		return ContextsLoadedMsg{contexts: contexts}
	}
}

// loadNamespacesCmd loads namespaces for the given context
func loadNamespacesCmd(discovery *k8s.Discovery, contextName string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), k8sAPITimeout)
		defer cancel()

		namespaces, err := discovery.ListNamespaces(ctx, contextName)
		if err != nil {
			return NamespacesLoadedMsg{err: err}
		}
		return NamespacesLoadedMsg{namespaces: namespaces}
	}
}

// loadPodsCmd loads pods for the given context and namespace
func loadPodsCmd(discovery *k8s.Discovery, contextName, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), k8sAPITimeout)
		defer cancel()

		pods, err := discovery.ListPods(ctx, contextName, namespace)
		if err != nil {
			return PodsLoadedMsg{err: err}
		}
		return PodsLoadedMsg{pods: pods}
	}
}

// loadServicesCmd loads services for the given context and namespace
func loadServicesCmd(discovery *k8s.Discovery, contextName, namespace string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), k8sAPITimeout)
		defer cancel()

		services, err := discovery.ListServices(ctx, contextName, namespace)
		if err != nil {
			return ServicesLoadedMsg{err: err}
		}
		return ServicesLoadedMsg{services: services}
	}
}

// validateSelectorCmd validates a label selector and returns matching pods
func validateSelectorCmd(discovery *k8s.Discovery, contextName, namespace, selector string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), k8sAPITimeout)
		defer cancel()

		pods, err := discovery.ListPodsWithSelector(ctx, contextName, namespace, selector)
		if err != nil {
			return SelectorValidatedMsg{valid: false, err: err}
		}

		return SelectorValidatedMsg{
			valid: len(pods) > 0,
			pods:  pods,
		}
	}
}

// checkPortCmd checks if a local port is available
func checkPortCmd(port int) tea.Cmd {
	return func() tea.Msg {
		available, processInfo, err := k8s.CheckPortAvailability(port)

		msg := ""
		if err != nil {
			msg = fmt.Sprintf("✗ Error: %v", err)
		} else if available {
			msg = fmt.Sprintf("✓ Port %d available", port)
		} else {
			msg = fmt.Sprintf("✗ Port %d in use by %s", port, processInfo)
		}

		return PortCheckedMsg{
			port:      port,
			available: available,
			message:   msg,
		}
	}
}

// saveForwardCmd saves a new forward to the configuration file
func saveForwardCmd(mutator *config.Mutator, contextName, namespace string, fwd config.Forward) tea.Cmd {
	return func() tea.Msg {
		err := mutator.AddForward(contextName, namespace, fwd)
		return ForwardSavedMsg{
			success: err == nil,
			err:     err,
		}
	}
}

// updateForwardCmd atomically updates an existing forward (used in edit mode)
func updateForwardCmd(mutator *config.Mutator, oldID, contextName, namespace string, fwd config.Forward) tea.Cmd {
	return func() tea.Msg {
		err := mutator.UpdateForward(oldID, contextName, namespace, fwd)
		return ForwardSavedMsg{
			success: err == nil,
			err:     err,
		}
	}
}

// removeForwardsCmd removes selected forwards from the configuration file
func removeForwardsCmd(mutator *config.Mutator, forwards []RemovableForward) tea.Cmd {
	return func() tea.Msg {
		// Create a map of IDs to remove
		idsToRemove := make(map[string]bool)
		for _, fwd := range forwards {
			idsToRemove[fwd.ID] = true
		}

		// Remove forwards matching the IDs
		err := mutator.RemoveForwards(func(ctx, ns string, fwd config.Forward) bool {
			return idsToRemove[fwd.ID()]
		})

		return ForwardsRemovedMsg{
			success: err == nil,
			count:   len(forwards),
			err:     err,
		}
	}
}

// removeForwardByIDCmd removes a single forward by its ID
func removeForwardByIDCmd(mutator *config.Mutator, id string) tea.Cmd {
	return func() tea.Msg {
		err := mutator.RemoveForwardByID(id)
		return ForwardsRemovedMsg{
			success: err == nil,
			count:   1,
			err:     err,
		}
	}
}
