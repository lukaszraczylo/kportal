package ui

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/k8s"
	"golang.design/x/clipboard"
)

// isFilterableStep returns true if the step supports search/filter
func isFilterableStep(step AddWizardStep) bool {
	switch step {
	case StepSelectContext, StepSelectNamespace:
		return true
	case StepEnterResource:
		// Only service selection is filterable (pod prefix and selector are text input)
		return true // We'll check resource type in the handler
	default:
		return false
	}
}

// handleMainViewKeys handles keyboard input in the main view
func (m model) handleMainViewKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// If delete confirmation is showing, handle it separately
	if m.ui.deleteConfirming {
		return m.handleDeleteConfirmation(msg)
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k":
		m.ui.moveSelection(-1)

	case "down", "j":
		m.ui.moveSelection(1)

	case "pgup", "ctrl+u":
		m.ui.moveSelection(-10)

	case "pgdown", "ctrl+d":
		m.ui.moveSelection(10)

	case " ", "enter":
		m.ui.toggleSelected()

	case "n": // Enter add wizard
		m.ui.mu.Lock()
		// Don't create a new wizard if one is already active
		if m.ui.addWizard != nil || m.ui.removeWizard != nil || m.ui.benchmarkState != nil || m.ui.httpLogState != nil {
			m.ui.mu.Unlock()
			return m, nil
		}
		if m.ui.discovery == nil || m.ui.mutator == nil {
			// Dependencies not set up
			m.ui.mu.Unlock()
			return m, nil
		}

		m.ui.viewMode = ViewModeAddWizard
		m.ui.addWizard = newAddWizardState()
		m.ui.addWizard.loading = true
		m.ui.mu.Unlock()

		// Load contexts
		return m, loadContextsCmd(m.ui.discovery)

	case "e": // Edit selected forward
		m.ui.mu.Lock()
		// Don't create a new wizard if one is already active
		if m.ui.addWizard != nil || m.ui.removeWizard != nil || m.ui.benchmarkState != nil || m.ui.httpLogState != nil {
			m.ui.mu.Unlock()
			return m, nil
		}

		if len(m.ui.forwardOrder) == 0 {
			// No forwards to edit
			m.ui.mu.Unlock()
			return m, nil
		}

		if m.ui.discovery == nil || m.ui.mutator == nil {
			// Dependencies not set up
			m.ui.mu.Unlock()
			return m, nil
		}

		// Get the currently selected forward
		currentSelectedIndex := m.ui.selectedIndex
		if currentSelectedIndex < 0 || currentSelectedIndex >= len(m.ui.forwardOrder) {
			m.ui.mu.Unlock()
			return m, nil
		}

		selectedID := m.ui.forwardOrder[currentSelectedIndex]
		selectedForward, ok := m.ui.forwards[selectedID]
		if !ok {
			m.ui.mu.Unlock()
			return m, nil
		}

		// Create an add wizard pre-filled with the current forward's values
		m.ui.viewMode = ViewModeAddWizard
		m.ui.addWizard = newAddWizardState()

		// Pre-fill the wizard with current values
		m.ui.addWizard.selectedContext = selectedForward.Context
		m.ui.addWizard.selectedNamespace = selectedForward.Namespace
		m.ui.addWizard.resourceValue = selectedForward.Resource
		m.ui.addWizard.remotePort = selectedForward.RemotePort
		m.ui.addWizard.localPort = selectedForward.LocalPort
		m.ui.addWizard.alias = selectedForward.Alias

		// Determine resource type from the resource string
		if strings.HasPrefix(selectedForward.Type, "service") {
			m.ui.addWizard.selectedResourceType = ResourceTypeService
		} else {
			m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
		}

		// Mark as edit mode and store original ID
		m.ui.addWizard.isEditing = true
		m.ui.addWizard.originalID = selectedID

		// Start at the remote port step (skip context/namespace/resource selection)
		m.ui.addWizard.step = StepEnterRemotePort

		// Load resources to detect ports
		m.ui.addWizard.loading = true
		m.ui.mu.Unlock()

		// Load pods or services to detect available ports
		if m.ui.addWizard.selectedResourceType == ResourceTypeService {
			return m, loadServicesCmd(m.ui.discovery, selectedForward.Context, selectedForward.Namespace)
		}
		return m, loadPodsCmd(m.ui.discovery, selectedForward.Context, selectedForward.Namespace)

	case "d": // Delete currently selected forward - show confirmation
		m.ui.mu.Lock()

		// Don't overwrite existing confirmation dialog
		if m.ui.deleteConfirming {
			m.ui.mu.Unlock()
			return m, nil
		}

		if len(m.ui.forwardOrder) == 0 {
			// No forwards to delete
			m.ui.mu.Unlock()
			return m, nil
		}

		if m.ui.mutator == nil {
			// Dependencies not set up
			m.ui.mu.Unlock()
			return m, nil
		}

		// Get the currently selected forward
		currentSelectedIndex := m.ui.selectedIndex
		if currentSelectedIndex < 0 || currentSelectedIndex >= len(m.ui.forwardOrder) {
			m.ui.mu.Unlock()
			return m, nil
		}

		selectedID := m.ui.forwardOrder[currentSelectedIndex]
		selectedForward, ok := m.ui.forwards[selectedID]
		if !ok {
			m.ui.mu.Unlock()
			return m, nil
		}

		// Show confirmation dialog
		m.ui.deleteConfirming = true
		m.ui.deleteConfirmID = selectedID
		m.ui.deleteConfirmAlias = selectedForward.Alias
		m.ui.deleteConfirmCursor = 1 // Default to "No" for safety

		m.ui.mu.Unlock()
		return m, nil

	case "b": // Benchmark selected forward
		m.ui.mu.Lock()
		// Don't create benchmark view if another modal is active
		if m.ui.addWizard != nil || m.ui.removeWizard != nil || m.ui.benchmarkState != nil || m.ui.httpLogState != nil {
			m.ui.mu.Unlock()
			return m, nil
		}

		if len(m.ui.forwardOrder) == 0 {
			m.ui.mu.Unlock()
			return m, nil
		}

		currentSelectedIndex := m.ui.selectedIndex
		if currentSelectedIndex < 0 || currentSelectedIndex >= len(m.ui.forwardOrder) {
			m.ui.mu.Unlock()
			return m, nil
		}

		selectedID := m.ui.forwardOrder[currentSelectedIndex]
		selectedForward, ok := m.ui.forwards[selectedID]
		if !ok {
			m.ui.mu.Unlock()
			return m, nil
		}

		// Create benchmark state
		m.ui.viewMode = ViewModeBenchmark
		m.ui.benchmarkState = newBenchmarkState(selectedID, selectedForward.Alias, selectedForward.LocalPort)
		// Initialize textInput with the first field's value
		m.ui.benchmarkState.textInput = m.ui.benchmarkState.urlPath

		m.ui.mu.Unlock()
		return m, nil

	case "l": // View HTTP logs for selected forward
		m.ui.mu.Lock()
		// Don't create log view if another modal is active
		if m.ui.addWizard != nil || m.ui.removeWizard != nil || m.ui.benchmarkState != nil || m.ui.httpLogState != nil {
			m.ui.mu.Unlock()
			return m, nil
		}

		if len(m.ui.forwardOrder) == 0 {
			m.ui.mu.Unlock()
			return m, nil
		}

		currentSelectedIndex := m.ui.selectedIndex
		if currentSelectedIndex < 0 || currentSelectedIndex >= len(m.ui.forwardOrder) {
			m.ui.mu.Unlock()
			return m, nil
		}

		selectedID := m.ui.forwardOrder[currentSelectedIndex]
		selectedForward, ok := m.ui.forwards[selectedID]
		if !ok {
			m.ui.mu.Unlock()
			return m, nil
		}

		// Create HTTP log state
		m.ui.viewMode = ViewModeHTTPLog
		m.ui.httpLogState = newHTTPLogState(selectedID, selectedForward.Alias)

		// Capture subscriber and UI reference for the callback
		subscriber := m.ui.httpLogSubscriber
		ui := m.ui
		m.ui.mu.Unlock()

		// Subscribe to HTTP logs if subscriber is available
		// This is done outside the lock to prevent deadlocks in the callback
		if subscriber != nil {
			cleanup := subscriber(selectedID, func(entry HTTPLogEntry) {
				// Recover from panics in the callback
				defer safeRecover("HTTPLogSubscriber callback")

				// Use RLock to safely access program
				ui.mu.RLock()
				program := ui.program
				ui.mu.RUnlock()

				// Send entry to program (thread-safe via Send)
				if program != nil {
					program.Send(HTTPLogEntryMsg{Entry: entry})
				}
			})
			ui.mu.Lock()
			ui.httpLogCleanup = cleanup
			ui.mu.Unlock()
		}

		return m, nil
	}

	return m, nil
}

