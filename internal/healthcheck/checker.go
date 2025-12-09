package healthcheck

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/events"
)

// bufferPool is a sync.Pool for reusing buffers in data transfer health checks.
// This reduces GC pressure by avoiding allocation of 1KB buffers on every health check.
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, dataTransferSize)
		return &buf
	},
}

const (
	startupGracePeriod = 10 * time.Second
	dataTransferSize   = 1024 // bytes to read in data transfer test
)

// Status represents the health status of a port forward
type Status string

const (
	StatusHealthy   Status = "Active"
	StatusUnhealthy Status = "Error"
	StatusStarting  Status = "Starting"
	StatusReconnect Status = "Reconnecting"
	StatusStale     Status = "Stale" // Connection is old or idle
)

// CheckMethod represents the health check method
type CheckMethod string

const (
	CheckMethodTCPDial      CheckMethod = "tcp-dial"      // Simple TCP connection test
	CheckMethodDataTransfer CheckMethod = "data-transfer" // Try to read data from connection
)

// PortHealth represents the health status of a single port
type PortHealth struct {
	Port           int
	LastCheck      time.Time
	Status         Status
	ErrorMessage   string
	RegisteredAt   time.Time // When this port was registered
	ConnectionTime time.Time // When current connection was established
	LastActivity   time.Time // Last time data was transferred
}

// StatusCallback is called when a port's health status changes
type StatusCallback func(forwardID string, status Status, errorMsg string)

// Checker performs periodic health checks on local ports.
// Uses a single goroutine to check all registered ports, reducing overhead
// compared to one goroutine per port.
type Checker struct {
	mu               sync.RWMutex
	ports            map[string]*PortHealth // key: forward ID
	callbacks        map[string]StatusCallback
	interval         time.Duration
	timeout          time.Duration
	method           CheckMethod
	maxConnectionAge time.Duration
	maxIdleTime      time.Duration
	ctx              context.Context
	cancel           context.CancelFunc
	wg               sync.WaitGroup
	started          bool
	eventBus         *events.Bus // Optional event bus for decoupled communication
}

// CheckerOptions configures the health checker
type CheckerOptions struct {
	Interval         time.Duration
	Timeout          time.Duration
	Method           CheckMethod
	MaxConnectionAge time.Duration
	MaxIdleTime      time.Duration
}

// NewChecker creates a new health checker with default options
func NewChecker(interval, timeout time.Duration) *Checker {
	return NewCheckerWithOptions(CheckerOptions{
		Interval:         interval,
		Timeout:          timeout,
		Method:           CheckMethodDataTransfer,
		MaxConnectionAge: config.DefaultMaxConnectionAge,
		MaxIdleTime:      config.DefaultMaxIdleTime,
	})
}

// NewCheckerWithOptions creates a new health checker with custom options
func NewCheckerWithOptions(opts CheckerOptions) *Checker {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Checker{
		ports:            make(map[string]*PortHealth),
		callbacks:        make(map[string]StatusCallback),
		interval:         opts.Interval,
		timeout:          opts.Timeout,
		method:           opts.Method,
		maxConnectionAge: opts.MaxConnectionAge,
		maxIdleTime:      opts.MaxIdleTime,
		ctx:              ctx,
		cancel:           cancel,
	}

	// Start the single monitoring loop
	c.wg.Add(1)
	go c.monitorLoop()
	c.started = true

	return c
}

// SetEventBus sets the event bus for publishing health events
func (c *Checker) SetEventBus(bus *events.Bus) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.eventBus = bus
}

// Register adds a port to monitor
func (c *Checker) Register(forwardID string, port int, callback StatusCallback) {
	c.mu.Lock()

	now := time.Now()
	c.ports[forwardID] = &PortHealth{
		Port:           port,
		LastCheck:      time.Time{},
		Status:         StatusStarting,
		RegisteredAt:   now,
		ConnectionTime: now,
		LastActivity:   now,
	}
	c.callbacks[forwardID] = callback
	c.mu.Unlock()

	// Perform immediate first check so status updates quickly
	// This prevents the forward from being stuck in "Starting" state
	// until the next ticker interval
	go c.checkPort(forwardID)
}

