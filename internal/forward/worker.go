package forward

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/healthcheck"
	"github.com/nvm/kportal/internal/httplog"
	"github.com/nvm/kportal/internal/k8s"
	"github.com/nvm/kportal/internal/logger"
	"github.com/nvm/kportal/internal/retry"
)

const (
	portForwardReadyTimeout = 30 * time.Second
	httpLogPortOffset       = 10000 // Offset for internal port when HTTP logging is enabled
)

// ForwardWorker manages a single port-forward connection with automatic retry.
type ForwardWorker struct {
	startTime       time.Time
	statusUI        StatusUpdater
	ctx             context.Context
	reconnectChan   chan string
	httpProxy       *httplog.Proxy
	watchdog        *Watchdog
	cancel          context.CancelFunc
	doneChan        chan struct{}
	portForwarder   *k8s.PortForwarder
	successChan     chan struct{}
	healthChecker   *healthcheck.Checker
	forwardCancel   context.CancelFunc
	stopChan        chan struct{}
	lastPod         string
	forward         config.Forward
	forwardCancelMu sync.Mutex
	verbose         bool
}

// NewForwardWorker creates a new ForwardWorker for a single forward configuration.
func NewForwardWorker(fwd config.Forward, portForwarder *k8s.PortForwarder, verbose bool, statusUI StatusUpdater, healthChecker *healthcheck.Checker, watchdog *Watchdog) *ForwardWorker {
	ctx, cancel := context.WithCancel(context.Background())

	return &ForwardWorker{
		forward:       fwd,
		portForwarder: portForwarder,
		ctx:           ctx,
		cancel:        cancel,
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
		reconnectChan: make(chan string, 1),   // Buffered to avoid blocking
		successChan:   make(chan struct{}, 1), // Buffered to avoid blocking
		verbose:       verbose,
		statusUI:      statusUI,
		healthChecker: healthChecker,
		watchdog:      watchdog,
		startTime:     time.Now(),
	}
}

// signalConnectionSuccess signals that a connection was successfully established.
// This is used to reset the backoff timer after a successful connection.
func (w *ForwardWorker) signalConnectionSuccess() {
	select {
	case w.successChan <- struct{}{}:
	default:
		// Channel already has pending signal
	}
}

// TriggerReconnect triggers a reconnection (e.g., due to stale connection)
func (w *ForwardWorker) TriggerReconnect(reason string) {
	// Cancel current forward if running
	w.forwardCancelMu.Lock()
	if w.forwardCancel != nil {
		w.forwardCancel()
	}
	w.forwardCancelMu.Unlock()

	// Send reconnect signal (non-blocking)
	select {
	case w.reconnectChan <- reason:
	default:
		// Channel already has pending reconnect
	}
}

// Start begins the port-forward worker in a goroutine.
// The worker will continuously retry on failures with exponential backoff.
func (w *ForwardWorker) Start() {
	go w.run()
}

// Stop gracefully stops the port-forward worker.
func (w *ForwardWorker) Stop() {
	w.cancel()
	close(w.stopChan)

	// Wait for worker to finish with timeout to prevent blocking forever
	select {
	case <-w.doneChan:
		// Worker finished gracefully
	case <-time.After(3 * time.Second):
		// Worker didn't finish in time, but we've cancelled its context
		// so it will clean up eventually
		log.Printf("[%s] Worker stop timed out, continuing...", w.forward.ID())
	}
}

// IsAlive implements HeartbeatResponder interface.
// Returns true if the worker goroutine is still running and responsive.
func (w *ForwardWorker) IsAlive() bool {
	select {
	case <-w.doneChan:
		return false
	case <-w.ctx.Done():
		return false
	default:
		return true
	}
}

// GetForwardID implements HeartbeatResponder interface.
func (w *ForwardWorker) GetForwardID() string {
	return w.forward.ID()
}

