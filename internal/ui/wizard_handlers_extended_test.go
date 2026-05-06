package ui

import (
	"os"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- helpers -------------------------------------------------------------

func newModelWithBenchmark() model {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeBenchmark
	ui.benchmarkState = newBenchmarkState("fwd-id", "alias", 8080)
	ui.mu.Unlock()
	return model{ui: ui, termWidth: 120, termHeight: 40}
}

func newModelWithHTTPLog() model {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "alias")
	ui.mu.Unlock()
	return model{ui: ui, termWidth: 120, termHeight: 40}
}

// ---- handleMainViewKeys: pgup / pgdown -----------------------------------

func TestHandleMainViewKeys_PageUpDown(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	for i := 0; i < 15; i++ {
		fwd := &config.Forward{Resource: "pod/app", Port: int(8080 + i), LocalPort: int(8080 + i)}
		ui.AddForward(string(rune('a'+i)), fwd)
	}
	ui.mu.Lock()
	ui.selectedIndex = 10
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyPgUp}
	m.handleMainViewKeys(keyMsg)
	ui.mu.RLock()
	assert.Equal(t, 0, ui.selectedIndex)
	ui.mu.RUnlock()

	// Reset and test pgdown.
	ui.mu.Lock()
	ui.selectedIndex = 0
	ui.mu.Unlock()
	keyMsg = tea.KeyMsg{Type: tea.KeyPgDown}
	m.handleMainViewKeys(keyMsg)
	ui.mu.RLock()
	assert.Equal(t, 10, ui.selectedIndex)
	ui.mu.RUnlock()
}

// ---- handleMainViewKeys: 'n' with real discovery (kubeconfig optional) --

// TestHandleMainViewKeys_NewWizard_WithDiscovery checks that the 'n' key activates
// the add wizard when a discovery + mutator are set.  The actual loadContextsCmd
// is NOT invoked (we only verify the wizard is opened and a cmd is returned).
func TestHandleMainViewKeys_NewWizard_WithDiscovery(t *testing.T) {
	realDiscovery := &k8s.Discovery{}
	realMutator := &config.Mutator{}
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.discovery = realDiscovery
	ui.mutator = realMutator
	ui.configPath = "/tmp/test.yaml"

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	_, cmd := m.handleMainViewKeys(keyMsg)

	ui.mu.RLock()
	assert.Equal(t, ViewModeAddWizard, ui.viewMode)
	require.NotNil(t, ui.addWizard)
	ui.mu.RUnlock()

	// The wizard was opened and a cmd was returned (loadContextsCmd).
	assert.NotNil(t, cmd)
}

// ---- handleMainViewKeys: 'n' blocks when wizard already active ----------

func TestHandleMainViewKeys_NewWizard_AlreadyActive(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.discovery = &k8s.Discovery{}
	ui.mutator = &config.Mutator{}
	ui.mu.Lock()
	ui.addWizard = newAddWizardState() // wizard already active
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	m.handleMainViewKeys(keyMsg)

	// Should still be the same wizard (not replaced).
	ui.mu.RLock()
	assert.Equal(t, StepSelectContext, ui.addWizard.step)
	ui.mu.RUnlock()
}

// ---- handleMainViewKeys: 'l' HTTP log subscription ----------------------

func TestHandleMainViewKeys_HttpLog_NoSubscriber(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	fwd := &config.Forward{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "app"}
	ui.AddForward("id-1", fwd)
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}
	m.handleMainViewKeys(keyMsg)

	ui.mu.RLock()
	assert.Equal(t, ViewModeHTTPLog, ui.viewMode)
	require.NotNil(t, ui.httpLogState)
	ui.mu.RUnlock()
}

func TestHandleMainViewKeys_HttpLog_WithSubscriber(t *testing.T) {
	mockSub := NewMockHTTPLogSubscriber()
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.SetHTTPLogSubscriber(mockSub.GetSubscriberFunc())
	fwd := &config.Forward{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "app"}
	ui.AddForward("id-1", fwd)
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")}
	m.handleMainViewKeys(keyMsg)

	// Subscription should be established.
	mockSub.mu.Lock()
	_, subscribed := mockSub.Subscriptions["id-1"]
	mockSub.mu.Unlock()
	assert.True(t, subscribed)
}

// ---- handleMainViewKeys: 'b' benchmark ----------------------------------

func TestHandleMainViewKeys_Benchmark(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	fwd := &config.Forward{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "app"}
	ui.AddForward("id-1", fwd)
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")}
	m.handleMainViewKeys(keyMsg)

	ui.mu.RLock()
	assert.Equal(t, ViewModeBenchmark, ui.viewMode)
	require.NotNil(t, ui.benchmarkState)
	ui.mu.RUnlock()
}

func TestHandleMainViewKeys_Benchmark_BlocksWhenActive(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	fwd := &config.Forward{Resource: "pod/app", Port: 8080, LocalPort: 8080, Alias: "app"}
	ui.AddForward("id-1", fwd)
	ui.mu.Lock()
	ui.benchmarkState = newBenchmarkState("id-1", "app", 8080)
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	prevState := ui.benchmarkState
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")}
	m.handleMainViewKeys(keyMsg)

	// Should not have replaced the benchmark state.
	ui.mu.RLock()
	assert.Equal(t, prevState, ui.benchmarkState)
	ui.mu.RUnlock()
}

// ---- handleAddWizardKeys: ctrl+c / esc ----------------------------------

func TestHandleAddWizardKeys_CtrlC(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)

	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	assert.NotNil(t, cmd) // tea.ClearScreen
	m.ui.mu.RLock()
	assert.Nil(t, m.ui.addWizard)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

func TestHandleAddWizardKeys_Esc_FirstStep_Cancels(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)

	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleAddWizardKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Nil(t, m.ui.addWizard)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

func TestHandleAddWizardKeys_Esc_MiddleStep_GoesBack(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)

	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleAddWizardKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Equal(t, StepSelectContext, m.ui.addWizard.step)
	m.ui.mu.RUnlock()
}

func TestHandleAddWizardKeys_Esc_ClearsSearchFilter_InsteadOfBack(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.searchFilter = "my"

	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleAddWizardKeys(keyMsg)

	m.ui.mu.RLock()
	// Should not go back, should just clear filter.
	assert.Equal(t, StepSelectContext, m.ui.addWizard.step)
	assert.Empty(t, m.ui.addWizard.searchFilter)
	m.ui.mu.RUnlock()
}

