package ui

import (
	"strings"
	"testing"

	"github.com/lukaszraczylo/kportal/internal/benchmark"
	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newModelWithWizard returns a model with an initialised AddWizardState at the given step.
func newModelWithWizard(step AddWizardStep) model {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = step
	w.selectedContext = "my-ctx"
	w.selectedNamespace = "default"
	w.resourceValue = "my-app"
	w.remotePort = 8080
	w.localPort = 8081
	w.contexts = []string{"my-ctx", "other-ctx"}
	w.namespaces = []string{"default", "kube-system"}
	ui.addWizard = w
	ui.mu.Unlock()
	return model{ui: ui, termWidth: 120, termHeight: 40}
}

// ----- renderAddWizard (dispatch) -----------------------------------------

func TestRenderAddWizard_NilWizard(t *testing.T) {
	m := newTestModel()
	result := m.renderAddWizard()
	assert.Empty(t, result)
}

// ----- renderSelectContext ------------------------------------------------

func TestRenderSelectContext_Loading(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.loading = true
	result := m.renderSelectContext()
	assert.Contains(t, result, "Loading")
}

func TestRenderSelectContext_Error(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.error = assert.AnError
	result := m.renderSelectContext()
	assert.Contains(t, result, "Error")
}

func TestRenderSelectContext_NoContexts(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.contexts = []string{}
	result := m.renderSelectContext()
	assert.Contains(t, result, "No contexts")
}

func TestRenderSelectContext_WithContexts(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	result := m.renderSelectContext()
	assert.Contains(t, result, "my-ctx")
	assert.Contains(t, result, "(current)")
}

func TestRenderSelectContext_WithSearchFilter(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.searchFilter = "my"
	result := m.renderSelectContext()
	assert.Contains(t, result, "Filter: ")
}

func TestRenderSelectContext_FilterNoMatch(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.searchFilter = "zzz"
	result := m.renderSelectContext()
	assert.Contains(t, result, "No matching")
}

func TestRenderSelectContext_ScrollIndicators(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	// Create many contexts to trigger scroll.
	many := make([]string, 30)
	for i := range many {
		many[i] = "ctx"
	}
	m.ui.addWizard.contexts = many
	m.ui.addWizard.scrollOffset = 5
	result := m.renderSelectContext()
	assert.Contains(t, result, "↑ More above ↑")
	assert.Contains(t, result, "↓ More below ↓")
}

// ----- renderSelectNamespace ---------------------------------------------

func TestRenderSelectNamespace_Loading(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.ui.addWizard.loading = true
	result := m.renderSelectNamespace()
	assert.Contains(t, result, "Loading namespaces")
}

func TestRenderSelectNamespace_Error(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.ui.addWizard.error = assert.AnError
	result := m.renderSelectNamespace()
	assert.Contains(t, result, "Error")
	assert.Contains(t, result, "unreachable")
}

func TestRenderSelectNamespace_NoNamespaces(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.ui.addWizard.namespaces = []string{}
	result := m.renderSelectNamespace()
	assert.Contains(t, result, "No namespaces")
}

func TestRenderSelectNamespace_WithNamespaces(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	result := m.renderSelectNamespace()
	assert.Contains(t, result, "default")
}

func TestRenderSelectNamespace_FilterNoMatch(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.ui.addWizard.searchFilter = "zzz"
	result := m.renderSelectNamespace()
	assert.Contains(t, result, "No matching")
}

func TestRenderSelectNamespace_WithSearchFilter(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.ui.addWizard.searchFilter = "def"
	result := m.renderSelectNamespace()
	assert.Contains(t, result, "Filter: ")
}

// ----- renderSelectResourceType ------------------------------------------

func TestRenderSelectResourceType_AllTypes(t *testing.T) {
	m := newModelWithWizard(StepSelectResourceType)
	result := m.renderSelectResourceType()
	assert.Contains(t, result, "Pod (by name prefix)")
	assert.Contains(t, result, "Pod (by label selector)")
	assert.Contains(t, result, "Service")
}

func TestRenderSelectResourceType_CursorHighlight(t *testing.T) {
	m := newModelWithWizard(StepSelectResourceType)
	m.ui.addWizard.cursor = 1
	result := m.renderSelectResourceType()
	// Description of second type should be shown.
	assert.Contains(t, result, "Flexible")
}

// ----- renderEnterResource -----------------------------------------------

func TestRenderEnterResource_PodPrefix_Loading(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
	m.ui.addWizard.loading = true
	result := m.renderEnterResource()
	assert.Contains(t, result, "Loading pods")
}

func TestRenderEnterResource_PodPrefix_WithPods(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
	m.ui.addWizard.pods = []k8s.PodInfo{{Name: "my-app-abc"}, {Name: "my-app-def"}}
	m.ui.addWizard.textInput = "my-app"
	result := m.renderEnterResource()
	assert.Contains(t, result, "my-app-abc")
	assert.Contains(t, result, "Matches")
}

func TestRenderEnterResource_PodPrefix_NoMatchingPods(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
	m.ui.addWizard.pods = []k8s.PodInfo{{Name: "other-pod"}}
	m.ui.addWizard.textInput = "nomatch"
	result := m.renderEnterResource()
	assert.Contains(t, result, "No matching pods")
}

func TestRenderEnterResource_PodPrefix_MorePodsIndicator(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
	pods := make([]k8s.PodInfo, 10)
	for i := range pods {
		pods[i] = k8s.PodInfo{Name: "my-pod"}
	}
	m.ui.addWizard.pods = pods
	result := m.renderEnterResource()
	assert.Contains(t, result, "more")
}

func TestRenderEnterResource_PodSelector_WithMatchingPods(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodSelector
	m.ui.addWizard.matchingPods = []k8s.PodInfo{{Name: "pod-1"}, {Name: "pod-2"}, {Name: "pod-3"}, {Name: "pod-4"}}
	result := m.renderEnterResource()
	assert.Contains(t, result, "Found 4 matching")
}

func TestRenderEnterResource_PodSelector_Error(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodSelector
	m.ui.addWizard.error = assert.AnError
	result := m.renderEnterResource()
	assert.Contains(t, result, "Invalid selector")
}

func TestRenderEnterResource_PodSelector_Loading(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodSelector
	m.ui.addWizard.loading = true
	result := m.renderEnterResource()
	assert.Contains(t, result, "Validating")
}

func TestRenderEnterResource_Service_Loading(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypeService
	m.ui.addWizard.loading = true
	result := m.renderEnterResource()
	assert.Contains(t, result, "Loading services")
}

func TestRenderEnterResource_Service_Empty(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypeService
	result := m.renderEnterResource()
	assert.Contains(t, result, "No services")
}

func TestRenderEnterResource_Service_WithServices(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypeService
	m.ui.addWizard.services = []k8s.ServiceInfo{{Name: "api-svc"}, {Name: "db-svc"}}
	result := m.renderEnterResource()
	assert.Contains(t, result, "api-svc")
}

func TestRenderEnterResource_Service_FilterNoMatch(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypeService
	m.ui.addWizard.services = []k8s.ServiceInfo{{Name: "api-svc"}}
	m.ui.addWizard.searchFilter = "zzz"
	result := m.renderEnterResource()
	assert.Contains(t, result, "No matching services")
}

func TestRenderEnterResource_Service_WithSearchFilter(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypeService
	m.ui.addWizard.services = []k8s.ServiceInfo{{Name: "api-svc"}}
	m.ui.addWizard.searchFilter = "api"
	result := m.renderEnterResource()
	assert.Contains(t, result, "Filter: ")
}

// ----- renderEnterRemotePort ---------------------------------------------

func TestRenderEnterRemotePort_TextMode(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	result := m.renderEnterRemotePort()
	assert.Contains(t, result, "Remote port:")
}

func TestRenderEnterRemotePort_TextMode_Error(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.error = assert.AnError
	result := m.renderEnterRemotePort()
	assert.Contains(t, result, "✗")
}

func TestRenderEnterRemotePort_TextMode_WithInput(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.textInput = "8080"
	result := m.renderEnterRemotePort()
	assert.Contains(t, result, "Press Enter")
}

func TestRenderEnterRemotePort_ListMode_WithPorts(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeList
	m.ui.addWizard.detectedPorts = []k8s.PortInfo{
		{Port: 80, Name: "http"},
		{Port: 8080, TargetPort: 9090, Name: "grpc"},
	}
	result := m.renderEnterRemotePort()
	assert.Contains(t, result, "Manual entry")
	assert.Contains(t, result, "80")
}

func TestRenderEnterRemotePort_ListMode_ScrollIndicators(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeList
	ports := make([]k8s.PortInfo, 25)
	for i := range ports {
		ports[i] = k8s.PortInfo{Port: int32(8000 + i)} //nolint:gosec // safe: i ≤ 24, no overflow
	}
	m.ui.addWizard.detectedPorts = ports
	m.ui.addWizard.scrollOffset = 3
	result := m.renderEnterRemotePort()
	assert.Contains(t, result, "↑ More above ↑")
}

func TestRenderEnterRemotePort_TextMode_DetectedPortsShown(t *testing.T) {
	// Text mode but with detected ports — shows them as reference.
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.detectedPorts = []k8s.PortInfo{{Port: 80}}
	result := m.renderEnterRemotePort()
	assert.Contains(t, result, "Detected ports")
}

func TestRenderEnterRemotePort_Selector(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.selector = "app=my-app"
	result := m.renderEnterRemotePort()
	assert.Contains(t, result, "app=my-app")
}

// ----- renderEnterLocalPort ----------------------------------------------

func TestRenderEnterLocalPort_Loading(t *testing.T) {
	m := newModelWithWizard(StepEnterLocalPort)
	m.ui.addWizard.loading = true
	result := m.renderEnterLocalPort()
	assert.Contains(t, result, "Checking availability")
}

func TestRenderEnterLocalPort_Error(t *testing.T) {
	m := newModelWithWizard(StepEnterLocalPort)
	m.ui.addWizard.error = assert.AnError
	result := m.renderEnterLocalPort()
	assert.Contains(t, result, "✗")
}

func TestRenderEnterLocalPort_PortAvailable(t *testing.T) {
	m := newModelWithWizard(StepEnterLocalPort)
	m.ui.addWizard.portAvailable = true
	m.ui.addWizard.portCheckMsg = "✓ Port 8081 available"
	result := m.renderEnterLocalPort()
	assert.Contains(t, result, "✓")
}

func TestRenderEnterLocalPort_PortUnavailable(t *testing.T) {
	m := newModelWithWizard(StepEnterLocalPort)
	m.ui.addWizard.portAvailable = false
	m.ui.addWizard.portCheckMsg = "✗ Port 8081 in use"
	result := m.renderEnterLocalPort()
	assert.Contains(t, result, "in use")
}

func TestRenderEnterLocalPort_WithInput(t *testing.T) {
	m := newModelWithWizard(StepEnterLocalPort)
	m.ui.addWizard.textInput = "8081"
	result := m.renderEnterLocalPort()
	assert.Contains(t, result, "Press Enter")
}

func TestRenderEnterLocalPort_WithSelector(t *testing.T) {
	m := newModelWithWizard(StepEnterLocalPort)
	m.ui.addWizard.selector = "app=db"
	result := m.renderEnterLocalPort()
	assert.Contains(t, result, "app=db")
}

// ----- renderConfirmation ------------------------------------------------

func TestRenderConfirmation_DefaultView(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusButtons
	m.ui.addWizard.cursor = 0
	result := m.renderConfirmation()
	assert.Contains(t, result, "Add to .kportal.yaml")
	assert.Contains(t, result, "my-ctx")
	assert.Contains(t, result, "default")
}

func TestRenderConfirmation_AliasFocus(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusAlias
	result := m.renderConfirmation()
	assert.Contains(t, result, "█") // cursor in alias field
}

func TestRenderConfirmation_ButtonsCancelSelected(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusButtons
	m.ui.addWizard.cursor = 1 // Cancel
	result := m.renderConfirmation()
	assert.Contains(t, result, "Cancel")
}

func TestRenderConfirmation_HTTPLogEnabled(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.httpLog = true
	result := m.renderConfirmation()
	assert.Contains(t, result, "[x] enabled")
}

func TestRenderConfirmation_HTTPLogDisabled(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.httpLog = false
	result := m.renderConfirmation()
	assert.Contains(t, result, "[ ] disabled")
}

func TestRenderConfirmation_SelectorResource(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.selector = "app=myapp"
	result := m.renderConfirmation()
	assert.Contains(t, result, "selector")
}

func TestRenderConfirmation_ServiceResource(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.selectedResourceType = ResourceTypeService
	result := m.renderConfirmation()
	assert.Contains(t, result, "service/")
}

func TestRenderConfirmation_PodPrefixResource(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
	result := m.renderConfirmation()
	assert.Contains(t, result, "pod/")
}

// ----- renderSuccess -----------------------------------------------------

func TestRenderSuccess_Success(t *testing.T) {
	m := newModelWithWizard(StepSuccess)
	m.ui.addWizard.alias = "my-alias"
	result := m.renderSuccess()
	assert.Contains(t, result, "Success")
	assert.Contains(t, result, "Add another")
	assert.Contains(t, result, "Return to main view")
}

func TestRenderSuccess_Error(t *testing.T) {
	m := newModelWithWizard(StepSuccess)
	m.ui.addWizard.error = assert.AnError
	result := m.renderSuccess()
	assert.Contains(t, result, "Error")
}

func TestRenderSuccess_CursorOnReturn(t *testing.T) {
	m := newModelWithWizard(StepSuccess)
	m.ui.addWizard.cursor = 1
	result := m.renderSuccess()
	assert.Contains(t, result, "▸ Return to main view")
}

func TestRenderSuccess_WithAlias(t *testing.T) {
	m := newModelWithWizard(StepSuccess)
	m.ui.addWizard.alias = "my-svc"
	result := m.renderSuccess()
	assert.Contains(t, result, "my-svc")
}

// ----- renderAddWizard dispatch ------------------------------------------

func TestRenderAddWizard_UnknownStep(t *testing.T) {
	m := newModelWithWizard(AddWizardStep(99))
	result := m.renderAddWizard()
	assert.Contains(t, result, "Unknown step")
}

// ----- renderRemoveWizard ------------------------------------------------

func TestRenderRemoveWizard_NilWizard(t *testing.T) {
	m := newTestModel()
	result := m.renderRemoveWizard()
	assert.Empty(t, result)
}

func TestRenderRemoveSelection_ShowsForwards(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{
			{ID: "f1", Alias: "alpha", Port: 80, LocalPort: 8080, Context: "ctx", Namespace: "ns", Resource: "pod/alpha"},
			{ID: "f2", Alias: "beta", Port: 81, LocalPort: 8081, Context: "ctx", Namespace: "ns", Resource: "pod/beta"},
		},
		selected: map[int]bool{0: true},
		cursor:   0,
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderRemoveSelection()
	assert.Contains(t, result, "alpha")
	assert.Contains(t, result, "beta")
	assert.Contains(t, result, "[✓]")
	assert.Contains(t, result, "1 of 2 selected")
}

func TestRenderRemoveConfirmation_Shows(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{
			{ID: "f1", Alias: "alpha", Port: 80, LocalPort: 8080, Context: "ctx", Namespace: "ns", Resource: "pod/alpha"},
		},
		selected:   map[int]bool{0: true},
		confirming: true,
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderRemoveConfirmation()
	assert.Contains(t, result, "alpha")
	assert.Contains(t, result, "Yes, remove")
	assert.Contains(t, result, "cannot be undone")
}

func TestRenderRemoveConfirmation_CancelSelected(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards:      []RemovableForward{{ID: "f1", Alias: "alpha"}},
		selected:      map[int]bool{0: true},
		confirming:    true,
		confirmCursor: 1,
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderRemoveConfirmation()
	assert.Contains(t, result, "▸ Cancel")
}

// ----- renderBenchmark ---------------------------------------------------

func TestRenderBenchmark_NilState(t *testing.T) {
	m := newTestModel()
	result := m.renderBenchmark()
	assert.Empty(t, result)
}

func TestRenderBenchmarkConfig(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("fwd-id", "my-svc", 8080)
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderBenchmarkConfig()
	assert.Contains(t, result, "URL Path")
	assert.Contains(t, result, "Method")
	assert.Contains(t, result, "Concurrency")
	assert.Contains(t, result, "Requests")
	assert.Contains(t, result, "my-svc")
}

func TestRenderBenchmarkRunning(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	state := newBenchmarkState("fwd-id", "my-svc", 8080)
	state.step = BenchmarkStepRunning
	state.running = true
	state.progress = 30
	state.total = 100
	ui.benchmarkState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderBenchmarkRunning()
	assert.Contains(t, result, "Running")
	assert.Contains(t, result, "30")
	assert.Contains(t, result, "100")
}

func TestRenderBenchmarkResults_WithError(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	state := newBenchmarkState("fwd-id", "my-svc", 8080)
	state.step = BenchmarkStepResults
	state.error = assert.AnError
	ui.benchmarkState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderBenchmarkResults()
	assert.Contains(t, result, "Error")
}

func TestRenderBenchmarkResults_NilResults(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	state := newBenchmarkState("fwd-id", "my-svc", 8080)
	state.step = BenchmarkStepResults
	ui.benchmarkState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderBenchmarkResults()
	assert.Contains(t, result, "No results")
}

func TestRenderBenchmarkResults_WithResults(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	state := newBenchmarkState("fwd-id", "my-svc", 8080)
	state.step = BenchmarkStepResults
	state.results = &BenchmarkResults{
		TotalRequests: 100,
		Successful:    95,
		Failed:        5,
		MinLatency:    1.0,
		MaxLatency:    50.0,
		AvgLatency:    10.0,
		P50Latency:    8.0,
		P95Latency:    40.0,
		P99Latency:    48.0,
		Throughput:    50.0,
		BytesRead:     10240,
		StatusCodes:   map[int]int{200: 95, 500: 5},
	}
	ui.benchmarkState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderBenchmarkResults()
	assert.Contains(t, result, "Total Requests:  100")
	assert.Contains(t, result, "Successful:")
	assert.Contains(t, result, "Failed:")
	assert.Contains(t, result, "P95")
	assert.Contains(t, result, "Status Codes")
}

func TestRenderBenchmark_Dispatch(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	state := newBenchmarkState("fwd-id", "my-svc", 8080)
	state.step = BenchmarkStepConfig
	ui.benchmarkState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderBenchmark()
	require.NotEmpty(t, result)
	assert.Contains(t, result, "URL Path")
}

// ----- renderHTTPLog -----------------------------------------------------

func TestRenderHTTPLog_NilState(t *testing.T) {
	m := newTestModel()
	result := m.renderHTTPLog()
	assert.Empty(t, result)
}

func TestRenderHTTPLog_NoEntries(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "my-svc")
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderHTTPLog()
	assert.Contains(t, result, "No HTTP traffic")
}

