package ui

import (
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/lukaszraczylo/kportal/internal/config"
)

// ForwardStatus represents the current status of a port forward
type ForwardStatus struct {
	HTTPLog    *config.HTTPLogSpec
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
	columns  []ResolvedColumn
	mu       sync.RWMutex
	verbose  bool
}

// NewTableUI creates a new table UI manager. cfg may be nil, in which case
// the built-in default column set and order is used.
func NewTableUI(verbose bool, cfg *config.Config) *TableUI {
	return &TableUI{
		forwards: make(map[string]*ForwardStatus),
		verbose:  verbose,
		columns:  ResolvePlainColumns(cfg),
	}
}

// SetColumns updates the table's column set and order, e.g. after a config
// hot-reload.
func (t *TableUI) SetColumns(cfg *config.Config) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.columns = ResolvePlainColumns(cfg)
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
	fmt.Println(plainRule("=", t.columns))

	// Table header
	fmt.Println(renderPlainHeaderRow(t.columns))
	fmt.Println(plainRule("-", t.columns))

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
		fmt.Println(renderPlainDataRow(t.columns, entry.fwd))
	}

	fmt.Println(plainRule("=", t.columns))
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
	fmt.Println(plainRule("=", t.columns))

	// Table header
	fmt.Println(renderPlainHeaderRow(t.columns))
	fmt.Println(plainRule("-", t.columns))

	// Print message if no forwards yet
	if len(t.forwards) == 0 {
		fmt.Println("Initializing port forwards...")
	}

	fmt.Println(plainRule("=", t.columns))
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

// legacyPlainRuleWidth is the rule width the default plain layout always used.
const legacyPlainRuleWidth = 130

// ansiSeq matches the SGR colour sequences emitted by formatStatusWithIndicator.
var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

// visibleWidth returns the number of runes a terminal shows for s.
func visibleWidth(s string) int {
	return utf8.RuneCountInString(ansiSeq.ReplaceAllString(s, ""))
}

// padVisible right-pads s with spaces to width visible runes.
func padVisible(s string, width int) string {
	if n := width - visibleWidth(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// plainRule returns a rule line as wide as the rendered columns: the sum of
// their widths plus one separator between each pair.
func plainRule(ch string, cols []ResolvedColumn) string {
	if slices.Equal(cols, plainDefaultColumns) {
		return strings.Repeat(ch, legacyPlainRuleWidth)
	}
	width := len(cols) - 1
	for _, col := range cols {
		width += col.Width
	}
	return strings.Repeat(ch, max(width, 0))
}

// renderPlainHeaderRow renders the header row for the plain table, padding
// every column to its width.
func renderPlainHeaderRow(cols []ResolvedColumn) string {
	cells := make([]string, len(cols))
	for i, col := range cols {
		cells[i] = padVisible(col.Header, col.Width)
	}
	return strings.Join(cells, " ")
}

// renderPlainDataRow renders a single forward's data row for the plain table,
// padding every column but the last so row cells line up with the header.
func renderPlainDataRow(cols []ResolvedColumn, fwd *ForwardStatus) string {
	cells := make([]string, len(cols))
	for i, col := range cols {
		if col.Key == ColKeyStatus {
			cells[i] = formatStatusWithIndicator(fwd.Status)
		} else {
			cells[i] = columnValue(col, fwd)
		}
		if i < len(cols)-1 {
			cells[i] = padVisible(cells[i], col.Width)
		}
	}
	return "  " + strings.Join(cells, " ")
}

// hyperlink wraps text in an OSC 8 terminal hyperlink escape sequence.
// Clicking the text opens the URL in terminals that support it (Ghostty, iTerm2,
// Windows Terminal, Kitty, WezTerm, etc.). Unsupported terminals show plain text.
func hyperlink(url, text string) string {
	return fmt.Sprintf("\x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", url, text)
}

// truncate truncates a string to maxLen runes, adding "..." if needed.
// It counts and slices by rune (not byte) so multibyte aliases/resource names
// are never cut mid-rune into mojibake.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	r := []rune(s)
	if maxLen <= 3 {
		return string(r[:maxLen])
	}
	return string(r[:maxLen-3]) + "..."
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
