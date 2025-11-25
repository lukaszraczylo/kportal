package ui

import (
	"fmt"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/k8s"
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

// HTTPLogSubscriber is a function that subscribes to HTTP logs for a forward
// It returns a cleanup function to call when unsubscribing
type HTTPLogSubscriber func(forwardID string, callback func(entry HTTPLogEntry)) func()

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

	// Update notification
	updateAvailable bool
	updateVersion   string
	updateURL       string

	// Modal wizard state
	viewMode     ViewMode
	addWizard    *AddWizardState
	removeWizard *RemoveWizardState

	// Delete confirmation state
	deleteConfirming    bool
	deleteConfirmID     string
	deleteConfirmAlias  string
	deleteConfirmCursor int // 0 = Yes, 1 = No

	// Benchmark state
	benchmarkState *BenchmarkState

	// HTTP log viewing state
	httpLogState *HTTPLogState

	// Log callback cleanup function
	httpLogCleanup func()

	// Dependencies for wizards
	discovery  *k8s.Discovery
	mutator    *config.Mutator
	configPath string

	// Manager for accessing workers
	httpLogSubscriber HTTPLogSubscriber
}

// bubbletea model
type model struct {
	ui         *BubbleTeaUI
	termWidth  int
	termHeight int
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
		viewMode:       ViewModeMain,
	}

	return ui
}

// SetWizardDependencies sets the dependencies needed for the add/remove wizards
func (ui *BubbleTeaUI) SetWizardDependencies(discovery *k8s.Discovery, mutator *config.Mutator, configPath string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	ui.discovery = discovery
	ui.mutator = mutator
	ui.configPath = configPath
}

// SetHTTPLogSubscriber sets the function to subscribe to HTTP logs
func (ui *BubbleTeaUI) SetHTTPLogSubscriber(subscriber HTTPLogSubscriber) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	ui.httpLogSubscriber = subscriber
}

// SetUpdateAvailable sets the update notification to be displayed
func (ui *BubbleTeaUI) SetUpdateAvailable(version, url string) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	ui.updateAvailable = true
	ui.updateVersion = version
	ui.updateURL = url
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
	// Only clear error when forward becomes Active again
	// This keeps error visible during Reconnecting/Starting states
	if status == "Active" {
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
	m.ui.mu.RLock()
	viewMode := m.ui.viewMode
	m.ui.mu.RUnlock()

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update terminal dimensions on resize
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Route based on current view mode
		switch viewMode {
		case ViewModeMain:
			return m.handleMainViewKeys(msg)
		case ViewModeAddWizard:
			return m.handleAddWizardKeys(msg)
		case ViewModeRemoveWizard:
			return m.handleRemoveWizardKeys(msg)
		case ViewModeBenchmark:
			return m.handleBenchmarkKeys(msg)
		case ViewModeHTTPLog:
			return m.handleHTTPLogKeys(msg)
		}

	// Forward management messages (always update main view data)
	case ForwardAddMsg, ForwardUpdateMsg, ForwardErrorMsg, ForwardRemoveMsg:
		return m, nil

	// Wizard-specific messages
	case ContextsLoadedMsg:
		return m.handleContextsLoaded(msg)
	case NamespacesLoadedMsg:
		return m.handleNamespacesLoaded(msg)
	case PodsLoadedMsg:
		return m.handlePodsLoaded(msg)
	case ServicesLoadedMsg:
		return m.handleServicesLoaded(msg)
	case SelectorValidatedMsg:
		return m.handleSelectorValidated(msg)
	case PortCheckedMsg:
		return m.handlePortChecked(msg)
	case ForwardSavedMsg:
		return m.handleForwardSaved(msg)
	case ForwardsRemovedMsg:
		return m.handleForwardsRemoved(msg)
	case WizardCompleteMsg:
		m.ui.mu.Lock()
		m.ui.viewMode = ViewModeMain
		m.ui.addWizard = nil
		m.ui.removeWizard = nil
		m.ui.mu.Unlock()
		return m, tea.ClearScreen

	case BenchmarkCompleteMsg:
		return m.handleBenchmarkComplete(msg)

	case BenchmarkProgressMsg:
		return m.handleBenchmarkProgress(msg)

	case HTTPLogEntryMsg:
		return m.handleHTTPLogEntry(msg)
	}

	return m, nil
}

func (m model) View() string {
	m.ui.mu.RLock()
	viewMode := m.ui.viewMode
	deleteConfirming := m.ui.deleteConfirming
	m.ui.mu.RUnlock()

	// Always render main view as base
	mainView := m.renderMainView()

	// Use actual terminal dimensions for proper centering
	termWidth := m.termWidth
	termHeight := m.termHeight

	// Fallback to reasonable defaults if dimensions not yet received
	if termWidth == 0 {
		termWidth = 120
	}
	if termHeight == 0 {
		termHeight = 40
	}

	// Overlay delete confirmation if active
	if deleteConfirming {
		modal := m.renderDeleteConfirmation()
		return overlayContent(mainView, modal, termWidth, termHeight)
	}

	// Overlay wizard if active
	switch viewMode {
	case ViewModeAddWizard:
		modal := m.renderAddWizard()
		return overlayContent(mainView, modal, termWidth, termHeight)
	case ViewModeRemoveWizard:
		modal := m.renderRemoveWizard()
		return overlayContent(mainView, modal, termWidth, termHeight)
	case ViewModeBenchmark:
		modal := m.renderBenchmark()
		return overlayContent(mainView, modal, termWidth, termHeight)
	case ViewModeHTTPLog:
		// HTTP Log is full-screen, don't overlay on main view
		return m.renderHTTPLog()
	default:
		return mainView
	}
}