func TestRenderHTTPLog_WithEntries(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	state := newHTTPLogState("fwd-id", "my-svc")
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/test", StatusCode: 200, Timestamp: "12:00:00"},
		{Method: "POST", Path: "/api/create", StatusCode: 500, Timestamp: "12:00:01"},
		{Method: "PUT", Path: "/api/update", StatusCode: 404, Timestamp: "12:00:02"},
	}
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderHTTPLog()
	assert.Contains(t, result, "GET")
	assert.Contains(t, result, "/api/test")
}

func TestRenderHTTPLog_FilterActive(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	state := newHTTPLogState("fwd-id", "my-svc")
	state.filterActive = true
	state.filterText = "test"
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderHTTPLog()
	assert.Contains(t, result, "Search: ")
}

func TestRenderHTTPLog_FilteredEntries(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	state := newHTTPLogState("fwd-id", "my-svc")
	state.entries = []HTTPLogEntry{
		{Method: "GET", Path: "/api/test", StatusCode: 200},
	}
	state.filterMode = HTTPLogFilterNon200
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderHTTPLog()
	// With Non200 filter active and only 200s, no entries match.
	assert.Contains(t, result, "No entries match filter")
}

func TestRenderHTTPLog_AutoScrollIndicator(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	state := newHTTPLogState("fwd-id", "my-svc")
	state.autoScroll = true
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderHTTPLog()
	assert.Contains(t, result, "Auto-scroll")
}

