package ui

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nvm/kportal/internal/config"
	"github.com/nvm/kportal/internal/k8s"
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

	case " ", "enter":
		m.ui.toggleSelected()

	case "n": // Enter add wizard
		m.ui.mu.Lock()
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
		m.ui.deleteConfirmCursor = 0 // Default to "No" for safety

		m.ui.mu.Unlock()
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
		m.ui.deleteConfirming = false
		m.ui.deleteConfirmID = ""
		m.ui.deleteConfirmAlias = ""
		m.ui.deleteConfirmCursor = 0 // Reset cursor
		m.ui.mu.Unlock()
		// Force a repaint by returning the model
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
			m.ui.deleteConfirming = false
			m.ui.deleteConfirmID = ""
			m.ui.deleteConfirmAlias = ""
			m.ui.mu.Unlock()
			return m, removeForwardByIDCmd(m.ui.mutator, id)
		}
		// Enter on No = cancel
		m.ui.deleteConfirming = false
		m.ui.deleteConfirmID = ""
		m.ui.deleteConfirmAlias = ""
		m.ui.deleteConfirmCursor = 0 // Reset cursor
		m.ui.mu.Unlock()
		return m, tea.ClearScreen

	case "n":
		// Quick 'n' for no
		m.ui.deleteConfirming = false
		m.ui.deleteConfirmID = ""
		m.ui.deleteConfirmAlias = ""
		m.ui.deleteConfirmCursor = 0 // Reset cursor
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
			wizard.cursor = 0
			wizard.clearTextInput()
			wizard.clearSearchFilter()
			wizard.error = nil

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
				wizard.remotePort = int(wizard.detectedPorts[wizard.cursor].Port)
				wizard.step = StepEnterLocalPort
				wizard.clearTextInput()
				wizard.inputMode = InputModeText
				wizard.error = nil
			}
		} else {
			// Text mode - manual entry
			port, err := strconv.Atoi(wizard.textInput)
			if err != nil || port < 1 || port > 65535 {
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
		if err != nil || port < 1 || port > 65535 {
			wizard.error = fmt.Errorf("invalid port number")
		} else {
			// Check port availability before proceeding
			wizard.localPort = port
			wizard.loading = true
			wizard.error = nil
			return m, checkPortCmd(port)
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
			// Cancelled
			m.ui.viewMode = ViewModeMain
			m.ui.addWizard = nil
		}

	case StepSuccess:
		if wizard.cursor == 0 {
			// Add another
			m.ui.addWizard = newAddWizardState()
			m.ui.addWizard.loading = true
			return m, loadContextsCmd(m.ui.discovery)
		} else {
			// Return to main view
			m.ui.viewMode = ViewModeMain
			m.ui.addWizard = nil
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
			// Get current context and move it to the top
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

	return m, nil
}
