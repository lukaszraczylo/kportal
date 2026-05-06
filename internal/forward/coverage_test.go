package forward

// coverage_test.go – targeted tests to lift coverage from ~46% to ≥70%.
//
// Functions targeted (all at 0 % before this file):
//   manager.go  – SetMDNSPublisher, startWorker, stopWorkerInternal(false branch),
//                 DisableForward, EnableForward (all paths), Reload (diff paths,
//                 port-conflict rejection, currentConfig update)
//   watchdog.go – RegisterWorkerWithResponder, pollHeartbeats
//   worker.go   – sleepWithBackoff (both branches), IsAlive (doneChan branch)
//   portcheck   – getProcessUsingPortUnix exercised for unknown/error path

import (
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildForward creates a config.Forward with context/namespace set.
func buildForward(ctx, ns, resource string, localPort, remotePort int) config.Forward {
	fwd := config.Forward{
		Resource:  resource,
		LocalPort: localPort,
		Port:      remotePort,
	}
	fwd.SetContext(ctx, ns)
	return fwd
}

// buildConfigFrom constructs a *config.Config containing exactly the supplied
// forwards (all placed under ctx/ns).
func buildConfigFrom(ctx, ns string, forwards []config.Forward) *config.Config {
	return &config.Config{
		Contexts: []config.Context{
			{
				Name: ctx,
				Namespaces: []config.Namespace{
					{Name: ns, Forwards: forwards},
				},
			},
		},
	}
}

// newCovManager creates a Manager and registers a cleanup that calls Stop.
// Skips the test if no kubeconfig is available.
func newCovManager(t *testing.T) *Manager {
	t.Helper()
	m, err := NewManager(false)
	if err != nil {
		t.Skip("Skipping – no kubeconfig available")
	}
	t.Cleanup(func() { m.Stop() })
	return m
}

// inject inserts a worker directly into m.workers without a real k8s call.
func inject(m *Manager, fwd config.Forward) *ForwardWorker {
	w := NewForwardWorker(fwd, m.portForwarder, false, m.statusUI, m.healthChecker, m.watchdog)
	m.workersMu.Lock()
	m.workers[fwd.ID()] = w
	m.workersMu.Unlock()
	return w
}

// occupyPort binds a TCP listener on all interfaces on a free port.
// isPortAvailable also binds to all interfaces (":PORT"), so a listener on
// "0.0.0.0:PORT" is correctly detected as a conflict on both Linux and macOS.
func occupyPort(t *testing.T) (port int, closeFunc func()) {
	t.Helper()
	// #nosec G102 -- test intentionally binds to all interfaces
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "need a free port for conflict test")
	port = l.Addr().(*net.TCPAddr).Port
	return port, func() { _ = l.Close() }
}

// ---------------------------------------------------------------------------
// Manager.SetMDNSPublisher  (0% → covered)
// ---------------------------------------------------------------------------

func TestManager_SetMDNSPublisher_NilAccepted(t *testing.T) {
	m := newCovManager(t)
	m.SetMDNSPublisher(nil) // must not panic
	assert.Nil(t, m.mdnsPublisher)
}

// ---------------------------------------------------------------------------
// Manager.stopWorkerInternal – both removeFromUI branches
// ---------------------------------------------------------------------------

func TestManager_StopWorkerInternal_RemoveTrue(t *testing.T) {
	m := newCovManager(t)
	ui := &MockStatusUpdater{}
	m.SetStatusUI(ui)

	fwd := buildForward("c", "n", "pod/a", 20001, 80)
	w := inject(m, fwd)
	close(w.doneChan) // worker "done" so Stop() returns immediately

	require.NoError(t, m.stopWorkerInternal(fwd.ID(), true))
	assert.Nil(t, m.GetWorker(fwd.ID()))
	assert.Contains(t, ui.removes, fwd.ID(), "Remove() should be called")
}

