package ui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lukaszraczylo/kportal/internal/config"
	"github.com/lukaszraczylo/kportal/internal/k8s"
)

// fakeMutator is a minimal MutatorInterface for tests that don't touch the
// filesystem. It records the order of AddForward calls.
type fakeMutator struct {
	addError error
	added    []config.Forward
	mu       sync.Mutex
}

func (f *fakeMutator) AddForward(ctxName, ns string, fwd config.Forward) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.addError != nil {
		return f.addError
	}
	fwd.SetContext(ctxName, ns)
	f.added = append(f.added, fwd)
	return nil
}

func (f *fakeMutator) RemoveForwards(predicate func(ctx, ns string, fwd config.Forward) bool) error {
	return nil
}
func (f *fakeMutator) RemoveForwardByID(id string) error { return nil }
func (f *fakeMutator) UpdateForward(oldID, newCtx, newNS string, newFwd config.Forward) error {
	return nil
}

// fakeDiscovery is a minimal DiscoveryInterface for tests.
type fakeDiscovery struct {
	servicesByNS     map[string][]k8s.ServiceInfo
	listNamespacesEr error
	listServicesEr   error
	namespaces       []string
}

func (f *fakeDiscovery) ListContexts() ([]string, error)    { return []string{"test"}, nil }
func (f *fakeDiscovery) GetCurrentContext() (string, error) { return "test", nil }
func (f *fakeDiscovery) ListNamespaces(_ context.Context, _ string) ([]string, error) {
	return f.namespaces, f.listNamespacesEr
}
func (f *fakeDiscovery) ListPods(_ context.Context, _, _ string) ([]k8s.PodInfo, error) {
	return nil, nil
}
func (f *fakeDiscovery) ListPodsWithSelector(_ context.Context, _, _, _ string) ([]k8s.PodInfo, error) {
	return nil, nil
}
func (f *fakeDiscovery) ListServices(_ context.Context, _, ns string) ([]k8s.ServiceInfo, error) {
	if f.listServicesEr != nil {
		return nil, f.listServicesEr
	}
	return f.servicesByNS[ns], nil
}

// keyOf builds a tea.KeyMsg the same way bubbletea does for typed runes.
func keyOf(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	case "backspace":
		return tea.KeyMsg{Type: tea.KeyBackspace}
	}
	if len(s) == 1 {
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// drainModel applies a sequence of messages and returns the final model.
func drainModel(t *testing.T, m tea.Model, msgs ...tea.Msg) tea.Model {
	t.Helper()
	cur := m
	for _, msg := range msgs {
		next, _ := cur.Update(msg)
		cur = next
	}
	return cur
}

func TestGenerateModel_NamespaceMultiSelect(t *testing.T) {
	disc := &fakeDiscovery{
		namespaces: []string{"alpha", "beta", "gamma"},
		servicesByNS: map[string][]k8s.ServiceInfo{
			"alpha": {{Name: "svc-a", Namespace: "alpha", Ports: []k8s.PortInfo{{Port: 80, Protocol: "TCP"}}}},
		},
	}
	mut := &fakeMutator{}
	m := NewGenerateModel(disc, mut, "ctx", "/tmp/x.yaml", true, nil)

	// Init (load namespaces)
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected Init to return command")
	}
	// Simulate the namespaces-loaded message.
	model, _ := m.Update(generateNamespacesLoadedMsg{namespaces: disc.namespaces})
	gm := model.(*GenerateModel)

	if gm.loading {
		t.Fatal("expected loading=false after namespaces loaded")
	}
	if len(gm.nsFilteredView) != 3 {
		t.Fatalf("want 3 namespaces, got %d", len(gm.nsFilteredView))
	}

	// Toggle first item with space — cursor starts at 0.
	gm2 := drainModel(t, gm, keyOf("space")).(*GenerateModel)
	if !gm2.nsSelected["alpha"] {
		t.Fatal("expected alpha to be selected")
	}

	// 'a' toggles all. Because alpha is selected and the others are not,
	// allSelected=false so the press selects everything visible.
	gm3 := drainModel(t, gm2, keyOf("a")).(*GenerateModel)
	for _, ns := range []string{"alpha", "beta", "gamma"} {
		if !gm3.nsSelected[ns] {
			t.Fatalf("expected %s to be selected after first toggle-all", ns)
		}
	}
	// Press again — now all are selected, so it should deselect all.
	gm4 := drainModel(t, gm3, keyOf("a")).(*GenerateModel)
	for _, ns := range []string{"alpha", "beta", "gamma"} {
		if gm4.nsSelected[ns] {
			t.Fatalf("expected %s to be unselected after second toggle-all", ns)
		}
	}
}

