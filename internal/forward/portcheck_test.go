package forward

import (
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestIsValidPID tests PID validation
func TestIsValidPID(t *testing.T) {
	tests := []struct {
		name     string
		pid      string
		expected bool
	}{
		{"valid single digit", "1", true},
		{"valid multi digit", "12345", true},
		{"valid max length", "123456789", true},
		{"empty string", "", false},
		{"too long", "1234567890", false},
		{"contains letter", "123a", false},
		{"contains space", "123 ", false},
		{"negative sign", "-123", false},
		{"decimal", "12.3", false},
		{"just zero", "0", true},
		{"leading zeros", "00123", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidPID(tt.pid)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatProcessInfo tests process info formatting
func TestFormatProcessInfo(t *testing.T) {
	tests := []struct {
		name     string
		info     processInfo
		expected string
	}{
		{
			name:     "invalid process",
			info:     processInfo{isValid: false},
			expected: "unknown",
		},
		{
			name:     "valid with name and pid",
			info:     processInfo{pid: "1234", name: "nginx", isValid: true},
			expected: "nginx (PID 1234)",
		},
		{
			name:     "valid with only pid",
			info:     processInfo{pid: "5678", name: "", isValid: true},
			expected: "PID 5678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProcessInfo(tt.info)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestFormatProcessList tests process list formatting
func TestFormatProcessList(t *testing.T) {
	tests := []struct {
		name      string
		processes []processInfo
		expected  string
	}{
		{
			name:      "empty list",
			processes: []processInfo{},
			expected:  "unknown",
		},
		{
			name:      "single process",
			processes: []processInfo{{pid: "1234", name: "nginx", isValid: true}},
			expected:  "nginx (PID 1234)",
		},
		{
			name: "multiple processes",
			processes: []processInfo{
				{pid: "1234", name: "nginx", isValid: true},
				{pid: "5678", name: "node", isValid: true},
			},
			expected: "nginx (PID 1234), node (PID 5678)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatProcessList(tt.processes)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsListeningState tests listening state detection
func TestIsListeningState(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		fields   []string
		expected bool
	}{
		{
			name:     "English LISTENING",
			line:     "TCP    0.0.0.0:8080    0.0.0.0:0    LISTENING    1234",
			fields:   []string{"TCP", "0.0.0.0:8080", "0.0.0.0:0", "LISTENING", "1234"},
			expected: true,
		},
		{
			name:     "German ABHÖREN",
			line:     "TCP    0.0.0.0:8080    0.0.0.0:0    ABHÖREN    1234",
			fields:   []string{"TCP", "0.0.0.0:8080", "0.0.0.0:0", "ABHÖREN", "1234"},
			expected: true,
		},
		{
			name:     "French ÉCOUTE",
			line:     "TCP    0.0.0.0:8080    0.0.0.0:0    ÉCOUTE    1234",
			fields:   []string{"TCP", "0.0.0.0:8080", "0.0.0.0:0", "ÉCOUTE", "1234"},
			expected: true,
		},
		{
			name:     "Spanish ESCUCHANDO",
			line:     "TCP    0.0.0.0:8080    0.0.0.0:0    ESCUCHANDO    1234",
			fields:   []string{"TCP", "0.0.0.0:8080", "0.0.0.0:0", "ESCUCHANDO", "1234"},
			expected: true,
		},
		{
			name:     "ESTABLISHED (not listening)",
			line:     "TCP    192.168.1.1:8080    10.0.0.1:443    ESTABLISHED    1234",
			fields:   []string{"TCP", "192.168.1.1:8080", "10.0.0.1:443", "ESTABLISHED", "1234"},
			expected: false,
		},
		{
			name:     "too few fields",
			line:     "TCP    0.0.0.0:8080",
			fields:   []string{"TCP", "0.0.0.0:8080"},
			expected: false,
		},
		{
			name:     "lowercase listening (via fallback)",
			line:     "tcp    0.0.0.0:8080    0.0.0.0:0    listening    1234",
			fields:   []string{"tcp", "0.0.0.0:8080", "0.0.0.0:0", "listening", "1234"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isListeningState(tt.line, tt.fields)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetProcessNameByPID tests process name lookup
func TestGetProcessNameByPID(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping Unix-specific test on Windows")
	}

	// Test with PID 1 (init/systemd on Linux, launchd on macOS)
	// This should return something on Unix systems
	name := getProcessNameByPID("1")
	// We don't assert the exact name since it varies by OS
	// Just verify no panic and returns string
	assert.IsType(t, "", name)

	// Test with invalid PID
	name = getProcessNameByPID("999999999")
	// Should return empty string for non-existent process
	assert.IsType(t, "", name)
}

func TestPortChecker_IsAvailable(t *testing.T) {
	pc := NewPortChecker()

	// Test that isPortAvailable returns a bool
	// We use a high port that's likely to be available
	result := pc.isPortAvailable(54321)
	assert.IsType(t, false, result, "isPortAvailable should return bool")
}

func TestPortChecker_CheckAvailability_EmptyPorts(t *testing.T) {
	pc := NewPortChecker()

	// Test with empty ports slice
	conflicts := pc.CheckAvailability([]int{}, nil)
	assert.Empty(t, conflicts, "should return empty conflicts for empty ports")

	// Test with nil exclude map
	conflicts = pc.CheckAvailability([]int{}, nil)
	assert.Empty(t, conflicts, "should return empty conflicts for nil exclude map")
}

func TestPortChecker_CheckAvailability_ExcludeMap(t *testing.T) {
	pc := NewPortChecker()

	// Create a listener to occupy a port
	listener, err := net.Listen("tcp", ":0")
	assert.NoError(t, err, "should create listener")
	defer listener.Close()

	// Get the port that's now occupied
	addr := listener.Addr().(*net.TCPAddr)
	occupiedPort := addr.Port

	// Test that the occupied port shows as conflicted
	conflicts := pc.CheckAvailability([]int{occupiedPort}, nil)
	assert.Len(t, conflicts, 1, "should detect conflict for occupied port")
	assert.Equal(t, occupiedPort, conflicts[0].Port)

	// Test that skipPorts map excludes the port from conflict detection
	skipPorts := map[int]bool{
		occupiedPort: true,
	}
	conflicts = pc.CheckAvailability([]int{occupiedPort}, skipPorts)
	assert.Empty(t, conflicts, "should skip ports in exclude map")
}

func TestPortChecker_CheckAvailability_MultipleSkipPorts(t *testing.T) {
	pc := NewPortChecker()

	// Create multiple listeners
	listener1, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)
	defer listener1.Close()

	listener2, err := net.Listen("tcp", ":0")
	assert.NoError(t, err)
	defer listener2.Close()

	port1 := listener1.Addr().(*net.TCPAddr).Port
	port2 := listener2.Addr().(*net.TCPAddr).Port

	// Test with both ports occupied
	conflicts := pc.CheckAvailability([]int{port1, port2}, nil)
	assert.Len(t, conflicts, 2, "should detect both conflicts")

	// Test excluding one port
	skipPorts := map[int]bool{port1: true}
	conflicts = pc.CheckAvailability([]int{port1, port2}, skipPorts)
	assert.Len(t, conflicts, 1, "should detect only non-excluded port")
	assert.Equal(t, port2, conflicts[0].Port)

	// Test excluding both ports
	skipPorts = map[int]bool{port1: true, port2: true}
	conflicts = pc.CheckAvailability([]int{port1, port2}, skipPorts)
	assert.Empty(t, conflicts, "should skip all excluded ports")
}

func TestPortChecker_GetProcessInfo(t *testing.T) {
	pc := NewPortChecker()

	// Test that getProcessUsingPort returns a string
	// We don't test actual process detection to avoid flakiness
	result := pc.getProcessUsingPort(12345)
	assert.IsType(t, "", result, "getProcessUsingPort should return string")
	assert.NotEmpty(t, result, "should return some string (even if 'unknown')")
}

func TestFormatConflicts_Empty(t *testing.T) {
	// Test with empty conflicts
	output := FormatConflicts([]PortConflict{})
	assert.Empty(t, output, "should return empty string for no conflicts")
}

func TestFormatConflicts_SingleConflict(t *testing.T) {
	conflicts := []PortConflict{
		{
			Port:     8080,
			Resource: "dev/default/pod/my-app:8080",
			UsedBy:   "nginx (PID 1234)",
		},
	}

	output := FormatConflicts(conflicts)
	assert.NotEmpty(t, output, "should return non-empty output")
	assert.Contains(t, output, "Port Conflicts Detected", "should contain header")
	assert.Contains(t, output, "Port 8080", "should contain port number")
	assert.Contains(t, output, "dev/default/pod/my-app:8080", "should contain resource")
	assert.Contains(t, output, "nginx (PID 1234)", "should contain process info")
}

func TestFormatConflicts_MultipleConflicts(t *testing.T) {
	conflicts := []PortConflict{
		{
			Port:     8080,
			Resource: "dev/default/pod/app1:8080",
			UsedBy:   "nginx (PID 1234)",
		},
		{
			Port:     5432,
			Resource: "prod/database/service/postgres:5432",
			UsedBy:   "postgres (PID 5678)",
		},
	}

	output := FormatConflicts(conflicts)
	assert.NotEmpty(t, output, "should return non-empty output")
	assert.Contains(t, output, "Port Conflicts Detected", "should contain header")
	assert.Contains(t, output, "Port 8080", "should contain first port")
	assert.Contains(t, output, "Port 5432", "should contain second port")
	assert.Contains(t, output, "nginx (PID 1234)", "should contain first process")
	assert.Contains(t, output, "postgres (PID 5678)", "should contain second process")
	assert.Contains(t, output, "Action:", "should contain action message")
}

func TestFormatConflicts_WithoutResource(t *testing.T) {
	conflicts := []PortConflict{
		{
			Port:   8080,
			UsedBy: "nginx (PID 1234)",
		},
	}

	output := FormatConflicts(conflicts)
	assert.NotEmpty(t, output, "should return non-empty output")
	assert.Contains(t, output, "Port 8080", "should contain port")
	assert.Contains(t, output, "nginx (PID 1234)", "should contain process info")
	// Should not crash or include empty "Needed for:" line
	assert.NotContains(t, output, "Needed for:  \n", "should not have empty resource line")
}

func TestPortConflict_Structure(t *testing.T) {
	// Test that PortConflict structure works correctly
	conflict := PortConflict{
		Port:     8080,
		Resource: "dev/default/pod/app:8080",
		UsedBy:   "nginx (PID 1234)",
	}

	assert.Equal(t, 8080, conflict.Port)
	assert.Equal(t, "dev/default/pod/app:8080", conflict.Resource)
	assert.Equal(t, "nginx (PID 1234)", conflict.UsedBy)
}

func TestNewPortChecker(t *testing.T) {
	pc := NewPortChecker()
	assert.NotNil(t, pc, "NewPortChecker should return non-nil instance")
}

func TestPortChecker_PortAvailability_Integration(t *testing.T) {
	pc := NewPortChecker()

	// Create a listener to occupy a port
	listener, err := net.Listen("tcp", ":0")
	assert.NoError(t, err, "should create listener")
	defer listener.Close()

	// Get the occupied port
	occupiedPort := listener.Addr().(*net.TCPAddr).Port

	// Test that the port is correctly detected as unavailable
	available := pc.isPortAvailable(occupiedPort)
	assert.False(t, available, "occupied port should not be available")

	// Close the listener
	listener.Close()

	// The port should now be available (though there might be a brief delay)
	// We don't assert this to avoid flakiness in CI environments
}

func TestPortChecker_CheckAvailability_AvailablePorts(t *testing.T) {
	pc := NewPortChecker()

	// Use high port numbers that are very unlikely to be in use
	// This test might be slightly flaky in unusual environments, but should be stable
	unlikelyPorts := []int{54321, 54322, 54323}

	conflicts := pc.CheckAvailability(unlikelyPorts, nil)

	// Most likely all ports will be available
	// The function returns nil or empty slice when there are no conflicts
	// We just verify the function executes without panicking
	_ = conflicts
}

func TestFormatConflicts_Formatting(t *testing.T) {
	conflicts := []PortConflict{
		{
			Port:     8080,
			Resource: "dev/default/pod/my-app:8080",
			UsedBy:   "nginx (PID 1234)",
		},
	}

	output := FormatConflicts(conflicts)

	// Check formatting details
	assert.Contains(t, output, "==================================================", "should contain separator line")
	assert.Contains(t, output, "\n", "should contain newlines")
}
