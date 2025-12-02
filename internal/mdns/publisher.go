package mdns

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/nvm/kportal/internal/logger"
)

const (
	// shutdownTimeout is the maximum time to wait for mDNS server shutdown
	shutdownTimeout = 2 * time.Second

	// mdnsDomain is the standard mDNS domain (RFC 6762)
	// This is always ".local" for multicast DNS - it's not configurable
	// and is different from your network's DNS search domain
	mdnsDomain = "local"
)

// Publisher manages mDNS hostname registrations for port forwards.
// It allows forwards with aliases to be accessible via <alias>.local hostnames.
type Publisher struct {
	mu       sync.RWMutex
	servers  map[string]*zeroconf.Server // forwardID -> server
	aliases  map[string]string           // forwardID -> alias (for logging)
	enabled  bool
	localIPs []string
}

// NewPublisher creates a new mDNS Publisher.
// If enabled is false, all registration calls will be no-ops.
func NewPublisher(enabled bool) *Publisher {
	p := &Publisher{
		servers:  make(map[string]*zeroconf.Server),
		aliases:  make(map[string]string),
		enabled:  enabled,
		localIPs: getLocalIPs(),
	}

	if enabled {
		logger.Info("mDNS publisher initialized", map[string]interface{}{
			"domain":    mdnsDomain,
			"local_ips": p.localIPs,
		})
	}

	return p
}

// Register publishes an mDNS hostname for a forward.
// The hostname will be <alias>.local and will resolve to 127.0.0.1.
// If the forward has no alias or mDNS is disabled, this is a no-op.
func (p *Publisher) Register(forwardID, alias string, localPort int) error {
	if !p.enabled || alias == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if already registered
	if _, exists := p.servers[forwardID]; exists {
		logger.Debug("mDNS hostname already registered", map[string]interface{}{
			"forward_id": forwardID,
			"alias":      alias,
		})
		return nil
	}

	// Register the mDNS service
	// We use a generic service type and rely on the hostname registration
	server, err := zeroconf.RegisterProxy(
		alias,                 // Instance name (shown in service discovery)
		"_kportal._tcp",       // Service type (custom for kportal)
		"local.",              // Domain
		localPort,             // Port
		alias,                 // Hostname (will be <alias>.local)
		[]string{"127.0.0.1"}, // IPs to resolve to
		[]string{fmt.Sprintf("forward=%s", forwardID)}, // TXT records
		nil, // interfaces (nil = all)
	)
	if err != nil {
		return fmt.Errorf("failed to register mDNS for %s: %w", alias, err)
	}

	p.servers[forwardID] = server
	p.aliases[forwardID] = alias

	// Allow zeroconf's internal goroutines (recv4, recv6) to fully initialize.
	// This prevents a race condition where shutdown() could set connections to nil
	// while recv goroutines are still starting up.
	time.Sleep(startupSettleTime)

	logger.Info("mDNS hostname registered", map[string]interface{}{
		"forward_id": forwardID,
		"hostname":   GetHostname(alias),
		"port":       localPort,
	})

	return nil
}

// Unregister removes the mDNS hostname for a forward.
func (p *Publisher) Unregister(forwardID string) {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	server, exists := p.servers[forwardID]
	if !exists {
		return
	}

	alias := p.aliases[forwardID]
	shutdownWithTimeout(server, forwardID)
	delete(p.servers, forwardID)
	delete(p.aliases, forwardID)

	logger.Info("mDNS hostname unregistered", map[string]interface{}{
		"forward_id": forwardID,
		"hostname":   GetHostname(alias),
	})
}

// Stop shuts down all mDNS registrations.
func (p *Publisher) Stop() {
	if !p.enabled {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Shutdown all servers concurrently with timeout
	var wg sync.WaitGroup
	for forwardID, server := range p.servers {
		wg.Add(1)
		go func(id string, srv *zeroconf.Server) {
			defer wg.Done()
			shutdownWithTimeout(srv, id)
		}(forwardID, server)
	}

	// Wait for all shutdowns to complete (or timeout)
	wg.Wait()

	p.servers = make(map[string]*zeroconf.Server)
	p.aliases = make(map[string]string)

	logger.Info("mDNS publisher stopped", nil)
}

// startupSettleTime is a small delay after zeroconf registration to allow internal
// goroutines (recv4, recv6) to fully initialize before any shutdown can occur.
// This works around a race condition in grandcat/zeroconf where shutdown() sets
// connections to nil while recv goroutines may still be initializing.
// See: https://github.com/grandcat/zeroconf/issues/95
const startupSettleTime = 50 * time.Millisecond

// shutdownSettleTime is a small delay after zeroconf shutdown to allow internal
// goroutines to exit cleanly. This works around a race condition in the
// grandcat/zeroconf library where recv4() can access ipv4conn after shutdown()
// sets it to nil. See: https://github.com/grandcat/zeroconf/issues/95
// Note: 100ms is needed for CI environments where timing can be more variable.
const shutdownSettleTime = 100 * time.Millisecond

// shutdownWithTimeout attempts to shutdown a zeroconf server with a timeout.
// If shutdown hangs, it logs a warning and returns anyway.
func shutdownWithTimeout(server *zeroconf.Server, forwardID string) {
	done := make(chan struct{})

	go func() {
		server.Shutdown()
		close(done)
	}()

	select {
	case <-done:
		// Shutdown completed successfully
		// Add a small settle time to let internal goroutines exit cleanly.
		// This works around a race condition in zeroconf where recv4() can
		// access ipv4conn after shutdown() sets it to nil.
		time.Sleep(shutdownSettleTime)
	case <-time.After(shutdownTimeout):
		logger.Warn("mDNS shutdown timed out, continuing anyway", map[string]interface{}{
			"forward_id": forwardID,
			"timeout":    shutdownTimeout.String(),
		})
	}
}

// IsEnabled returns whether mDNS publishing is enabled.
func (p *Publisher) IsEnabled() bool {
	return p.enabled
}

// GetDomain returns the mDNS domain being used (always "local" per RFC 6762).
func (p *Publisher) GetDomain() string {
	return mdnsDomain
}

// GetHostname returns the full mDNS hostname for an alias.
// Example: GetHostname("myapp") returns "myapp.local"
func GetHostname(alias string) string {
	return alias + "." + mdnsDomain
}

// GetRegisteredCount returns the number of currently registered hostnames.
func (p *Publisher) GetRegisteredCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.servers)
}

// getLocalIPs returns the local IP addresses for logging purposes.
func getLocalIPs() []string {
	var ips []string

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return []string{"127.0.0.1"}
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}

	if len(ips) == 0 {
		return []string{"127.0.0.1"}
	}

	return ips
}
