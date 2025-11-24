package forward

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/healthcheck"
	"github.com/nvm/kportal/internal/k8s"
	"github.com/nvm/kportal/internal/logger"
)

// StatusUpdater is an interface for updating forward status
type StatusUpdater interface {
	UpdateStatus(id string, status string)
	AddForward(id string, fwd *config.Forward)
	Remove(id string)
}

// Manager orchestrates all port-forward workers.
// It handles starting, stopping, and hot-reloading forwards.
type Manager struct {
	workers       map[string]*ForwardWorker // key: forward.ID()
	workersMu     sync.RWMutex
	clientPool    *k8s.ClientPool
	resolver      *k8s.ResourceResolver
	portForwarder *k8s.PortForwarder
	portChecker   *PortChecker
	healthChecker *healthcheck.Checker
	verbose       bool
	currentConfig *config.Config
	statusUI      StatusUpdater
}

// NewManager creates a new forward Manager.
// The health checker will be created with default settings and can be
// reconfigured via SetConfig().
func NewManager(verbose bool) (*Manager, error) {
	clientPool, err := k8s.NewClientPool()
	if err != nil {
		return nil, fmt.Errorf("failed to create client pool: %w", err)
	}

	resolver := k8s.NewResourceResolver(clientPool)
	portForwarder := k8s.NewPortForwarder(clientPool, resolver)

	// Create health checker with defaults: check every 3 seconds with 2 second timeout
	// Will be reconfigured when config is loaded
	healthChecker := healthcheck.NewChecker(3*time.Second, 2*time.Second)

	return &Manager{
		workers:       make(map[string]*ForwardWorker),
		clientPool:    clientPool,
		resolver:      resolver,
		portForwarder: portForwarder,
		portChecker:   NewPortChecker(),
		healthChecker: healthChecker,
		verbose:       verbose,
	}, nil
}

// configureHealthChecker creates a new health checker with settings from config
func (m *Manager) configureHealthChecker(cfg *config.Config) {
	// Stop existing health checker
	if m.healthChecker != nil {
		m.healthChecker.Stop()
	}

	// Parse check method
	methodStr := cfg.GetHealthCheckMethod()
	var method healthcheck.CheckMethod
	switch methodStr {
	case "tcp-dial":
		method = healthcheck.CheckMethodTCPDial
	case "data-transfer":
		method = healthcheck.CheckMethodDataTransfer
	default:
		method = healthcheck.CheckMethodDataTransfer
	}

	// Create new health checker with config settings
	m.healthChecker = healthcheck.NewCheckerWithOptions(healthcheck.CheckerOptions{
		Interval:         cfg.GetHealthCheckIntervalOrDefault(),
		Timeout:          cfg.GetHealthCheckTimeoutOrDefault(),
		Method:           method,
		MaxConnectionAge: cfg.GetMaxConnectionAge(),
		MaxIdleTime:      cfg.GetMaxIdleTime(),
	})

	// Configure TCP settings on port forwarder
	tcpKeepalive := cfg.GetTCPKeepalive()
	dialTimeout := cfg.GetDialTimeout()
	m.portForwarder.SetTCPKeepalive(tcpKeepalive)
	m.portForwarder.SetDialTimeout(dialTimeout)

	logger.Info("Health checker and reliability configured", map[string]interface{}{
		"interval":           cfg.GetHealthCheckIntervalOrDefault().String(),
		"timeout":            cfg.GetHealthCheckTimeoutOrDefault().String(),
		"method":             methodStr,
		"max_connection_age": cfg.GetMaxConnectionAge().String(),
		"max_idle_time":      cfg.GetMaxIdleTime().String(),
		"tcp_keepalive":      tcpKeepalive.String(),
		"dial_timeout":       dialTimeout.String(),
	})
}

// SetStatusUI sets the status updater for the manager
func (m *Manager) SetStatusUI(ui StatusUpdater) {
	m.statusUI = ui
}

// Start initializes and starts all port-forwards from the configuration.
func (m *Manager) Start(cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("configuration is nil")
	}

	m.currentConfig = cfg

	// Configure health checker with settings from config
	m.configureHealthChecker(cfg)

	// Get all forwards from config
	forwards := cfg.GetAllForwards()

	if len(forwards) == 0 {
		return fmt.Errorf("no forwards configured")
	}

	// Check port availability before starting
	ports := m.extractPorts(forwards)
	conflicts := m.portChecker.CheckAvailability(ports, nil)
	if len(conflicts) > 0 {
		// Add resource information to conflicts
		for i := range conflicts {
			conflicts[i].Resource = m.getResourceForPort(forwards, conflicts[i].Port)
		}
		return fmt.Errorf("port conflicts detected:\n%s", FormatConflicts(conflicts))
	}

	// Start all workers
	log.Printf("Starting %d port-forward(s)...", len(forwards))

	for _, fwd := range forwards {
		if err := m.startWorker(fwd); err != nil {
			logger.Error("Failed to start worker", map[string]interface{}{
				"forward_id": fwd.ID(),
				"context":    fwd.GetContext(),
				"namespace":  fwd.GetNamespace(),
				"resource":   fwd.Resource,
				"local_port": fwd.LocalPort,
				"error":      err.Error(),
			})
			// Continue with other workers
		}
	}

	log.Printf("All port-forwards started")
	return nil
}