// handleDeleteConfirmation handles keyboard input for delete confirmation dialog
func (m model) handleDeleteConfirmation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()

	switch msg.String() {
	case "ctrl+c", "esc":
		// Cancel deletion
		m.ui.resetDeleteConfirmation()
		m.ui.mu.Unlock()
		return m, tea.ClearScreen

	case "left", "h", "right", "l":
		// Toggle between Yes/No
		m.ui.deleteConfirmCursor = 1 - m.ui.deleteConfirmCursor
		m.ui.mu.Unlock()
		return m, nil

	case "enter", "y":
		// Confirm deletion (either Enter on Yes or pressing 'y')
		if m.ui.deleteConfirmCursor == 0 || msg.String() == "y" {
			id := m.ui.deleteConfirmID
			m.ui.resetDeleteConfirmation()
			m.ui.mu.Unlock()
			return m, removeForwardByIDCmd(m.ui.mutator, id)
		}
		// Enter on No = cancel
		m.ui.resetDeleteConfirmation()
		m.ui.mu.Unlock()
		return m, tea.ClearScreen

	case "n":
		// Quick 'n' for no
		m.ui.resetDeleteConfirmation()
		m.ui.mu.Unlock()
		return m, tea.ClearScreen
	}

	m.ui.mu.Unlock()
	return m, nil
}