// MarkConnected marks a forward as having established a new connection.
// This updates connection timestamps and triggers an immediate health check
// to verify the connection is actually working.
func (c *Checker) MarkConnected(forwardID string) {
	c.mu.Lock()

	health, exists := c.ports[forwardID]
	if !exists {
		c.mu.Unlock()
		return
	}

	now := time.Now()
	health.ConnectionTime = now
	health.LastActivity = now
	c.mu.Unlock()

	// Trigger immediate health check to verify connection and update status
	go c.checkPort(forwardID)
}

// RecordActivity records data transfer activity for a forward
func (c *Checker) RecordActivity(forwardID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if health, exists := c.ports[forwardID]; exists {
		health.LastActivity = time.Now()
	}
}

// Unregister removes a port from monitoring
func (c *Checker) Unregister(forwardID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.ports, forwardID)
	delete(c.callbacks, forwardID)
}

// markStatus is a helper to set a forward's status and notify on change.
func (c *Checker) markStatus(forwardID string, newStatus Status) {
	c.mu.Lock()

	health, exists := c.ports[forwardID]
	if !exists {
		c.mu.Unlock()
		return
	}

	oldStatus := health.Status
	health.Status = newStatus
	health.LastCheck = time.Now()
	c.mu.Unlock()

	if oldStatus != newStatus {
		c.notifyStatusChange(forwardID, newStatus, "")
	}
}

// MarkReconnecting marks a forward as reconnecting (called by worker)
func (c *Checker) MarkReconnecting(forwardID string) {
	c.markStatus(forwardID, StatusReconnect)
}

// MarkStarting marks a forward as starting (called by worker)
func (c *Checker) MarkStarting(forwardID string) {
	c.markStatus(forwardID, StatusStarting)
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

// GetLastCheckTime returns the last health check time for a forward
func (c *Checker) GetLastCheckTime(forwardID string) (time.Time, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if health, exists := c.ports[forwardID]; exists {
		return health.LastCheck, true
	}
	return time.Time{}, false
}

// GetAllErrors returns all forwards with errors and their error messages
func (c *Checker) GetAllErrors() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	errors := make(map[string]string)
	for forwardID, health := range c.ports {
		if health.Status == StatusUnhealthy && health.ErrorMessage != "" {
			errors[forwardID] = health.ErrorMessage
		}
	}
	return errors
}

// Stop stops all health checking
func (c *Checker) Stop() {
	c.cancel()
	c.wg.Wait()
}

// monitorLoop is the single goroutine that checks all registered ports periodically.
// This is more efficient than one goroutine per port as it reduces:
// - Goroutine overhead (stack memory, scheduler work)
// - Timer/ticker allocations
// - Lock contention (one lock acquisition per interval vs N)
func (c *Checker) monitorLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.checkAllPorts()
		}
	}
}