// Stop gracefully stops all port-forward workers.
func (m *Manager) Stop() {
	log.Printf("Stopping all port-forwards...")

	// Stop health checker first
	m.healthChecker.Stop()

	m.workersMu.Lock()
	workers := make([]*ForwardWorker, 0, len(m.workers))
	for _, worker := range m.workers {
		workers = append(workers, worker)
	}
	m.workersMu.Unlock()

	// Stop all workers
	var wg sync.WaitGroup
	for _, worker := range workers {
		wg.Add(1)
		go func(w *ForwardWorker) {
			defer wg.Done()
			w.Stop()
		}(worker)
	}

	wg.Wait()

	// Clear workers map
	m.workersMu.Lock()
	m.workers = make(map[string]*ForwardWorker)
	m.workersMu.Unlock()

	log.Printf("All port-forwards stopped")
}

// Reload applies a new configuration with hot-reload logic.
// It diffs the new config against the current one and:
// - Stops removed forwards
// - Keeps unchanged forwards running
// - Starts new forwards
func (m *Manager) Reload(newCfg *config.Config) error {
	if newCfg == nil {
		return fmt.Errorf("new configuration is nil")
	}

	logger.Info("Reloading configuration", map[string]interface{}{
		"new_forwards_count": len(newCfg.GetAllForwards()),
	})

	// Get all forwards from new config
	newForwards := newCfg.GetAllForwards()

	if len(newForwards) == 0 {
		log.Printf("New configuration has no forwards, stopping all")
		m.Stop()
		m.currentConfig = newCfg
		return nil
	}

	// Create maps for easier comparison
	newForwardsMap := make(map[string]config.Forward)
	for _, fwd := range newForwards {
		newForwardsMap[fwd.ID()] = fwd
	}

	m.workersMu.RLock()
	currentForwardsMap := make(map[string]config.Forward)
	for id, worker := range m.workers {
		currentForwardsMap[id] = worker.GetForward()
	}
	m.workersMu.RUnlock()

	// Determine changes
	var toAdd []config.Forward
	var toRemove []string
	var toKeep []string

	// Find forwards to add and keep
	for id, fwd := range newForwardsMap {
		if _, exists := currentForwardsMap[id]; exists {
			toKeep = append(toKeep, id)
		} else {
			toAdd = append(toAdd, fwd)
		}
	}

	// Find forwards to remove
	for id := range currentForwardsMap {
		if _, exists := newForwardsMap[id]; !exists {
			toRemove = append(toRemove, id)
		}
	}

	// Check port availability for new forwards
	if len(toAdd) > 0 {
		// Get currently managed ports to skip in availability check
		managedPorts := make(map[int]bool)
		for _, id := range toKeep {
			managedPorts[currentForwardsMap[id].LocalPort] = true
		}

		// Check new ports
		newPorts := m.extractPorts(toAdd)
		conflicts := m.portChecker.CheckAvailability(newPorts, managedPorts)
		if len(conflicts) > 0 {
			// Add resource information to conflicts
			for i := range conflicts {
				conflicts[i].Resource = m.getResourceForPort(toAdd, conflicts[i].Port)
			}
			log.Printf("Config change rejected due to port conflicts:\n%s", FormatConflicts(conflicts))
			log.Printf("Keeping previous configuration active")
			return fmt.Errorf("port conflicts detected")
		}
	}

	// Apply changes
	log.Printf("Configuration diff: %d to add, %d to remove, %d to keep",
		len(toAdd), len(toRemove), len(toKeep))

	// Stop removed forwards
	for _, id := range toRemove {
		if err := m.stopWorker(id); err != nil {
			log.Printf("Failed to stop worker %s: %v", id, err)
		} else {
			log.Printf("Stopped: %s", id)
		}
	}

	// Start new forwards
	for _, fwd := range toAdd {
		if err := m.startWorker(fwd); err != nil {
			log.Printf("Failed to start worker for %s: %v", fwd.ID(), err)
		} else {
			log.Printf("Started: %s", fwd.ID())
		}
	}

	// Update current config
	m.currentConfig = newCfg

	log.Printf("Configuration reloaded successfully")
	return nil
}