// handleAddWizardKeys handles keyboard input in the add wizard
func (m model) handleAddWizardKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	wizard := m.ui.addWizard
	if wizard == nil {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		// Hard cancel
		m.ui.viewMode = ViewModeMain
		m.ui.addWizard = nil
		return m, tea.ClearScreen

	case "esc":
		// If there's an active search filter, clear it instead of going back
		if wizard.searchFilter != "" && isFilterableStep(wizard.step) {
			wizard.clearSearchFilter()
			return m, nil
		}

		// In edit mode, Esc always cancels (don't navigate back through skipped steps)
		if wizard.isEditing {
			m.ui.viewMode = ViewModeMain
			m.ui.addWizard = nil
			return m, tea.ClearScreen
		}

		// In add mode, go back or cancel
		if wizard.step == StepSelectContext {
			// On first step, cancel entirely
			m.ui.viewMode = ViewModeMain
			m.ui.addWizard = nil
			return m, tea.ClearScreen
		} else {
			// Go back one step
			wizard.step--
			wizard.resetInput()

			// Reset input mode based on the step we're going back to
			switch wizard.step {
			case StepSelectContext, StepSelectNamespace, StepSelectResourceType:
				wizard.inputMode = InputModeList
			case StepEnterResource:
				if wizard.selectedResourceType == ResourceTypeService {
					wizard.inputMode = InputModeList
				} else {
					wizard.inputMode = InputModeText
				}
			case StepEnterRemotePort, StepEnterLocalPort:
				wizard.inputMode = InputModeText
			case StepConfirmation:
				wizard.inputMode = InputModeList
			}
		}
		return m, nil

	case "up", "k":
		// In confirmation step, toggle between alias and buttons
		if wizard.step == StepConfirmation {
			if wizard.confirmationFocus == FocusButtons {
				wizard.confirmationFocus = FocusAlias
			}
		} else {
			wizard.moveCursor(-1)
		}

	case "down", "j":
		// In confirmation step, toggle between alias and buttons
		if wizard.step == StepConfirmation {
			if wizard.confirmationFocus == FocusAlias {
				wizard.confirmationFocus = FocusButtons
				wizard.cursor = 0
			} else {
				wizard.moveCursor(1) // Navigate between buttons
			}
		} else {
			wizard.moveCursor(1)
		}

	case "pgup", "ctrl+u":
		// Page up - move 10 items
		wizard.moveCursor(-10)

	case "pgdown", "ctrl+d":
		// Page down - move 10 items
		wizard.moveCursor(10)

	case "tab":
		// Tab moves between alias field and buttons in confirmation
		if wizard.step == StepConfirmation {
			if wizard.confirmationFocus == FocusAlias {
				wizard.confirmationFocus = FocusButtons
				wizard.cursor = 0
			} else {
				wizard.confirmationFocus = FocusAlias
			}
		}

	case "enter":
		return m.handleAddWizardEnter()

	case "backspace":
		// Allow backspace in text input mode OR when focused on alias in confirmation OR when filtering
		canBackspace := wizard.inputMode == InputModeText ||
			(wizard.step == StepConfirmation && wizard.confirmationFocus == FocusAlias) ||
			(wizard.inputMode == InputModeList && isFilterableStep(wizard.step) && len(wizard.searchFilter) > 0)

		if canBackspace {
			if isFilterableStep(wizard.step) && wizard.inputMode == InputModeList && len(wizard.searchFilter) > 0 {
				// Backspace in search filter
				wizard.searchFilter = wizard.searchFilter[:len(wizard.searchFilter)-1]
				wizard.cursor = 0
				wizard.scrollOffset = 0
			} else if len(wizard.textInput) > 0 {
				wizard.textInput = wizard.textInput[:len(wizard.textInput)-1]
			}
		}

	default:
		// Handle text input
		canTypeText := wizard.inputMode == InputModeText ||
			(wizard.step == StepConfirmation && wizard.confirmationFocus == FocusAlias) ||
			(wizard.inputMode == InputModeList && isFilterableStep(wizard.step))

		if canTypeText && len(msg.String()) == 1 {
			// If in list mode on filterable step, add to search filter instead of textInput
			if wizard.inputMode == InputModeList && isFilterableStep(wizard.step) {
				char := rune(msg.String()[0])
				// Only allow printable characters
				if char >= 32 && char < 127 {
					wizard.searchFilter += string(char)
					wizard.cursor = 0
					wizard.scrollOffset = 0
				}
			} else {
				wizard.handleTextInput(rune(msg.String()[0]))

				// Trigger validation for selector
				if wizard.step == StepEnterResource && wizard.selectedResourceType == ResourceTypePodSelector {
					if len(wizard.textInput) > 0 {
						wizard.loading = true
						wizard.error = nil
						return m, validateSelectorCmd(m.ui.discovery, wizard.selectedContext, wizard.selectedNamespace, wizard.textInput)
					}
				}
			}
		}
	}

	return m, nil
}