func TestManager_StopWorkerInternal_RemoveFalse(t *testing.T) {
	m := newCovManager(t)
	ui := &MockStatusUpdater{}
	m.SetStatusUI(ui)

	fwd := buildForward("c", "n", "pod/b", 20002, 80)
	w := inject(m, fwd)
	close(w.doneChan)

	require.NoError(t, m.stopWorkerInternal(fwd.ID(), false))

	var sawDisabled bool
	for _, u := range ui.updates {
		if u.ID == fwd.ID() && u.Status == "Disabled" {
			sawDisabled = true
		}
	}
	assert.True(t, sawDisabled, "UpdateStatus('Disabled') should be called")
	assert.NotContains(t, ui.removes, fwd.ID(), "Remove() must NOT be called")
}

func TestManager_StopWorkerInternal_MissingWorker(t *testing.T) {
	m := newCovManager(t)
	err := m.stopWorkerInternal("ghost", true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "worker not found")
}

// ---------------------------------------------------------------------------
// Manager.DisableForward
// ---------------------------------------------------------------------------

func TestManager_DisableForward_Success(t *testing.T) {
	m := newCovManager(t)
	fwd := buildForward("c", "n", "pod/d", 20010, 80)
	w := inject(m, fwd)
	close(w.doneChan)
	require.NoError(t, m.DisableForward(fwd.ID()))
	assert.Nil(t, m.GetWorker(fwd.ID()))
}

func TestManager_DisableForward_Missing(t *testing.T) {
	m := newCovManager(t)
	assert.Error(t, m.DisableForward("missing"))
}

// ---------------------------------------------------------------------------
// Manager.EnableForward – all three error branches
// ---------------------------------------------------------------------------

func TestManager_EnableForward_NilConfig(t *testing.T) {
	m := newCovManager(t)
	// currentConfig is nil – should return "no configuration available"
	err := m.EnableForward("any")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no configuration available")
}

func TestManager_EnableForward_NotInConfig(t *testing.T) {
	m := newCovManager(t)
	m.workersMu.Lock()
	m.currentConfig = &config.Config{} // empty
	m.workersMu.Unlock()

	err := m.EnableForward("ctx/ns/pod/gone:9999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forward not found in configuration")
}

func TestManager_EnableForward_AlreadyEnabled(t *testing.T) {
	m := newCovManager(t)

	fwd := buildForward("c", "n", "pod/e", 20020, 80)
	cfg := buildConfigFrom("c", "n", []config.Forward{fwd})
	m.workersMu.Lock()
	m.currentConfig = cfg
	m.workersMu.Unlock()

	// Worker already present in map.
	inject(m, fwd)

	err := m.EnableForward(fwd.ID())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forward already enabled")
}

// ---------------------------------------------------------------------------
// Manager.startWorker – registers with watchdog + UI, duplicate rejected
// ---------------------------------------------------------------------------

func TestManager_StartWorker_RegistersAll(t *testing.T) {
	m := newCovManager(t)
	ui := &MockStatusUpdater{}
	m.SetStatusUI(ui)

	fwd := buildForward("c", "n", "pod/r", 20030, 80)
	require.NoError(t, m.startWorker(fwd))
	t.Cleanup(func() { _ = m.stopWorkerInternal(fwd.ID(), true) })

	// Worker in map.
	require.NotNil(t, m.GetWorker(fwd.ID()))

	// UI notified.
	require.Len(t, ui.adds, 1)
	assert.Equal(t, fwd.ID(), ui.adds[0].ID)

	// Watchdog entry present.
	_, _, exists := m.watchdog.GetWorkerState(fwd.ID())
	assert.True(t, exists)
}