func TestHandleAddWizardKeys_Esc_EditMode_AlwaysCancels(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.isEditing = true

	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleAddWizardKeys(keyMsg)

	m.ui.mu.RLock()
	assert.Nil(t, m.ui.addWizard)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

// ---- handleAddWizardKeys: navigation ------------------------------------

func TestHandleAddWizardKeys_Navigation(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.contexts = []string{"ctx-a", "ctx-b", "ctx-c"}

	keyMsg := tea.KeyMsg{Type: tea.KeyDown}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, 1, m.ui.addWizard.cursor)

	keyMsg = tea.KeyMsg{Type: tea.KeyUp}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, 0, m.ui.addWizard.cursor)
}

func TestHandleAddWizardKeys_PageNavigation(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	contexts := make([]string, 30)
	for i := range contexts {
		contexts[i] = "ctx"
	}
	m.ui.addWizard.contexts = contexts
	m.ui.addWizard.cursor = 15

	keyMsg := tea.KeyMsg{Type: tea.KeyPgUp}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, 5, m.ui.addWizard.cursor)

	keyMsg = tea.KeyMsg{Type: tea.KeyPgDown}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, 15, m.ui.addWizard.cursor)
}

// ---- handleAddWizardKeys: confirmation step navigation ------------------

func TestHandleAddWizardKeys_ConfirmationStep_UpFocusAlias(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusButtons

	keyMsg := tea.KeyMsg{Type: tea.KeyUp}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, FocusAlias, m.ui.addWizard.confirmationFocus)
}

func TestHandleAddWizardKeys_ConfirmationStep_DownFocusButtons(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusAlias

	keyMsg := tea.KeyMsg{Type: tea.KeyDown}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, FocusButtons, m.ui.addWizard.confirmationFocus)
}

func TestHandleAddWizardKeys_Tab_TogglesFocus(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusAlias

	keyMsg := tea.KeyMsg{Type: tea.KeyTab}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, FocusButtons, m.ui.addWizard.confirmationFocus)

	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, FocusAlias, m.ui.addWizard.confirmationFocus)
}

// ---- handleAddWizardKeys: backspace ------------------------------------

func TestHandleAddWizardKeys_Backspace_TextMode(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.textInput = "808"

	keyMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, "80", m.ui.addWizard.textInput)
}

func TestHandleAddWizardKeys_Backspace_SearchFilter(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.searchFilter = "my"

	keyMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, "m", m.ui.addWizard.searchFilter)
}

func TestHandleAddWizardKeys_Backspace_EmptyFilter_NoOp(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.searchFilter = ""

	keyMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	// Must not panic.
	m.handleAddWizardKeys(keyMsg)
}

// ---- handleAddWizardKeys: text input in filterable list mode ------------

func TestHandleAddWizardKeys_TypeCharAddsToSearchFilter(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	// Context step is filterable in list mode.

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, "p", m.ui.addWizard.searchFilter)
}

// ---- handleAddWizardKeys: text input in text mode ----------------------

func TestHandleAddWizardKeys_TypeCharInTextMode(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("8")}
	m.handleAddWizardKeys(keyMsg)
	assert.Equal(t, "8", m.ui.addWizard.textInput)
}

// ---- handleAddWizardEnter: StepSelectContext ----------------------------

func TestHandleAddWizardEnter_SelectContext(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	// Use a real discovery (may be nil cluster, but we only need the cmd to be returned).
	ui.discovery = &k8s.Discovery{}
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepSelectContext
	w.contexts = []string{"prod", "staging"}
	w.cursor = 0
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.Equal(t, StepSelectNamespace, ui.addWizard.step)
	assert.Equal(t, "prod", ui.addWizard.selectedContext)
	ui.mu.RUnlock()

	// A cmd is returned (loadNamespacesCmd) — we just verify it's not nil.
	assert.NotNil(t, cmd)
}

func TestHandleAddWizardEnter_SelectContext_Loading_NoOp(t *testing.T) {
	m := newModelWithWizard(StepSelectContext)
	m.ui.addWizard.loading = true

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)
	assert.Nil(t, cmd)
}

// ---- handleAddWizardEnter: StepSelectNamespace --------------------------

func TestHandleAddWizardEnter_SelectNamespace(t *testing.T) {
	m := newModelWithWizard(StepSelectNamespace)
	m.ui.addWizard.namespaces = []string{"default", "kube-system"}
	m.ui.addWizard.cursor = 1 // kube-system

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepSelectResourceType, m.ui.addWizard.step)
	assert.Equal(t, "kube-system", m.ui.addWizard.selectedNamespace)
}

// ---- handleAddWizardEnter: StepSelectResourceType ----------------------

func TestHandleAddWizardEnter_SelectResourceType_Service(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.discovery = &k8s.Discovery{}
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepSelectResourceType
	w.selectedContext = "ctx"
	w.selectedNamespace = "ns"
	w.cursor = 2 // ResourceTypeService
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.Equal(t, StepEnterResource, ui.addWizard.step)
	assert.Equal(t, ResourceTypeService, ui.addWizard.selectedResourceType)
	ui.mu.RUnlock()

	// A cmd (loadServicesCmd) is returned.
	assert.NotNil(t, cmd)
}

func TestHandleAddWizardEnter_SelectResourceType_PodPrefix(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.discovery = &k8s.Discovery{}
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepSelectResourceType
	w.selectedContext = "ctx"
	w.selectedNamespace = "ns"
	w.cursor = 0 // ResourceTypePodPrefix
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.Equal(t, StepEnterResource, ui.addWizard.step)
	ui.mu.RUnlock()

	// A cmd (loadPodsCmd) is returned.
	assert.NotNil(t, cmd)
}

// ---- handleAddWizardEnter: StepEnterResource PodPrefix -----------------

func TestHandleAddWizardEnter_EnterResource_PodPrefix(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.textInput = "my-app"
	m.ui.addWizard.pods = []k8s.PodInfo{{
		Name:       "my-app-abc",
		Containers: []k8s.ContainerInfo{{Name: "main", Ports: []k8s.PortInfo{{Port: 8080}}}},
	}}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterRemotePort, m.ui.addWizard.step)
	assert.Equal(t, "my-app", m.ui.addWizard.resourceValue)
}

