// Package ui provides the terminal user interface for kportal using bubbletea.
// It displays port-forward status in an interactive table and provides wizards
// for adding, editing, and removing forwards.
//
// The main components are:
//   - BubbleTeaUI: The interactive TUI with table display and modal dialogs
//   - TableUI: A simpler non-interactive status display for verbose mode
//   - Wizards: Step-by-step interfaces for configuration changes
//   - Controller: Coordinates UI with the forward manager
//
// Key bindings in the main view:
//   - ↑↓/jk: Navigate forwards
//   - Space: Toggle forward enabled/disabled
//   - n: New forward wizard
//   - e: Edit forward wizard
//   - d: Delete forward
//   - b: Benchmark forward
//   - l: View HTTP logs
//   - q: Quit
package ui

import (
	"fmt"
	"log"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/k8s"
)

// safeRecover recovers from panics and logs them
// Use with defer at the start of goroutines and callbacks that could panic
func safeRecover(context string) {
	if r := recover(); r != nil {
		log.Printf("[UI] Panic recovered in %s: %v", context, r)
	}
}

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
	Forward *ForwardStatus
	ID      string
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
	discovery           *k8s.Discovery
	program             *tea.Program
	forwards            map[string]*ForwardStatus
	benchmarkState      *BenchmarkState
	httpLogSubscriber   HTTPLogSubscriber
	disabledMap         map[string]bool
	toggleCallback      func(id string, enable bool)
	httpLogCleanup      func()
	httpLogState        *HTTPLogState
	errors              map[string]string
	mutator             *config.Mutator
	removeWizard        *RemoveWizardState
	addWizard           *AddWizardState
	updateVersion       string
	updateURL           string
	configPath          string
	deleteConfirmID     string
	deleteConfirmAlias  string
	version             string
	forwardOrder        []string
	viewMode            ViewMode
	deleteConfirmCursor int
	selectedIndex       int
	mu                  sync.RWMutex
	deleteConfirming    bool
	updateAvailable     bool
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
		// Clear any previous error when re-enabling
		delete(ui.errors, id)
		ui.mu.Unlock()

		if ui.program != nil {
			ui.program.Send(ForwardUpdateMsg{ID: id, Status: "Starting"})
		}
		return
	}

	// Parse resource (e.g., "pod/my-app" -> type="pod", name="my-app")
	resourceType := "pod"
	resourceName := fwd.Resource
	if parts := strings.SplitN(fwd.Resource, "/", 2); len(parts) == 2 {
		resourceType = parts[0]
		resourceName = parts[1]
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

	// Clear any error associated with this forward
	delete(ui.errors, id)

	// Remove from order
	removedIndex := -1
	for i, fid := range ui.forwardOrder {
		if fid == id {
			removedIndex = i
			ui.forwardOrder = append(ui.forwardOrder[:i], ui.forwardOrder[i+1:]...)
			break
		}
	}

	// Adjust selectedIndex if necessary
	if removedIndex >= 0 {
		// If we removed the selected item or an item before it, adjust
		if ui.selectedIndex >= len(ui.forwardOrder) {
			ui.selectedIndex = len(ui.forwardOrder) - 1
		}
		// Ensure selectedIndex is never negative
		if ui.selectedIndex < 0 {
			ui.selectedIndex = 0
		}
	}

	// Clear delete confirmation if we're deleting the same forward
	if ui.deleteConfirming && ui.deleteConfirmID == id {
		ui.resetDeleteConfirmation()
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

	case clearCopyMessageMsg:
		m.ui.mu.Lock()
		if m.ui.httpLogState != nil {
			m.ui.httpLogState.copyMessage = ""
		}
		m.ui.mu.Unlock()
		return m, nil
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
		termWidth = DefaultTermWidth
	}
	if termHeight == 0 {
		termHeight = DefaultTermHeight
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

// mainViewColors holds the color palette for the main view
type mainViewColors struct {
	header     lipgloss.Color
	active     lipgloss.Color
	warning    lipgloss.Color
	errorColor lipgloss.Color
	muted      lipgloss.Color
	selectedBg lipgloss.Color
	selectedFg lipgloss.Color
}

// defaultMainViewColors returns the default color palette
func defaultMainViewColors() mainViewColors {
	return mainViewColors{
		header:     lipgloss.Color("220"), // Yellow
		active:     lipgloss.Color("46"),  // Green
		warning:    lipgloss.Color("220"), // Yellow
		errorColor: lipgloss.Color("196"), // Red
		muted:      lipgloss.Color("240"), // Gray
		selectedBg: lipgloss.Color("240"), // Gray background
		selectedFg: lipgloss.Color("230"), // Light foreground
	}
}

// keyBinding represents a keyboard shortcut and its description
type keyBinding struct {
	key  string
	desc string
}

// mainViewKeyBindings returns the key bindings for the main view
func mainViewKeyBindings() []keyBinding {
	return []keyBinding{
		{"↑↓/jk", "Navigate"},
		{"Space", "Toggle"},
		{"n", "New"},
		{"e", "Edit"},
		{"d", "Delete"},
		{"b", "Bench"},
		{"l", "Logs"},
		{"q", "Quit"},
	}
}

func (m model) renderMainView() string {
	m.ui.mu.RLock()
	defer m.ui.mu.RUnlock()

	var b strings.Builder
	colors := defaultMainViewColors()

	// Get terminal dimensions for proper sizing
	termWidth, termHeight := m.getTermDimensions()

	// Render title header
	b.WriteString(m.renderTitle(colors.header))

	// Render forwards table or empty message
	if len(m.ui.forwardOrder) == 0 {
		b.WriteString(m.renderEmptyMessage(colors.muted))
	} else {
		b.WriteString(m.renderForwardsTable(colors))
	}

	// Render error section if any errors exist
	if len(m.ui.errors) > 0 {
		b.WriteString(m.renderErrorSection())
	}

	// Render footer with proper spacing
	b.WriteString(m.renderFooterWithSpacing(termWidth, termHeight, &b))

	return b.String()
}

// getTermDimensions returns terminal dimensions with fallback defaults
func (m model) getTermDimensions() (width, height int) {
	width = m.termWidth
	height = m.termHeight
	if width == 0 {
		width = DefaultTermWidth
	}
	if height == 0 {
		height = DefaultTermHeight
	}
	return
}

// renderTitle renders the title bar with version and optional update notification
func (m model) renderTitle(headerColor lipgloss.Color) string {
	var b strings.Builder

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

	return b.String()
}

// renderEmptyMessage renders the message shown when no forwards are configured
func (m model) renderEmptyMessage(mutedColor lipgloss.Color) string {
	disabledStyle := lipgloss.NewStyle().Foreground(mutedColor)
	return disabledStyle.Render("No forwards configured\n")
}

// renderForwardsTable renders the forwards table with all styling
func (m model) renderForwardsTable(colors mainViewColors) string {
	var b strings.Builder

	// Build table rows
	rows := m.buildTableRows()

	// Create table with styling (no borders for cleaner look)
	t := table.New().
		Border(lipgloss.HiddenBorder()).
		Headers("CONTEXT", "NAMESPACE", "ALIAS", "TYPE", "RESOURCE", "REMOTE", "LOCAL", "STATUS").
		Rows(rows...).
		StyleFunc(m.createTableStyleFunc(colors))

	b.WriteString(t.Render())
	b.WriteString("\n")

	return b.String()
}

// buildTableRows builds the data rows for the forwards table
func (m model) buildTableRows() [][]string {
	var rows [][]string

	for _, id := range m.ui.forwardOrder {
		fwd, ok := m.ui.forwards[id]
		if !ok {
			continue
		}

		statusIcon, statusText := m.getStatusIconAndText(id, fwd)

		localPortText := fmt.Sprintf("%d", fwd.LocalPort)
		if fwd.Status == "Active" && !m.ui.isForwardDisabled(id) {
			localPortText = hyperlink(fmt.Sprintf("http://127.0.0.1:%d", fwd.LocalPort), fmt.Sprintf("%d→", fwd.LocalPort))
		}

		rows = append(rows, []string{
			truncate(fwd.Context, ColumnWidthContext),
			truncate(fwd.Namespace, ColumnWidthNamespace),
			truncate(fwd.Alias, ColumnWidthAlias),
			truncate(fwd.Type, ColumnWidthType),
			truncate(fwd.Resource, ColumnWidthResource),
			fmt.Sprintf("%d", fwd.RemotePort),
			localPortText,
			statusIcon + " " + statusText,
		})
	}

	return rows
}

// getStatusIconAndText returns the appropriate status icon and text for a forward
func (m model) getStatusIconAndText(id string, fwd *ForwardStatus) (icon, text string) {
	icon = "●"
	text = fwd.Status

	if m.ui.isForwardDisabled(id) {
		return "○", "Disabled"
	}

	switch fwd.Status {
	case "Starting":
		icon = "○"
	case "Reconnecting":
		icon = "◐"
	case "Error":
		icon = "✗"
	}

	return icon, text
}

// createTableStyleFunc creates the style function for the forwards table
func (m model) createTableStyleFunc(colors mainViewColors) func(row, col int) lipgloss.Style {
	return func(row, col int) lipgloss.Style {
		// Header row
		if row == table.HeaderRow {
			return lipgloss.NewStyle().
				Bold(true).
				Foreground(colors.header).
				Padding(0, 1)
		}

		baseStyle := lipgloss.NewStyle().Padding(0, 1)

		if row >= 0 && row < len(m.ui.forwardOrder) {
			id := m.ui.forwardOrder[row]
			fwd, ok := m.ui.forwards[id]
			isSelected := row == m.ui.selectedIndex
			isDisabled := m.ui.isForwardDisabled(id)

			// Selected row gets background highlight
			if isSelected {
				return baseStyle.
					Background(colors.selectedBg).
					Foreground(colors.selectedFg)
			}

			// Disabled rows are muted
			if isDisabled {
				return baseStyle.Foreground(colors.muted)
			}

			// Status column gets colored based on status
			if col == ColumnStatus && ok {
				switch fwd.Status {
				case "Active":
					return baseStyle.Foreground(colors.active)
				case "Starting", "Reconnecting":
					return baseStyle.Foreground(colors.warning)
				case "Error":
					return baseStyle.Foreground(colors.errorColor)
				}
			}

		}

		return baseStyle
	}
}

// renderErrorSection renders the error display section
func (m model) renderErrorSection() string {
	var b strings.Builder

	b.WriteString("\n\n")
	errorHeaderStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("196"))

	b.WriteString(errorHeaderStyle.Render("Errors:"))
	b.WriteString("\n")

	errorLineStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("196")).
		Width(ErrorDisplayWidth).
		MaxWidth(ErrorDisplayWidth)

	for id, errMsg := range m.ui.errors {
		// Find the forward to display its alias
		if fwd, ok := m.ui.forwards[id]; ok {
			b.WriteString(m.renderErrorLine(fwd.Alias, errMsg, errorLineStyle))
		}
	}

	return b.String()
}