// handleAddWizardEnter handles Enter key in the add wizard
func (m model) handleAddWizardEnter() (tea.Model, tea.Cmd) {
	wizard := m.ui.addWizard

	// Don't process Enter if we're currently loading
	if wizard.loading {
		return m, nil
	}

	switch wizard.step {
	case StepSelectContext:
		filteredContexts := wizard.getFilteredContexts()
		if wizard.cursor >= 0 && wizard.cursor < len(filteredContexts) {
			wizard.selectedContext = filteredContexts[wizard.cursor]
			wizard.step = StepSelectNamespace
			wizard.cursor = 0
			wizard.clearSearchFilter()
			wizard.loading = true
			return m, loadNamespacesCmd(m.ui.discovery, wizard.selectedContext)
		}

	case StepSelectNamespace:
		filteredNamespaces := wizard.getFilteredNamespaces()
		if wizard.cursor >= 0 && wizard.cursor < len(filteredNamespaces) {
			wizard.selectedNamespace = filteredNamespaces[wizard.cursor]
			wizard.step = StepSelectResourceType
			wizard.cursor = 0
			wizard.clearSearchFilter()
			wizard.inputMode = InputModeList
		}

	case StepSelectResourceType:
		if wizard.cursor >= 0 && wizard.cursor < 3 {
			wizard.selectedResourceType = ResourceType(wizard.cursor)
			wizard.step = StepEnterResource
			wizard.cursor = 0

			if wizard.selectedResourceType == ResourceTypeService {
				wizard.inputMode = InputModeList
				wizard.loading = true
				return m, loadServicesCmd(m.ui.discovery, wizard.selectedContext, wizard.selectedNamespace)
			} else {
				wizard.inputMode = InputModeText
				wizard.loading = true
				return m, loadPodsCmd(m.ui.discovery, wizard.selectedContext, wizard.selectedNamespace)
			}
		}

	case StepEnterResource:
		switch wizard.selectedResourceType {
		case ResourceTypePodPrefix:
			if wizard.textInput != "" {
				wizard.resourceValue = wizard.textInput
				wizard.step = StepEnterRemotePort
				wizard.clearTextInput()

				// Detect ports from matching pods
				wizard.detectedPorts = k8s.GetUniquePorts(wizard.pods)
				if len(wizard.detectedPorts) > 0 {
					wizard.inputMode = InputModeList
					wizard.cursor = 0
				} else {
					wizard.inputMode = InputModeText
				}
			}

		case ResourceTypePodSelector:
			if wizard.textInput != "" && len(wizard.matchingPods) > 0 {
				wizard.resourceValue = "pod"
				wizard.selector = wizard.textInput
				wizard.step = StepEnterRemotePort
				wizard.clearTextInput()

				// Detect ports from matching pods
				wizard.detectedPorts = k8s.GetUniquePorts(wizard.matchingPods)
				if len(wizard.detectedPorts) > 0 {
					wizard.inputMode = InputModeList
					wizard.cursor = 0
				} else {
					wizard.inputMode = InputModeText
				}
			}

		case ResourceTypeService:
			filteredServices := wizard.getFilteredServices()
			if wizard.cursor >= 0 && wizard.cursor < len(filteredServices) {
				wizard.resourceValue = filteredServices[wizard.cursor].Name

				// Get ports from selected service (must do this BEFORE clearing search filter)
				wizard.detectedPorts = filteredServices[wizard.cursor].Ports

				wizard.step = StepEnterRemotePort
				wizard.clearTextInput()
				wizard.clearSearchFilter()

				if len(wizard.detectedPorts) > 0 {
					wizard.inputMode = InputModeList
					wizard.cursor = 0
				} else {
					wizard.inputMode = InputModeText
				}
			}
		}

	case StepEnterRemotePort:
		if wizard.inputMode == InputModeList && len(wizard.detectedPorts) > 0 {
			// List mode - user selected from detected ports
			if wizard.cursor == len(wizard.detectedPorts) {
				// Selected "Manual entry" option
				wizard.inputMode = InputModeText
				wizard.clearTextInput()
			} else if wizard.cursor >= 0 && wizard.cursor < len(wizard.detectedPorts) {
				// Selected a detected port
				// For services, use TargetPort (actual pod port) if available
				// For pods, TargetPort is 0, so use Port (container port)
				selectedPort := wizard.detectedPorts[wizard.cursor]
				if selectedPort.TargetPort > 0 {
					wizard.remotePort = int(selectedPort.TargetPort)
				} else {
					wizard.remotePort = int(selectedPort.Port)
				}
				wizard.step = StepEnterLocalPort
				wizard.clearTextInput()
				wizard.inputMode = InputModeText
				wizard.error = nil
			}
		} else {
			// Text mode - manual entry
			port, err := strconv.Atoi(wizard.textInput)
			if err != nil || !config.IsValidPort(port) {
				wizard.error = fmt.Errorf("invalid port number")
			} else {
				wizard.remotePort = port
				wizard.step = StepEnterLocalPort
				wizard.clearTextInput()
				wizard.error = nil
			}
		}

	case StepEnterLocalPort:
		port, err := strconv.Atoi(wizard.textInput)
		if err != nil || !config.IsValidPort(port) {
			wizard.error = fmt.Errorf("invalid port number")
		} else {
			// Check port availability before proceeding
			wizard.localPort = port
			wizard.loading = true
			wizard.error = nil
			return m, checkPortCmd(port, m.ui.configPath)
		}

	case StepConfirmation:
		// If focused on alias field, move to buttons
		if wizard.confirmationFocus == FocusAlias {
			wizard.confirmationFocus = FocusButtons
			wizard.cursor = 0
			return m, nil
		}

		// Handle button selection
		if wizard.cursor == 0 {
			// Check if port is available before saving
			if !wizard.portAvailable {
				wizard.error = fmt.Errorf("port %d is not available. Please choose a different port", wizard.localPort)
				return m, nil
			}

			// Confirmed - save the forward
			wizard.alias = wizard.textInput

			// Build the forward config
			fwd := config.Forward{
				Protocol:  "tcp",
				Port:      wizard.remotePort,
				LocalPort: wizard.localPort,
				Alias:     wizard.alias,
			}

			if wizard.selectedResourceType == ResourceTypePodPrefix {
				fwd.Resource = "pod/" + wizard.resourceValue
			} else if wizard.selectedResourceType == ResourceTypePodSelector {
				fwd.Resource = wizard.resourceValue
				fwd.Selector = wizard.selector
			} else if wizard.selectedResourceType == ResourceTypeService {
				fwd.Resource = "service/" + wizard.resourceValue
			}

			wizard.loading = true

			// If editing, use atomic update operation
			if wizard.isEditing {
				return m, updateForwardCmd(m.ui.mutator, wizard.originalID, wizard.selectedContext, wizard.selectedNamespace, fwd)
			}

			return m, saveForwardCmd(m.ui.mutator, wizard.selectedContext, wizard.selectedNamespace, fwd)
		} else {
			// Cancelled - return to main view with screen clear
			m.ui.viewMode = ViewModeMain
			m.ui.addWizard = nil
			return m, tea.ClearScreen
		}

	case StepSuccess:
		if wizard.cursor == 0 {
			// Add another
			m.ui.addWizard = newAddWizardState()
			m.ui.addWizard.loading = true
			return m, loadContextsCmd(m.ui.discovery)
		} else {
			// Return to main view with screen clear
			m.ui.viewMode = ViewModeMain
			m.ui.addWizard = nil
			return m, tea.ClearScreen
		}
	}

	return m, nil
}