func TestHandleAddWizardEnter_EnterResource_PodPrefix_EmptyInput_NoOp(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodPrefix
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.textInput = ""

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterResource, m.ui.addWizard.step) // should not advance
}

// ---- handleAddWizardEnter: StepEnterResource PodSelector ---------------

func TestHandleAddWizardEnter_EnterResource_PodSelector(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodSelector
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.textInput = "app=my"
	m.ui.addWizard.matchingPods = []k8s.PodInfo{{Name: "my-pod"}}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterRemotePort, m.ui.addWizard.step)
	assert.Equal(t, "app=my", m.ui.addWizard.selector)
}

func TestHandleAddWizardEnter_EnterResource_PodSelector_NoMatchingPods_NoOp(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypePodSelector
	m.ui.addWizard.textInput = "app=my"
	m.ui.addWizard.matchingPods = nil

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterResource, m.ui.addWizard.step)
}

// ---- handleAddWizardEnter: StepEnterResource Service -------------------

func TestHandleAddWizardEnter_EnterResource_Service(t *testing.T) {
	m := newModelWithWizard(StepEnterResource)
	m.ui.addWizard.selectedResourceType = ResourceTypeService
	m.ui.addWizard.inputMode = InputModeList
	m.ui.addWizard.services = []k8s.ServiceInfo{
		{Name: "api-svc", Ports: []k8s.PortInfo{{Port: 80}}},
	}
	m.ui.addWizard.cursor = 0

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterRemotePort, m.ui.addWizard.step)
	assert.Equal(t, "api-svc", m.ui.addWizard.resourceValue)
}

// ---- handleAddWizardEnter: StepEnterRemotePort -------------------------

func TestHandleAddWizardEnter_RemotePort_TextMode_ValidPort(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.textInput = "8080"

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterLocalPort, m.ui.addWizard.step)
	assert.Equal(t, 8080, m.ui.addWizard.remotePort)
}

func TestHandleAddWizardEnter_RemotePort_TextMode_InvalidPort(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeText
	m.ui.addWizard.textInput = "invalid"

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterRemotePort, m.ui.addWizard.step)
	assert.NotNil(t, m.ui.addWizard.error)
}

func TestHandleAddWizardEnter_RemotePort_ListMode_SelectPort(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeList
	m.ui.addWizard.detectedPorts = []k8s.PortInfo{
		{Port: 80},
		{Port: 8080, TargetPort: 9090},
	}
	m.ui.addWizard.cursor = 1 // 8080 → TargetPort 9090

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, StepEnterLocalPort, m.ui.addWizard.step)
	assert.Equal(t, 9090, m.ui.addWizard.remotePort) // TargetPort used
}

func TestHandleAddWizardEnter_RemotePort_ListMode_ManualEntry(t *testing.T) {
	m := newModelWithWizard(StepEnterRemotePort)
	m.ui.addWizard.inputMode = InputModeList
	m.ui.addWizard.detectedPorts = []k8s.PortInfo{{Port: 80}}
	m.ui.addWizard.cursor = 1 // "Manual entry" option

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, InputModeText, m.ui.addWizard.inputMode)
	assert.Equal(t, StepEnterRemotePort, m.ui.addWizard.step)
}

// ---- handleAddWizardEnter: StepEnterLocalPort --------------------------

func TestHandleAddWizardEnter_LocalPort_ValidPort(t *testing.T) {
	tmpDir := t.TempDir()
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.configPath = tmpDir + "/nonexistent.yaml"
	w := newAddWizardState()
	w.step = StepEnterLocalPort
	w.textInput = "19876"
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	// loading set, cmd generated.
	require.NotNil(t, cmd)
	ui.mu.RLock()
	assert.True(t, ui.addWizard.loading)
	assert.Equal(t, 19876, ui.addWizard.localPort)
	ui.mu.RUnlock()
}

func TestHandleAddWizardEnter_LocalPort_InvalidPort(t *testing.T) {
	m := newModelWithWizard(StepEnterLocalPort)
	m.ui.addWizard.textInput = "0" // invalid (must be > 0)

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.NotNil(t, m.ui.addWizard.error)
}

// ---- handleAddWizardEnter: StepConfirmation ----------------------------

func TestHandleAddWizardEnter_Confirmation_FocusAlias_MoveToButtons(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusAlias

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.Equal(t, FocusButtons, m.ui.addWizard.confirmationFocus)
}

func TestHandleAddWizardEnter_Confirmation_CancelButton(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusButtons
	m.ui.addWizard.cursor = 1 // Cancel

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	assert.NotNil(t, cmd) // tea.ClearScreen
	m.ui.mu.RLock()
	assert.Nil(t, m.ui.addWizard)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

func TestHandleAddWizardEnter_Confirmation_PortNotAvailable_Error(t *testing.T) {
	m := newModelWithWizard(StepConfirmation)
	m.ui.addWizard.confirmationFocus = FocusButtons
	m.ui.addWizard.cursor = 0 // Yes / Add
	m.ui.addWizard.portAvailable = false
	m.ui.addWizard.localPort = 8080

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleAddWizardKeys(keyMsg)

	assert.NotNil(t, m.ui.addWizard.error)
	assert.Equal(t, StepConfirmation, m.ui.addWizard.step) // should stay
}

func TestHandleAddWizardEnter_Confirmation_Save_NewForward(t *testing.T) {
	mutator := newTempMutator(t)
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mutator = mutator
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepConfirmation
	w.confirmationFocus = FocusButtons
	w.cursor = 0 // Yes/Add
	w.portAvailable = true
	w.selectedContext = "ctx"
	w.selectedNamespace = "ns"
	w.resourceValue = "my-app"
	w.selectedResourceType = ResourceTypePodPrefix
	w.remotePort = 80
	w.localPort = 18090
	w.textInput = "my-alias"
	w.httpLog = false
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	require.NotNil(t, cmd)
	msg := cmd()
	savedMsg, ok := msg.(ForwardSavedMsg)
	require.True(t, ok)
	assert.True(t, savedMsg.success)
}

func TestHandleAddWizardEnter_Confirmation_Save_NewForward_WithHTTPLog(t *testing.T) {
	mutator := newTempMutator(t)
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mutator = mutator
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepConfirmation
	w.confirmationFocus = FocusButtons
	w.cursor = 0
	w.portAvailable = true
	w.selectedContext = "ctx"
	w.selectedNamespace = "ns"
	w.resourceValue = "my-app"
	w.selectedResourceType = ResourceTypePodPrefix
	w.remotePort = 80
	w.localPort = 18091
	w.httpLog = true
	// Simulate having an existing HTTPLog config with advanced fields.
	w.httpLogOriginal = &config.HTTPLogSpec{Enabled: false, IncludeHeaders: true, MaxBodySize: 4096}
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)
	require.NotNil(t, cmd)
	msg := cmd()
	savedMsg, ok := msg.(ForwardSavedMsg)
	require.True(t, ok)
	assert.True(t, savedMsg.success)
}

