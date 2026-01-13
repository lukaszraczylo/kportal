package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nvm/kportal/internal/benchmark"
	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/k8s"
	"github.com/nvm/kportal/internal/logger"
)

const (
	k8sAPITimeout = 10 * time.Second
)

// Messages sent from async commands back to the update loop

// ContextsLoadedMsg is sent when contexts have been loaded
type ContextsLoadedMsg struct {
	err      error
	contexts []string
}

// NamespacesLoadedMsg is sent when namespaces have been loaded
type NamespacesLoadedMsg struct {
	err        error
	namespaces []string
}

// PodsLoadedMsg is sent when pods have been loaded
type PodsLoadedMsg struct {
	err  error
	pods []k8s.PodInfo
}

// ServicesLoadedMsg is sent when services have been loaded
type ServicesLoadedMsg struct {
	err      error
	services []k8s.ServiceInfo
}

// SelectorValidatedMsg is sent when a selector has been validated
type SelectorValidatedMsg struct {
	err   error
	pods  []k8s.PodInfo
	valid bool
}

// PortCheckedMsg is sent when a port's availability has been checked
type PortCheckedMsg struct {
	message   string
	port      int
	available bool
}

// ForwardSavedMsg is sent when a forward has been saved to config
type ForwardSavedMsg struct {
	err     error
	success bool
}

// ForwardsRemovedMsg is sent when forwards have been removed from config
type ForwardsRemovedMsg struct {
	err     error
	count   int
	success bool
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
func checkPortCmd(port int, configPath string) tea.Cmd {
	return func() tea.Msg {
		// First check if port is already in the configuration
		cfg, err := config.LoadConfig(configPath)
		if err == nil {
			// Check all forwards in config for this port
			allForwards := cfg.GetAllForwards()
			for _, fwd := range allForwards {
				if fwd.LocalPort == port {
					return PortCheckedMsg{
						port:      port,
						available: false,
						message:   fmt.Sprintf("✗ Port %d already assigned to %s", port, fwd.ID()),
					}
				}
			}
		}

		// Then check if port is available at OS level
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

// BenchmarkCompleteMsg is sent when a benchmark run completes
type BenchmarkCompleteMsg struct {
	Error     error
	Results   *benchmark.Results
	ForwardID string
}

// BenchmarkProgressMsg is sent periodically during benchmark execution
type BenchmarkProgressMsg struct {
	ForwardID string
	Completed int
	Total     int
}

// HTTPLogEntryMsg is sent when a new HTTP log entry is received
type HTTPLogEntryMsg struct {
	Entry HTTPLogEntry
}

// clearCopyMessageMsg is sent to clear the copy confirmation message
type clearCopyMessageMsg struct{}

// listenBenchmarkProgressCmd listens for progress updates from the benchmark
func listenBenchmarkProgressCmd(progressCh <-chan BenchmarkProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-progressCh
		if !ok {
			// Channel closed, benchmark complete
			return nil
		}
		return msg
	}
}

// runBenchmarkCmd runs a benchmark against the given port forward
// It sends progress updates via tea.Batch until completion
// The ctx parameter allows the benchmark to be cancelled from outside
func runBenchmarkCmd(ctx context.Context, forwardID string, localPort int, urlPath, method string, concurrency, requests int, progressCh chan<- BenchmarkProgressMsg) tea.Cmd {
	return func() tea.Msg {
		runner := benchmark.NewRunner()

		url := fmt.Sprintf("http://localhost:%d%s", localPort, urlPath)
		cfg := benchmark.Config{
			URL:         url,
			Method:      method,
			Concurrency: concurrency,
			Requests:    requests,
			Timeout:     30 * time.Second,
			ProgressCallback: func(completed, total int) {
				// Recover from panics in the callback
				defer func() {
					if r := recover(); r != nil {
						logger.Debug("recovered from panic in progress callback", map[string]any{"panic": r})
					}
				}()
				// Non-blocking send to progress channel
				select {
				case progressCh <- BenchmarkProgressMsg{
					ForwardID: forwardID,
					Completed: completed,
					Total:     total,
				}:
				default:
					// Drop if channel is full
				}
			},
		}

		// Use the provided context with a timeout as a safety limit
		benchCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		results, err := runner.Run(benchCtx, forwardID, cfg)

		// Close the progress channel when done
		close(progressCh)

		// Check if cancelled
		if ctx.Err() != nil {
			return BenchmarkCompleteMsg{
				ForwardID: forwardID,
				Results:   nil,
				Error:     fmt.Errorf("benchmark cancelled"),
			}
		}

		return BenchmarkCompleteMsg{
			ForwardID: forwardID,
			Results:   results,
			Error:     err,
		}
	}
}