func TestRenderHTTPLog_FilterIndicator(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	state := newHTTPLogState("fwd-id", "my-svc")
	state.filterMode = HTTPLogFilterErrors
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderHTTPLog()
	assert.Contains(t, result, "Filter:")
}

func TestRenderHTTPLog_DetailView(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	state := newHTTPLogState("fwd-id", "my-svc")
	state.entries = []HTTPLogEntry{
		{
			Method:     "GET",
			Path:       "/detail",
			StatusCode: 200,
			LatencyMs:  100,
			RequestHeaders: map[string]string{
				"Content-Type": "application/json",
			},
			ResponseHeaders: map[string]string{
				"X-Request-ID": "abc123",
			},
			RequestBody:  `{"key": "value"}`,
			ResponseBody: `{"result": "ok"}`,
		},
	}
	state.cursor = 0
	state.showingDetail = true
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderHTTPLog()
	assert.Contains(t, result, "Request Detail")
}

// ----- renderHTTPLogDetail -----------------------------------------------

func TestRenderHTTPLogDetail_HighLatency(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	state := newHTTPLogState("fwd-id", "my-svc")
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	entry := HTTPLogEntry{
		Method:    "GET",
		Path:      "/slow",
		LatencyMs: 2500, // >1000ms → should display as seconds
	}
	result := m.renderHTTPLogDetail(entry, 120, 40)
	assert.Contains(t, result, "2.50s")
}