func TestHandleAddWizardEnter_Confirmation_Update_EditMode(t *testing.T) {
	mutator := newTempMutator(t)
	// Add the original forward first so the update has something to update.
	origFwd := config.Forward{Resource: "pod/my-app", Port: 80, LocalPort: 18092}
	require.NoError(t, mutator.AddForward("ctx", "ns", origFwd))

	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mutator = mutator
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepConfirmation
	w.confirmationFocus = FocusButtons
	w.cursor = 0
	w.portAvailable = true
	w.isEditing = true
	w.originalID = "ctx/ns/pod/my-app:18092"
	w.selectedContext = "ctx"
	w.selectedNamespace = "ns"
	w.resourceValue = "my-app"
	w.selectedResourceType = ResourceTypePodPrefix
	w.remotePort = 80
	w.localPort = 18092
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)
	require.NotNil(t, cmd)
	msg := cmd()
	savedMsg, ok := msg.(ForwardSavedMsg)
	require.True(t, ok)
	assert.True(t, savedMsg.success)
}

// ---- handleAddWizardEnter: StepSuccess ---------------------------------

func TestHandleAddWizardEnter_Success_AddAnother(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.discovery = &k8s.Discovery{}
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepSuccess
	w.cursor = 0 // "Add another"
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	// A cmd (loadContextsCmd) must be returned.
	assert.NotNil(t, cmd)
}

func TestHandleAddWizardEnter_Success_ReturnToMain(t *testing.T) {
	m := newModelWithWizard(StepSuccess)
	m.ui.addWizard.cursor = 1 // "Return to main"

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	assert.NotNil(t, cmd)
	m.ui.mu.RLock()
	assert.Nil(t, m.ui.addWizard)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

// ---- handleAddWizardKeys: selector validation on type -----------------

func TestHandleAddWizardKeys_SelectorValidation_OnChar(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.discovery = &k8s.Discovery{}
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepEnterResource
	w.selectedResourceType = ResourceTypePodSelector
	w.inputMode = InputModeText
	w.textInput = "app=my"
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	// Typing a char in PodSelector step should return a validation cmd.
	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("-")}
	_, cmd := m.handleAddWizardKeys(keyMsg)

	// A cmd is returned (validateSelectorCmd); we don't invoke it as it needs a real cluster.
	assert.NotNil(t, cmd)
}

// ---- handleRemoveWizardKeys: Space, a, n keys --------------------------

func TestHandleRemoveWizardKeys_SpaceToggle(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{{ID: "f1", Alias: "alpha"}, {ID: "f2", Alias: "beta"}},
		selected: map[int]bool{},
		cursor:   0,
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeySpace}
	m.handleRemoveWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.True(t, ui.removeWizard.selected[0])
	ui.mu.RUnlock()

	// Toggle again to deselect.
	m.handleRemoveWizardKeys(keyMsg)
	ui.mu.RLock()
	assert.False(t, ui.removeWizard.selected[0])
	ui.mu.RUnlock()
}

func TestHandleRemoveWizardKeys_SelectAll(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{{ID: "f1"}, {ID: "f2"}, {ID: "f3"}},
		selected: map[int]bool{},
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	m.handleRemoveWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.True(t, ui.removeWizard.selected[0])
	assert.True(t, ui.removeWizard.selected[1])
	assert.True(t, ui.removeWizard.selected[2])
	ui.mu.RUnlock()
}

func TestHandleRemoveWizardKeys_SelectNone(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{{ID: "f1"}, {ID: "f2"}},
		selected: map[int]bool{0: true, 1: true},
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}
	m.handleRemoveWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.Equal(t, 0, len(ui.removeWizard.selected))
	ui.mu.RUnlock()
}

func TestHandleRemoveWizardKeys_CtrlC_ExitsAlways(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards:   []RemovableForward{{ID: "f1"}},
		selected:   map[int]bool{0: true},
		confirming: true,
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyCtrlC}
	m.handleRemoveWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.Nil(t, ui.removeWizard)
	assert.Equal(t, ViewModeMain, ui.viewMode)
	ui.mu.RUnlock()
}

func TestHandleRemoveWizardKeys_Enter_NothingSelected_NoOp(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{{ID: "f1"}},
		selected: map[int]bool{},
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleRemoveWizardKeys(keyMsg)

	assert.Nil(t, cmd)
	ui.mu.RLock()
	assert.False(t, ui.removeWizard.confirming)
	ui.mu.RUnlock()
}

func TestHandleRemoveWizardKeys_Enter_ShowConfirmation(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{{ID: "f1"}},
		selected: map[int]bool{0: true},
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleRemoveWizardKeys(keyMsg)

	ui.mu.RLock()
	assert.True(t, ui.removeWizard.confirming)
	ui.mu.RUnlock()
}

func TestHandleRemoveWizardKeys_Enter_ConfirmNo(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards:      []RemovableForward{{ID: "f1"}},
		selected:      map[int]bool{0: true},
		confirming:    true,
		confirmCursor: 1, // "No"
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleRemoveWizardKeys(keyMsg)

	assert.Nil(t, cmd)
	ui.mu.RLock()
	assert.False(t, ui.removeWizard.confirming) // returned to selection
	ui.mu.RUnlock()
}

func TestHandleRemoveWizardKeys_Navigation_InSelection(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeRemoveWizard
	ui.removeWizard = &RemoveWizardState{
		forwards: []RemovableForward{{ID: "f1"}, {ID: "f2"}, {ID: "f3"}},
		selected: map[int]bool{},
		cursor:   0,
	}
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	keyMsg := tea.KeyMsg{Type: tea.KeyDown}
	m.handleRemoveWizardKeys(keyMsg)
	ui.mu.RLock()
	assert.Equal(t, 1, ui.removeWizard.cursor)
	ui.mu.RUnlock()

	keyMsg = tea.KeyMsg{Type: tea.KeyUp}
	m.handleRemoveWizardKeys(keyMsg)
	ui.mu.RLock()
	assert.Equal(t, 0, ui.removeWizard.cursor)
	ui.mu.RUnlock()
}