func TestGenerateModel_NamespaceFilter(t *testing.T) {
	disc := &fakeDiscovery{namespaces: []string{"alpha", "beta", "gamma"}}
	m := NewGenerateModel(disc, &fakeMutator{}, "ctx", "/tmp/x.yaml", true, nil)
	model, _ := m.Update(generateNamespacesLoadedMsg{namespaces: disc.namespaces})
	gm := model.(*GenerateModel)

	// Enter filter mode
	gm = drainModel(t, gm, keyOf("/")).(*GenerateModel)
	if !gm.nsFiltering {
		t.Fatal("expected to enter filter mode")
	}
	gm = drainModel(t, gm, keyOf("b")).(*GenerateModel)
	if gm.nsFilter != "b" {
		t.Fatalf("expected filter=b, got %q", gm.nsFilter)
	}
	if len(gm.nsFilteredView) != 1 || gm.nsFilteredView[0] != "beta" {
		t.Fatalf("expected [beta], got %v", gm.nsFilteredView)
	}
	// Exit filter
	gm = drainModel(t, gm, tea.KeyMsg{Type: tea.KeyEnter}).(*GenerateModel)
	if gm.nsFiltering {
		t.Fatal("expected filtering to be off after enter")
	}
}

func TestGenerateModel_ServiceMultiSelectAndLock(t *testing.T) {
	disc := &fakeDiscovery{
		namespaces: []string{"ns1"},
		servicesByNS: map[string][]k8s.ServiceInfo{
			"ns1": {
				{Name: "svc-a", Namespace: "ns1", Ports: []k8s.PortInfo{{Port: 80, Protocol: "TCP"}, {Port: 443, Protocol: "TCP"}}},
				{Name: "svc-udp", Namespace: "ns1", Ports: []k8s.PortInfo{{Port: 53, Protocol: "UDP"}}},
			},
		},
	}
	// One forward already configured: svc-a:80 in ns1
	existing := []config.Forward{makeFwd("ctx", "ns1", "service/svc-a", 80, 9000, "tcp")}

	m := NewGenerateModel(disc, &fakeMutator{}, "ctx", "/tmp/x.yaml", true, existing)
	// Drive past the namespace step.
	model, _ := m.Update(generateNamespacesLoadedMsg{namespaces: disc.namespaces})
	gm := model.(*GenerateModel)
	gm.nsSelected["ns1"] = true
	// Press enter to advance to services step.
	model2, _ := gm.Update(keyOf("enter"))
	gm2 := model2.(*GenerateModel)
	// Provide the loaded services.
	model3, _ := gm2.Update(generateServicesLoadedMsg{servicesByNS: map[string][]ServiceCandidate{
		"ns1": {
			{Namespace: "ns1", Service: "svc-a", Port: 80, Protocol: "TCP"},
			{Namespace: "ns1", Service: "svc-a", Port: 443, Protocol: "TCP"},
			{Namespace: "ns1", Service: "svc-udp", Port: 53, Protocol: "UDP"},
		},
	}})
	gm3 := model3.(*GenerateModel)

	if len(gm3.svcOrder) != 3 {
		t.Fatalf("want 3 candidates, got %d", len(gm3.svcOrder))
	}
	if !gm3.svcLocked[(ServiceCandidate{Namespace: "ns1", Service: "svc-a", Port: 80, Protocol: "TCP"}).Key()] {
		t.Fatal("svc-a:80 should be locked (already in config)")
	}

	// Move to svc-a:443 (cursor index 1) and toggle.
	gm4 := drainModel(t, gm3, keyOf("down"), keyOf("space")).(*GenerateModel)
	sel := gm4.selectedCandidates()
	if len(sel) != 1 || sel[0].Service != "svc-a" || sel[0].Port != 443 {
		t.Fatalf("expected [svc-a:443], got %v", sel)
	}

	// Try to toggle the locked row (cursor 0) — should remain unselected.
	gm5 := drainModel(t, gm4, keyOf("up"), keyOf("space")).(*GenerateModel)
	for _, c := range gm5.selectedCandidates() {
		if c.Port == 80 {
			t.Fatal("locked row was selectable")
		}
	}

	// Toggle-all should select all selectable (i.e., svc-a:443 only — the others are locked or non-TCP).
	gm6 := drainModel(t, gm5, keyOf("a")).(*GenerateModel)
	// First press: all eligible already selected (svc-a:443) → deselect.
	if len(gm6.selectedCandidates()) != 0 {
		t.Fatalf("expected toggle-all to deselect, got %d", len(gm6.selectedCandidates()))
	}
	gm7 := drainModel(t, gm6, keyOf("a")).(*GenerateModel)
	if len(gm7.selectedCandidates()) != 1 {
		t.Fatalf("expected 1 selected after second toggle-all, got %d", len(gm7.selectedCandidates()))
	}
}

