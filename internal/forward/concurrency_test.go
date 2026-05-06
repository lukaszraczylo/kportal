package forward

import (
	"sync"
	"testing"
	"time"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/events"
	"github.com/stretchr/testify/assert"
)

// TestForwardWorker_Stop_Concurrent verifies that concurrent calls to Stop()
// are safe and do not panic from a double-close of stopChan (Bug 4).
// Run under -race to catch the underlying issue.
func TestForwardWorker_Stop_Concurrent(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 18080,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	// Pretend the run loop has finished so Stop() does not block on doneChan.
	close(worker.doneChan)

	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	start := make(chan struct{})

	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			// Each call must complete without panicking.
			worker.Stop()
		}()
	}

	close(start) // Release all goroutines simultaneously.
	wg.Wait()

	// stopChan must be closed exactly once and observable as closed.
	select {
	case <-worker.stopChan:
		// closed — expected
	default:
		t.Fatal("stopChan should be closed after Stop()")
	}
}

// TestForwardWorker_Stop_Idempotent verifies sequential repeated Stop calls
// also do not panic.
func TestForwardWorker_Stop_Idempotent(t *testing.T) {
	fwd := config.Forward{
		Resource:  "pod/my-app",
		LocalPort: 18081,
		Port:      80,
	}

	worker := NewForwardWorker(fwd, nil, false, nil, nil, nil)
	close(worker.doneChan)

	worker.Stop()
	worker.Stop()
	worker.Stop()
}

// TestManager_Reload_EmptyKeepsInfraAlive verifies Bug 2 fix: a Reload that
// drops to zero forwards must NOT tear down healthChecker / watchdog /
// eventBus, so subsequent reloads with forwards continue to work.
func TestManager_Reload_EmptyKeepsInfraAlive(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	// Start with an empty config (Start tolerates this without errors).
	emptyCfg := &config.Config{}
	if err := manager.Start(emptyCfg); err != nil {
		t.Fatalf("Start(empty) failed: %v", err)
	}

	// Capture references to long-lived components.
	hcBefore := manager.healthChecker
	wdBefore := manager.watchdog
	busBefore := manager.eventBus

	// Reload with another empty config - must not destroy these.
	if err := manager.Reload(&config.Config{}); err != nil {
		t.Fatalf("Reload(empty) failed: %v", err)
	}

	assert.Same(t, hcBefore, manager.healthChecker, "healthChecker must be preserved across empty reload")
	assert.Same(t, wdBefore, manager.watchdog, "watchdog must be preserved across empty reload")
	assert.Same(t, busBefore, manager.eventBus, "eventBus must be preserved across empty reload")

	// Event bus must still accept subscribers (would panic / fail if Close was called).
	manager.eventBus.SubscribeAll(func(_ events.Event) {})
}

// TestManager_CurrentConfig_RaceFree exercises Bug 1: concurrent Reload and
// reads of currentConfig (as performed by the health-checker callback path)
// must be race-free under -race.
func TestManager_CurrentConfig_RaceFree(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	defer manager.Stop()

	cfgA := &config.Config{}
	cfgB := &config.Config{}
	if err := manager.Start(cfgA); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Writer goroutine: alternates between two configs via Reload.
	wg.Add(1)
	go func() {
		defer wg.Done()
		toggle := false
		for {
			select {
			case <-stop:
				return
			default:
			}
			if toggle {
				_ = manager.Reload(cfgA)
			} else {
				_ = manager.Reload(cfgB)
			}
			toggle = !toggle
		}
	}()

	// Reader goroutines: emulate health-checker callback's read of
	// currentConfig. Use the same locking discipline as the production code.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				manager.workersMu.RLock()
				cfg := manager.currentConfig
				_ = cfg
				manager.workersMu.RUnlock()
			}
		}()
	}

	time.Sleep(150 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestManager_Stop_Idempotent verifies that calling Manager.Stop() multiple
// times — sequentially or concurrently — does not panic from a double-close
// of eventBus or a double Stop on healthChecker/watchdog. The body of Stop()
// is wrapped in sync.Once.
func TestManager_Stop_Idempotent(t *testing.T) {
	manager, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	if err := manager.Start(&config.Config{}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Sequential double-stop must not panic.
	manager.Stop()
	manager.Stop()

	// Build a second manager and call Stop concurrently from many goroutines —
	// any non-idempotent close path would panic at least one of them.
	m2, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping test - no kubeconfig available")
	}
	if err := m2.Start(&config.Config{}); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			<-start
			m2.Stop()
		}()
	}
	close(start)
	wg.Wait()
}