// ---- handleBenchmarkKeys: tab / text input / backspace -----------------

func TestHandleBenchmarkKeys_Tab(t *testing.T) {
	m := newModelWithBenchmark()
	m.ui.benchmarkState.step = BenchmarkStepConfig
	m.ui.benchmarkState.cursor = 0

	keyMsg := tea.KeyMsg{Type: tea.KeyTab}
	m.handleBenchmarkKeys(keyMsg)

	assert.Equal(t, 1, m.ui.benchmarkState.cursor)
}

func TestHandleBenchmarkKeys_Tab_Wraps(t *testing.T) {
	m := newModelWithBenchmark()
	m.ui.benchmarkState.step = BenchmarkStepConfig
	m.ui.benchmarkState.cursor = 3

	keyMsg := tea.KeyMsg{Type: tea.KeyTab}
	m.handleBenchmarkKeys(keyMsg)

	assert.Equal(t, 0, m.ui.benchmarkState.cursor)
}

func TestHandleBenchmarkKeys_Backspace(t *testing.T) {
	m := newModelWithBenchmark()
	m.ui.benchmarkState.step = BenchmarkStepConfig
	m.ui.benchmarkState.cursor = 0
	m.ui.benchmarkState.textInput = "/api"

	keyMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	m.handleBenchmarkKeys(keyMsg)

	assert.Equal(t, "/ap", m.ui.benchmarkState.textInput)
	assert.Equal(t, "/ap", m.ui.benchmarkState.urlPath)
}

func TestHandleBenchmarkKeys_TypeChar_UpdatesField(t *testing.T) {
	m := newModelWithBenchmark()
	m.ui.benchmarkState.step = BenchmarkStepConfig
	m.ui.benchmarkState.cursor = 1 // Method field
	m.ui.benchmarkState.textInput = "GE"

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("T")}
	m.handleBenchmarkKeys(keyMsg)

	assert.Equal(t, "GET", m.ui.benchmarkState.method)
}

func TestHandleBenchmarkKeys_Enter_ResultsStep_ReturnsToMain(t *testing.T) {
	m := newModelWithBenchmark()
	m.ui.benchmarkState.step = BenchmarkStepResults

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleBenchmarkKeys(keyMsg)

	assert.NotNil(t, cmd) // tea.ClearScreen
	m.ui.mu.RLock()
	assert.Nil(t, m.ui.benchmarkState)
	assert.Equal(t, ViewModeMain, m.ui.viewMode)
	m.ui.mu.RUnlock()
}

func TestHandleBenchmarkKeys_Enter_ConfigStep_StartsRunning(t *testing.T) {
	m := newModelWithBenchmark()
	m.ui.benchmarkState.step = BenchmarkStepConfig

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	_, cmd := m.handleBenchmarkKeys(keyMsg)

	require.NotNil(t, cmd)
	m.ui.mu.RLock()
	assert.Equal(t, BenchmarkStepRunning, m.ui.benchmarkState.step)
	assert.True(t, m.ui.benchmarkState.running)
	m.ui.mu.RUnlock()
}

// ---- applyBenchmarkTextInput / getBenchmarkFieldValue ------------------

func TestApplyBenchmarkTextInput_AllFields(t *testing.T) {
	m := newModelWithBenchmark()
	state := m.ui.benchmarkState
	state.step = BenchmarkStepConfig

	// URL path (cursor 0)
	state.cursor = 0
	state.textInput = "/new"
	m.applyBenchmarkTextInput()
	assert.Equal(t, "/new", state.urlPath)

	// Method (cursor 1)
	state.cursor = 1
	state.textInput = "post"
	m.applyBenchmarkTextInput()
	assert.Equal(t, "POST", state.method)

	// Concurrency (cursor 2)
	state.cursor = 2
	state.textInput = "5"
	m.applyBenchmarkTextInput()
	assert.Equal(t, 5, state.concurrency)

	// Requests (cursor 3)
	state.cursor = 3
	state.textInput = "50"
	m.applyBenchmarkTextInput()
	assert.Equal(t, 50, state.requests)
}

func TestApplyBenchmarkTextInput_ConcurrencyCappedAtRequests(t *testing.T) {
	m := newModelWithBenchmark()
	state := m.ui.benchmarkState
	state.requests = 10
	state.cursor = 2
	state.textInput = "999"
	m.applyBenchmarkTextInput()
	assert.Equal(t, 10, state.concurrency)
}

func TestGetBenchmarkFieldValue_AllFields(t *testing.T) {
	m := newModelWithBenchmark()
	state := m.ui.benchmarkState
	state.urlPath = "/test"
	state.method = "DELETE"
	state.concurrency = 7
	state.requests = 77

	assert.Equal(t, "/test", m.getBenchmarkFieldValue(0))
	assert.Equal(t, "DELETE", m.getBenchmarkFieldValue(1))
	assert.Equal(t, "7", m.getBenchmarkFieldValue(2))
	assert.Equal(t, "77", m.getBenchmarkFieldValue(3))
	assert.Equal(t, "", m.getBenchmarkFieldValue(99))
}

func TestGetBenchmarkFieldValue_NilState(t *testing.T) {
	m := newTestModel()
	result := m.getBenchmarkFieldValue(0)
	assert.Equal(t, "", result)
}

// ---- handleHTTPLogKeys: detail view keys --------------------------------

func TestHandleHTTPLogKeys_Detail_Esc(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.showingDetail = true
	m.ui.httpLogState.entries = []HTTPLogEntry{{Method: "GET", Path: "/test"}}

	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleHTTPLogKeys(keyMsg)

	assert.False(t, m.ui.httpLogState.showingDetail)
}

func TestHandleHTTPLogKeys_Detail_UpDown(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.showingDetail = true
	m.ui.httpLogState.detailScroll = 5
	m.ui.httpLogState.entries = []HTTPLogEntry{{Method: "GET"}}

	keyMsg := tea.KeyMsg{Type: tea.KeyUp}
	m.handleHTTPLogKeys(keyMsg)
	assert.Equal(t, 4, m.ui.httpLogState.detailScroll)

	keyMsg = tea.KeyMsg{Type: tea.KeyDown}
	m.handleHTTPLogKeys(keyMsg)
	assert.Equal(t, 5, m.ui.httpLogState.detailScroll)
}