// readyModel returns a model with loading already cleared so step-level
// behaviour can be tested without injecting load messages first.
func readyModel(disc DiscoveryInterface, mut MutatorInterface, ctx, cfg string, dryRun bool, existing []config.Forward) *GenerateModel {
	m := NewGenerateModel(disc, mut, ctx, cfg, dryRun, existing)
	m.loading = false
	return m
}

func TestGenerateModel_PortAssignmentWithCollisions(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".kportal.yaml")
	// Seed an existing config that has localPort 10000 and 10002 already used.
	seed := []byte(`contexts:
  - name: ctx
    namespaces:
      - name: existing
        forwards:
          - resource: service/legacy
            port: 8080
            localPort: 10000
            protocol: tcp
          - resource: service/legacy2
            port: 8080
            localPort: 10002
            protocol: tcp
`)
	if err := os.WriteFile(configPath, seed, 0o600); err != nil {
		t.Fatal(err)
	}

	// Re-load to grab the existing forwards.
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	mut := config.NewMutator(configPath)

	disc := &fakeDiscovery{}
	m := readyModel(disc, mut, "ctx", configPath, false, cfg.GetAllForwards())
	// Pre-populate svcOrder with three candidates that need ports.
	m.svcOrder = []ServiceCandidate{
		{Namespace: "ns1", Service: "alpha", Port: 80, Protocol: "TCP"},
		{Namespace: "ns1", Service: "beta", Port: 80, Protocol: "TCP"},
		{Namespace: "ns1", Service: "gamma", Port: 80, Protocol: "TCP"},
	}
	for _, c := range m.svcOrder {
		m.svcSelected[c.Key()] = true
	}

	planned := m.assignPorts(10000)
	if len(planned) != 3 {
		t.Fatalf("expected 3 planned forwards, got %d", len(planned))
	}
	got := []int{planned[0].LocalPort, planned[1].LocalPort, planned[2].LocalPort}
	want := []int{10001, 10003, 10004} // 10000 and 10002 taken
	for i, p := range want {
		if got[i] != p {
			t.Fatalf("planned[%d] localPort: want %d, got %d (full=%v)", i, p, got[i], got)
		}
	}

	// Now invoke saveCmd through the model and verify mutator side-effects.
	m.startingPortStr = "10000"
	for _, c := range m.svcOrder {
		m.svcSelected[c.Key()] = true
	}
	m.step = GenerateStepPortAssign
	model2, cmd := m.Update(keyOf("enter"))
	if cmd == nil {
		t.Fatal("expected save command")
	}
	msg := cmd()
	saved, ok := msg.(generateSavedMsg)
	if !ok {
		t.Fatalf("expected generateSavedMsg, got %T", msg)
	}
	if saved.added != 3 {
		t.Fatalf("expected 3 added, got %d (errors=%v)", saved.added, saved.errors)
	}

	// Verify config file now has 5 forwards total.
	cfg2, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if len(cfg2.GetAllForwards()) != 5 {
		t.Fatalf("expected 5 forwards after save, got %d", len(cfg2.GetAllForwards()))
	}
	_ = model2
}