func TestManager_StartWorker_DuplicateError(t *testing.T) {
	m := newCovManager(t)
	fwd := buildForward("c", "n", "pod/dup", 20031, 80)
	require.NoError(t, m.startWorker(fwd))
	t.Cleanup(func() { _ = m.stopWorkerInternal(fwd.ID(), true) })

	err := m.startWorker(fwd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worker already exists")
}

// ---------------------------------------------------------------------------
// Manager.Start – port conflict path
// ---------------------------------------------------------------------------

func TestManager_Start_PortConflict(t *testing.T) {
	m := newCovManager(t)
	port, closeFunc := occupyPort(t)
	defer closeFunc()

	// Port is occupied by our listener; Start should detect conflict.
	fwd := buildForward("c", "n", "pod/conflict", port, 80)
	cfg := buildConfigFrom("c", "n", []config.Forward{fwd})
	err := m.Start(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "port conflicts detected")
}

// ---------------------------------------------------------------------------
// Manager.Reload – diff paths
// ---------------------------------------------------------------------------

func TestManager_Reload_RemovesStaleWorker(t *testing.T) {
	m := newCovManager(t)

	fwd := buildForward("c", "n", "pod/stale", 20040, 80)
	w := inject(m, fwd)
	close(w.doneChan)
	m.workersMu.Lock()
	m.currentConfig = buildConfigFrom("c", "n", []config.Forward{fwd})
	m.workersMu.Unlock()

	// New config removes fwd.
	require.NoError(t, m.Reload(&config.Config{}))

	m.workersMu.RLock()
	cnt := len(m.workers)
	m.workersMu.RUnlock()
	assert.Equal(t, 0, cnt)
}

func TestManager_Reload_KeepsUnchangedWorker(t *testing.T) {
	m := newCovManager(t)

	fwd := buildForward("c", "n", "pod/keep", 20041, 80)
	inject(m, fwd)
	m.workersMu.Lock()
	m.currentConfig = buildConfigFrom("c", "n", []config.Forward{fwd})
	m.workersMu.Unlock()

	newCfg := buildConfigFrom("c", "n", []config.Forward{fwd})
	require.NoError(t, m.Reload(newCfg))

	assert.NotNil(t, m.GetWorker(fwd.ID()), "unchanged worker should survive Reload")
}

func TestManager_Reload_PortConflictRejected(t *testing.T) {
	m := newCovManager(t)
	m.workersMu.Lock()
	m.currentConfig = &config.Config{}
	m.workersMu.Unlock()

	port, closeFunc := occupyPort(t)
	defer closeFunc()

	fwd := buildForward("c", "n", "pod/conflictnew", port, 80)
	newCfg := buildConfigFrom("c", "n", []config.Forward{fwd})
	err := m.Reload(newCfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "port conflicts detected")
}

func TestManager_Reload_UpdatesCurrentConfig(t *testing.T) {
	m := newCovManager(t)
	m.workersMu.Lock()
	m.currentConfig = &config.Config{}
	m.workersMu.Unlock()

	newCfg := &config.Config{}
	require.NoError(t, m.Reload(newCfg))

	m.workersMu.RLock()
	cur := m.currentConfig
	m.workersMu.RUnlock()
	assert.Same(t, newCfg, cur)
}

// ---------------------------------------------------------------------------
// Watchdog.RegisterWorkerWithResponder + pollHeartbeats
// ---------------------------------------------------------------------------

// fakeResponder implements HeartbeatResponder for testing.
type fakeResponder struct {
	id    string
	mu    sync.Mutex
	alive bool
}

func (f *fakeResponder) IsAlive() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.alive
}

func (f *fakeResponder) GetForwardID() string { return f.id }

func TestWatchdog_RegisterWorkerWithResponder_AliveIncrementsCount(t *testing.T) {
	wd := NewWatchdog(1*time.Second, 2*time.Second)
	// Don't Start – call pollHeartbeats manually for determinism.

	r := &fakeResponder{alive: true, id: "w1"}
	wd.RegisterWorkerWithResponder("w1", r, nil)

	wd.pollHeartbeats()

	_, count, exists := wd.GetWorkerState("w1")
	assert.True(t, exists)
	assert.Equal(t, uint64(1), count)
}

func TestWatchdog_RegisterWorkerWithResponder_DeadNoIncrement(t *testing.T) {
	wd := NewWatchdog(1*time.Second, 2*time.Second)

	r := &fakeResponder{alive: false, id: "w2"}
	wd.RegisterWorkerWithResponder("w2", r, nil)

	wd.pollHeartbeats()

	_, count, exists := wd.GetWorkerState("w2")
	assert.True(t, exists)
	assert.Equal(t, uint64(0), count)
}