func TestHandleHTTPLogKeys_Detail_PgUpDown(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.showingDetail = true
	m.ui.httpLogState.detailScroll = 25
	m.ui.httpLogState.entries = []HTTPLogEntry{{Method: "GET"}}

	keyMsg := tea.KeyMsg{Type: tea.KeyPgUp}
	m.handleHTTPLogKeys(keyMsg)
	assert.Equal(t, 5, m.ui.httpLogState.detailScroll)

	keyMsg = tea.KeyMsg{Type: tea.KeyPgDown}
	m.handleHTTPLogKeys(keyMsg)
	assert.Equal(t, 25, m.ui.httpLogState.detailScroll)
}

func TestHandleHTTPLogKeys_Detail_GoToTop(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.showingDetail = true
	m.ui.httpLogState.detailScroll = 99
	m.ui.httpLogState.entries = []HTTPLogEntry{{Method: "GET"}}

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}
	m.handleHTTPLogKeys(keyMsg)

	assert.Equal(t, 0, m.ui.httpLogState.detailScroll)
}

// ---- handleHTTPLogKeys: list view keys ----------------------------------

func TestHandleHTTPLogKeys_Enter_ShowDetail(t *testing.T) {
	m := newModelWithHTTPLog()
	// StatusCode must be non-zero or getFilteredEntries will skip the entry.
	m.ui.httpLogState.entries = []HTTPLogEntry{{Method: "GET", Path: "/test", StatusCode: 200}}
	m.ui.httpLogState.cursor = 0

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleHTTPLogKeys(keyMsg)

	assert.True(t, m.ui.httpLogState.showingDetail)
}

func TestHandleHTTPLogKeys_Navigate_UpDown(t *testing.T) {
	m := newModelWithHTTPLog()
	// StatusCode must be non-zero or getFilteredEntries will skip entries.
	m.ui.httpLogState.entries = []HTTPLogEntry{
		{Method: "GET", StatusCode: 200}, {Method: "POST", StatusCode: 200}, {Method: "PUT", StatusCode: 200},
	}
	m.ui.httpLogState.cursor = 1

	keyMsg := tea.KeyMsg{Type: tea.KeyUp}
	m.handleHTTPLogKeys(keyMsg)
	assert.Equal(t, 0, m.ui.httpLogState.cursor)

	keyMsg = tea.KeyMsg{Type: tea.KeyDown}
	m.handleHTTPLogKeys(keyMsg)
	assert.Equal(t, 1, m.ui.httpLogState.cursor)
}

func TestHandleHTTPLogKeys_Down_AtBottom_EnablesAutoScroll(t *testing.T) {
	m := newModelWithHTTPLog()
	// StatusCode must be non-zero or getFilteredEntries will skip entries.
	m.ui.httpLogState.entries = []HTTPLogEntry{{Method: "GET", StatusCode: 200}, {Method: "POST", StatusCode: 200}}
	m.ui.httpLogState.cursor = 0
	m.ui.httpLogState.autoScroll = false

	keyMsg := tea.KeyMsg{Type: tea.KeyDown}
	m.handleHTTPLogKeys(keyMsg)

	assert.True(t, m.ui.httpLogState.autoScroll)
}

func TestHandleHTTPLogKeys_GoToTop_G(t *testing.T) {
	m := newModelWithHTTPLog()
	// StatusCode must be non-zero or getFilteredEntries will skip entries.
	m.ui.httpLogState.entries = []HTTPLogEntry{
		{Method: "GET", StatusCode: 200}, {Method: "POST", StatusCode: 200}, {Method: "PUT", StatusCode: 200},
	}
	m.ui.httpLogState.cursor = 2

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")}
	m.handleHTTPLogKeys(keyMsg)

	assert.Equal(t, 0, m.ui.httpLogState.cursor)
	assert.False(t, m.ui.httpLogState.autoScroll)
}

func TestHandleHTTPLogKeys_GoToBottom_CapitalG(t *testing.T) {
	m := newModelWithHTTPLog()
	// StatusCode must be non-zero or getFilteredEntries will skip entries.
	m.ui.httpLogState.entries = []HTTPLogEntry{
		{Method: "GET", StatusCode: 200}, {Method: "POST", StatusCode: 200}, {Method: "PUT", StatusCode: 200},
	}
	m.ui.httpLogState.cursor = 0

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")}
	m.handleHTTPLogKeys(keyMsg)

	assert.Equal(t, 2, m.ui.httpLogState.cursor)
	assert.True(t, m.ui.httpLogState.autoScroll)
}

func TestHandleHTTPLogKeys_ToggleAutoScroll(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.autoScroll = false

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}
	m.handleHTTPLogKeys(keyMsg)
	assert.True(t, m.ui.httpLogState.autoScroll)

	m.handleHTTPLogKeys(keyMsg)
	assert.False(t, m.ui.httpLogState.autoScroll)
}

func TestHandleHTTPLogKeys_PgUpDown_List(t *testing.T) {
	m := newModelWithHTTPLog()
	entries := make([]HTTPLogEntry, 30)
	for i := range entries {
		// StatusCode must be non-zero or getFilteredEntries will skip entries.
		entries[i] = HTTPLogEntry{Method: "GET", StatusCode: 200}
	}
	m.ui.httpLogState.entries = entries
	m.ui.httpLogState.cursor = 25

	keyMsg := tea.KeyMsg{Type: tea.KeyPgUp}
	m.handleHTTPLogKeys(keyMsg)
	assert.Equal(t, 5, m.ui.httpLogState.cursor)
	assert.False(t, m.ui.httpLogState.autoScroll)

	keyMsg = tea.KeyMsg{Type: tea.KeyPgDown}
	m.handleHTTPLogKeys(keyMsg)
	// PgDown +20 from 5 = 25; index 25 is in range (30 items, indices 0-29).
	assert.Equal(t, 25, m.ui.httpLogState.cursor)
}

// ---- handleHTTPLogKeys: filter active (text input mode) ----------------

func TestHandleHTTPLogKeys_FilterActive_Typing(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.filterActive = true
	m.ui.httpLogState.filterText = "te"

	keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")}
	m.handleHTTPLogKeys(keyMsg)

	assert.Equal(t, "tes", m.ui.httpLogState.filterText)
}