func TestRenderHTTPLogDetail_Status500(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	state := newHTTPLogState("fwd-id", "my-svc")
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	entry := HTTPLogEntry{Method: "POST", Path: "/fail", StatusCode: 500}
	result := m.renderHTTPLogDetail(entry, 120, 40)
	assert.Contains(t, result, "500")
}

func TestRenderHTTPLogDetail_Status400(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	state := newHTTPLogState("fwd-id", "my-svc")
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	entry := HTTPLogEntry{Method: "GET", Path: "/notfound", StatusCode: 404}
	result := m.renderHTTPLogDetail(entry, 120, 40)
	assert.Contains(t, result, "404")
}

func TestRenderHTTPLogDetail_WithError(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	state := newHTTPLogState("fwd-id", "my-svc")
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	entry := HTTPLogEntry{Method: "GET", Path: "/err", Error: "connection reset"}
	result := m.renderHTTPLogDetail(entry, 120, 40)
	assert.Contains(t, result, "connection reset")
}

func TestRenderHTTPLogDetail_BinaryRequestBody(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	state := newHTTPLogState("fwd-id", "my-svc")
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	entry := HTTPLogEntry{
		Method:      "POST",
		Path:        "/upload",
		RequestBody: "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a",
		RequestHeaders: map[string]string{
			"Content-Type": "application/octet-stream",
		},
	}
	result := m.renderHTTPLogDetail(entry, 120, 40)
	assert.Contains(t, result, "Binary data")
}