// handleRemoveWizardKeys handles keyboard input in the remove wizard
func (m model) handleRemoveWizardKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	wizard := m.ui.removeWizard
	if wizard == nil {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c":
		// Hard cancel - always exit
		m.ui.viewMode = ViewModeMain
		m.ui.removeWizard = nil
		return m, tea.ClearScreen

	case "esc":
		if wizard.confirming {
			// In confirmation mode, Esc confirms the removal (same as pressing Yes)
			selectedForwards := wizard.getSelectedForwards()
			return m, removeForwardsCmd(m.ui.mutator, selectedForwards)
		} else {
			// Not confirming yet - cancel entirely
			m.ui.viewMode = ViewModeMain
			m.ui.removeWizard = nil
		}
		return m, tea.ClearScreen

	case "up", "k":
		wizard.moveCursor(-1)

	case "down", "j":
		wizard.moveCursor(1)

	case "pgup", "ctrl+u":
		wizard.moveCursor(-10)

	case "pgdown", "ctrl+d":
		wizard.moveCursor(10)

	case " ":
		if !wizard.confirming {
			wizard.toggleSelection()
		}

	case "a":
		wizard.selectAll()

	case "n":
		wizard.selectNone()

	case "enter":
		if !wizard.confirming {
			if wizard.getSelectedCount() == 0 {
				// Nothing selected
				return m, nil
			}
			// Show confirmation
			wizard.confirming = true
			wizard.confirmCursor = 0
		} else {
			// Confirmed
			if wizard.confirmCursor == 0 {
				// Yes, remove
				selectedForwards := wizard.getSelectedForwards()
				return m, removeForwardsCmd(m.ui.mutator, selectedForwards)
			} else {
				// No, cancel
				wizard.confirming = false
			}
		}
	}

	return m, nil
}

// Message handlers

func (m model) handleContextsLoaded(msg ContextsLoadedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.addWizard != nil {
		m.ui.addWizard.loading = false
		m.ui.addWizard.error = msg.err
		if msg.err == nil {
			// Get current context and move it to the top (if discovery is available)
			if m.ui.discovery != nil {
				currentCtx, err := m.ui.discovery.GetCurrentContext()
				if err == nil && currentCtx != "" {
					// Reorder contexts with current first
					reordered := []string{currentCtx}
					for _, ctx := range msg.contexts {
						if ctx != currentCtx {
							reordered = append(reordered, ctx)
						}
					}
					m.ui.addWizard.contexts = reordered
				} else {
					m.ui.addWizard.contexts = msg.contexts
				}
			} else {
				m.ui.addWizard.contexts = msg.contexts
			}
		}
	}

	return m, nil
}

func (m model) handleNamespacesLoaded(msg NamespacesLoadedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.addWizard != nil {
		m.ui.addWizard.loading = false
		m.ui.addWizard.error = msg.err
		if msg.err == nil {
			m.ui.addWizard.namespaces = msg.namespaces
		}
	}

	return m, nil
}

func (m model) handlePodsLoaded(msg PodsLoadedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.addWizard != nil {
		m.ui.addWizard.loading = false
		m.ui.addWizard.error = msg.err
		if msg.err == nil {
			m.ui.addWizard.pods = msg.pods

			// If we're at the remote port step (edit mode), detect ports now
			if m.ui.addWizard.step == StepEnterRemotePort {
				m.ui.addWizard.detectedPorts = k8s.GetUniquePorts(msg.pods)
				if len(m.ui.addWizard.detectedPorts) > 0 {
					m.ui.addWizard.inputMode = InputModeList
					m.ui.addWizard.cursor = 0
				} else {
					m.ui.addWizard.inputMode = InputModeText
					m.ui.addWizard.textInput = fmt.Sprintf("%d", m.ui.addWizard.remotePort)
				}
			}
		}
	}

	return m, nil
}