func TestGenerateModel_PortBelow1024Rejected(t *testing.T) {
	m := readyModel(&fakeDiscovery{}, &fakeMutator{}, "ctx", "/tmp/x.yaml", true, nil)
	m.step = GenerateStepPortAssign
	m.svcOrder = []ServiceCandidate{{Namespace: "ns1", Service: "x", Port: 80, Protocol: "TCP"}}
	m.svcSelected[m.svcOrder[0].Key()] = true
	m.startingPortStr = "80"

	model, cmd := m.Update(keyOf("enter"))
	gm := model.(*GenerateModel)
	if cmd != nil {
		t.Fatal("expected no command (rejected)")
	}
	if gm.portError == "" {
		t.Fatal("expected port error to be set")
	}
	if gm.step != GenerateStepPortAssign {
		t.Fatal("expected to remain on port step after invalid input")
	}

	// Backspace + retype a valid value should clear the error and allow continuing.
	gm.startingPortStr = "1024"
	model2, cmd2 := gm.Update(keyOf("enter"))
	gm2 := model2.(*GenerateModel)
	if cmd2 == nil {
		t.Fatal("expected save command after valid port")
	}
	if gm2.portError != "" {
		t.Fatalf("expected port error cleared, got %q", gm2.portError)
	}
}

func TestGenerateModel_DryRunDoesNotInvokeMutator(t *testing.T) {
	mut := &fakeMutator{}
	m := readyModel(&fakeDiscovery{}, mut, "ctx", "/tmp/x.yaml", true, nil)
	m.step = GenerateStepPortAssign
	m.svcOrder = []ServiceCandidate{{Namespace: "ns1", Service: "x", Port: 80, Protocol: "TCP"}}
	m.svcSelected[m.svcOrder[0].Key()] = true
	m.startingPortStr = "10000"

	model, cmd := m.Update(keyOf("enter"))
	gm := model.(*GenerateModel)
	if !gm.result.UsedDryRun {
		t.Fatal("expected dry-run flag set in result")
	}
	if len(mut.added) != 0 {
		t.Fatalf("expected mutator untouched in dry-run, got %d adds", len(mut.added))
	}
	if cmd == nil {
		t.Fatal("expected quit command from dry-run path")
	}
	if msg := cmd(); msg == nil {
		// Quit returns a tea.QuitMsg — just ensure it's non-nil.
		t.Fatal("expected non-nil quit message")
	}
}