// renderErrorLine renders a single error line with proper wrapping
func (m model) renderErrorLine(alias, errMsg string, style lipgloss.Style) string {
	var b strings.Builder

	// Format: "  • alias: error message"
	prefix := fmt.Sprintf("  • %s: ", alias)

	// Wrap the error message if it's too long
	maxErrLen := ErrorDisplayWidth - len(prefix)
	wrappedMsg := wrapText(errMsg, maxErrLen)

	// Render first line with prefix
	lines := strings.Split(wrappedMsg, "\n")
	if len(lines) > 0 {
		b.WriteString(style.Render(prefix + lines[0]))
		b.WriteString("\n")

		// Render subsequent lines with indentation
		indent := strings.Repeat(" ", len(prefix))
		for i := 1; i < len(lines); i++ {
			b.WriteString(style.Render(indent + lines[i]))
			b.WriteString("\n")
		}
	}

	return b.String()
}

// renderFooterWithSpacing renders the footer with proper vertical spacing
func (m model) renderFooterWithSpacing(termWidth, termHeight int, content *strings.Builder) string {
	var b strings.Builder

	// Calculate current content height
	currentContent := content.String()
	currentLines := strings.Count(currentContent, "\n") + 1

	// Build footer content
	footerLines := m.buildFooterLines(termWidth)

	// Calculate footer height and add spacing
	footerHeight := len(footerLines) + 1 // +1 for the blank line before footer
	remainingLines := termHeight - currentLines - footerHeight
	if remainingLines > 0 {
		b.WriteString(strings.Repeat("\n", remainingLines))
	}

	// Add footer at bottom
	footerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	b.WriteString("\n")
	for i, line := range footerLines {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(footerStyle.Render(line))
	}

	return b.String()
}