func (m model) renderMainView() string {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()

	var b strings.Builder

	// Get terminal dimensions for proper sizing
	termHeight := m.termHeight
	if termHeight == 0 {
		termHeight = 40 // Fallback
	}

	// Color palette
	headerColor := lipgloss.Color("220")  // Yellow
	activeColor := lipgloss.Color("46")   // Green
	warningColor := lipgloss.Color("220") // Yellow
	errorColor := lipgloss.Color("196")   // Red
	mutedColor := lipgloss.Color("240")   // Gray
	selectedBg := lipgloss.Color("240")   // Gray background
	selectedFg := lipgloss.Color("230")   // Light foreground

	// Title with version
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(headerColor).
		Padding(0, 1)

	title := fmt.Sprintf("kportal v%s - Port Forwarding Status", m.ui.version)
	b.WriteString(titleStyle.Render(title))

	// Show update notification if available
	if m.ui.updateAvailable {
		updateStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("42")). // Green
			Bold(true)
		updateMsg := fmt.Sprintf("  Update available: v%s", m.ui.updateVersion)
		b.WriteString(updateStyle.Render(updateMsg))
	}
	b.WriteString("\n\n")

	// No forwards
	if len(m.ui.forwardOrder) == 0 {
		disabledStyle := lipgloss.NewStyle().Foreground(mutedColor)
		b.WriteString(disabledStyle.Render("No forwards configured\n"))
	} else {
		// Build table rows
		var rows [][]string
		for _, id := range m.ui.forwardOrder {
			fwd, ok := m.ui.forwards[id]
			if !ok {
				continue
			}

			isDisabled := m.ui.disabledMap[id] || fwd.Status == "Disabled"

			// Status icon and text
			statusIcon := "●"
			statusText := fwd.Status

			if isDisabled {
				statusIcon = "○"
				statusText = "Disabled"
			} else {
				switch fwd.Status {
				case "Starting":
					statusIcon = "○"
				case "Reconnecting":
					statusIcon = "◐"
				case "Error":
					statusIcon = "✗"
				}
			}

			rows = append(rows, []string{
				truncate(fwd.Context, 14),
				truncate(fwd.Namespace, 16),
				truncate(fwd.Alias, 18),
				truncate(fwd.Type, 8),
				truncate(fwd.Resource, 20),
				fmt.Sprintf("%d", fwd.RemotePort),
				fmt.Sprintf("%d", fwd.LocalPort),
				statusIcon + " " + statusText,
			})
		}

		// Create table with styling (no borders for cleaner look)
		t := table.New().
			Border(lipgloss.HiddenBorder()).
			Headers("CONTEXT", "NAMESPACE", "ALIAS", "TYPE", "RESOURCE", "REMOTE", "LOCAL", "STATUS").
			Rows(rows...).
			StyleFunc(func(row, col int) lipgloss.Style {
				// Header row
				if row == table.HeaderRow {
					return lipgloss.NewStyle().
						Bold(true).
						Foreground(headerColor).
						Padding(0, 1)
				}

				// Get the forward for this row to check its status
				baseStyle := lipgloss.NewStyle().Padding(0, 1)

				if row >= 0 && row < len(m.ui.forwardOrder) {
					id := m.ui.forwardOrder[row]
					fwd, ok := m.ui.forwards[id]
					isSelected := row == m.ui.selectedIndex
					isDisabled := m.ui.disabledMap[id] || (ok && fwd.Status == "Disabled")

					// Selected row gets background highlight
					if isSelected {
						return baseStyle.
							Background(selectedBg).
							Foreground(selectedFg)
					}

					// Disabled rows are muted
					if isDisabled {
						return baseStyle.Foreground(mutedColor)
					}

					// Status column gets colored based on status
					if col == 7 && ok { // STATUS column
						switch fwd.Status {
						case "Active":
							return baseStyle.Foreground(activeColor)
						case "Starting", "Reconnecting":
							return baseStyle.Foreground(warningColor)
						case "Error":
							return baseStyle.Foreground(errorColor)
						}
					}
				}

				return baseStyle
			})

		b.WriteString(t.Render())
		b.WriteString("\n")
	}

	// Display errors if any (before footer)
	if len(m.ui.errors) > 0 {
		b.WriteString("\n\n")
		errorHeaderStyle := lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("196"))

		b.WriteString(errorHeaderStyle.Render("Errors:"))
		b.WriteString("\n")

		errorLineStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Width(118). // Slightly less than table width (120) for padding
			MaxWidth(118)

		for id, errMsg := range m.ui.errors {
			// Find the forward to display its alias
			if fwd, ok := m.ui.forwards[id]; ok {
				// Format: "  • alias: error message"
				prefix := fmt.Sprintf("  • %s: ", fwd.Alias)

				// Wrap the error message if it's too long
				// Max line length is 118, subtract prefix length
				maxErrLen := 118 - len(prefix)
				wrappedMsg := wrapText(errMsg, maxErrLen)

				// Render first line with prefix
				lines := strings.Split(wrappedMsg, "\n")
				if len(lines) > 0 {
					b.WriteString(errorLineStyle.Render(prefix + lines[0]))
					b.WriteString("\n")

					// Render subsequent lines with indentation
					indent := strings.Repeat(" ", len(prefix))
					for i := 1; i < len(lines); i++ {
						b.WriteString(errorLineStyle.Render(indent + lines[i]))
						b.WriteString("\n")
					}
				}
			}
		}
	}

	// Calculate current content height
	currentContent := b.String()
	currentLines := strings.Count(currentContent, "\n") + 1

	// Footer styles
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))

	footer := fmt.Sprintf("%s/%s: Navigate  %s: Toggle  %s: New  %s: Edit  %s: Delete  %s: Bench  %s: Logs  %s: Quit  │  Total: %d",
		keyStyle.Render("↑↓"),
		keyStyle.Render("jk"),
		keyStyle.Render("Space"),
		keyStyle.Render("n"),
		keyStyle.Render("e"),
		keyStyle.Render("d"),
		keyStyle.Render("b"),
		keyStyle.Render("l"),
		keyStyle.Render("q"),
		len(m.ui.forwardOrder))

	// Fill space to push footer to bottom (reserve 2 lines: 1 for spacing, 1 for footer)
	footerHeight := 2
	remainingLines := termHeight - currentLines - footerHeight
	if remainingLines > 0 {
		b.WriteString(strings.Repeat("\n", remainingLines))
	}

	// Add footer at bottom
	b.WriteString("\n")
	b.WriteString(footerStyle.Render(footer))

	return b.String()
}