// checkAllPorts performs health checks on all registered ports
func (c *Checker) checkAllPorts() {
	// Get snapshot of ports to check
	c.mu.RLock()
	forwardIDs := make([]string, 0, len(c.ports))
	for id := range c.ports {
		forwardIDs = append(forwardIDs, id)
	}
	c.mu.RUnlock()

	// Check each port
	for _, forwardID := range forwardIDs {
		// Check if still registered (may have been unregistered during iteration)
		c.mu.RLock()
		_, exists := c.ports[forwardID]
		c.mu.RUnlock()

		if !exists {
			continue
		}

		c.checkPort(forwardID)
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
	registeredAt := health.RegisteredAt
	connectionTime := health.ConnectionTime
	lastActivity := health.LastActivity
	c.mu.RUnlock()

	now := time.Now()
	newStatus := StatusHealthy
	errorMsg := ""

	// Check for stale connections based on age or idle time
	connectionAge := now.Sub(connectionTime)
	idleTime := now.Sub(lastActivity)

	// Only enforce max connection age if the connection is ALSO idle
	// This prevents interrupting active transfers (e.g., database dumps)
	if c.maxConnectionAge > 0 && connectionAge > c.maxConnectionAge && idleTime > c.maxIdleTime {
		newStatus = StatusStale
		errorMsg = fmt.Sprintf("connection age %v exceeds max %v (and idle for %v)",
			connectionAge.Round(time.Second), c.maxConnectionAge, idleTime.Round(time.Second))
	} else if c.maxIdleTime > 0 && idleTime > c.maxIdleTime {
		newStatus = StatusStale
		errorMsg = fmt.Sprintf("idle time %v exceeds max %v", idleTime.Round(time.Second), c.maxIdleTime)
	} else {
		// Perform connectivity check
		var checkErr error
		switch c.method {
		case CheckMethodDataTransfer:
			checkErr = c.checkDataTransfer(port)
		case CheckMethodTCPDial:
			checkErr = c.checkTCPDial(port)
		default:
			checkErr = c.checkTCPDial(port)
		}

		if checkErr != nil {
			// Grace period: if forward is less than 10 seconds old, keep it as "Starting"
			// This avoids scary "Error" messages during initial connection attempts
			timeSinceStart := now.Sub(registeredAt)
			if timeSinceStart < startupGracePeriod {
				newStatus = StatusStarting
			} else {
				newStatus = StatusUnhealthy
			}
			errorMsg = checkErr.Error()
		}
	}

	// Update health status
	c.mu.Lock()
	if health, exists := c.ports[forwardID]; exists {
		health.Status = newStatus
		health.LastCheck = now
		health.ErrorMessage = errorMsg

		// Successful health check indicates connection is active
		// This prevents false positives where healthy connections are marked as idle
		if newStatus == StatusHealthy {
			health.LastActivity = now
		}
	}
	c.mu.Unlock()

	// Notify if status changed
	if oldStatus != newStatus {
		c.notifyStatusChange(forwardID, newStatus, errorMsg)

		// Publish to event bus if available
		c.mu.RLock()
		bus := c.eventBus
		c.mu.RUnlock()

		if bus != nil {
			if newStatus == StatusStale {
				bus.Publish(events.NewStaleEvent(forwardID, errorMsg))
			} else {
				bus.Publish(events.NewHealthEvent(forwardID, string(newStatus), errorMsg))
			}
		}
	}
}

// checkTCPDial performs a simple TCP dial test
func (c *Checker) checkTCPDial(port int) error {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// checkDataTransfer attempts to read data from the connection to verify tunnel health
func (c *Checker) checkDataTransfer(port int) error {
	ctx, cancel := context.WithTimeout(c.ctx, c.timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	defer conn.Close()

	// Set a short read deadline to detect hung connections
	// We don't expect to receive data, but we want to verify the connection isn't hung
	_ = conn.SetReadDeadline(time.Now().Add(c.timeout))

	// Try to read a small amount of data
	// Most servers will either:
	// 1. Send a banner (SSH, FTP, etc) - we'll read it successfully
	// 2. Wait for client to send first (HTTP, postgres) - we'll timeout (which is OK)
	// 3. Hung/stale connection - will timeout with different error
	bufPtr := bufferPool.Get().(*[]byte)
	buf := *bufPtr
	defer bufferPool.Put(bufPtr)
	_, err = conn.Read(buf)

	// We expect either:
	// - No error (banner received)
	// - EOF (connection closed by server after connect)
	// - Timeout (server waiting for client)
	// All of these indicate the tunnel is working
	if err == nil || err == io.EOF {
		return nil
	}

	// Timeout is acceptable - server is waiting for us to send data first
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return nil
	}

	// Other errors indicate a problem
	return fmt.Errorf("data transfer check failed: %w", err)
}

// notifyStatusChange calls the callback for a forward
func (c *Checker) notifyStatusChange(forwardID string, status Status, errorMsg string) {
	c.mu.RLock()
	callback, exists := c.callbacks[forwardID]
	c.mu.RUnlock()

	if exists && callback != nil {
		callback(forwardID, status, errorMsg)
	}
}
