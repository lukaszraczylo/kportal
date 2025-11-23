package healthcheck

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// Status represents the health status of a port forward
type Status string

const (
	StatusHealthy   Status = "Active"
	StatusUnhealthy Status = "Error"
	StatusStarting  Status = "Starting"
	StatusReconnect Status = "Reconnecting"
)

// PortHealth represents the health status of a single port
type PortHealth struct {
	Port         int
	LastCheck    time.Time
	Status       Status
	ErrorMessage string
}

// StatusCallback is called when a port's health status changes
type StatusCallback func(forwardID string, status Status)

// Checker performs periodic health checks on local ports
type Checker struct {
	mu        sync.RWMutex
	ports     map[string]*PortHealth // key: forward ID
	callbacks map[string]StatusCallback
	interval  time.Duration
	timeout   time.Duration
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewChecker creates a new health checker
func NewChecker(interval, timeout time.Duration) *Checker {
	ctx, cancel := context.WithCancel(context.Background())
	return &Checker{
		ports:     make(map[string]*PortHealth),
		callbacks: make(map[string]StatusCallback),
		interval:  interval,
		timeout:   timeout,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Register adds a port to monitor
func (c *Checker) Register(forwardID string, port int, callback StatusCallback) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ports[forwardID] = &PortHealth{
		Port:      port,
		LastCheck: time.Time{},
		Status:    StatusStarting,
	}
	c.callbacks[forwardID] = callback

	// Start checking this port
	c.wg.Add(1)
	go c.checkLoop(forwardID)
}

// Unregister removes a port from monitoring
func (c *Checker) Unregister(forwardID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.ports, forwardID)
	delete(c.callbacks, forwardID)
}

// MarkReconnecting marks a forward as reconnecting (called by worker)
func (c *Checker) MarkReconnecting(forwardID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if health, exists := c.ports[forwardID]; exists {
		oldStatus := health.Status
		health.Status = StatusReconnect
		health.LastCheck = time.Now()

		// Notify if status changed
		if oldStatus != StatusReconnect {
			c.mu.Unlock()
			c.notifyStatusChange(forwardID, StatusReconnect)
			c.mu.Lock()
		}
	}
}

// MarkStarting marks a forward as starting (called by worker)
func (c *Checker) MarkStarting(forwardID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if health, exists := c.ports[forwardID]; exists {
		oldStatus := health.Status
		health.Status = StatusStarting
		health.LastCheck = time.Now()

		// Notify if status changed
		if oldStatus != StatusStarting {
			c.mu.Unlock()
			c.notifyStatusChange(forwardID, StatusStarting)
			c.mu.Lock()
		}
	}
}

// GetStatus returns the current health status of a forward
func (c *Checker) GetStatus(forwardID string) (Status, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if health, exists := c.ports[forwardID]; exists {
		return health.Status, true
	}
	return StatusUnhealthy, false
}

// Stop stops all health checking
func (c *Checker) Stop() {
	c.cancel()
	c.wg.Wait()
}

// checkLoop continuously checks a single port's health
func (c *Checker) checkLoop(forwardID string) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// Initial check after a short delay to let port-forward establish
	time.Sleep(2 * time.Second)
	c.checkPort(forwardID)

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			// Check if this forward still exists
			c.mu.RLock()
			_, exists := c.ports[forwardID]
			c.mu.RUnlock()

			if !exists {
				return
			}

			c.checkPort(forwardID)
		}
	}
}

// checkPort performs a single health check on a port
func (c *Checker) checkPort(forwardID string) {
	c.mu.RLock()
	health, exists := c.ports[forwardID]
	if !exists {
		c.mu.RUnlock()
		return
	}
	port := health.Port
	oldStatus := health.Status
	c.mu.RUnlock()

	// Attempt to connect to the local port
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))

	newStatus := StatusHealthy
	errorMsg := ""

	if err != nil {
		newStatus = StatusUnhealthy
		errorMsg = err.Error()
	} else {
		conn.Close()
	}

	// Update health status
	c.mu.Lock()
	if health, exists := c.ports[forwardID]; exists {
		health.Status = newStatus
		health.LastCheck = time.Now()
		health.ErrorMessage = errorMsg
	}
	c.mu.Unlock()

	// Notify if status changed
	if oldStatus != newStatus {
		c.notifyStatusChange(forwardID, newStatus)
	}
}

// notifyStatusChange calls the callback for a forward
func (c *Checker) notifyStatusChange(forwardID string, status Status) {
	c.mu.RLock()
	callback, exists := c.callbacks[forwardID]
	c.mu.RUnlock()

	if exists && callback != nil {
		callback(forwardID, status)
	}
}