// run is the main worker loop that handles retries.
func (w *ForwardWorker) run() {
	defer close(w.doneChan)
	defer w.stopHTTPProxy() // Ensure proxy is stopped on exit

	// Note: Heartbeat management is now centralized in the Watchdog.
	// The watchdog polls workers via the HeartbeatResponder interface (IsAlive method)
	// instead of each worker spawning its own heartbeat goroutine.
	// This reduces goroutine count from 2N to N for N workers.

	// Start HTTP logging proxy if enabled
	if err := w.startHTTPProxy(); err != nil {
		logger.Error("Failed to start HTTP logging proxy", map[string]any{
			"forward_id": w.forward.ID(),
			"error":      err.Error(),
		})
		// Continue without HTTP logging
	}

	backoff := retry.NewBackoff()

	for {
		// Check if we should stop or reset backoff on successful connection
		select {
		case <-w.ctx.Done():
			if w.verbose {
				log.Printf("[%s] Worker stopped", w.forward.ID())
			}
			return
		case <-w.successChan:
			// Reset backoff after successful connection
			backoff.Reset()
		default:
		}

		// Resolve the resource to get current pod name
		podName, err := w.portForwarder.GetPodForResource(
			w.ctx,
			w.forward.GetContext(),
			w.forward.GetNamespace(),
			w.forward.Resource,
			w.forward.Selector,
		)

		if err != nil {
			logger.Error("Failed to resolve resource", map[string]any{
				"forward_id": w.forward.ID(),
				"context":    w.forward.GetContext(),
				"namespace":  w.forward.GetNamespace(),
				"resource":   w.forward.Resource,
				"error":      err.Error(),
			})
			w.sleepWithBackoff(backoff)
			continue
		}

		// Check if pod changed (restart detected)
		if w.lastPod != "" && w.lastPod != podName {
			if w.healthChecker != nil {
				w.healthChecker.MarkReconnecting(w.forward.ID())
			}
			logger.Info("Pod restart detected, switching to new pod", map[string]any{
				"forward_id": w.forward.ID(),
				"old_pod":    w.lastPod,
				"new_pod":    podName,
				"context":    w.forward.GetContext(),
				"namespace":  w.forward.GetNamespace(),
			})
		} else if w.lastPod == "" {
			logger.Info("Starting port forward", map[string]any{
				"forward_id": w.forward.ID(),
				"target":     w.forward.String(),
				"local_port": w.forward.LocalPort,
				"pod":        podName,
			})
			if w.healthChecker != nil {
				w.healthChecker.MarkStarting(w.forward.ID())
			}
		}

		w.lastPod = podName

		// Establish port-forward connection
		err = w.establishForward(podName)

		if err != nil {
			// Connection failed or was interrupted
			if w.ctx.Err() != nil {
				// Context was cancelled, exit gracefully
				return
			}

			// Update status to reconnecting
			if w.healthChecker != nil {
				w.healthChecker.MarkReconnecting(w.forward.ID())
			}

			// Log the error
			logger.Warn("Port-forward connection failed, will retry", map[string]any{
				"forward_id": w.forward.ID(),
				"context":    w.forward.GetContext(),
				"namespace":  w.forward.GetNamespace(),
				"resource":   w.forward.Resource,
				"local_port": w.forward.LocalPort,
				"error":      err.Error(),
			})

			// Clear last pod so we re-resolve on next attempt
			w.lastPod = ""

			// Wait with backoff before retrying
			w.sleepWithBackoff(backoff)
			continue
		}

		// Connection closed normally (shouldn't happen unless stopped)
		if w.ctx.Err() != nil {
			return
		}

		// Connection closed unexpectedly, retry
		log.Printf("[%s] Connection closed unexpectedly, retrying...", w.forward.ID())
		w.lastPod = ""
		w.sleepWithBackoff(backoff)
	}
}

