package forward

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nvm/kportal/internal/logger"
)

const (
	// maxPIDLength is the maximum length of a valid PID string (9 digits covers PIDs up to 999,999,999)
	maxPIDLength = 9
	// minNetstatFields is the minimum number of fields expected in netstat output
	minNetstatFields = 5
)

// isValidPID validates that a PID string contains only digits
func isValidPID(pid string) bool {
	if len(pid) == 0 || len(pid) > maxPIDLength {
		return false
	}
	for _, c := range pid {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// processInfo holds information about a process using a port
type processInfo struct {
	pid     string
	name    string
	isValid bool
}

// formatProcessInfo formats process information for display
func formatProcessInfo(info processInfo) string {
	if !info.isValid {
		return "unknown"
	}
	if info.name != "" {
		return fmt.Sprintf("%s (PID %s)", info.name, info.pid)
	}
	return fmt.Sprintf("PID %s", info.pid)
}

// getProcessNameByPID retrieves the process name for a given PID on Unix systems
func getProcessNameByPID(pid string) string {
	cmd := exec.Command("ps", "-p", pid, "-o", "comm=")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// getProcessNameByPIDWindows retrieves the process name for a given PID on Windows
func getProcessNameByPIDWindows(pid string) string {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %s", pid), "/FO", "CSV", "/NH")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse CSV output: "process.exe","1234","Console","1","12,345 K"
	csvLine := strings.TrimSpace(string(output))
	if csvLine == "" {
		return ""
	}

	parts := strings.Split(csvLine, ",")
	if len(parts) > 0 {
		return strings.Trim(parts[0], "\"")
	}
	return ""
}

// PortConflict represents a local port that is already in use.
type PortConflict struct {
	Port     int    // The conflicting port number
	Resource string // The forward resource that needs this port
	UsedBy   string // Process information (PID, command) using the port
}

// PortChecker checks port availability on the local system.
type PortChecker struct{}

// NewPortChecker creates a new PortChecker instance.
func NewPortChecker() *PortChecker {
	return &PortChecker{}
}

// CheckAvailability checks if the given ports are available for binding.
// It returns a list of conflicts for ports that are already in use.
// The skipPorts map contains ports currently managed by kportal that should be excluded from the check.
func (pc *PortChecker) CheckAvailability(ports []int, skipPorts map[int]bool) []PortConflict {
	var conflicts []PortConflict

	for _, port := range ports {
		// Skip ports that are already managed by kportal
		if skipPorts[port] {
			continue
		}

		// Try to bind to the port
		if !pc.isPortAvailable(port) {
			// Port is in use, get process info
			usedBy := pc.getProcessUsingPort(port)
			conflicts = append(conflicts, PortConflict{
				Port:   port,
				UsedBy: usedBy,
			})
		}
	}

	return conflicts
}

// isPortAvailable checks if a port is available by attempting to bind to it.
func (pc *PortChecker) isPortAvailable(port int) bool {
	// Try to listen on the port
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// getProcessUsingPort returns information about the process using the given port.
// Returns a string like "nginx (PID 1234)" or "unknown" if the process cannot be determined.
func (pc *PortChecker) getProcessUsingPort(port int) string {
	switch runtime.GOOS {
	case "darwin", "linux":
		return pc.getProcessUsingPortUnix(port)
	case "windows":
		return pc.getProcessUsingPortWindows(port)
	default:
		return "unknown"
	}
}

// getProcessUsingPortUnix uses lsof to find the process using a port on Unix systems.
func (pc *PortChecker) getProcessUsingPortUnix(port int) string {
	// Use lsof to find the process
	// lsof -i :PORT -sTCP:LISTEN -t returns PIDs
	cmd := exec.Command("lsof", "-i", fmt.Sprintf(":%d", port), "-sTCP:LISTEN", "-t")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	pidStr := strings.TrimSpace(string(output))
	if pidStr == "" {
		return "unknown"
	}

	// Handle multiple PIDs (multiple processes on same port)
	pids := strings.Split(pidStr, "\n")
	var validProcesses []processInfo

	for _, pid := range pids {
		pid = strings.TrimSpace(pid)
		if pid == "" {
			continue
		}

		if !isValidPID(pid) {
			logger.Debug("Invalid PID format from lsof output", map[string]interface{}{
				"port":    port,
				"raw_pid": pid,
			})
			continue
		}

		procName := getProcessNameByPID(pid)
		validProcesses = append(validProcesses, processInfo{
			pid:     pid,
			name:    procName,
			isValid: true,
		})
	}

	if len(validProcesses) == 0 {
		return "unknown"
	}

	// Format output - show all processes if multiple
	if len(validProcesses) == 1 {
		return formatProcessInfo(validProcesses[0])
	}

	// Multiple processes - format as list
	var parts []string
	for _, p := range validProcesses {
		parts = append(parts, formatProcessInfo(p))
	}
	return strings.Join(parts, ", ")
}

// isListeningState checks if a netstat line indicates a listening state.
// This handles both English and potentially other locales by checking for common patterns.
func isListeningState(line string, fields []string) bool {
	upperLine := strings.ToUpper(line)

	// Check for common listening state indicators across locales
	// English: LISTENING, German: ABHÖREN, French: ÉCOUTE, etc.
	// The most reliable check is the state field position (4th field, 0-indexed = 3)
	// and that it's a TCP connection with 0.0.0.0:0 or *:* as foreign address
	if len(fields) >= minNetstatFields {
		state := strings.ToUpper(fields[3])
		// Common listening state values across Windows locales
		if state == "LISTENING" || state == "ABHÖREN" || state == "ÉCOUTE" ||
			state == "ESCUCHANDO" || state == "ASCOLTO" || state == "NASŁUCHIWANIE" {
			return true
		}
	}

	// Fallback: check if line contains LISTENING (most common case)
	return strings.Contains(upperLine, "LISTENING")
}

// getProcessUsingPortWindows uses netstat to find the process using a port on Windows.
func (pc *PortChecker) getProcessUsingPortWindows(port int) string {
	// Use netstat to find the process
	// netstat -ano | findstr :PORT
	cmd := exec.Command("netstat", "-ano")
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}

	lines := strings.Split(string(output), "\n")
	portStr := fmt.Sprintf(":%d", port)

	var validProcesses []processInfo

	for _, line := range lines {
		if !strings.Contains(line, portStr) {
			continue
		}

		// Parse the line to extract PID
		// Format: TCP    0.0.0.0:8080    0.0.0.0:0    LISTENING    1234
		fields := strings.Fields(line)
		if len(fields) < minNetstatFields {
			continue
		}

		// Check if this is a LISTENING state (locale-aware)
		if !isListeningState(line, fields) {
			continue
		}

		// Verify the local address field actually contains our port
		// (avoid matching port in foreign address)
		localAddr := fields[1]
		if !strings.HasSuffix(localAddr, portStr) {
			continue
		}

		pid := fields[len(fields)-1]

		if !isValidPID(pid) {
			logger.Debug("Invalid PID format from netstat output", map[string]interface{}{
				"port":    port,
				"raw_pid": pid,
				"line":    line,
			})
			continue
		}

		procName := getProcessNameByPIDWindows(pid)
		validProcesses = append(validProcesses, processInfo{
			pid:     pid,
			name:    procName,
			isValid: true,
		})
	}

	if len(validProcesses) == 0 {
		return "unknown"
	}

	// Format output - show all processes if multiple
	if len(validProcesses) == 1 {
		return formatProcessInfo(validProcesses[0])
	}

	// Multiple processes - format as list
	var parts []string
	for _, p := range validProcesses {
		parts = append(parts, formatProcessInfo(p))
	}
	return strings.Join(parts, ", ")
}

// FormatConflicts formats port conflicts into a human-readable error message.
func FormatConflicts(conflicts []PortConflict) string {
	if len(conflicts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\nPort Conflicts Detected:\n")
	sb.WriteString(strings.Repeat("=", 50) + "\n\n")

	for _, conflict := range conflicts {
		sb.WriteString(fmt.Sprintf("Port %d\n", conflict.Port))
		if conflict.Resource != "" {
			sb.WriteString(fmt.Sprintf("  Needed for: %s\n", conflict.Resource))
		}
		sb.WriteString(fmt.Sprintf("  Currently used by: %s\n", conflict.UsedBy))
		sb.WriteString("\n")
	}

	sb.WriteString("Action: Stop conflicting processes or change localPort in config.\n")

	return sb.String()
}