func TestHandleHTTPLogKeys_FilterActive_Backspace(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.filterActive = true
	m.ui.httpLogState.filterText = "tes"

	keyMsg := tea.KeyMsg{Type: tea.KeyBackspace}
	m.handleHTTPLogKeys(keyMsg)

	assert.Equal(t, "te", m.ui.httpLogState.filterText)
}

func TestHandleHTTPLogKeys_FilterActive_EscClearsFilter(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.filterActive = true
	m.ui.httpLogState.filterText = "test"

	keyMsg := tea.KeyMsg{Type: tea.KeyEsc}
	m.handleHTTPLogKeys(keyMsg)

	assert.False(t, m.ui.httpLogState.filterActive)
	assert.Empty(t, m.ui.httpLogState.filterText)
}

func TestHandleHTTPLogKeys_FilterActive_EnterConfirms(t *testing.T) {
	m := newModelWithHTTPLog()
	m.ui.httpLogState.filterActive = true
	m.ui.httpLogState.filterText = "api"

	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	m.handleHTTPLogKeys(keyMsg)

	assert.False(t, m.ui.httpLogState.filterActive)
	assert.Equal(t, "api", m.ui.httpLogState.filterText)
}

// ---- handleHTTPLogEntry: response merging, capping ---------------------

func TestHandleHTTPLogEntry_ResponseMerging(t *testing.T) {
	m := newModelWithHTTPLog()
	reqID := "req-123"

	// Add a request entry first.
	requestEntry := HTTPLogEntry{
		Direction: "request",
		Method:    "GET",
		Path:      "/api/test",
		RequestID: reqID,
	}
	m.ui.httpLogState.entries = []HTTPLogEntry{requestEntry}

	// Now handle the response.
	responseEntry := HTTPLogEntry{
		Direction:    "response",
		RequestID:    reqID,
		StatusCode:   200,
		LatencyMs:    50,
		BodySize:     100,
		ResponseBody: "ok",
	}
	m.handleHTTPLogEntry(HTTPLogEntryMsg{Entry: responseEntry})

	// Should be merged into the single request entry.
	assert.Len(t, m.ui.httpLogState.entries, 1)
	assert.Equal(t, 200, m.ui.httpLogState.entries[0].StatusCode)
	assert.Equal(t, "response", m.ui.httpLogState.entries[0].Direction)
}

func TestHandleHTTPLogEntry_UnmatchedResponse_Appended(t *testing.T) {
	m := newModelWithHTTPLog()

	responseEntry := HTTPLogEntry{
		Direction:  "response",
		RequestID:  "no-match-id",
		StatusCode: 404,
	}
	m.handleHTTPLogEntry(HTTPLogEntryMsg{Entry: responseEntry})

	assert.Len(t, m.ui.httpLogState.entries, 1)
}

func TestHandleHTTPLogEntry_CapsAt10000(t *testing.T) {
	m := newModelWithHTTPLog()

	// Fill to 10000.
	entries := make([]HTTPLogEntry, 10000)
	for i := range entries {
		entries[i] = HTTPLogEntry{Method: "GET", Path: "/old"}
	}
	m.ui.httpLogState.entries = entries

	// Add one more.
	m.handleHTTPLogEntry(HTTPLogEntryMsg{Entry: HTTPLogEntry{Method: "POST", Path: "/new"}})

	assert.Len(t, m.ui.httpLogState.entries, 10000)
	// The newest entry should be at the end.
	last := m.ui.httpLogState.entries[len(m.ui.httpLogState.entries)-1]
	assert.Equal(t, "/new", last.Path)
}

// ---- handleContextsLoaded: without discovery (nil) ---------------------

// TestHandleContextsLoaded_NilDiscovery_UsesMessagesDirectly verifies that
// when discovery is nil (no kubeconfig), the contexts from the message are
// used as-is without reordering.
func TestHandleContextsLoaded_NilDiscovery_UsesMessagesDirectly(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	// No discovery set — contexts must still be loaded from the message.
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	ui.mu.Unlock()

	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := ContextsLoadedMsg{
		contexts: []string{"default", "production", "staging"},
	}
	m.handleContextsLoaded(msg)

	ui.mu.RLock()
	defer ui.mu.RUnlock()
	assert.Contains(t, ui.addWizard.contexts, "staging")
	assert.Contains(t, ui.addWizard.contexts, "default")
}

// ---- handlePodsLoaded: edit mode port detection ------------------------

func TestHandlePodsLoaded_EditMode_DetectsPorts(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepEnterRemotePort
	w.isEditing = true
	w.remotePort = 8080
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	pods := []k8s.PodInfo{
		{
			Name: "my-pod",
			Containers: []k8s.ContainerInfo{
				{Name: "main", Ports: []k8s.PortInfo{{Port: 8080}}},
			},
		},
	}
	m.handlePodsLoaded(PodsLoadedMsg{pods: pods})

	ui.mu.RLock()
	assert.NotEmpty(t, ui.addWizard.detectedPorts)
	ui.mu.RUnlock()
}

// ---- handleServicesLoaded: edit mode port detection --------------------

func TestHandleServicesLoaded_EditMode_DetectsPorts(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	w := newAddWizardState()
	w.step = StepEnterRemotePort
	w.isEditing = true
	w.resourceValue = "api-svc"
	w.remotePort = 80
	ui.addWizard = w
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	services := []k8s.ServiceInfo{
		{Name: "api-svc", Ports: []k8s.PortInfo{{Port: 80}}},
	}
	m.handleServicesLoaded(ServicesLoadedMsg{services: services})

	ui.mu.RLock()
	assert.NotEmpty(t, ui.addWizard.detectedPorts)
	ui.mu.RUnlock()
}

// ---- handleForwardSaved: error path ------------------------------------

func TestHandleForwardSaved_Error(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.step = StepConfirmation
	ui.addWizard.loading = true
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := ForwardSavedMsg{success: false, err: assert.AnError}
	m.handleForwardSaved(msg)

	// On failure, step stays at StepConfirmation (not moved to StepSuccess).
	// loading is cleared and error is populated.
	ui.mu.RLock()
	assert.Equal(t, StepConfirmation, ui.addWizard.step)
	assert.NotNil(t, ui.addWizard.error)
	assert.False(t, ui.addWizard.loading)
	ui.mu.RUnlock()
}

// ---- handleSelectorValidated: invalid path ----------------------------

