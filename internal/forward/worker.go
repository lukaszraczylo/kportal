package forward

import (
	"context"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/healthcheck"
	"github.com/nvm/kportal/internal/k8s"
	"github.com/nvm/kportal/internal/retry"
)

// ForwardWorker manages a single port-forward connection with automatic retry.
type ForwardWorker struct {
	forward       config.Forward
	portForwarder *k8s.PortForwarder
	ctx           context.Context
	cancel        context.CancelFunc
	stopChan      chan struct{}
	doneChan      chan struct{}
	verbose       bool
	lastPod       string // Track the last pod we connected to
	statusUI      StatusUpdater
	healthChecker *healthcheck.Checker
	startTime     time.Time // Track when the worker started
}

// NewForwardWorker creates a new ForwardWorker for a single forward configuration.
func NewForwardWorker(fwd config.Forward, portForwarder *k8s.PortForwarder, verbose bool, statusUI StatusUpdater, healthChecker *healthcheck.Checker) *ForwardWorker {
	ctx, cancel := context.WithCancel(context.Background())

	return &ForwardWorker{
		forward:       fwd,
		portForwarder: portForwarder,
		ctx:           ctx,
		cancel:        cancel,
		stopChan:      make(chan struct{}),
		doneChan:      make(chan struct{}),
		verbose:       verbose,
		statusUI:      statusUI,
		healthChecker: healthChecker,
		startTime:     time.Now(),
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
	<-w.doneChan // Wait for worker to finish
}

// run is the main worker loop that handles retries.
func (w *ForwardWorker) run() {
	defer close(w.doneChan)

	backoff := retry.NewBackoff()

	for {
		// Check if we should stop
		select {
		case <-w.ctx.Done():
			if w.verbose {
				log.Printf("[%s] Worker stopped", w.forward.ID())
			}
			return
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
			log.Printf("[%s] Failed to resolve resource: %v", w.forward.ID(), err)
			w.sleepWithBackoff(backoff)
			continue
		}

		// Check if pod changed (restart detected)
		if w.lastPod != "" && w.lastPod != podName {
			if w.healthChecker != nil {
				w.healthChecker.MarkReconnecting(w.forward.ID())
			}
			log.Printf("[%s] Switched to new pod: %s → %s", w.forward.ID(), w.lastPod, podName)
		} else if w.lastPod == "" {
			log.Printf("[%s] Forwarding %s → localhost:%d",
				w.forward.ID(), w.forward.String(), w.forward.LocalPort)
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
			log.Printf("[%s] Port-forward connection failed: %v", w.forward.ID(), err)

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

	// Start a goroutine to monitor for stop signal
	go func() {
		select {
		case <-w.stopChan:
			close(stopChan)
		case <-forwardCtx.Done():
			close(stopChan)
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

	// Create forward request
	req := &k8s.ForwardRequest{
		ContextName: w.forward.GetContext(),
		Namespace:   w.forward.GetNamespace(),
		Resource:    w.forward.Resource,
		Selector:    w.forward.Selector,
		LocalPort:   w.forward.LocalPort,
		RemotePort:  w.forward.Port,
		StopChan:    stopChan,
		ReadyChan:   readyChan,
		Out:         out,
		ErrOut:      errOut,
	}

	// Start port forwarding in a goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- w.portForwarder.Forward(forwardCtx, req)
	}()

	// Wait for ready or error
	select {
	case <-readyChan:
		if w.verbose {
			log.Printf("[%s] Port-forward connection established", w.forward.ID())
		}
	case err := <-errChan:
		return fmt.Errorf("failed to establish forward: %w", err)
	case <-w.ctx.Done():
		return nil
	case <-time.After(30 * time.Second):
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

// logWriter implements io.Writer to write log messages with a prefix.
type logWriter struct {
	prefix string
}

func (lw *logWriter) Write(p []byte) (n int, err error) {
	log.Printf("%s%s", lw.prefix, string(p))
	return len(p), nil
}