func TestRenderHTTPLogDetail_CopyMessage(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	state := newHTTPLogState("fwd-id", "my-svc")
	state.copyMessage = "Copied!"
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	entry := HTTPLogEntry{Method: "GET", Path: "/x"}
	result := m.renderHTTPLogDetail(entry, 120, 40)
	assert.Contains(t, result, "Copied!")
}

func TestRenderHTTPLogDetail_ScrollIndicator(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	state := newHTTPLogState("fwd-id", "my-svc")
	// Force a long response body to make content exceed viewport.
	bodyLines := strings.Repeat("a line of body content\n", 100)
	state.entries = []HTTPLogEntry{{
		Method:       "GET",
		Path:         "/long",
		StatusCode:   200,
		ResponseBody: bodyLines,
		ResponseHeaders: map[string]string{
			"Content-Type": "text/plain",
		},
	}}
	state.detailScroll = 5
	ui.httpLogState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 10}

	entry := state.entries[0]
	result := m.renderHTTPLogDetail(entry, 120, 10)
	// Should contain a scroll percentage indicator.
	assert.Contains(t, result, "%")
}

// ----- renderMainView helpers -------------------------------------------

func TestRenderMainView_EmptyForwards(t *testing.T) {
	m := newTestModel()
	result := m.renderMainView()
	assert.Contains(t, result, "No forwards configured")
}

func TestRenderMainView_WithForwards(t *testing.T) {
	m := newTestModelWithForward()
	result := m.renderMainView()
	assert.Contains(t, result, "my-app")
	assert.Contains(t, result, "CONTEXT")
	assert.Contains(t, result, "STATUS")
}