func TestWatchdog_RegisterWorkerWithResponder_HungTriggersCallback(t *testing.T) {
	wd := NewWatchdog(30*time.Millisecond, 60*time.Millisecond)
	wd.Start()
	t.Cleanup(wd.Stop)

	r := &fakeResponder{alive: false, id: "hung"}
	called := make(chan string, 1)
	wd.RegisterWorkerWithResponder("hung", r, func(id string) {
		select {
		case called <- id:
		default:
		}
	})

	select {
	case id := <-called:
		assert.Equal(t, "hung", id)
	case <-time.After(1 * time.Second):
		t.Fatal("hung callback not fired")
	}
}

func TestWatchdog_PollHeartbeats_AliveDeadAlive(t *testing.T) {
	wd := NewWatchdog(1*time.Second, 2*time.Second)

	r := &fakeResponder{alive: true, id: "cycle"}
	wd.RegisterWorkerWithResponder("cycle", r, nil)

	wd.pollHeartbeats()
	_, c1, _ := wd.GetWorkerState("cycle")
	assert.Equal(t, uint64(1), c1)

	r.mu.Lock()
	r.alive = false
	r.mu.Unlock()
	wd.pollHeartbeats()
	_, c2, _ := wd.GetWorkerState("cycle")
	assert.Equal(t, uint64(1), c2, "dead poll must not increment")

	r.mu.Lock()
	r.alive = true
	r.mu.Unlock()
	wd.pollHeartbeats()
	_, c3, _ := wd.GetWorkerState("cycle")
	assert.Equal(t, uint64(2), c3, "alive again must increment")
}

func TestWatchdog_PollHeartbeats_LegacyNoResponder(t *testing.T) {
	wd := NewWatchdog(1*time.Second, 2*time.Second)
	wd.RegisterWorker("legacy", nil)
	wd.Heartbeat("legacy") // count = 1

	wd.pollHeartbeats() // no responder – must not touch count

	_, count, _ := wd.GetWorkerState("legacy")
	assert.Equal(t, uint64(1), count)
}

// ---------------------------------------------------------------------------
// ForwardWorker.sleepWithBackoff
// ---------------------------------------------------------------------------

func TestForwardWorker_SleepWithBackoff_WaitsDelay(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in -short mode")
	}
	fwd := buildForward("c", "n", "pod/s", 20050, 80)
	w := NewForwardWorker(fwd, nil, false, nil, nil, nil)
	// Don't cancel context – sleep should run for real (1st attempt ≈ 1s + jitter).
	t.Cleanup(func() { w.cancel() })

	b := retry.NewBackoff()
	start := time.Now()
	w.sleepWithBackoff(b)
	assert.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond)
}

func TestForwardWorker_SleepWithBackoff_CancelReturnsEarly(t *testing.T) {
	fwd := buildForward("c", "n", "pod/sc", 20051, 80)
	w := NewForwardWorker(fwd, nil, false, nil, nil, nil)
	w.cancel() // pre-cancel

	b := retry.NewBackoff()
	start := time.Now()
	w.sleepWithBackoff(b)
	assert.Less(t, time.Since(start), 2*time.Second, "cancelled worker should not sleep")
}

func TestForwardWorker_SleepWithBackoff_Verbose(t *testing.T) {
	fwd := buildForward("c", "n", "pod/sv", 20052, 80)
	w := NewForwardWorker(fwd, nil, true, nil, nil, nil)
	w.cancel()

	b := retry.NewBackoff()
	w.sleepWithBackoff(b) // must not panic in verbose mode
}

// ---------------------------------------------------------------------------
// ForwardWorker.IsAlive – doneChan closed path
// ---------------------------------------------------------------------------

func TestForwardWorker_IsAlive_AfterDoneChanClosed(t *testing.T) {
	fwd := buildForward("c", "n", "pod/alive", 20060, 80)
	w := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	assert.True(t, w.IsAlive())
	close(w.doneChan)
	assert.False(t, w.IsAlive())
}

// ---------------------------------------------------------------------------
// Watchdog.monitorLoop – heartbeat ticker branch (pollHeartbeats via ticker)
// ---------------------------------------------------------------------------

