package forward

import (
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
)

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

	// Get the first PID if multiple are returned
	pids := strings.Split(pidStr, "\n")
	pid := pids[0]

	// Get process name using ps
	cmd = exec.Command("ps", "-p", pid, "-o", "comm=")
	output, err = cmd.Output()
	if err != nil {
		return fmt.Sprintf("PID %s", pid)
	}

	procName := strings.TrimSpace(string(output))
	if procName == "" {
		return fmt.Sprintf("PID %s", pid)
	}

	return fmt.Sprintf("%s (PID %s)", procName, pid)
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

	for _, line := range lines {
		if !strings.Contains(line, portStr) {
			continue
		}

		// Parse the line to extract PID
		// Format: TCP    0.0.0.0:8080    0.0.0.0:0    LISTENING    1234
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}

		// Check if this is a LISTENING state
		if !strings.Contains(strings.ToUpper(line), "LISTENING") {
			continue
		}

		pid := fields[len(fields)-1]

		// Get process name using tasklist
		cmd = exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %s", pid), "/FO", "CSV", "/NH")
		output, err = cmd.Output()
		if err != nil {
			return fmt.Sprintf("PID %s", pid)
		}

		// Parse CSV output: "process.exe","1234","Console","1","12,345 K"
		csvLine := strings.TrimSpace(string(output))
		if csvLine == "" {
			return fmt.Sprintf("PID %s", pid)
		}

		parts := strings.Split(csvLine, ",")
		if len(parts) > 0 {
			procName := strings.Trim(parts[0], "\"")
			return fmt.Sprintf("%s (PID %s)", procName, pid)
		}

		return fmt.Sprintf("PID %s", pid)
	}

	return "unknown"
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

// GetPortsFromForwards extracts all local ports from a list of forward configurations.
func GetPortsFromForwards(forwards []interface{}) []int {
	ports := make([]int, 0, len(forwards))
	for _, fwd := range forwards {
		// This function expects a generic interface to work with different forward types
		// The actual implementation should use the Forward struct from config package
		if f, ok := fwd.(interface{ GetLocalPort() int }); ok {
			ports = append(ports, f.GetLocalPort())
		}
	}
	return ports
}
