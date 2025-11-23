package ui

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/term"
)

// InteractiveController handles keyboard input and selection state
type InteractiveController struct {
	mu             sync.RWMutex
	selectedIndex  int
	forwardIDs     []string        // Ordered list of forward IDs
	disabledMap    map[string]bool // Tracks which forwards are disabled
	toggleCallback func(id string, enable bool)
	enabled        bool
	oldTermState   *term.State
}

// NewInteractiveController creates a new interactive controller
func NewInteractiveController(toggleCallback func(id string, enable bool)) *InteractiveController {
	return &InteractiveController{
		selectedIndex:  0,
		forwardIDs:     make([]string, 0),
		disabledMap:    make(map[string]bool),
		toggleCallback: toggleCallback,
		enabled:        false,
	}
}

// Enable puts the terminal in raw mode for keyboard input
func (ic *InteractiveController) Enable() error {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.enabled {
		return nil
	}

	// Save current terminal state
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("failed to enable raw mode: %w", err)
	}

	ic.oldTermState = oldState
	ic.enabled = true
	return nil
}

// Disable restores the terminal to normal mode
func (ic *InteractiveController) Disable() error {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if !ic.enabled {
		return nil
	}

	if ic.oldTermState != nil {
		if err := term.Restore(int(os.Stdin.Fd()), ic.oldTermState); err != nil {
			return fmt.Errorf("failed to restore terminal: %w", err)
		}
	}

	ic.enabled = false
	return nil
}

// UpdateForwardsList updates the list of forwards for navigation
func (ic *InteractiveController) UpdateForwardsList(ids []string) {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	ic.forwardIDs = ids

	// Ensure selected index is valid
	if ic.selectedIndex >= len(ic.forwardIDs) {
		ic.selectedIndex = len(ic.forwardIDs) - 1
	}
	if ic.selectedIndex < 0 && len(ic.forwardIDs) > 0 {
		ic.selectedIndex = 0
	}
}

// MoveUp moves selection up
func (ic *InteractiveController) MoveUp() {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.selectedIndex > 0 {
		ic.selectedIndex--
	}
}

// MoveDown moves selection down
func (ic *InteractiveController) MoveDown() {
	ic.mu.Lock()
	defer ic.mu.Unlock()

	if ic.selectedIndex < len(ic.forwardIDs)-1 {
		ic.selectedIndex++
	}
}

// ToggleSelected toggles the enable/disable state of the selected forward
func (ic *InteractiveController) ToggleSelected() {
	ic.mu.Lock()
	if ic.selectedIndex < 0 || ic.selectedIndex >= len(ic.forwardIDs) {
		ic.mu.Unlock()
		return
	}

	selectedID := ic.forwardIDs[ic.selectedIndex]
	currentlyDisabled := ic.disabledMap[selectedID]
	newState := !currentlyDisabled
	ic.disabledMap[selectedID] = newState
	ic.mu.Unlock()

	// Call the toggle callback
	if ic.toggleCallback != nil {
		ic.toggleCallback(selectedID, !newState) // enable is inverse of disabled
	}
}

// GetSelectedIndex returns the current selection index
func (ic *InteractiveController) GetSelectedIndex() int {
	ic.mu.RLock()
	defer ic.mu.RUnlock()
	return ic.selectedIndex
}

// IsDisabled returns whether a forward is disabled
func (ic *InteractiveController) IsDisabled(id string) bool {
	ic.mu.RLock()
	defer ic.mu.RUnlock()
	return ic.disabledMap[id]
}

// GetSelectedID returns the ID of the currently selected forward
func (ic *InteractiveController) GetSelectedID() string {
	ic.mu.RLock()
	defer ic.mu.RUnlock()

	if ic.selectedIndex < 0 || ic.selectedIndex >= len(ic.forwardIDs) {
		return ""
	}
	return ic.forwardIDs[ic.selectedIndex]
}

// HandleKey processes keyboard input and returns true if should continue
func (ic *InteractiveController) HandleKey(b []byte) bool {
	if len(b) == 0 {
		return true
	}

	// Handle single byte keys
	if len(b) == 1 {
		switch b[0] {
		case 'q', 'Q', 3: // q, Q, or Ctrl+C
			return false
		case ' ', '\r': // Space or Enter to toggle
			ic.ToggleSelected()
			return true
		}
	}

	// Handle escape sequences (arrow keys)
	if len(b) == 3 && b[0] == 27 && b[1] == 91 {
		switch b[2] {
		case 65: // Up arrow
			ic.MoveUp()
		case 66: // Down arrow
			ic.MoveDown()
		}
	}

	return true
}