func TestGenerateModel_EndToEnd(t *testing.T) {
	disc := &fakeDiscovery{
		namespaces: []string{"ns1"},
	}
	mut := &fakeMutator{}
	m := NewGenerateModel(disc, mut, "ctx", "/tmp/x.yaml", false, nil)

	// Init returns a Cmd; we don't run it directly. Instead we manually
	// inject the messages it would produce.
	_ = m.Init()

	// 1. Namespaces load.
	model, _ := m.Update(generateNamespacesLoadedMsg{namespaces: disc.namespaces})
	gm := model.(*GenerateModel)

	// 2. Toggle ns1 + enter.
	gm = drainModel(t, gm, keyOf("space"), keyOf("enter")).(*GenerateModel)
	if gm.step != GenerateStepServices {
		t.Fatalf("expected services step, got %v", gm.step)
	}

	// 3. Provide loaded services.
	model2, _ := gm.Update(generateServicesLoadedMsg{servicesByNS: map[string][]ServiceCandidate{
		"ns1": {{Namespace: "ns1", Service: "svc", Port: 8080, Protocol: "TCP"}},
	}})
	gm = model2.(*GenerateModel)

	// 4. Toggle the (only) service + enter.
	gm = drainModel(t, gm, keyOf("space"), keyOf("enter")).(*GenerateModel)
	if gm.step != GenerateStepPortAssign {
		t.Fatalf("expected port-assign step, got %v", gm.step)
	}

	// 5. Press enter on the default port (10000).
	model3, cmd := gm.Update(keyOf("enter"))
	gm = model3.(*GenerateModel)
	if cmd == nil {
		t.Fatal("expected save command")
	}
	msg := cmd()
	saved := msg.(generateSavedMsg)
	if saved.added != 1 {
		t.Fatalf("expected 1 added, got %d (errs=%v)", saved.added, saved.errors)
	}

	// 6. Process the saved message → step should be Done.
	model4, _ := gm.Update(saved)
	final := model4.(*GenerateModel)
	if final.step != GenerateStepDone {
		t.Fatalf("expected Done step, got %v", final.step)
	}
	if final.result.Added != 1 {
		t.Fatalf("expected result.Added=1, got %d", final.result.Added)
	}
	if len(mut.added) != 1 {
		t.Fatalf("expected mutator to record 1 forward, got %d", len(mut.added))
	}
	if mut.added[0].Resource != "service/svc" || mut.added[0].LocalPort != 10000 {
		t.Fatalf("unexpected forward recorded: %+v", mut.added[0])
	}
}

// makeFwd is a small helper to build a Forward with context/namespace pre-set.
func makeFwd(ctxName, ns, resource string, port, localPort int, proto string) config.Forward {
	f := config.Forward{
		Resource:  resource,
		Port:      port,
		LocalPort: localPort,
		Protocol:  proto,
	}
	f.SetContext(ctxName, ns)
	return f
}

func TestGenerateModel_ParseStartingPortBoundary(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantOK  bool
		wantVal int
	}{
		{"empty", "", false, 0},
		{"non-numeric", "abc", false, 0},
		{"below min", "1023", false, 0},
		{"at min", "1024", true, 1024},
		{"above max", "70000", false, 0},
		{"valid", "10000", true, 10000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewGenerateModel(&fakeDiscovery{}, &fakeMutator{}, "c", "/tmp/x.yaml", true, nil)
			m.startingPortStr = tc.input
			got, ok := m.parseStartingPort()
			if ok != tc.wantOK {
				t.Fatalf("ok mismatch: want %v, got %v (err=%q)", tc.wantOK, ok, m.portError)
			}
			if ok && got != tc.wantVal {
				t.Fatalf("val mismatch: want %d, got %d", tc.wantVal, got)
			}
		})
	}
}

// TestGenerateModel_PortStepView ensures the port-step view renders without panic.
func TestGenerateModel_PortStepView(t *testing.T) {
	m := readyModel(&fakeDiscovery{}, &fakeMutator{}, "ctx", "/tmp/x.yaml", true, nil)
	m.step = GenerateStepPortAssign
	m.svcOrder = []ServiceCandidate{{Namespace: "ns", Service: "svc", Port: 80, Protocol: "TCP"}}
	m.svcSelected[m.svcOrder[0].Key()] = true
	view := m.View()
	if !contains(view, "Step 3 / 3") {
		t.Fatalf("expected step header in view, got: %s", view)
	}
	if !contains(view, "10000") {
		t.Fatalf("expected default port in view, got: %s", view)
	}
}

// contains is a tiny strings.Contains wrapper that also gives a clearer test failure message.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (sub == "" || stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Sanity: ensure the model satisfies tea.Model interface — compile-time check.
var _ tea.Model = (*GenerateModel)(nil)

// Sanity: ensure existing key generation matches a manually-built one.
func TestServiceCandidate_KeyDeterministic(t *testing.T) {
	c := ServiceCandidate{Namespace: "ns1", Service: "svc", Port: 80, Protocol: "TCP"}
	want := fmt.Sprintf("%s|%s|%s|%d", "ns1", "service/svc", "tcp", 80)
	if c.Key() != want {
		t.Fatalf("Key() mismatch: want %q, got %q", want, c.Key())
	}
}