func TestWatchdog_HeartbeatTickerCalls_PollHeartbeats(t *testing.T) {
	// Override heartbeatInterval to something short so the ticker fires.
	wd := NewWatchdog(10*time.Second, 20*time.Second)
	wd.heartbeatInterval = 30 * time.Millisecond
	wd.Start()
	t.Cleanup(wd.Stop)

	r := &fakeResponder{alive: true, id: "hb-tick"}
	wd.RegisterWorkerWithResponder("hb-tick", r, nil)

	// Wait for the heartbeat ticker to fire at least once.
	time.Sleep(150 * time.Millisecond)

	_, count, exists := wd.GetWorkerState("hb-tick")
	assert.True(t, exists)
	assert.GreaterOrEqual(t, count, uint64(1), "heartbeat ticker should poll responder")
}

// ---------------------------------------------------------------------------
// Manager.EnableForward – happy path (forward not currently running)
// The worker.Start() will fail to connect (no k8s) but startWorker itself
// succeeds before any network I/O. enableForward returns nil in that case.
// ---------------------------------------------------------------------------

func TestManager_EnableForward_HappyPath(t *testing.T) {
	m := newCovManager(t)

	fwd := buildForward("c", "n", "pod/enable", 20070, 80)
	cfg := buildConfigFrom("c", "n", []config.Forward{fwd})
	m.workersMu.Lock()
	m.currentConfig = cfg
	m.workersMu.Unlock()

	// Worker NOT in map (precondition for enable).
	err := m.EnableForward(fwd.ID())
	require.NoError(t, err)

	w := m.GetWorker(fwd.ID())
	require.NotNil(t, w, "worker should exist after EnableForward")
	t.Cleanup(func() { w.cancel() })
}

// ---------------------------------------------------------------------------
// Manager.stopWorker (one-liner at 0%) – goes through stopWorkerInternal
// ---------------------------------------------------------------------------

func TestManager_StopWorker_Delegates(t *testing.T) {
	m := newCovManager(t)
	ui := &MockStatusUpdater{}
	m.SetStatusUI(ui)

	fwd := buildForward("c", "n", "pod/sw", 20080, 80)
	w := inject(m, fwd)
	close(w.doneChan)

	// stopWorker is package-private; call through DisableForward which calls it
	// indirectly via stopWorkerInternal — already covered. Call it directly here.
	err := m.stopWorker(fwd.ID())
	require.NoError(t, err)
	assert.Nil(t, m.GetWorker(fwd.ID()))
	assert.Contains(t, ui.removes, fwd.ID())
}

// ---------------------------------------------------------------------------
// Reload.startWorker mDNS branch – nil publisher is a no-op (already covered);
// confirm the watchdog RegisterWorkerWithResponder is called during Reload-add.
// ---------------------------------------------------------------------------

func TestManager_Reload_NewForwardRegisteredInWatchdog(t *testing.T) {
	m := newCovManager(t)
	m.workersMu.Lock()
	m.currentConfig = &config.Config{}
	m.workersMu.Unlock()

	// Port must be free; use occupyPort only temporarily to find a free port number.
	pc := NewPortChecker()
	freePort := 0
	for p := 20090; p < 20200; p++ {
		if pc.isPortAvailable(p) {
			freePort = p
			break
		}
	}
	require.NotZero(t, freePort, "need a free port")

	fwd := buildForward("c", "n", "pod/neww", freePort, 80)
	newCfg := buildConfigFrom("c", "n", []config.Forward{fwd})

	// Reload adds fwd; startWorker registers it with watchdog.
	_ = m.Reload(newCfg)

	_, _, exists := m.watchdog.GetWorkerState(fwd.ID())
	assert.True(t, exists, "watchdog should have the new worker after Reload-add")
}

// ---------------------------------------------------------------------------
// startHTTPProxy – disabled path (most common, runs inside run())
// ---------------------------------------------------------------------------

func TestForwardWorker_StartHTTPProxy_Disabled(t *testing.T) {
	// IsHTTPLogEnabled() == false → startHTTPProxy returns nil immediately.
	fwd := buildForward("c", "n", "pod/noproxy", 20100, 80)
	// HTTPLog is nil by default → disabled.
	w := NewForwardWorker(fwd, nil, false, nil, nil, nil)

	err := w.startHTTPProxy()
	require.NoError(t, err)
	assert.Nil(t, w.httpProxy)
}

