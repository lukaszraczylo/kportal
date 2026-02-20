package forward

import (
	"context"
	"sync"
	"time"

	"github.com/lukaszraczylo/kportal/internal/events"
	"github.com/lukaszraczylo/kportal/internal/logger"
)

const (
	// defaultHeartbeatInterval is how often the watchdog sends heartbeats to workers
	defaultHeartbeatInterval = 15 * time.Second
)

// Watchdog monitors worker goroutines to detect hung workers.
// It centralizes heartbeat management - instead of each worker sending heartbeats,
// the watchdog polls workers periodically. This reduces goroutine count and
// simplifies worker implementation.
type Watchdog struct {
	ctx               context.Context
	workers           map[string]*workerState
	cancel            context.CancelFunc
	eventBus          *events.Bus
	wg                sync.WaitGroup
	checkInterval     time.Duration
	hangThreshold     time.Duration
	heartbeatInterval time.Duration
	mu                sync.RWMutex
}

// workerState tracks the health of a single worker
type workerState struct {
	lastHeartbeat  time.Time
	worker         HeartbeatResponder
	onHungCallback func(forwardID string)
	forwardID      string
	heartbeatCount uint64
	isHung         bool
}

// HeartbeatResponder is an interface for workers that can respond to heartbeat checks
type HeartbeatResponder interface {
	// IsAlive returns true if the worker is still responsive
	IsAlive() bool
	// GetForwardID returns the forward ID this worker manages
	GetForwardID() string
}

// NewWatchdog creates a new goroutine watchdog
func NewWatchdog(checkInterval, hangThreshold time.Duration) *Watchdog {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watchdog{
		workers:           make(map[string]*workerState),
		checkInterval:     checkInterval,
		hangThreshold:     hangThreshold,
		heartbeatInterval: defaultHeartbeatInterval,
		ctx:               ctx,
		cancel:            cancel,
	}
}

// SetEventBus sets the event bus for publishing watchdog events
func (w *Watchdog) SetEventBus(bus *events.Bus) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.eventBus = bus
}

// Start begins the watchdog monitoring loop
func (w *Watchdog) Start() {
	w.wg.Add(1)
	go w.monitorLoop()
}

// Stop stops the watchdog
func (w *Watchdog) Stop() {
	w.cancel()
	w.wg.Wait()
}

// RegisterWorker adds a worker to monitor
func (w *Watchdog) RegisterWorker(forwardID string, onHungCallback func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.workers[forwardID] = &workerState{
		forwardID:      forwardID,
		lastHeartbeat:  time.Now(),
		heartbeatCount: 0,
		isHung:         false,
		onHungCallback: onHungCallback,
	}

	logger.Debug("Watchdog registered worker", map[string]interface{}{
		"forward_id": forwardID,
	})
}

// RegisterWorkerWithResponder adds a worker to monitor with heartbeat polling support
func (w *Watchdog) RegisterWorkerWithResponder(forwardID string, responder HeartbeatResponder, onHungCallback func(string)) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.workers[forwardID] = &workerState{
		forwardID:      forwardID,
		lastHeartbeat:  time.Now(),
		heartbeatCount: 0,
		isHung:         false,
		onHungCallback: onHungCallback,
		worker:         responder,
	}

	logger.Debug("Watchdog registered worker with responder", map[string]interface{}{
		"forward_id": forwardID,
	})
}

// UnregisterWorker removes a worker from monitoring
func (w *Watchdog) UnregisterWorker(forwardID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.workers, forwardID)

	logger.Debug("Watchdog unregistered worker", map[string]interface{}{
		"forward_id": forwardID,
	})
}

// Heartbeat records that a worker is alive and processing.
// This can be called by workers directly (legacy) or the watchdog can poll
// workers via HeartbeatResponder interface.
func (w *Watchdog) Heartbeat(forwardID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if state, exists := w.workers[forwardID]; exists {
		state.lastHeartbeat = time.Now()
		state.heartbeatCount++
		state.isHung = false
	}
}

// GetWorkerState returns the current state of a worker (for testing)
func (w *Watchdog) GetWorkerState(forwardID string) (lastHeartbeat time.Time, count uint64, exists bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if state, ok := w.workers[forwardID]; ok {
		return state.lastHeartbeat, state.heartbeatCount, true
	}
	return time.Time{}, 0, false
}

// monitorLoop periodically checks all workers and polls for heartbeats
func (w *Watchdog) monitorLoop() {
	defer w.wg.Done()

	checkTicker := time.NewTicker(w.checkInterval)
	defer checkTicker.Stop()

	// Heartbeat polling ticker - polls workers for heartbeat more frequently
	heartbeatTicker := time.NewTicker(w.heartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-heartbeatTicker.C:
			// Poll all workers for heartbeat (centralized heartbeat management)
			w.pollHeartbeats()
		case <-checkTicker.C:
			// Check for hung workers
			w.checkWorkers()
		}
	}
}

// pollHeartbeats polls all registered workers for heartbeat.
// This centralizes heartbeat management in the watchdog instead of having
// each worker spawn its own heartbeat goroutine.
func (w *Watchdog) pollHeartbeats() {
	w.mu.Lock()
	defer w.mu.Unlock()

	now := time.Now()
	for forwardID, state := range w.workers {
		// If worker has a responder, poll it
		if state.worker != nil {
			if state.worker.IsAlive() {
				state.lastHeartbeat = now
				state.heartbeatCount++
				state.isHung = false
			}
		}
		// If no responder, worker must call Heartbeat() directly (legacy mode)
		// This maintains backward compatibility
		_ = forwardID // Avoid unused variable warning
	}
}

// hungWorkerInfo stores information about a hung worker for deferred callback execution
type hungWorkerInfo struct {
	callback  func(string)
	forwardID string
}

// checkWorkers checks all registered workers for hung state
func (w *Watchdog) checkWorkers() {
	// Collect hung workers while holding the lock
	var hungWorkers []hungWorkerInfo
	var eventBus *events.Bus

	w.mu.Lock()
	eventBus = w.eventBus
	now := time.Now()
	for forwardID, state := range w.workers {
		timeSinceHeartbeat := now.Sub(state.lastHeartbeat)

		// Check if worker is hung
		if timeSinceHeartbeat > w.hangThreshold {
			if !state.isHung {
				// First time detecting hung state
				state.isHung = true

				logger.Warn("Watchdog detected hung worker", map[string]interface{}{
					"forward_id":           forwardID,
					"time_since_heartbeat": timeSinceHeartbeat.String(),
					"hang_threshold":       w.hangThreshold.String(),
					"heartbeat_count":      state.heartbeatCount,
				})

				// Collect callback for deferred execution outside the lock
				if state.onHungCallback != nil {
					hungWorkers = append(hungWorkers, hungWorkerInfo{
						forwardID: forwardID,
						callback:  state.onHungCallback,
					})
				}
			}
		}
	}
	w.mu.Unlock()

	// Execute callbacks outside the lock to prevent deadlocks and ensure
	// consistent state during callback execution. Callbacks are idempotent
	// (they trigger reconnection via channels), so concurrent state changes
	// between detection and callback execution are safe.
	for _, hw := range hungWorkers {
		// Publish event if event bus is available
		if eventBus != nil {
			eventBus.Publish(events.NewWorkerHungEvent(hw.forwardID, w.hangThreshold.String()))
		}

		hw.callback(hw.forwardID)
	}
}