// establishForward establishes a port-forward connection.
// This blocks until the connection is closed or an error occurs.
func (w *ForwardWorker) establishForward(podName string) error {
	// Create channels for this forward
	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{}, 1)

	// Create a context for this forward attempt
	forwardCtx, forwardCancel := context.WithCancel(w.ctx)
	defer forwardCancel()

	// Store cancel function so TriggerReconnect can use it
	w.forwardCancelMu.Lock()
	w.forwardCancel = forwardCancel
	w.forwardCancelMu.Unlock()

	defer func() {
		w.forwardCancelMu.Lock()
		w.forwardCancel = nil
		w.forwardCancelMu.Unlock()
	}()

	// Use sync.Once to ensure stopChan is closed exactly once
	var closeOnce sync.Once
	closeStopChan := func() {
		closeOnce.Do(func() {
			close(stopChan)
		})
	}

	// Ensure stopChan is closed when this function exits (prevents goroutine leak)
	defer closeStopChan()

	// Start a goroutine to monitor for stop signal and reconnect triggers
	go func() {
		select {
		case <-w.stopChan:
			closeStopChan()
		case <-w.reconnectChan:
			closeStopChan()
		case <-forwardCtx.Done():
			closeStopChan()
		}
	}()

	// Set up output writers
	var out, errOut io.Writer
	if w.verbose {
		out = &logWriter{prefix: fmt.Sprintf("[%s] ", w.forward.ID())}
		errOut = &logWriter{prefix: fmt.Sprintf("[%s] ERROR: ", w.forward.ID())}
	} else {
		out = io.Discard
		errOut = io.Discard
	}

	// Determine local port for k8s port-forward
	// If HTTP logging is enabled, we bind to an internal port and the proxy listens on the user-facing port
	localPort := w.forward.LocalPort
	if w.httpProxy != nil {
		localPort = w.httpProxy.GetTargetPort()
	}

	// Create forward request
	req := &k8s.ForwardRequest{
		ContextName: w.forward.GetContext(),
		Namespace:   w.forward.GetNamespace(),
		Resource:    w.forward.Resource,
		Selector:    w.forward.Selector,
		LocalPort:   localPort,
		RemotePort:  w.forward.Port,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		Out:         out,
		ErrOut:      errOut,
	}

	// Start port forwarding in a goroutine
	errChan := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				errChan <- fmt.Errorf("port forward panicked: %v", r)
			}
		}()
		errChan <- w.portForwarder.Forward(forwardCtx, req)
	}()

	// Wait for ready or error
	select {
	case <-readyChan:
		if w.verbose {
			log.Printf("[%s] Port-forward connection established", w.forward.ID())
		}
		// Mark connection as established in health checker
		if w.healthChecker != nil {
			w.healthChecker.MarkConnected(w.forward.ID())
		}
		// Signal success back to caller so backoff can be reset
		w.signalConnectionSuccess()
	case err := <-errChan:
		return fmt.Errorf("failed to establish forward: %w", err)
	case <-w.ctx.Done():
		return nil
	case <-time.After(portForwardReadyTimeout):
		return fmt.Errorf("timeout waiting for port-forward to become ready")
	}

	// Wait for connection to close or error
	select {
	case err := <-errChan:
		return err
	case <-w.ctx.Done():
		return nil
	}
}

// sleepWithBackoff waits for the next backoff duration.
// Returns early if the worker is stopped.
func (w *ForwardWorker) sleepWithBackoff(backoff *retry.Backoff) {
	delay := backoff.Next()

	if w.verbose {
		log.Printf("[%s] Retrying in %v (attempt %d)", w.forward.ID(), delay, backoff.Attempt())
	}

	select {
	case <-time.After(delay):
		// Continue with retry
	case <-w.ctx.Done():
		// Worker stopped
	}
}

// GetForward returns the forward configuration for this worker.
func (w *ForwardWorker) GetForward() config.Forward {
	return w.forward
}

// IsRunning returns true if the worker is running.
func (w *ForwardWorker) IsRunning() bool {
	select {
	case <-w.doneChan:
		return false
	default:
		return true
	}
}

// startHTTPProxy starts the HTTP logging proxy if enabled
func (w *ForwardWorker) startHTTPProxy() error {
	if !w.forward.IsHTTPLogEnabled() {
		return nil
	}

	// Calculate internal port for k8s tunnel
	targetPort := w.forward.LocalPort + httpLogPortOffset

	// Validate that the target port is available before attempting to bind
	portChecker := NewPortChecker()
	if !portChecker.isPortAvailable(targetPort) {
		usedBy := portChecker.getProcessUsingPort(targetPort)
		return fmt.Errorf("HTTP proxy target port %d is already in use by %s (forward port %d + offset %d)",
			targetPort, usedBy, w.forward.LocalPort, httpLogPortOffset)
	}

	proxy, err := httplog.NewProxy(&w.forward, targetPort)
	if err != nil {
		return fmt.Errorf("failed to create HTTP proxy: %w", err)
	}

	if err := proxy.Start(); err != nil {
		return fmt.Errorf("failed to start HTTP proxy: %w", err)
	}

	w.httpProxy = proxy

	logger.Info("HTTP logging proxy started", map[string]any{
		"forward_id":  w.forward.ID(),
		"local_port":  w.forward.LocalPort,
		"target_port": targetPort,
	})

	return nil
}

// stopHTTPProxy stops the HTTP logging proxy if running
func (w *ForwardWorker) stopHTTPProxy() {
	if w.httpProxy != nil {
		if err := w.httpProxy.Stop(); err != nil {
			logger.Warn("Failed to stop HTTP proxy", map[string]any{
				"forward_id": w.forward.ID(),
				"error":      err.Error(),
			})
		}
		w.httpProxy = nil
	}
}

// GetHTTPProxy returns the HTTP logging proxy if active
func (w *ForwardWorker) GetHTTPProxy() *httplog.Proxy {
	return w.httpProxy
}

// logWriter implements io.Writer to write log messages with a prefix.
type logWriter struct {
	prefix string
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	log.Printf("%s%s", lw.prefix, string(p))
	return len(p), nil
}