// wrapText wraps text to the specified width, breaking at word boundaries
func wrapText(text string, width int) string {
	if len(text) <= width {
		return text
	}

	var result strings.Builder
	var line strings.Builder
	words := strings.Fields(text)

	for i, word := range words {
		// If adding this word would exceed width, start new line
		if line.Len()+len(word)+1 > width && line.Len() > 0 {
			result.WriteString(line.String())
			result.WriteString("\n")
			line.Reset()
		}

		// Add space before word (except first word on line)
		if line.Len() > 0 {
			line.WriteString(" ")
		}
		line.WriteString(word)

		// Last word - flush the line
		if i == len(words)-1 {
			result.WriteString(line.String())
		}
	}

	return result.String()
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

// resetDeleteConfirmation resets the delete confirmation dialog state.
// Caller must hold ui.mu lock.
func (ui *BubbleTeaUI) resetDeleteConfirmation() {
	ui.deleteConfirming = false
	ui.deleteConfirmID = ""
	ui.deleteConfirmAlias = ""
	ui.deleteConfirmCursor = 0
}

// renderDeleteConfirmation renders the delete confirmation dialog
func (m model) renderDeleteConfirmation() string {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()

	var b strings.Builder

	// Use wizard color palette for consistency
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(warningColor). // Yellow for warning (delete action)
		Padding(0, 1)

	buttonSelectedStyle := lipgloss.NewStyle().
		Background(primaryColor).          // Pink/Magenta background
		Foreground(lipgloss.Color("230")). // Light yellow text
		Bold(true).
		Padding(0, 1)

	buttonUnselectedStyle := lipgloss.NewStyle().
		Foreground(mutedColor). // Gray
		Padding(0, 1)

	deleteInfoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")). // Light gray for info text
		Italic(true)

	// Title
	b.WriteString(titleStyle.Render("⚠ Delete Port Forward"))
	b.WriteString("\n\n")

	// Message
	b.WriteString("Are you sure you want to delete:\n\n")
	b.WriteString(deleteInfoStyle.Render("  " + m.ui.deleteConfirmAlias))
	b.WriteString("\n\n")

	// Buttons
	if m.ui.deleteConfirmCursor == 0 {
		b.WriteString(buttonSelectedStyle.Render(" Yes "))
		b.WriteString("  ")
		b.WriteString(buttonUnselectedStyle.Render(" No "))
	} else {
		b.WriteString(buttonUnselectedStyle.Render(" Yes "))
		b.WriteString("  ")
		b.WriteString(buttonSelectedStyle.Render(" No "))
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("←/→: Navigate  Enter: Confirm  Esc: Cancel"))

	// Wrap in a box using wizard style
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor). // Purple border like other wizards
		Padding(1, 2)

	return boxStyle.Render(b.String())
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