// buildFooterLines builds the footer lines that fit within terminal width
func (m model) buildFooterLines(termWidth int) []string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))
	bindings := mainViewKeyBindings()

	var footerLines []string
	var currentLine strings.Builder
	currentLineVisualLen := 0

	// Calculate how much space we need for the total count suffix
	totalSuffix := fmt.Sprintf("  │  Total: %d", len(m.ui.forwardOrder))
	totalSuffixLen := len(totalSuffix)

	// Available width (account for some margin)
	availableWidth := termWidth - 4

	for i, binding := range bindings {
		// Build this binding's text
		keyRendered := keyStyle.Render(binding.key)
		bindingText := keyRendered + ": " + binding.desc
		// Visual length without ANSI codes
		bindingVisualLen := len(binding.key) + 2 + len(binding.desc)

		// Add separator if not first item on line
		separator := ""
		separatorLen := 0
		if currentLine.Len() > 0 {
			separator = "  "
			separatorLen = 2
		}

		// Check if this binding fits on current line
		// For the last binding, also need to fit the total suffix
		neededWidth := currentLineVisualLen + separatorLen + bindingVisualLen
		if i == len(bindings)-1 {
			neededWidth += totalSuffixLen
		}

		if neededWidth > availableWidth && currentLine.Len() > 0 {
			// Start a new line
			footerLines = append(footerLines, currentLine.String())
			currentLine.Reset()
			currentLineVisualLen = 0
			separator = ""
			separatorLen = 0
		}

		currentLine.WriteString(separator)
		currentLine.WriteString(bindingText)
		currentLineVisualLen += separatorLen + bindingVisualLen
	}

	// Add total count to the last line
	currentLine.WriteString(totalSuffix)
	footerLines = append(footerLines, currentLine.String())

	return footerLines
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
	b.WriteString(wrapHelpText("←/→: Navigate  Enter: Confirm  Esc: Cancel", wizardHelpWidth(m.termWidth)))

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

// isForwardDisabled checks if a forward is disabled.
// A forward is considered disabled if either:
// 1. The user has disabled it via the UI (tracked in disabledMap)
// 2. The forward's status is "Disabled" (from the manager)
// Caller must hold ui.mu.RLock or ui.mu.Lock.
func (ui *BubbleTeaUI) isForwardDisabled(id string) bool {
	if ui.disabledMap[id] {
		return true
	}
	if fwd, ok := ui.forwards[id]; ok && fwd.Status == "Disabled" {
		return true
	}
	return false
}
