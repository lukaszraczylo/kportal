package ui

import (
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nvm/kportal/internal/config"
)

// ForwardUpdateMsg is sent when a forward status changes
type ForwardUpdateMsg struct {
	ID     string
	Status string
}

// ForwardErrorMsg is sent when a forward has an error
type ForwardErrorMsg struct {
	ID    string
	Error string
}

// ForwardAddMsg is sent when a new forward is added
type ForwardAddMsg struct {
	ID      string
	Forward *ForwardStatus
}

// ForwardRemoveMsg is sent when a forward is removed
type ForwardRemoveMsg struct {
	ID string
}

// BubbleTeaUI is a bubbletea-based terminal UI
type BubbleTeaUI struct {
	mu             sync.RWMutex
	program        *tea.Program
	forwards       map[string]*ForwardStatus
	forwardOrder   []string
	selectedIndex  int
	disabledMap    map[string]bool
	toggleCallback func(id string, enable bool)
	version        string
	errors         map[string]string // Track error messages by forward ID
}

// bubbletea model
type model struct {
	ui *BubbleTeaUI
}

// NewBubbleTeaUI creates a new bubbletea-based UI
func NewBubbleTeaUI(toggleCallback func(id string, enable bool), version string) *BubbleTeaUI {
	ui := &BubbleTeaUI{
		forwards:       make(map[string]*ForwardStatus),
		forwardOrder:   make([]string, 0),
		selectedIndex:  0,
		disabledMap:    make(map[string]bool),
		toggleCallback: toggleCallback,
		version:        version,
		errors:         make(map[string]string),
	}

	return ui
}

// Start starts the bubbletea application
func (ui *BubbleTeaUI) Start() error {
	m := model{ui: ui}
	ui.program = tea.NewProgram(m, tea.WithAltScreen())
	_, err := ui.program.Run()
	return err
}

// Stop stops the application
func (ui *BubbleTeaUI) Stop() {
	if ui.program != nil {
		ui.program.Quit()
	}
}

// AddForward adds a forward to display
func (ui *BubbleTeaUI) AddForward(id string, fwd *config.Forward) {
	ui.mu.Lock()

	// Check if already exists (re-enabling case)
	if existing, ok := ui.forwards[id]; ok {
		existing.Status = "Starting"
		ui.disabledMap[id] = false
		ui.mu.Unlock()

		if ui.program != nil {
			ui.program.Send(ForwardUpdateMsg{ID: id, Status: "Starting"})
		}
		return
	}

	// Parse resource
	resourceType := "pod"
	resourceName := fwd.Resource
	for idx := 0; idx < len(fwd.Resource); idx++ {
		if fwd.Resource[idx] == '/' {
			resourceType = fwd.Resource[:idx]
			resourceName = fwd.Resource[idx+1:]
			break
		}
	}

	alias := fwd.Alias
	if alias == "" {
		alias = resourceName
	}

	status := &ForwardStatus{
		Context:    fwd.GetContext(),
		Namespace:  fwd.GetNamespace(),
		Alias:      alias,
		Type:       resourceType,
		Resource:   resourceName,
		RemotePort: fwd.Port,
		LocalPort:  fwd.LocalPort,
		Status:     "Starting",
	}

	ui.forwards[id] = status
	ui.forwardOrder = append(ui.forwardOrder, id)
	ui.mu.Unlock()

	if ui.program != nil {
		ui.program.Send(ForwardAddMsg{ID: id, Forward: status})
	}
}

// UpdateStatus updates forward status
func (ui *BubbleTeaUI) UpdateStatus(id string, status string) {
	ui.mu.Lock()
	if fwd, ok := ui.forwards[id]; ok {
		fwd.Status = status
	}
	// Clear error if status is not Error
	if status != "Error" {
		delete(ui.errors, id)
	}
	ui.mu.Unlock()

	if ui.program != nil {
		ui.program.Send(ForwardUpdateMsg{ID: id, Status: status})
	}
}

// SetError sets an error message for a forward
func (ui *BubbleTeaUI) SetError(id, msg string) {
	ui.mu.Lock()
	ui.errors[id] = msg
	ui.mu.Unlock()

	if ui.program != nil {
		ui.program.Send(ForwardErrorMsg{ID: id, Error: msg})
	}
}

// Remove removes a forward
func (ui *BubbleTeaUI) Remove(id string) {
	ui.mu.Lock()
	delete(ui.forwards, id)

	// Remove from order
	for i, fid := range ui.forwardOrder {
		if fid == id {
			ui.forwardOrder = append(ui.forwardOrder[:i], ui.forwardOrder[i+1:]...)
			break
		}
	}
	ui.mu.Unlock()

	if ui.program != nil {
		ui.program.Send(ForwardRemoveMsg{ID: id})
	}
}

// Bubble Tea Model Implementation

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			m.ui.moveSelection(-1)
		case "down", "j":
			m.ui.moveSelection(1)
		case " ", "enter":
			m.ui.toggleSelected()
		}

	case ForwardAddMsg:
		// Already handled in AddForward, just trigger re-render
		return m, nil

	case ForwardUpdateMsg:
		// Already handled in UpdateStatus, just trigger re-render
		return m, nil

	case ForwardErrorMsg:
		// Already handled in SetError, just trigger re-render
		return m, nil

	case ForwardRemoveMsg:
		// Already handled in Remove, just trigger re-render
		return m, nil
	}

	return m, nil
}