func TestRenderMainView_WithErrors(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	fwd := &config.Forward{Resource: "pod/my-app", Port: 8080, LocalPort: 8080, Alias: "my-app"}
	ui.AddForward("id-1", fwd)
	ui.SetError("id-1", "connection refused")
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderMainView()
	assert.Contains(t, result, "Errors:")
	assert.Contains(t, result, "connection refused")
}

func TestRenderMainView_UpdateAvailable(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.SetUpdateAvailable("2.0.0", "http://example.com")
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderMainView()
	assert.Contains(t, result, "Update available")
	assert.Contains(t, result, "2.0.0")
}

func TestRenderDeleteConfirmation_YesSelected(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmID = "id-1"
	ui.deleteConfirmAlias = "my-service"
	ui.deleteConfirmCursor = 0 // Yes
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderDeleteConfirmation()
	assert.Contains(t, result, "my-service")
	assert.Contains(t, result, "Yes")
	assert.Contains(t, result, "No")
}

func TestRenderDeleteConfirmation_NoSelected(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmAlias = "my-service"
	ui.deleteConfirmCursor = 1 // No
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.renderDeleteConfirmation()
	assert.Contains(t, result, "Yes")
	assert.Contains(t, result, "No")
}

// ----- View() dispatch --------------------------------------------------

func TestModelView_MainViewDefault(t *testing.T) {
	m := newTestModel()
	result := m.View()
	assert.NotEmpty(t, result)
}

func TestModelView_DeleteConfirmationOverlay(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	fwd := &config.Forward{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "app"}
	ui.AddForward("id-1", fwd)
	ui.mu.Lock()
	ui.deleteConfirming = true
	ui.deleteConfirmAlias = "app"
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.View()
	assert.Contains(t, result, "Delete Port Forward")
}

func TestModelView_AddWizardOverlay(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.View()
	assert.NotEmpty(t, result)
}

func TestModelView_RemoveWizardOverlay(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{{ID: "f1", Alias: "alpha"}},
		selected: map[int]bool{},
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.View()
	assert.Contains(t, result, "alpha")
}

func TestModelView_BenchmarkOverlay(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("fwd-id", "my-svc", 8080)
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.View()
	assert.NotEmpty(t, result)
}

func TestModelView_HTTPLogFullScreen(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "my-svc")
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	result := m.View()
	assert.Contains(t, result, "HTTP Traffic Log")
}

func TestModelView_ZeroTermSize(t *testing.T) {
	m := newTestModel()
	m.termWidth = 0
	m.termHeight = 0
	// Must not panic even with zero term dims.
	result := m.View()
	assert.NotEmpty(t, result)
}

// ----- decompressContent -------------------------------------------------

func TestDecompressContent_NoEncoding(t *testing.T) {
	content := "hello world"
	result := decompressContent(content, map[string]string{})
	assert.Equal(t, content, result)
}

func TestDecompressContent_UnknownEncoding(t *testing.T) {
	content := "hello world"
	result := decompressContent(content, map[string]string{"Content-Encoding": "br"})
	assert.Equal(t, content, result)
}

func TestDecompressContent_InvalidGzip(t *testing.T) {
	content := "not-gzip-data"
	result := decompressContent(content, map[string]string{"Content-Encoding": "gzip"})
	assert.Equal(t, content, result)
}

func TestDecompressContent_InvalidDeflate(t *testing.T) {
	content := "not-deflate-data"
	result := decompressContent(content, map[string]string{"Content-Encoding": "deflate"})
	// Should return original since invalid deflate data.
	assert.NotEmpty(t, result)
}

// ----- isBinaryContent ---------------------------------------------------

func TestIsBinaryContent_ImageContentType(t *testing.T) {
	result := isBinaryContent("data", map[string]string{"Content-Type": "image/png"})
	assert.True(t, result)
}

func TestIsBinaryContent_AudioContentType(t *testing.T) {
	result := isBinaryContent("data", map[string]string{"Content-Type": "audio/mp3"})
	assert.True(t, result)
}

func TestIsBinaryContent_OctetStream(t *testing.T) {
	result := isBinaryContent("data", map[string]string{"Content-Type": "application/octet-stream"})
	assert.True(t, result)
}

func TestIsBinaryContent_TextContent(t *testing.T) {
	result := isBinaryContent("hello world", map[string]string{"Content-Type": "text/plain"})
	assert.False(t, result)
}

func TestIsBinaryContent_BinaryBytes(t *testing.T) {
	// Many non-printable bytes should be detected as binary.
	binary := strings.Repeat("\x00\x01\x02", 50)
	result := isBinaryContent(binary, map[string]string{})
	assert.True(t, result)
}

func TestIsBinaryContent_EmptyContent(t *testing.T) {
	result := isBinaryContent("", map[string]string{})
	assert.False(t, result)
}

// ----- formatJSONContent ------------------------------------------------

func TestFormatJSONContent_ValidJSON(t *testing.T) {
	json := `{"key":"value","num":42}`
	result := formatJSONContent(json, map[string]string{"Content-Type": "application/json"})
	assert.Contains(t, result, "key")
	assert.Contains(t, result, "value")
}

func TestFormatJSONContent_NotJSON(t *testing.T) {
	text := "plain text content"
	result := formatJSONContent(text, map[string]string{"Content-Type": "text/plain"})
	assert.Equal(t, text, result)
}