func (m model) handleServicesLoaded(msg ServicesLoadedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.addWizard != nil {
		m.ui.addWizard.loading = false
		m.ui.addWizard.error = msg.err
		if msg.err == nil {
			m.ui.addWizard.services = msg.services

			// If we're at the remote port step (edit mode), detect ports now
			if m.ui.addWizard.step == StepEnterRemotePort {
				// Find the service by name
				for _, svc := range msg.services {
					if svc.Name == m.ui.addWizard.resourceValue {
						m.ui.addWizard.detectedPorts = svc.Ports
						if len(m.ui.addWizard.detectedPorts) > 0 {
							m.ui.addWizard.inputMode = InputModeList
							m.ui.addWizard.cursor = 0
						} else {
							m.ui.addWizard.inputMode = InputModeText
							m.ui.addWizard.textInput = fmt.Sprintf("%d", m.ui.addWizard.remotePort)
						}
						break
					}
				}
			}
		}
	}

	return m, nil
}

func (m model) handleSelectorValidated(msg SelectorValidatedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.addWizard != nil {
		m.ui.addWizard.loading = false
		m.ui.addWizard.error = msg.err
		if msg.valid {
			m.ui.addWizard.matchingPods = msg.pods
		} else {
			m.ui.addWizard.matchingPods = nil
		}
	}

	return m, nil
}

func (m model) handlePortChecked(msg PortCheckedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.addWizard != nil {
		m.ui.addWizard.loading = false
		m.ui.addWizard.portAvailable = msg.available
		m.ui.addWizard.portCheckMsg = msg.message

		// Only proceed to confirmation if port is available
		if msg.available {
			m.ui.addWizard.step = StepConfirmation
			m.ui.addWizard.clearTextInput()
			m.ui.addWizard.cursor = 0
			m.ui.addWizard.inputMode = InputModeList
		} else {
			// Port is not available - show error and stay on local port step
			m.ui.addWizard.error = fmt.Errorf("port %d is in use, please choose another port", msg.port)
		}
	}

	return m, nil
}

func (m model) handleForwardSaved(msg ForwardSavedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.addWizard != nil {
		m.ui.addWizard.loading = false
		if msg.success {
			// Move to success step
			m.ui.addWizard.step = StepSuccess
			m.ui.addWizard.cursor = 0
			m.ui.addWizard.inputMode = InputModeList
		} else {
			m.ui.addWizard.error = msg.err
		}
	}

	return m, nil
}

func (m model) handleForwardsRemoved(msg ForwardsRemovedMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	// Delete now happens directly without wizard
	// Just ensure we're back in main view
	m.ui.viewMode = ViewModeMain
	m.ui.removeWizard = nil

	// If there was an error, it will be logged but we don't show it in UI for now
	// The config watcher will either reload (success) or keep old config (failure)

	return m, tea.ClearScreen
}

// handleBenchmarkKeys handles keyboard input in the benchmark view
func (m model) handleBenchmarkKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	state := m.ui.benchmarkState
	if state == nil {
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		// Cancel the running benchmark if active
		if state.cancelFunc != nil {
			state.cancelFunc()
		}
		// Return to main view
		m.ui.viewMode = ViewModeMain
		m.ui.benchmarkState = nil
		return m, tea.ClearScreen

	case "up", "k":
		if state.step == BenchmarkStepConfig && state.cursor > 0 {
			state.cursor--
			// Load current field value into textInput
			state.textInput = m.getBenchmarkFieldValue(state.cursor)
		}

	case "down", "j":
		if state.step == BenchmarkStepConfig && state.cursor < 3 {
			state.cursor++
			// Load current field value into textInput
			state.textInput = m.getBenchmarkFieldValue(state.cursor)
		}

	case "tab":
		// Tab also cycles through fields
		if state.step == BenchmarkStepConfig {
			state.cursor = (state.cursor + 1) % 4
			state.textInput = m.getBenchmarkFieldValue(state.cursor)
		}

	case "enter":
		switch state.step {
		case BenchmarkStepConfig:
			// Start running the benchmark
			state.step = BenchmarkStepRunning
			state.running = true
			state.progress = 0
			state.total = state.requests
			// Create progress channel with buffer for non-blocking sends
			state.progressCh = make(chan BenchmarkProgressMsg, 10)
			// Create cancellable context for the benchmark
			ctx, cancel := context.WithCancel(context.Background())
			state.cancelFunc = cancel
			// Return batch command to run benchmark and listen for progress
			return m, tea.Batch(
				runBenchmarkCmd(ctx, state.forwardID, state.localPort, state.urlPath, state.method, state.concurrency, state.requests, state.progressCh),
				listenBenchmarkProgressCmd(state.progressCh),
			)
		case BenchmarkStepResults:
			// Return to main view
			m.ui.viewMode = ViewModeMain
			m.ui.benchmarkState = nil
			return m, tea.ClearScreen
		}

	case "backspace":
		if state.step == BenchmarkStepConfig {
			if len(state.textInput) > 0 {
				state.textInput = state.textInput[:len(state.textInput)-1]
				m.applyBenchmarkTextInput()
			}
		}

	default:
		// Handle text input in config step
		if state.step == BenchmarkStepConfig && len(msg.String()) == 1 {
			char := rune(msg.String()[0])
			if char >= 32 && char < 127 {
				state.textInput += string(char)
				m.applyBenchmarkTextInput()
			}
		}
	}

	return m, nil
}