func (m model) View() string {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()

	var b strings.Builder

	// Styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220")).
		Padding(0, 1)

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("220"))

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	selectedStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("240")).
		Foreground(lipgloss.Color("230"))

	disabledStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("46"))

	startingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("220"))

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196"))

	// Title with version
	title := fmt.Sprintf("kportal v%s - Port Forwarding Status", m.ui.version)
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n\n")

	// Header
	header := fmt.Sprintf("%-15s %-18s %-20s %-10s %-21s %7s %7s  %s",
		"CONTEXT", "NAMESPACE", "ALIAS", "TYPE", "RESOURCE", "REMOTE", "LOCAL", "STATUS")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(separatorStyle.Render(strings.Repeat("─", 120)))
	b.WriteString("\n")

	// No forwards
	if len(m.ui.forwardOrder) == 0 {
		b.WriteString(disabledStyle.Render("\nNo forwards configured\n"))
	} else {
		// Display forwards
		for idx, id := range m.ui.forwardOrder {
			fwd, ok := m.ui.forwards[id]
			if !ok {
				continue
			}

			isSelected := (idx == m.ui.selectedIndex)
			isDisabled := m.ui.disabledMap[id]

			// Selection indicator
			indicator := "  "
			if isSelected {
				indicator = "> "
			}

			// Status icon and text
			statusIcon := "● "
			statusText := fwd.Status

			if isDisabled {
				statusIcon = "○ "
				statusText = "Disabled"
			} else {
				switch fwd.Status {
				case "Starting":
					statusIcon = "○ "
				case "Reconnecting":
					statusIcon = "◐ "
				case "Error":
					statusIcon = "✗ "
				}
			}

			// Format row
			row := fmt.Sprintf("%s%-15s %-18s %-20s %-10s %-21s %7d %7d  %s%s",
				indicator,
				truncate(fwd.Context, 15),
				truncate(fwd.Namespace, 18),
				truncate(fwd.Alias, 20),
				truncate(fwd.Type, 10),
				truncate(fwd.Resource, 21),
				fwd.RemotePort,
				fwd.LocalPort,
				statusIcon,
				statusText)

			// Apply styling
			if isSelected {
				row = selectedStyle.Render(row)
			} else if isDisabled {
				row = disabledStyle.Render(row)
			} else {
				// Color the status part
				switch fwd.Status {
				case "Active":
					parts := strings.Split(row, statusIcon)
					if len(parts) == 2 {
						row = parts[0] + activeStyle.Render(statusIcon+statusText)
					}
				case "Starting", "Reconnecting":
					parts := strings.Split(row, statusIcon)
					if len(parts) == 2 {
						row = parts[0] + startingStyle.Render(statusIcon+statusText)
					}
				case "Error":
					parts := strings.Split(row, statusIcon)
					if len(parts) == 2 {
						row = parts[0] + errorStyle.Render(statusIcon+statusText)
					}
				}
			}

			b.WriteString(row)
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))

	footer := fmt.Sprintf("%s/%s: Navigate  %s: Toggle  %s: Quit  │  Total: %d",
		keyStyle.Render("↑↓"),
		keyStyle.Render("jk"),
		keyStyle.Render("Space"),
		keyStyle.Render("q"),
		len(m.ui.forwardOrder))

	b.WriteString(footerStyle.Render(footer))

	// Display errors if any
	if len(m.ui.errors) > 0 {
		b.WriteString("\n\n")
		errorHeaderStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))

		b.WriteString(errorHeaderStyle.Render("Errors:"))
		b.WriteString("\n")

		for id, errMsg := range m.ui.errors {
			// Find the forward to display its alias
			if fwd, ok := m.ui.forwards[id]; ok {
				errorLineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
				line := fmt.Sprintf("  • %s: %s", fwd.Alias, errMsg)
				b.WriteString(errorLineStyle.Render(line))
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// moveSelection moves the selection up or down
func (ui *BubbleTeaUI) moveSelection(delta int) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if len(ui.forwardOrder) == 0 {
		return
	}

	ui.selectedIndex += delta
	if ui.selectedIndex < 0 {
		ui.selectedIndex = 0
	}
	if ui.selectedIndex >= len(ui.forwardOrder) {
		ui.selectedIndex = len(ui.forwardOrder) - 1
	}
}

// toggleSelected toggles the selected forward on/off
func (ui *BubbleTeaUI) toggleSelected() {
	ui.mu.Lock()

	if ui.selectedIndex < 0 || ui.selectedIndex >= len(ui.forwardOrder) {
		ui.mu.Unlock()
		return
	}

	selectedID := ui.forwardOrder[ui.selectedIndex]
	currentlyDisabled := ui.disabledMap[selectedID]
	newState := !currentlyDisabled
	ui.disabledMap[selectedID] = newState

	ui.mu.Unlock()

	// Call the toggle callback in a goroutine to avoid blocking the UI
	if ui.toggleCallback != nil {
		go ui.toggleCallback(selectedID, !newState) // enable is inverse of disabled
	}
}