// startWorker creates and starts a new forward worker.
func (m *Manager) startWorker(fwd config.Forward) error {
	m.workersMu.Lock()
	defer m.workersMu.Unlock()

	// Check if worker already exists
	if _, exists := m.workers[fwd.ID()]; exists {
		return fmt.Errorf("worker already exists for %s", fwd.ID())
	}

	// Notify UI about new forward
	if m.statusUI != nil {
		m.statusUI.AddForward(fwd.ID(), &fwd)
	}

	// Register with health checker
	m.healthChecker.Register(fwd.ID(), fwd.LocalPort, func(forwardID string, status healthcheck.Status, errorMsg string) {
		if m.statusUI != nil {
			m.statusUI.UpdateStatus(forwardID, string(status))
			// Send error separately if there is one
			if (status == healthcheck.StatusUnhealthy || status == healthcheck.StatusStale) && errorMsg != "" {
				if ui, ok := m.statusUI.(interface{ SetError(id, msg string) }); ok {
					ui.SetError(forwardID, errorMsg)
				}
			}
		}

		// Handle stale connections: trigger reconnection if retryOnStale is enabled
		if status == healthcheck.StatusStale && m.currentConfig.GetRetryOnStale() {
			logger.Info("Stale connection detected, triggering reconnection", map[string]interface{}{
				"forward_id": forwardID,
				"reason":     errorMsg,
			})

			// Find and notify the worker to reconnect
			m.workersMu.RLock()
			worker, exists := m.workers[forwardID]
			m.workersMu.RUnlock()

			if exists {
				worker.TriggerReconnect("stale connection")
			}
		}
	})

	// Create and start worker
	worker := NewForwardWorker(fwd, m.portForwarder, m.verbose, m.statusUI, m.healthChecker)
	worker.Start()

	// Store worker
	m.workers[fwd.ID()] = worker

	return nil
}

// stopWorker stops and removes a forward worker.
func (m *Manager) stopWorker(id string) error {
	return m.stopWorkerInternal(id, true)
}

// stopWorkerInternal stops a worker with option to remove from UI or just update status
func (m *Manager) stopWorkerInternal(id string, removeFromUI bool) error {
	m.workersMu.Lock()
	worker, exists := m.workers[id]
	if !exists {
		m.workersMu.Unlock()
		return fmt.Errorf("worker not found: %s", id)
	}
	delete(m.workers, id)
	m.workersMu.Unlock()

	// Unregister from health checker
	m.healthChecker.Unregister(id)

	// Notify UI - either remove or update to disabled status
	if m.statusUI != nil {
		if removeFromUI {
			m.statusUI.Remove(id)
		} else {
			m.statusUI.UpdateStatus(id, "Disabled")
		}
	}

	// Stop the worker
	worker.Stop()

	return nil
}

// GetActiveForwards returns a list of all active forward IDs.
func (m *Manager) GetActiveForwards() []string {
	m.workersMu.RLock()
	defer m.workersMu.RUnlock()

	ids := make([]string, 0, len(m.workers))
	for id := range m.workers {
		ids = append(ids, id)
	}

	return ids
}

// GetWorkerCount returns the number of active workers.
func (m *Manager) GetWorkerCount() int {
	m.workersMu.RLock()
	defer m.workersMu.RUnlock()

	return len(m.workers)
}

// extractPorts extracts all local ports from a list of forwards.
func (m *Manager) extractPorts(forwards []config.Forward) []int {
	ports := make([]int, len(forwards))
	for i, fwd := range forwards {
		ports[i] = fwd.LocalPort
	}
	return ports
}

// getResourceForPort finds the resource (forward ID) that uses a given port.
func (m *Manager) getResourceForPort(forwards []config.Forward, port int) string {
	for _, fwd := range forwards {
		if fwd.LocalPort == port {
			return fwd.ID()
		}
	}
	return "unknown"
}

// DisableForward temporarily stops a forward by ID
func (m *Manager) DisableForward(id string) error {
	if err := m.stopWorkerInternal(id, false); err != nil {
		return err
	}
	log.Printf("Disabled: %s", id)
	return nil
}

// EnableForward re-enables a previously disabled forward
func (m *Manager) EnableForward(id string) error {
	// Find the forward configuration in current config
	if m.currentConfig == nil {
		return fmt.Errorf("no configuration available")
	}

	forwards := m.currentConfig.GetAllForwards()
	var targetFwd *config.Forward
	for _, fwd := range forwards {
		if fwd.ID() == id {
			targetFwd = &fwd
			break
		}
	}

	if targetFwd == nil {
		return fmt.Errorf("forward not found in configuration: %s", id)
	}

	// Check if already running
	m.workersMu.RLock()
	_, exists := m.workers[id]
	m.workersMu.RUnlock()

	if exists {
		return fmt.Errorf("forward already enabled: %s", id)
	}

	// Start the worker
	if err := m.startWorker(*targetFwd); err != nil {
		return fmt.Errorf("failed to enable forward: %w", err)
	}

	log.Printf("Enabled: %s", id)
	return nil
}