func TestFormatJSONContent_InvalidJSON(t *testing.T) {
	bad := `{"key": invalid}`
	result := formatJSONContent(bad, map[string]string{"Content-Type": "application/json"})
	assert.Equal(t, bad, result)
}

func TestFormatJSONContent_AutoDetect(t *testing.T) {
	json := `{"auto": true}`
	result := formatJSONContent(json, map[string]string{})
	assert.Contains(t, result, "auto")
}

func TestFormatJSONContent_ArrayJSON(t *testing.T) {
	json := `[1, 2, 3]`
	result := formatJSONContent(json, map[string]string{})
	assert.Contains(t, result, "1")
}

func TestFormatJSONContent_JSONPlusType(t *testing.T) {
	json := `{"ok":true}`
	result := formatJSONContent(json, map[string]string{"Content-Type": "application/vnd.api+json"})
	assert.Contains(t, result, "ok")
}

// ----- colorizeJSON / colorizeLine / colorizeValue / isJSONNumber --------

func TestColorizeJSON_RoundTrip(t *testing.T) {
	json := `{
  "key": "value",
  "num": 42,
  "flag": true,
  "nothing": null,
  "arr": []
}`
	result := colorizeJSON(json)
	assert.Contains(t, result, "key")
	assert.Contains(t, result, "value")
	assert.Contains(t, result, "42")
}

func TestColorizeValue_Null(t *testing.T) {
	result := colorizeValue("null")
	assert.Contains(t, result, "null")
}

func TestColorizeValue_Bool(t *testing.T) {
	assert.Contains(t, colorizeValue("true"), "true")
	assert.Contains(t, colorizeValue("false"), "false")
}

func TestColorizeValue_String(t *testing.T) {
	result := colorizeValue(`"hello"`)
	assert.Contains(t, result, "hello")
}

func TestColorizeValue_Number(t *testing.T) {
	result := colorizeValue("42")
	assert.Contains(t, result, "42")
}

func TestColorizeValue_Structural(t *testing.T) {
	for _, v := range []string{"{", "}", "[", "]", "{}", "[]"} {
		assert.Equal(t, v, colorizeValue(v))
	}
}

func TestColorizeValue_Unknown(t *testing.T) {
	result := colorizeValue("weird_value")
	assert.Equal(t, "weird_value", result)
}

func TestColorizeValue_Empty(t *testing.T) {
	result := colorizeValue("")
	assert.Equal(t, "", result)
}

func TestIsJSONNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"42", true},
		{"3.14", true},
		{"-5", true},
		{"1e10", true},
		{"1.5E-3", true},
		{"", false},
		{"-", false},
		{"abc", false},
		{"1.2.3", false},
		{"1e", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, isJSONNumber(tt.input))
		})
	}
}

// ----- renderStyles helpers ----------------------------------------------

func TestRenderProgress(t *testing.T) {
	result := renderProgress(3, 7)
	assert.Contains(t, result, "3")
	assert.Contains(t, result, "7")
}

func TestRenderHeader(t *testing.T) {
	result := renderHeader("My Title", "Step 1/5")
	assert.Contains(t, result, "My Title")
	assert.Contains(t, result, "Step 1/5")
}

func TestRenderHeader_NoProgress(t *testing.T) {
	result := renderHeader("Title Only", "")
	assert.Contains(t, result, "Title Only")
}

func TestRenderBreadcrumb(t *testing.T) {
	result := renderBreadcrumb("ctx", "ns")
	assert.Contains(t, result, "ctx")
	assert.Contains(t, result, "ns")
}

func TestRenderList(t *testing.T) {
	items := []string{"alpha", "beta", "gamma"}
	result := renderList(items, 1, "  ", 0)
	assert.Contains(t, result, "alpha")
	assert.Contains(t, result, "beta")
	assert.Contains(t, result, "gamma")
	assert.Contains(t, result, "▸") // cursor on item 1
}

func TestRenderList_ScrollIndicators(t *testing.T) {
	// Need offset > 0 for the "above" indicator and offset+ViewportHeight < totalItems for the "below" indicator.
	// ViewportHeight = 20, so with offset=5 we need totalItems > 25.
	items := make([]string, 27)
	for i := range items {
		items[i] = "item"
	}
	// cursor=22 is within the visible window [5,25), scrollOffset=5.
	// end = 5+20 = 25 < 27, so "↓ More below ↓" is rendered.
	result := renderList(items, 22, "  ", 5)
	assert.Contains(t, result, "↑ More above ↑")
	assert.Contains(t, result, "↓ More below ↓")
}

func TestRenderTextInput_Valid(t *testing.T) {
	result := renderTextInput("Label: ", "hello", true)
	assert.Contains(t, result, "Label: ")
	assert.Contains(t, result, "hello")
	assert.Contains(t, result, "█")
}

func TestRenderTextInput_Invalid(t *testing.T) {
	result := renderTextInput("Port: ", "abc", false)
	assert.Contains(t, result, "abc")
}