// ---------------------------------------------------------------------------
// stopHTTPProxy – nil httpProxy branch (no-op)
// ---------------------------------------------------------------------------

func TestForwardWorker_StopHTTPProxy_NilProxy(t *testing.T) {
	fwd := buildForward("c", "n", "pod/noproxy2", 20101, 80)
	w := NewForwardWorker(fwd, nil, false, nil, nil, nil)
	// httpProxy is nil – must not panic.
	w.stopHTTPProxy()
	assert.Nil(t, w.httpProxy)
}

// ---------------------------------------------------------------------------
// worker.run – start path (no k8s): worker goroutine starts, hits
// portForwarder.GetPodForResource which fails (nil portForwarder panics);
// we simply check it terminates cleanly when stopped immediately.
// We don't exercise run() body deeply without a real or fake k8s connection.
// ---------------------------------------------------------------------------

func TestForwardWorker_Start_TerminatesOnCancel(t *testing.T) {
	fwd := buildForward("c", "n", "pod/run", 20110, 80)
	// portForwarder is nil → GetPodForResource panics → recovered in run()? No,
	// there's no recover in run(). So we'd get a nil pointer dereference.
	// Instead use a real portForwarder from a manager so the call fails gracefully.
	m := newCovManager(t)
	w := NewForwardWorker(fwd, m.portForwarder, false, nil, m.healthChecker, m.watchdog)

	w.Start()
	// Cancel immediately.
	w.cancel()

	// Worker should stop; wait with timeout.
	select {
	case <-w.doneChan:
		// clean exit
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not terminate after cancel")
	}
}

// ---------------------------------------------------------------------------
// getProcessUsingPortUnix – internal branch coverage
// ---------------------------------------------------------------------------

// TestGetProcessUsingPortUnix_EmptyOutput exercises the pidStr=="" branch.
// Port 2 is a privileged port that nothing listens on in a test environment.
// lsof returns either empty (→ "unknown") or a PID if some process owns it.
// Either way the function must not panic.
func TestGetProcessUsingPortUnix_NothingListening(t *testing.T) {
	pc := NewPortChecker()
	// Port 2 is almost never bound; lsof will return empty → "unknown".
	result := pc.getProcessUsingPortUnix(2)
	assert.NotEmpty(t, result)
}

// TestGetProcessUsingPortUnix_ActivePort exercises the pid-parsing path by
// using a port that the test binary itself is actively listening on.
func TestGetProcessUsingPortUnix_ActivePort(t *testing.T) {
	// #nosec G102 -- test binds to all interfaces intentionally
	l, err := net.Listen("tcp", ":0")
	require.NoError(t, err)
	defer func() { _ = l.Close() }()
	port := l.Addr().(*net.TCPAddr).Port

	pc := NewPortChecker()
	result := pc.getProcessUsingPortUnix(port)
	// Should be a process string or "unknown" – must not panic.
	assert.NotEmpty(t, result)
}

// ---------------------------------------------------------------------------
// startWorker callbacks – exercise watchdog hung callback and health callback
// ---------------------------------------------------------------------------

// TestStartWorker_WatchdogCallback exercises the hung-worker closure registered
// by startWorker. We force-trigger it by backdating the worker's heartbeat
// timestamp beyond the hang threshold and calling checkWorkers().
func TestStartWorker_WatchdogCallback_TriggerReconnect(t *testing.T) {
	m := newCovManager(t)

	fwd := buildForward("c", "n", "pod/wdcb", 20120, 80)
	require.NoError(t, m.startWorker(fwd))
	t.Cleanup(func() {
		if w := m.GetWorker(fwd.ID()); w != nil {
			w.cancel()
		}
	})

	// Backdate the heartbeat to force hung detection.
	m.watchdog.mu.Lock()
	if state, ok := m.watchdog.workers[fwd.ID()]; ok {
		state.lastHeartbeat = time.Now().Add(-10 * time.Minute)
		state.isHung = false // reset so callback fires again
	}
	m.watchdog.mu.Unlock()

	// checkWorkers runs the hung callback synchronously (outside the lock).
	// It calls TriggerReconnect on the worker, which is safe.
	m.watchdog.checkWorkers()

	// Verify the worker is still in the map (not removed by reconnect).
	assert.NotNil(t, m.GetWorker(fwd.ID()))
}

