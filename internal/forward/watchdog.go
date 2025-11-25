package forward

import (
	"context"
	"sync"
	"time"

	"github.com/nvm/kportal/internal/logger"
)

// Watchdog monitors worker goroutines to detect hung workers
type Watchdog struct {
	mu            sync.RWMutex
	workers       map[string]*workerState // key: forward ID
	checkInterval time.Duration
	hangThreshold time.Duration // How long without heartbeat before considered hung
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// workerState tracks the health of a single worker
type workerState struct {
	forwardID      string
	lastHeartbeat  time.Time
	heartbeatCount uint64
	isHung         bool
	onHungCallback func(forwardID string)
}

// NewWatchdog creates a new goroutine watchdog
func NewWatchdog(checkInterval, hangThreshold time.Duration) *Watchdog {
	ctx, cancel := context.WithCancel(context.Background())
	return &Watchdog{
		workers:       make(map[string]*workerState),
		checkInterval: checkInterval,
		hangThreshold: hangThreshold,
		ctx:           ctx,
		cancel:        cancel,
	}
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

// UnregisterWorker removes a worker from monitoring
func (w *Watchdog) UnregisterWorker(forwardID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.workers, forwardID)

	logger.Debug("Watchdog unregistered worker", map[string]interface{}{
		"forward_id": forwardID,
	})
}

// Heartbeat records that a worker is alive and processing
// Workers should call this periodically (e.g., in their main loop)
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

// monitorLoop periodically checks all workers
func (w *Watchdog) monitorLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.checkWorkers()
		}
	}
}

// hungWorkerInfo stores information about a hung worker for deferred callback execution
type hungWorkerInfo struct {
	forwardID string
	callback  func(string)
}

// checkWorkers checks all registered workers for hung state
func (w *Watchdog) checkWorkers() {
	// Collect hung workers while holding the lock
	var hungWorkers []hungWorkerInfo

	w.mu.Lock()
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
		hw.callback(hw.forwardID)
	}
}