// getBenchmarkFieldValue returns the current value of the selected benchmark field
func (m model) getBenchmarkFieldValue(cursor int) string {
	state := m.ui.benchmarkState
	if state == nil {
		return ""
	}

	switch cursor {
	case 0:
		return state.urlPath
	case 1:
		return state.method
	case 2:
		return fmt.Sprintf("%d", state.concurrency)
	case 3:
		return fmt.Sprintf("%d", state.requests)
	default:
		return ""
	}
}

// applyBenchmarkTextInput applies the current text input to the selected field
func (m model) applyBenchmarkTextInput() {
	state := m.ui.benchmarkState
	if state == nil {
		return
	}

	switch state.cursor {
	case 0: // URL path
		state.urlPath = state.textInput
	case 1: // Method
		state.method = strings.ToUpper(state.textInput)
	case 2: // Concurrency
		if val, err := strconv.Atoi(state.textInput); err == nil && val > 0 {
			state.concurrency = val
			// Cap concurrency at requests
			if state.concurrency > state.requests {
				state.concurrency = state.requests
			}
		}
	case 3: // Requests
		if val, err := strconv.Atoi(state.textInput); err == nil && val > 0 {
			state.requests = val
			// Cap concurrency at requests
			if state.concurrency > state.requests {
				state.concurrency = state.requests
			}
		}
	}
}

// handleHTTPLogKeys handles keyboard input in the HTTP log view
func (m model) handleHTTPLogKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	state := m.ui.httpLogState
	if state == nil {
		return m, nil
	}

	// If filter input is active, handle text input
	if state.filterActive {
		switch msg.String() {
		case "esc":
			// Cancel filter input, clear text
			state.filterActive = false
			state.filterText = ""
			state.cursor = 0
			state.scrollOffset = 0
			return m, nil
		case "enter":
			// Confirm filter
			state.filterActive = false
			state.cursor = 0
			state.scrollOffset = 0
			return m, nil
		case "backspace":
			if len(state.filterText) > 0 {
				state.filterText = state.filterText[:len(state.filterText)-1]
			}
			return m, nil
		default:
			// Add character to filter
			if len(msg.String()) == 1 {
				char := rune(msg.String()[0])
				if char >= 32 && char < 127 {
					state.filterText += string(char)
					state.cursor = 0
					state.scrollOffset = 0
				}
			}
			return m, nil
		}
	}

	filteredEntries := state.getFilteredEntries()

	// If viewing detail, handle detail view keys
	if state.showingDetail {
		switch msg.String() {
		case "esc", "q", "enter":
			// Return to list view
			state.showingDetail = false
			state.detailScroll = 0
			state.copyMessage = ""
			return m, nil
		case "up", "k":
			if state.detailScroll > 0 {
				state.detailScroll--
			}
			return m, nil
		case "down", "j":
			state.detailScroll++
			return m, nil
		case "pgup", "ctrl+u":
			state.detailScroll -= 20
			if state.detailScroll < 0 {
				state.detailScroll = 0
			}
			return m, nil
		case "pgdown", "ctrl+d":
			state.detailScroll += 20
			return m, nil
		case "g":
			state.detailScroll = 0
			return m, nil
		case "c":
			// Copy response body to clipboard
			if state.cursor >= 0 && state.cursor < len(filteredEntries) {
				entry := filteredEntries[state.cursor]
				if entry.ResponseBody != "" {
					// Decompress if needed before copying
					body := decompressContent(entry.ResponseBody, entry.ResponseHeaders)
					if err := clipboard.Init(); err == nil {
						clipboard.Write(clipboard.FmtText, []byte(body))
						state.copyMessage = "Copied!"
						// Clear the message after 2 seconds
						return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
							return clearCopyMessageMsg{}
						})
					} else {
						state.copyMessage = "Clipboard unavailable"
						return m, tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
							return clearCopyMessageMsg{}
						})
					}
				}
			}
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc", "q":
		// Cleanup subscription before closing
		if m.ui.httpLogCleanup != nil {
			m.ui.httpLogCleanup()
			m.ui.httpLogCleanup = nil
		}
		// Return to main view
		m.ui.viewMode = ViewModeMain
		m.ui.httpLogState = nil
		return m, tea.ClearScreen

	case "enter":
		// Show detail view for selected entry
		if len(filteredEntries) > 0 && state.cursor >= 0 && state.cursor < len(filteredEntries) {
			state.showingDetail = true
			state.detailScroll = 0
		}
		return m, nil

	case "up", "k":
		if state.cursor > 0 {
			state.cursor--
			state.autoScroll = false
		}

	case "down", "j":
		if state.cursor < len(filteredEntries)-1 {
			state.cursor++
		}
		// If at bottom, enable auto-scroll
		if state.cursor >= len(filteredEntries)-1 {
			state.autoScroll = true
		}

	case "pgup", "ctrl+u":
		// Page up - move 20 entries
		state.cursor -= 20
		if state.cursor < 0 {
			state.cursor = 0
		}
		state.autoScroll = false

	case "pgdown", "ctrl+d":
		// Page down - move 20 entries
		state.cursor += 20
		if state.cursor >= len(filteredEntries) {
			state.cursor = len(filteredEntries) - 1
		}
		if state.cursor < 0 {
			state.cursor = 0
		}
		// If at bottom, enable auto-scroll
		if state.cursor >= len(filteredEntries)-1 {
			state.autoScroll = true
		}

	case "g":
		// Go to top
		state.cursor = 0
		state.scrollOffset = 0
		state.autoScroll = false

	case "G":
		// Go to bottom
		if len(filteredEntries) > 0 {
			state.cursor = len(filteredEntries) - 1
			state.autoScroll = true
		}

	case "a":
		// Toggle auto-scroll
		state.autoScroll = !state.autoScroll

	case "f":
		// Cycle filter mode (skip Text mode when cycling - use '/' for text filter)
		state.filterMode = (state.filterMode + 1) % 4
		if state.filterMode == HTTPLogFilterText {
			// Skip Text mode when using 'f' - it's only accessible via '/'
			state.filterMode = HTTPLogFilterNon200
		}
		state.cursor = 0
		state.scrollOffset = 0

	case "/":
		// Enter text filter mode
		state.filterActive = true
		state.filterText = ""

	case "c":
		// Clear all filters
		state.filterMode = HTTPLogFilterNone
		state.filterText = ""
		state.cursor = 0
		state.scrollOffset = 0
	}

	return m, nil
}