func TestHandleSelectorValidated_Invalid(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeAddWizard
	ui.addWizard = newAddWizardState()
	ui.addWizard.loading = true
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	msg := SelectorValidatedMsg{valid: false, err: assert.AnError}
	m.handleSelectorValidated(msg)

	ui.mu.RLock()
	assert.Nil(t, ui.addWizard.matchingPods)
	assert.NotNil(t, ui.addWizard.error)
	ui.mu.RUnlock()
}

// ---- isFilterableStep --------------------------------------------------

func TestIsFilterableStep(t *testing.T) {
	assert.True(t, isFilterableStep(StepSelectContext))
	assert.True(t, isFilterableStep(StepSelectNamespace))
	assert.True(t, isFilterableStep(StepEnterResource))
	assert.False(t, isFilterableStep(StepEnterRemotePort))
	assert.False(t, isFilterableStep(StepEnterLocalPort))
	assert.False(t, isFilterableStep(StepConfirmation))
	assert.False(t, isFilterableStep(StepSuccess))
}

// ---- clearCopyMessageMsg handling --------------------------------------

func TestModel_Update_ClearCopyMessage(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	ui.mu.Lock()
	ui.viewMode = ViewModeHTTPLog
	ui.httpLogState = newHTTPLogState("fwd-id", "alias")
	ui.httpLogState.copyMessage = "Copied!"
	ui.mu.Unlock()
	m := model{ui: ui, termWidth: 120, termHeight: 40}

	newM, _ := m.Update(clearCopyMessageMsg{})
	updatedUI := newM.(model).ui
	updatedUI.mu.RLock()
	assert.Empty(t, updatedUI.httpLogState.copyMessage)
	updatedUI.mu.RUnlock()
}

// ---- ForwardAddMsg / ForwardUpdateMsg / ForwardRemoveMsg routing --------

func TestModel_Update_ForwardMessages_NoPanic(t *testing.T) {
	m := newTestModelWithForward()

	msgs := []tea.Msg{
		ForwardAddMsg{ID: "x", Forward: &ForwardStatus{}},
		ForwardUpdateMsg{ID: "test-id", Status: "Active"},
		ForwardErrorMsg{ID: "test-id", Error: "boom"},
		ForwardRemoveMsg{ID: "test-id"},
	}

	for _, msg := range msgs {
		assert.NotPanics(t, func() {
			_, _ = m.Update(msg)
		})
	}
}

// ---- SetHTTPLogSubscriber -----------------------------------------------

func TestBubbleTeaUI_SetHTTPLogSubscriber(t *testing.T) {
	ui := NewBubbleTeaUI(nil, "1.0.0")
	mock := NewMockHTTPLogSubscriber()
	ui.SetHTTPLogSubscriber(mock.GetSubscriberFunc())

	ui.mu.RLock()
	assert.NotNil(t, ui.httpLogSubscriber)
	ui.mu.RUnlock()
}

// ---- ResourceType.String / Description ---------------------------------

func TestResourceType_StringAndDescription(t *testing.T) {
	tests := []struct {
		expStr          string
		expDescContains string
		rt              ResourceType
	}{
		{"Pod (by name prefix)", "specific", ResourceTypePodPrefix},
		{"Pod (by label selector)", "survives", ResourceTypePodSelector},
		{"Service", "stable", ResourceTypeService},
		{"Unknown", "", ResourceType(99)},
	}

	for _, tt := range tests {
		t.Run(tt.expStr, func(t *testing.T) {
			assert.Equal(t, tt.expStr, tt.rt.String())
			desc := tt.rt.Description()
			if tt.expDescContains != "" {
				assert.Contains(t, desc, tt.expDescContains)
			}
		})
	}
}

// ---- wizard_commands: saveForwardCmd / removeForwardsCmd / removeForwardByIDCmd
// These use *config.Mutator (concrete type), so we create a real mutator
// backed by a temp config file.

func newTempMutator(t *testing.T) *config.Mutator {
	t.Helper()
	tmpDir := t.TempDir()
	cfgPath := tmpDir + "/.kportal.yaml"
	if err := os.WriteFile(cfgPath, []byte("contexts: []\n"), 0o600); err != nil {
		t.Fatalf("create temp config: %v", err)
	}
	return config.NewMutator(cfgPath)
}

func TestSaveForwardCmd_Success(t *testing.T) {
	mutator := newTempMutator(t)
	fwd := config.Forward{Resource: "pod/app", Port: 80, LocalPort: 18081}
	cmd := saveForwardCmd(mutator, "ctx", "ns", fwd)
	msg := cmd()
	savedMsg, ok := msg.(ForwardSavedMsg)
	require.True(t, ok)
	assert.True(t, savedMsg.success)
}

func TestRemoveForwardsCmd_Success(t *testing.T) {
	mutator := newTempMutator(t)
	// Add two forwards: removing one must leave the context non-empty to pass validation.
	fwd := config.Forward{Resource: "pod/app", Port: 80, LocalPort: 18082}
	fwdKeep := config.Forward{Resource: "pod/app2", Port: 80, LocalPort: 18090}
	require.NoError(t, mutator.AddForward("ctx", "ns", fwd))
	require.NoError(t, mutator.AddForward("ctx", "ns", fwdKeep))

	forwards := []RemovableForward{{ID: "ctx/ns/pod/app:18082", Alias: "app"}}
	cmd := removeForwardsCmd(mutator, forwards)
	msg := cmd()
	removedMsg, ok := msg.(ForwardsRemovedMsg)
	require.True(t, ok)
	assert.True(t, removedMsg.success)
	assert.Equal(t, 1, removedMsg.count)
}

func TestRemoveForwardByIDCmd_Success(t *testing.T) {
	mutator := newTempMutator(t)
	// Add two forwards: removing one must leave the context non-empty to pass validation.
	fwd := config.Forward{Resource: "pod/app", Port: 80, LocalPort: 18083}
	fwdKeep := config.Forward{Resource: "pod/app2", Port: 80, LocalPort: 18091}
	require.NoError(t, mutator.AddForward("ctx", "ns", fwd))
	require.NoError(t, mutator.AddForward("ctx", "ns", fwdKeep))

	id := "ctx/ns/pod/app:18083"
	cmd := removeForwardByIDCmd(mutator, id)
	msg := cmd()
	removedMsg, ok := msg.(ForwardsRemovedMsg)
	require.True(t, ok)
	assert.True(t, removedMsg.success)
}