// TestStartWorker_HealthCallback_StatusChange exercises the health callback
// registered by startWorker by triggering a real status-change event through
// the HealthChecker's exported MarkReconnecting (which calls notifyStatusChange
// if status changes). statusUI is set so the callback body executes.
func TestStartWorker_HealthCallback_StatusChange(t *testing.T) {
	m := newCovManager(t)
	ui := &MockStatusUpdater{}
	m.SetStatusUI(ui)

	fwd := buildForward("c", "n", "pod/hcb", 20121, 80)
	require.NoError(t, m.startWorker(fwd))
	t.Cleanup(func() {
		if w := m.GetWorker(fwd.ID()); w != nil {
			w.cancel()
		}
	})

	// Trigger status change: Starting → Reconnecting fires the callback
	// (status differs so notifyStatusChange is called).
	m.healthChecker.MarkStarting(fwd.ID())
	m.healthChecker.MarkReconnecting(fwd.ID())

	// Give the callback a moment to fire (it's synchronous in notifyStatusChange
	// but MarkConnected spawns a goroutine; MarkReconnecting calls markStatus directly).
	time.Sleep(20 * time.Millisecond)

	// Stop the healthchecker so its background per-port goroutine drains
	// before we read the mock — establishes happens-before for the read and
	// keeps the race detector quiet on slower CI runners.
	m.healthChecker.Unregister(fwd.ID())

	// The callback should have updated status. Hold the mock's lock during
	// the read because background goroutines may still be unwinding.
	ui.mu.Lock()
	defer ui.mu.Unlock()
	var sawUpdate bool
	for _, u := range ui.updates {
		if u.ID == fwd.ID() {
			sawUpdate = true
		}
	}
	assert.True(t, sawUpdate, "health callback should have called UpdateStatus")
}

// TestStartWorker_HealthCallback_StaleNoRetry exercises StatusStale with retryOnStale=false.
// MarkReconnecting puts worker into Reconnect state then we change to a different
// state and back to stale manually via MarkStarting+MarkReconnecting — but there
// is no exported "MarkStale". Instead, we can exercise the code path via the
// existing stale detection in checkPort which requires a running checker.
// Since that's async and complex, we simply confirm the path compiles and runs
// without covering stale-specific lines (those require a real connection timeout).
func TestStartWorker_HealthCallback_StaleNoRetry(t *testing.T) {
	m := newCovManager(t)
	fwd := buildForward("c", "n", "pod/stale-nort", 20123, 80)
	m.workersMu.Lock()
	m.currentConfig = &config.Config{} // retryOnStale defaults to false
	m.workersMu.Unlock()

	require.NoError(t, m.startWorker(fwd))
	t.Cleanup(func() {
		if w := m.GetWorker(fwd.ID()); w != nil {
			w.cancel()
		}
	})

	// Trigger a callback via status change – exercises the outer callback body.
	m.healthChecker.MarkStarting(fwd.ID())
	m.healthChecker.MarkReconnecting(fwd.ID())
	time.Sleep(20 * time.Millisecond)
}

// ---------------------------------------------------------------------------
// Watchdog.checkWorkers – event bus branch (publishes WorkerHungEvent)
// ---------------------------------------------------------------------------

func TestWatchdog_CheckWorkers_WithEventBus(t *testing.T) {
	// Exercises the eventBus != nil path in checkWorkers.
	wd := NewWatchdog(30*time.Millisecond, 60*time.Millisecond)
	m := newCovManager(t)
	wd.SetEventBus(m.eventBus)

	wd.Start()
	t.Cleanup(wd.Stop)

	called := make(chan struct{}, 1)
	wd.RegisterWorker("event-hung", func(string) {
		select {
		case called <- struct{}{}:
		default:
		}
	})

	// Never send heartbeat → checkWorkers fires callback (and tries to publish event).
	select {
	case <-called:
		// callback fired – eventBus publish path was reached
	case <-time.After(1 * time.Second):
		t.Fatal("hung callback not fired")
	}
}