// handleHTTPLogEntry handles incoming HTTP log entries
func (m model) handleHTTPLogEntry(msg HTTPLogEntryMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.httpLogState == nil {
		return m, nil
	}

	state := m.ui.httpLogState
	entry := msg.Entry

	// If this is a response, try to find and merge with the matching request
	if entry.Direction == "response" && entry.RequestID != "" {
		// Search backwards (responses follow requests closely)
		for i := len(state.entries) - 1; i >= 0 && i >= len(state.entries)-100; i-- {
			if state.entries[i].RequestID == entry.RequestID && state.entries[i].Direction == "request" {
				// Merge response data into the existing request entry
				state.entries[i].Direction = "response"
				state.entries[i].StatusCode = entry.StatusCode
				state.entries[i].LatencyMs = entry.LatencyMs
				state.entries[i].BodySize = entry.BodySize
				state.entries[i].ResponseHeaders = entry.ResponseHeaders
				state.entries[i].ResponseBody = entry.ResponseBody
				state.entries[i].Error = entry.Error
				return m, nil
			}
		}
	}

	// For requests or unmatched responses, append as new entry
	state.entries = append(state.entries, entry)

	// Cap entries to prevent memory growth (keep last 10000 entries)
	const maxEntries = 10000
	if len(state.entries) > maxEntries {
		// Remove oldest entries
		state.entries = state.entries[len(state.entries)-maxEntries:]
		// Adjust cursor if needed
		if state.cursor >= len(state.entries) {
			state.cursor = len(state.entries) - 1
		}
	}

	// Auto-scroll to bottom if enabled
	if state.autoScroll && len(state.entries) > 0 {
		filteredEntries := state.getFilteredEntries()
		state.cursor = len(filteredEntries) - 1
		if state.cursor < 0 {
			state.cursor = 0
		}
	}

	return m, nil
}

// handleBenchmarkProgress handles progress updates during benchmark execution
func (m model) handleBenchmarkProgress(msg BenchmarkProgressMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.benchmarkState == nil || !m.ui.benchmarkState.running {
		return m, nil
	}

	state := m.ui.benchmarkState
	state.progress = msg.Completed
	state.total = msg.Total

	// Continue listening for more progress updates
	if state.progressCh != nil {
		return m, listenBenchmarkProgressCmd(state.progressCh)
	}

	return m, nil
}

// handleBenchmarkComplete handles the benchmark completion message
func (m model) handleBenchmarkComplete(msg BenchmarkCompleteMsg) (tea.Model, tea.Cmd) {
	m.ui.mu.Lock()
	defer m.ui.mu.Unlock()

	if m.ui.benchmarkState == nil {
		return m, nil
	}

	state := m.ui.benchmarkState
	state.running = false
	state.step = BenchmarkStepResults
	state.progressCh = nil // Clear progress channel since benchmark is complete

	if msg.Error != nil {
		state.error = msg.Error
		state.results = nil
	} else if msg.Results != nil {
		stats := msg.Results.CalculateStats()
		state.results = &BenchmarkResults{
			TotalRequests: msg.Results.TotalRequests,
			Successful:    msg.Results.Successful,
			Failed:        msg.Results.Failed,
			MinLatency:    float64(stats.MinLatency.Milliseconds()),
			MaxLatency:    float64(stats.MaxLatency.Milliseconds()),
			AvgLatency:    float64(stats.AvgLatency.Milliseconds()),
			P50Latency:    float64(stats.P50Latency.Milliseconds()),
			P95Latency:    float64(stats.P95Latency.Milliseconds()),
			P99Latency:    float64(stats.P99Latency.Milliseconds()),
			Throughput:    stats.Throughput,
			BytesRead:     msg.Results.BytesRead,
			StatusCodes:   msg.Results.StatusCodes,
		}
	}

	return m, nil
}