func TestWizardHelpWidth_Zero(t *testing.T) {
	w := wizardHelpWidth(0)
	assert.Greater(t, w, 0)
}

func TestWizardHelpWidth_Narrow(t *testing.T) {
	w := wizardHelpWidth(30)
	assert.LessOrEqual(t, w, 30)
}

func TestWizardHelpWidth_Normal(t *testing.T) {
	w := wizardHelpWidth(120)
	assert.Equal(t, 70, w) // capped at 70
}

// ----- overlayContent ---------------------------------------------------

func TestOverlayContent_BasicPlacement(t *testing.T) {
	bg := strings.Repeat("background\n", 20)
	overlay := "MODAL"
	result := overlayContent(bg, overlay, 80, 20)
	assert.Contains(t, result, "MODAL")
}

// ----- buildFooterLines -------------------------------------------------

func TestBuildFooterLines_NormalWidth(t *testing.T) {
	m := newTestModel()
	lines := m.buildFooterLines(120)
	require.NotEmpty(t, lines)
	// Last line should contain the total count.
	lastLine := lines[len(lines)-1]
	assert.Contains(t, lastLine, "Total:")
}

func TestBuildFooterLines_NarrowTerminal(t *testing.T) {
	m := newTestModel()
	// Narrow terminal forces wrapping.
	lines := m.buildFooterLines(40)
	require.NotEmpty(t, lines)
}

// ----- getTermDimensions ------------------------------------------------

func TestGetTermDimensions_Defaults(t *testing.T) {
	m := model{ui: NewBubbleTeaUI(nil, "1.0.0"), termWidth: 0, termHeight: 0}
	w, h := m.getTermDimensions()
	assert.Equal(t, DefaultTermWidth, w)
	assert.Equal(t, DefaultTermHeight, h)
}

func TestGetTermDimensions_Set(t *testing.T) {
	m := model{ui: NewBubbleTeaUI(nil, "1.0.0"), termWidth: 200, termHeight: 50}
	w, h := m.getTermDimensions()
	assert.Equal(t, 200, w)
	assert.Equal(t, 50, h)
}

// ----- getStatusIconAndText ---------------------------------------------

func TestGetStatusIconAndText(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	fwd := &config.Forward{Resource: "pod/app", Port: 80, LocalPort: 8080}
	ui.AddForward("id-1", fwd)
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	tests := []struct {
		status     string
		expectIcon string
		expectText string
		disabled   bool
	}{
		{"Active", "●", "Active", false},
		{"Starting", "○", "Starting", false},
		{"Reconnecting", "◐", "Reconnecting", false},
		{"Error", "✗", "Error", false},
		{"Active", "○", "Disabled", true},
	}

	for _, tt := range tests {
		t.Run(tt.status+"_disabled="+func() string {
			if tt.disabled {
				return "true"
			}
			return "false"
		}(), func(t *testing.T) {
			ui.mu.Lock()
			ui.forwards["id-1"].Status = tt.status
			ui.disabledMap["id-1"] = tt.disabled
			ui.mu.Unlock()

			ui.mu.RLock()
			icon, text := m.getStatusIconAndText("id-1", ui.forwards["id-1"])
			ui.mu.RUnlock()

			assert.Equal(t, tt.expectIcon, icon)
			assert.Equal(t, tt.expectText, text)
		})
	}
}

// ----- defaultMainViewColors / mainViewKeyBindings ----------------------

func TestDefaultMainViewColors(t *testing.T) {
	colors := defaultMainViewColors()
	assert.NotEmpty(t, string(colors.header))
	assert.NotEmpty(t, string(colors.active))
}

func TestMainViewKeyBindings(t *testing.T) {
	bindings := mainViewKeyBindings()
	require.NotEmpty(t, bindings)
	// Spot-check a few expected bindings.
	var keys []string
	for _, b := range bindings {
		keys = append(keys, b.key)
	}
	assert.Contains(t, keys, "n")
	assert.Contains(t, keys, "d")
}

// ----- safeRecover -------------------------------------------------------

func TestSafeRecover_DoesNotPanicOnRecover(t *testing.T) {
	// Must not propagate panic.
	assert.NotPanics(t, func() {
		func() {
			defer safeRecover("test context")
			panic("test panic")
		}()
	})
}

// ----- BenchmarkResults (populate via handleBenchmarkComplete) -----------

func TestBenchmarkResults_FromCompleteMsg(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	state := newBenchmarkState("fwd-id", "alias", 8080)
	state.running = true
	ui.benchmarkState = state
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	results := &benchmark.Results{
		TotalRequests: 100,
		Successful:    100,
		Failed:        0,
	}
	// CalculateStats operates on the Latencies slice; with no durations, stats are zero.
	_ = results.CalculateStats()

	msg := BenchmarkCompleteMsg{
		ForwardID: "fwd-id",
		Results:   results,
	}
	m.handleBenchmarkComplete(msg)

	ui.mu.RLock()
	defer ui.mu.RUnlock()
	require.NotNil(t, ui.benchmarkState.results)
	assert.Equal(t, 100, ui.benchmarkState.results.TotalRequests)
}
