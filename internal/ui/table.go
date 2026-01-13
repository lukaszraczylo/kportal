package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/nvm/kportal/internal/config"
)

// ForwardStatus represents the current status of a port forward
type ForwardStatus struct {
	Context    string
	Namespace  string
	Alias      string
	Type       string
	Resource   string
	Status     string
	RemotePort int
	LocalPort  int
}

// TableUI manages the terminal table display
type TableUI struct {
	forwards map[string]*ForwardStatus
	mu       sync.RWMutex
	verbose  bool
}

// NewTableUI creates a new table UI manager
func NewTableUI(verbose bool) *TableUI {
	return &TableUI{
		forwards: make(map[string]*ForwardStatus),
		verbose:  verbose,
	}
}

// AddForward registers a new forward for display
func (t *TableUI) AddForward(id string, fwd *config.Forward) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Parse resource type and name
	resourceType := "pod"
	resourceName := fwd.Resource

	parts := strings.Split(fwd.Resource, "/")
	if len(parts) == 2 {
		resourceType = parts[0]
		resourceName = parts[1]
	}

	status := &ForwardStatus{
		Context:    fwd.GetContext(),
		Namespace:  fwd.GetNamespace(),
		Alias:      fwd.Alias,
		Type:       resourceType,
		Resource:   resourceName,
		RemotePort: fwd.Port,
		LocalPort:  fwd.LocalPort,
		Status:     "Starting",
	}

	// If no alias, use resource name as display name
	if status.Alias == "" {
		status.Alias = resourceName
	}

	t.forwards[id] = status
}

// UpdateStatus updates the status of a forward
func (t *TableUI) UpdateStatus(id string, status string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if fwd, ok := t.forwards[id]; ok {
		fwd.Status = status
	}
}

// Render displays the current table
func (t *TableUI) Render() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Clear screen and move cursor to top
	if !t.verbose {
		fmt.Print("\033[2J\033[H")
	}

	// Print header
	fmt.Println("kportal - Port Forwarding Status")
	fmt.Println(strings.Repeat("=", 130))

	// Table header
	fmt.Printf("%-15s %-18s %-25s %-10s %-25s %-12s %-12s %-12s\n",
		"CONTEXT", "NAMESPACE", "ALIAS", "TYPE", "RESOURCE", "REMOTE PORT", "LOCAL PORT", "STATUS")
	fmt.Println(strings.Repeat("-", 130))

	// Sort forwards by local port for consistent display
	type sortEntry struct {
		fwd *ForwardStatus
		id  string
	}
	var entries []sortEntry
	for id, fwd := range t.forwards {
		entries = append(entries, sortEntry{fwd: fwd, id: id})
	}

	// Simple sort by local port
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].fwd.LocalPort > entries[j].fwd.LocalPort {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// Print each forward
	for _, entry := range entries {
		fwd := entry.fwd

		// Truncate long names
		alias := truncate(fwd.Alias, 25)
		resource := truncate(fwd.Resource, 25)

		// Color code status with indicator
		statusStr := formatStatusWithIndicator(fwd.Status)

		// Print the row
		fmt.Printf("  %-15s %-18s %-25s %-10s %-25s %-12d %-12d %s\n",
			fwd.Context,
			fwd.Namespace,
			alias,
			fwd.Type,
			resource,
			fwd.RemotePort,
			fwd.LocalPort,
			statusStr)
	}

	fmt.Println(strings.Repeat("=", 130))
	fmt.Printf("Total forwards: %d | Press Ctrl+C to stop\n", len(t.forwards))

	// In verbose mode, add a newline to separate from logs
	if t.verbose {
		fmt.Println()
	}
}

// RenderInitial renders the table once without clearing screen
func (t *TableUI) RenderInitial() {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Print header
	fmt.Println("\nkportal - Port Forwarding Status")
	fmt.Println(strings.Repeat("=", 130))

	// Table header
	fmt.Printf("%-15s %-18s %-25s %-10s %-25s %-12s %-12s %-12s\n",
		"CONTEXT", "NAMESPACE", "ALIAS", "TYPE", "RESOURCE", "REMOTE PORT", "LOCAL PORT", "STATUS")
	fmt.Println(strings.Repeat("-", 130))

	// Print message if no forwards yet
	if len(t.forwards) == 0 {
		fmt.Println("Initializing port forwards...")
	}

	fmt.Println(strings.Repeat("=", 130))
	fmt.Println()
}

// GetForward returns a forward status by ID
func (t *TableUI) GetForward(id string) *ForwardStatus {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.forwards[id]
}

// Remove removes a forward from the display
func (t *TableUI) Remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.forwards, id)
}

// truncate truncates a string to maxLen, adding "..." if needed
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// formatStatusWithIndicator adds color-coded indicator symbols to status
func formatStatusWithIndicator(status string) string {
	// Check if stdout is a terminal
	if fileInfo, _ := os.Stdout.Stat(); (fileInfo.Mode() & os.ModeCharDevice) == 0 {
		// Not a terminal, return plain text with simple indicator
		switch status {
		case "Active":
			return "✓ " + status
		case "Starting":
			return "⋯ " + status
		case "Reconnecting":
			return "↻ " + status
		case "Error", "Failed":
			return "✗ " + status
		default:
			return status
		}
	}

	// Terminal with color support
	switch status {
	case "Active":
		return "\033[32m●\033[0m " + status // Green circle
	case "Starting":
		return "\033[33m○\033[0m " + status // Yellow circle (hollow)
	case "Reconnecting":
		return "\033[33m◐\033[0m " + status // Yellow half-circle
	case "Error", "Failed":
		return "\033[31m●\033[0m " + status // Red circle
	default:
		return status
	}
}
