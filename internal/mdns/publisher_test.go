package mdns

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Note: Tests that actually register mDNS services require network I/O
// and can be slow or hang in CI environments. We test the logic paths
// without actually calling zeroconf for most tests.

func TestNewPublisher_Disabled(t *testing.T) {
	p := NewPublisher(false)

	assert.False(t, p.IsEnabled())
	assert.Equal(t, 0, p.GetRegisteredCount())
}

func TestNewPublisher_Enabled(t *testing.T) {
	p := NewPublisher(true)

	assert.True(t, p.IsEnabled())
	assert.Equal(t, 0, p.GetRegisteredCount())
}

func TestRegister_WhenDisabled_NoOp(t *testing.T) {
	p := NewPublisher(false)

	err := p.Register("forward-1", "test-alias", 8080)

	assert.NoError(t, err)
	assert.Equal(t, 0, p.GetRegisteredCount())
}

func TestRegister_EmptyAlias_NoOp(t *testing.T) {
	p := NewPublisher(true)

	err := p.Register("forward-1", "", 8080)

	assert.NoError(t, err)
	assert.Equal(t, 0, p.GetRegisteredCount())
}

func TestUnregister_WhenDisabled_NoOp(t *testing.T) {
	p := NewPublisher(false)

	// Should not panic
	p.Unregister("forward-1")
}

func TestUnregister_NotRegistered_NoOp(t *testing.T) {
	p := NewPublisher(true)

	// Should not panic
	p.Unregister("non-existent")
	assert.Equal(t, 0, p.GetRegisteredCount())
}

func TestStop_WhenDisabled_NoOp(t *testing.T) {
	p := NewPublisher(false)

	// Should not panic
	p.Stop()
}

func TestStop_WhenNoRegistrations(t *testing.T) {
	p := NewPublisher(true)

	// Should not panic
	p.Stop()
	assert.Equal(t, 0, p.GetRegisteredCount())
}

func TestGetLocalIPs(t *testing.T) {
	ips := getLocalIPs()

	// Should return at least one IP
	assert.NotEmpty(t, ips, "getLocalIPs should return at least one IP")

	// All IPs should be non-empty strings
	for _, ip := range ips {
		assert.NotEmpty(t, ip, "IP address should not be empty")
	}
}

// Integration tests - only run when explicitly requested
// These tests actually register mDNS services and require network access

func TestRegister_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mDNS integration test in short mode")
	}

	p := NewPublisher(true)
	defer p.Stop()

	err := p.Register("forward-1", "test-service", 8080)

	assert.NoError(t, err)
	assert.Equal(t, 1, p.GetRegisteredCount())
}

func TestRegister_Duplicate_Idempotent_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mDNS integration test in short mode")
	}

	p := NewPublisher(true)
	defer p.Stop()

	// First registration
	err := p.Register("forward-1", "test-service", 8080)
	assert.NoError(t, err)
	assert.Equal(t, 1, p.GetRegisteredCount())

	// Second registration with same ID should be idempotent
	err = p.Register("forward-1", "test-service", 8080)
	assert.NoError(t, err)
	assert.Equal(t, 1, p.GetRegisteredCount())
}

func TestRegister_MultipleForwards_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mDNS integration test in short mode")
	}

	p := NewPublisher(true)
	defer p.Stop()

	err1 := p.Register("forward-1", "service-a", 8080)
	err2 := p.Register("forward-2", "service-b", 8081)
	err3 := p.Register("forward-3", "service-c", 8082)

	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NoError(t, err3)
	assert.Equal(t, 3, p.GetRegisteredCount())
}

func TestUnregister_Success_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping mDNS integration test in short mode")
	}

	p := NewPublisher(true)
	defer p.Stop()

	p.Register("forward-1", "test-service", 8080)
	assert.Equal(t, 1, p.GetRegisteredCount())

	p.Unregister("forward-1")
	assert.Equal(t, 0, p.GetRegisteredCount())
}
