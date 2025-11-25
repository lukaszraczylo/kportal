package ui

import (
	"strings"

	"github.com/nvm/kportal/internal/k8s"
)

// filterStrings filters a slice of strings by a search filter (case-insensitive substring match)
func filterStrings(items []string, filter string) []string {
	if filter == "" {
		return items
	}
	filtered := []string{}
	filterLower := strings.ToLower(filter)
	for _, item := range items {
		if strings.Contains(strings.ToLower(item), filterLower) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

// matchesFilter checks if a string matches the filter (case-insensitive substring match)
func matchesFilter(item, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(item), strings.ToLower(filter))
}

// ViewMode represents the current view state of the UI
type ViewMode int

const (
	ViewModeMain ViewMode = iota
	ViewModeAddWizard
	ViewModeRemoveWizard
	ViewModeBenchmark
	ViewModeHTTPLog
)

// InputMode represents whether the wizard is in list selection or text input mode
type InputMode int

const (
	InputModeList InputMode = iota
	InputModeText
)

// AddWizardStep represents the current step in the add wizard flow
type AddWizardStep int

const (
	StepSelectContext AddWizardStep = iota
	StepSelectNamespace
	StepSelectResourceType
	StepEnterResource
	StepEnterRemotePort
	StepEnterLocalPort
	StepConfirmation
	StepSuccess
)

// ConfirmationFocus represents what the user is focused on in confirmation step
type ConfirmationFocus int

const (
	FocusAlias ConfirmationFocus = iota
	FocusButtons
)

// ResourceType represents the type of Kubernetes resource to forward to
type ResourceType int

const (
	ResourceTypePodPrefix ResourceType = iota
	ResourceTypePodSelector
	ResourceTypeService
)

// String returns a human-readable name for the resource type
func (r ResourceType) String() string {
	switch r {
	case ResourceTypePodPrefix:
		return "Pod (by name prefix)"
	case ResourceTypePodSelector:
		return "Pod (by label selector)"
	case ResourceTypeService:
		return "Service"
	default:
		return "Unknown"
	}
}

// Description returns a description of the resource type
func (r ResourceType) Description() string {
	switch r {
	case ResourceTypePodPrefix:
		return "Recommended for specific pod instances"
	case ResourceTypePodSelector:
		return "Flexible, survives pod restarts automatically"
	case ResourceTypeService:
		return "Most stable, load-balanced"
	default:
		return ""
	}
}

// AddWizardState maintains the state for the add port forward wizard
type AddWizardState struct {
	step         AddWizardStep
	inputMode    InputMode
	cursor       int
	scrollOffset int // For scrolling long lists
	textInput    string
	searchFilter string // For filtering lists (contexts, namespaces, services)
	loading      bool
	error        error

	// Selections made by user
	selectedContext      string
	selectedNamespace    string
	selectedResourceType ResourceType
	resourceValue        string // pod prefix or service name
	selector             string // for pod selector type
	remotePort           int
	localPort            int
	alias                string

	// Available options (loaded asynchronously from k8s)
	contexts   []string
	namespaces []string
	pods       []k8s.PodInfo
	services   []k8s.ServiceInfo

	// Validation state
	portAvailable bool
	portCheckMsg  string
	matchingPods  []k8s.PodInfo

	// Edit mode
	isEditing  bool
	originalID string // ID of the forward being edited

	// Detected ports from resources
	detectedPorts []k8s.PortInfo

	// Confirmation focus (alias field vs buttons)
	confirmationFocus ConfirmationFocus
}

// newAddWizardState creates a new add wizard state initialized to the first step
func newAddWizardState() *AddWizardState {
	return &AddWizardState{
		step:      StepSelectContext,
		inputMode: InputModeList,
		cursor:    0,
		contexts:  []string{},
	}
}

// moveCursor moves the cursor up or down in list selection mode
func (w *AddWizardState) moveCursor(delta int) {
	if w.inputMode != InputModeList {
		return
	}

	var maxItems int

	switch w.step {
	case StepSelectContext:
		maxItems = len(w.getFilteredContexts())
	case StepSelectNamespace:
		maxItems = len(w.getFilteredNamespaces())
	case StepSelectResourceType:
		maxItems = 3 // Three resource types
	case StepEnterResource:
		if w.selectedResourceType == ResourceTypeService {
			maxItems = len(w.getFilteredServices())
		}
	case StepEnterRemotePort:
		if len(w.detectedPorts) > 0 {
			maxItems = len(w.detectedPorts) + 1 // +1 for "Manual entry" option
		}
	}

	w.cursor += delta
	if w.cursor < 0 {
		w.cursor = 0
	}
	if w.cursor >= maxItems && maxItems > 0 {
		w.cursor = maxItems - 1
	}

	// Adjust scroll offset to keep cursor visible
	// Viewport shows max 20 items at a time
	const viewportHeight = 20

	// If cursor moved below visible area, scroll down
	if w.cursor >= w.scrollOffset+viewportHeight {
		w.scrollOffset = w.cursor - viewportHeight + 1
	}

	// If cursor moved above visible area, scroll up
	if w.cursor < w.scrollOffset {
		w.scrollOffset = w.cursor
	}

	// Ensure scroll offset is valid
	if w.scrollOffset < 0 {
		w.scrollOffset = 0
	}
}

// handleTextInput handles a single character input in text mode
func (w *AddWizardState) handleTextInput(char rune) {
	// Note: Caller already checks if text input is allowed (inputMode or confirmation step)
	// so we don't need to check inputMode here

	// Handle backspace
	if char == 127 || char == 8 {
		if len(w.textInput) > 0 {
			w.textInput = w.textInput[:len(w.textInput)-1]
		}
		return
	}

	// Only allow printable characters
	if char >= 32 && char < 127 {
		w.textInput += string(char)
	}
}

// clearTextInput clears the text input field
func (w *AddWizardState) clearTextInput() {
	w.textInput = ""
}

// RemoveWizardState maintains the state for the remove port forward wizard
type RemoveWizardState struct {
	forwards      []RemovableForward
	cursor        int
	selected      map[int]bool
	confirming    bool
	confirmCursor int // 0 = Yes, 1 = No
}

// RemovableForward represents a forward that can be removed
type RemovableForward struct {
	ID        string
	Context   string
	Namespace string
	Alias     string
	Resource  string
	Selector  string
	Port      int
	LocalPort int
}

// moveCursor moves the cursor up or down
func (w *RemoveWizardState) moveCursor(delta int) {
	if w.confirming {
		// Move between Yes/No in confirmation
		w.confirmCursor += delta
		if w.confirmCursor < 0 {
			w.confirmCursor = 0
		}
		if w.confirmCursor > 1 {
			w.confirmCursor = 1
		}
	} else {
		// Move between forwards
		w.cursor += delta
		if w.cursor < 0 {
			w.cursor = 0
		}
		if w.cursor >= len(w.forwards) {
			w.cursor = len(w.forwards) - 1
		}
	}
}

// toggleSelection toggles the selection of the current forward
func (w *RemoveWizardState) toggleSelection() {
	if w.confirming {
		return
	}
	w.selected[w.cursor] = !w.selected[w.cursor]
}

// selectAll selects all forwards for removal
func (w *RemoveWizardState) selectAll() {
	if w.confirming {
		return
	}
	for i := range w.forwards {
		w.selected[i] = true
	}
}

// selectNone deselects all forwards
func (w *RemoveWizardState) selectNone() {
	if w.confirming {
		return
	}
	w.selected = make(map[int]bool)
}

// getSelectedCount returns the number of selected forwards
func (w *RemoveWizardState) getSelectedCount() int {
	count := 0
	for _, selected := range w.selected {
		if selected {
			count++
		}
	}
	return count
}

// getSelectedForwards returns a list of selected forwards
func (w *RemoveWizardState) getSelectedForwards() []RemovableForward {
	selected := make([]RemovableForward, 0)
	for i, fwd := range w.forwards {
		if w.selected[i] {
			selected = append(selected, fwd)
		}
	}
	return selected
}

// getFilteredContexts returns contexts filtered by search string
func (w *AddWizardState) getFilteredContexts() []string {
	if w.searchFilter == "" {
		return w.contexts
	}
	return filterStrings(w.contexts, w.searchFilter)
}

// getFilteredNamespaces returns namespaces filtered by search string
func (w *AddWizardState) getFilteredNamespaces() []string {
	if w.searchFilter == "" {
		return w.namespaces
	}
	return filterStrings(w.namespaces, w.searchFilter)
}

// getFilteredServices returns services filtered by search string
func (w *AddWizardState) getFilteredServices() []k8s.ServiceInfo {
	if w.searchFilter == "" {
		return w.services
	}
	filtered := []k8s.ServiceInfo{}
	for _, svc := range w.services {
		if matchesFilter(svc.Name, w.searchFilter) {
			filtered = append(filtered, svc)
		}
	}
	return filtered
}

// clearSearchFilter clears the search filter and resets cursor/scroll
func (w *AddWizardState) clearSearchFilter() {
	w.searchFilter = ""
	w.cursor = 0
	w.scrollOffset = 0
}

// resetInput clears text input, search filter, and error state.
// Use this when navigating between wizard steps.
func (w *AddWizardState) resetInput() {
	w.textInput = ""
	w.searchFilter = ""
	w.cursor = 0
	w.scrollOffset = 0
	w.error = nil
}

// BenchmarkStep represents the current step in the benchmark wizard
type BenchmarkStep int

const (
	BenchmarkStepConfig BenchmarkStep = iota
	BenchmarkStepRunning
	BenchmarkStepResults
)

// BenchmarkState maintains the state for the benchmark wizard
type BenchmarkState struct {
	step         BenchmarkStep
	forwardID    string
	forwardAlias string
	localPort    int

	// Configuration
	urlPath     string
	method      string
	concurrency int
	requests    int
	cursor      int // Current field being edited
	textInput   string

	// Running state
	running    bool
	progress   int
	total      int
	progressCh chan BenchmarkProgressMsg // Channel for progress updates

	// Results
	results *BenchmarkResults
	error   error
}

// BenchmarkResults holds benchmark results for display
type BenchmarkResults struct {
	TotalRequests int
	Successful    int
	Failed        int
	MinLatency    float64 // milliseconds
	MaxLatency    float64
	AvgLatency    float64
	P50Latency    float64
	P95Latency    float64
	P99Latency    float64
	Throughput    float64 // requests per second
	BytesRead     int64
	StatusCodes   map[int]int
}

// newBenchmarkState creates a new benchmark state for a forward
func newBenchmarkState(forwardID, alias string, localPort int) *BenchmarkState {
	return &BenchmarkState{
		step:         BenchmarkStepConfig,
		forwardID:    forwardID,
		forwardAlias: alias,
		localPort:    localPort,
		urlPath:      "/",
		method:       "GET",
		concurrency:  10,
		requests:     100,
		cursor:       0,
	}
}

// HTTPLogState maintains the state for HTTP log viewing
type HTTPLogState struct {
	forwardID    string
	forwardAlias string
	entries      []HTTPLogEntry
	cursor       int
	scrollOffset int
	autoScroll   bool
}

// HTTPLogEntry represents a single HTTP log entry for display
type HTTPLogEntry struct {
	Timestamp  string
	Direction  string
	Method     string
	Path       string
	StatusCode int
	LatencyMs  int64
	BodySize   int
}

// newHTTPLogState creates a new HTTP log viewing state
func newHTTPLogState(forwardID, alias string) *HTTPLogState {
	return &HTTPLogState{
		forwardID:    forwardID,
		forwardAlias: alias,
		entries:      make([]HTTPLogEntry, 0),
		autoScroll:   true,
	}
}
