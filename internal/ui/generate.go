package ui

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lukaszraczylo/kportal/internal/config"
)

// Generate flow constants
const (
	// GenerateMinLocalPort is the minimum allowed starting local port for generated forwards.
	// Ports below 1024 are reserved on most systems and require elevated privileges.
	GenerateMinLocalPort = 1024

	// GenerateMaxLocalPort is the maximum valid TCP port number.
	GenerateMaxLocalPort = 65535

	// GenerateDefaultStartingPort is the default starting local port.
	GenerateDefaultStartingPort = 10000

	// GenerateListTimeout is the per-step timeout for k8s list operations.
	GenerateListTimeout = 30 * time.Second

	// GenerateConcurrency is the maximum number of concurrent ListServices calls.
	GenerateConcurrency = 8
)

// GenerateStep represents the current step in the generate flow.
type GenerateStep int

const (
	GenerateStepNamespaces GenerateStep = iota
	GenerateStepServices
	GenerateStepPortAssign
	GenerateStepDone
	GenerateStepCancelled
)

// generateNamespacesLoadedMsg is fired when namespace listing completes.
type generateNamespacesLoadedMsg struct {
	err        error
	namespaces []string
}

// generateServicesLoadedMsg is fired when concurrent service listing completes.
type generateServicesLoadedMsg struct {
	err          error
	servicesByNS map[string][]ServiceCandidate
}

// generateSavedMsg is fired after AddForward calls complete.
type generateSavedMsg struct {
	errors []string
	added  int
}

// generateTickMsg drives the spinner.
type generateTickMsg struct{}

// ServiceCandidate represents a single service-port row in the generate flow.
type ServiceCandidate struct {
	Namespace string
	Service   string
	Protocol  string
	Port      int32
}

// Key returns a stable lookup key for collision detection against existing config.
func (c ServiceCandidate) Key() string {
	return fmt.Sprintf("%s|%s|%s|%d", c.Namespace, "service/"+c.Service, "tcp", c.Port)
}

// GenerateResult is reported by GenerateModel after the program exits.
type GenerateResult struct {
	Errors          []string
	PlannedForwards []config.Forward
	Added           int
	SkippedNonTCP   int
	Cancelled       bool
	UsedDryRun      bool
}

// GenerateModel is the bubbletea model driving the generate flow.
//
// Field ordering is governed by govet's fieldalignment check: interfaces and
// other 16-byte values come first, then 8-byte pointers/maps/slices/strings,
// followed by ints and finally bools.
type GenerateModel struct {
	// 16-byte interfaces
	discovery DiscoveryInterface
	mutator   MutatorInterface

	// Pointers/maps/slices/strings (8-byte aligned, header sizes vary)
	existingKeys       map[string]struct{}
	existingLocalPorts map[int]struct{}
	nsSelected         map[string]bool
	servicesByNS       map[string][]ServiceCandidate
	svcSelected        map[string]bool
	svcLocked          map[string]bool

	namespaces      []string
	nsFilteredView  []string
	svcOrder        []ServiceCandidate
	svcFilteredView []ServiceCandidate

	contextName     string
	configPath      string
	loadErr         string
	nsFilter        string
	svcFilter       string
	startingPortStr string
	portError       string

	// Composite result struct
	result GenerateResult

	// Ints
	step         GenerateStep
	spinnerFrame int
	nsCursor     int
	nsScroll     int
	svcCursor    int
	svcScroll    int
	termWidth    int
	termHeight   int

	// Bools last (smallest alignment)
	dryRun       bool
	loading      bool
	nsFiltering  bool
	svcFiltering bool
}

// NewGenerateModel constructs a fresh generate model.
// existingForwards is the slice from config.Config.GetAllForwards() and is used
// for both collision detection and to mark already-configured rows as locked.
func NewGenerateModel(
	discovery DiscoveryInterface,
	mutator MutatorInterface,
	contextName string,
	configPath string,
	dryRun bool,
	existingForwards []config.Forward,
) *GenerateModel {
	keys := make(map[string]struct{}, len(existingForwards))
	ports := make(map[int]struct{}, len(existingForwards))
	for _, f := range existingForwards {
		// Only track entries from the same context — collisions across contexts
		// matter for local port assignment but not for "already configured" lock.
		if f.GetContext() == contextName {
			k := fmt.Sprintf("%s|%s|%s|%d", f.GetNamespace(), f.Resource, strings.ToLower(f.Protocol), f.Port)
			keys[k] = struct{}{}
		}
		// Local-port collisions span the whole config file.
		ports[f.LocalPort] = struct{}{}
	}

	return &GenerateModel{
		discovery:          discovery,
		mutator:            mutator,
		contextName:        contextName,
		configPath:         configPath,
		dryRun:             dryRun,
		existingKeys:       keys,
		existingLocalPorts: ports,
		step:               GenerateStepNamespaces,
		loading:            true,
		nsSelected:         map[string]bool{},
		svcSelected:        map[string]bool{},
		svcLocked:          map[string]bool{},
		startingPortStr:    strconv.Itoa(GenerateDefaultStartingPort),
		termWidth:          DefaultTermWidth,
		termHeight:         DefaultTermHeight,
	}
}

// Init returns the initial command (load namespaces).
func (m *GenerateModel) Init() tea.Cmd {
	return tea.Batch(
		m.loadNamespacesCmd(),
		tickCmd(),
	)
}

// Result exposes the final outcome after the program quits.
func (m *GenerateModel) Result() GenerateResult { return m.result }

func tickCmd() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return generateTickMsg{} })
}

// ---------- Commands ----------

func (m *GenerateModel) loadNamespacesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), GenerateListTimeout)
		defer cancel()
		ns, err := m.discovery.ListNamespaces(ctx, m.contextName)
		if err == nil {
			sort.Strings(ns)
		}
		return generateNamespacesLoadedMsg{namespaces: ns, err: err}
	}
}

func (m *GenerateModel) loadServicesCmd(namespaces []string) tea.Cmd {
	return func() tea.Msg {
		out := make(map[string][]ServiceCandidate, len(namespaces))
		var (
			mu   sync.Mutex
			wg   sync.WaitGroup
			sem  = make(chan struct{}, GenerateConcurrency)
			errs []string
		)
		for _, ns := range namespaces {
			wg.Add(1)
			sem <- struct{}{}
			go func(ns string) {
				defer wg.Done()
				defer func() { <-sem }()
				ctx, cancel := context.WithTimeout(context.Background(), GenerateListTimeout)
				defer cancel()
				svcs, err := m.discovery.ListServices(ctx, m.contextName, ns)
				if err != nil {
					mu.Lock()
					errs = append(errs, fmt.Sprintf("%s: %v", ns, err))
					mu.Unlock()
					return
				}
				rows := make([]ServiceCandidate, 0, len(svcs))
				for _, s := range svcs {
					for _, p := range s.Ports {
						proto := strings.ToUpper(p.Protocol)
						if proto == "" {
							proto = "TCP"
						}
						rows = append(rows, ServiceCandidate{
							Namespace: s.Namespace,
							Service:   s.Name,
							Port:      p.Port,
							Protocol:  proto,
						})
					}
				}
				mu.Lock()
				out[ns] = rows
				mu.Unlock()
			}(ns)
		}
		wg.Wait()
		var combinedErr error
		if len(errs) > 0 {
			combinedErr = fmt.Errorf("failed to list services in %d namespaces: %s", len(errs), strings.Join(errs, "; "))
		}
		return generateServicesLoadedMsg{servicesByNS: out, err: combinedErr}
	}
}

func (m *GenerateModel) saveCmd(forwards []config.Forward) tea.Cmd {
	return func() tea.Msg {
		var errs []string
		added := 0
		for _, f := range forwards {
			if err := m.mutator.AddForward(f.GetContext(), f.GetNamespace(), f); err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s/%s:%d: %v", f.GetContext(), f.GetNamespace(), f.Resource, f.Port, err))
				// Continue trying remaining ones — but spec says stop on first error.
				// Spec: "Stop on the first error and report which ones succeeded vs failed".
				return generateSavedMsg{added: added, errors: errs}
			}
			added++
		}
		return generateSavedMsg{added: added, errors: errs}
	}
}

// ---------- Update ----------

// Update implements tea.Model.
func (m *GenerateModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.termWidth = msg.Width
		m.termHeight = msg.Height
		return m, nil

	case generateTickMsg:
		m.spinnerFrame++
		if m.loading {
			return m, tickCmd()
		}
		return m, nil

	case generateNamespacesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err.Error()
			return m, nil
		}
		m.namespaces = msg.namespaces
		m.recomputeNamespaceFilter()
		return m, nil

	case generateServicesLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.loadErr = msg.err.Error()
		}
		m.servicesByNS = msg.servicesByNS
		m.buildServiceOrder()
		m.recomputeServiceFilter()
		return m, nil

	case generateSavedMsg:
		m.result.Added = msg.added
		m.result.Errors = msg.errors
		m.step = GenerateStepDone
		return m, tea.Quit

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *GenerateModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		// Allow only ctrl+c / esc while loading
		switch msg.String() {
		case "ctrl+c", "esc":
			m.step = GenerateStepCancelled
			m.result.Cancelled = true
			return m, tea.Quit
		}
		return m, nil
	}

	switch m.step {
	case GenerateStepNamespaces:
		return m.handleNamespaceKey(msg)
	case GenerateStepServices:
		return m.handleServiceKey(msg)
	case GenerateStepPortAssign:
		return m.handlePortKey(msg)
	}
	return m, nil
}

// ---------- Namespace step ----------

func (m *GenerateModel) handleNamespaceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.nsFiltering {
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEsc:
			m.nsFiltering = false
			if msg.Type == tea.KeyEsc {
				m.nsFilter = ""
				m.recomputeNamespaceFilter()
			}
			return m, nil
		case tea.KeyBackspace:
			if len(m.nsFilter) > 0 {
				m.nsFilter = m.nsFilter[:len(m.nsFilter)-1]
				m.recomputeNamespaceFilter()
			}
			return m, nil
		case tea.KeyRunes, tea.KeySpace:
			m.nsFilter += string(msg.Runes)
			m.recomputeNamespaceFilter()
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		m.step = GenerateStepCancelled
		m.result.Cancelled = true
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(&m.nsCursor, &m.nsScroll, len(m.nsFilteredView), -1)
	case "down", "j":
		m.moveCursor(&m.nsCursor, &m.nsScroll, len(m.nsFilteredView), 1)
	case "pgup":
		m.moveCursor(&m.nsCursor, &m.nsScroll, len(m.nsFilteredView), -10)
	case "pgdown":
		m.moveCursor(&m.nsCursor, &m.nsScroll, len(m.nsFilteredView), 10)
	case " ":
		if len(m.nsFilteredView) > 0 {
			ns := m.nsFilteredView[m.nsCursor]
			m.nsSelected[ns] = !m.nsSelected[ns]
		}
	case "a":
		m.toggleAllNamespaces()
	case "/":
		m.nsFiltering = true
	case "enter":
		selected := m.selectedNamespaces()
		if len(selected) == 0 {
			return m, nil
		}
		m.step = GenerateStepServices
		m.loading = true
		m.loadErr = ""
		return m, tea.Batch(m.loadServicesCmd(selected), tickCmd())
	}
	return m, nil
}

func (m *GenerateModel) recomputeNamespaceFilter() {
	m.nsFilteredView = filterStrings(m.namespaces, m.nsFilter)
	if m.nsCursor >= len(m.nsFilteredView) {
		m.nsCursor = max(0, len(m.nsFilteredView)-1)
	}
	if m.nsScroll > m.nsCursor {
		m.nsScroll = m.nsCursor
	}
}

func (m *GenerateModel) toggleAllNamespaces() {
	// If everything visible is selected, deselect; otherwise select all visible.
	allSelected := true
	for _, ns := range m.nsFilteredView {
		if !m.nsSelected[ns] {
			allSelected = false
			break
		}
	}
	for _, ns := range m.nsFilteredView {
		m.nsSelected[ns] = !allSelected
	}
}

func (m *GenerateModel) selectedNamespaces() []string {
	out := make([]string, 0, len(m.nsSelected))
	for ns, sel := range m.nsSelected {
		if sel {
			out = append(out, ns)
		}
	}
	sort.Strings(out)
	return out
}

// ---------- Services step ----------

func (m *GenerateModel) buildServiceOrder() {
	rows := make([]ServiceCandidate, 0)
	for _, list := range m.servicesByNS {
		rows = append(rows, list...)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Namespace != rows[j].Namespace {
			return rows[i].Namespace < rows[j].Namespace
		}
		if rows[i].Service != rows[j].Service {
			return rows[i].Service < rows[j].Service
		}
		return rows[i].Port < rows[j].Port
	})
	m.svcOrder = rows
	m.svcLocked = make(map[string]bool, len(rows))
	for _, r := range rows {
		// Use TCP-canonical key for matching against config (config keeps lowercase tcp).
		canonical := fmt.Sprintf("%s|%s|%s|%d", r.Namespace, "service/"+r.Service, "tcp", r.Port)
		if _, found := m.existingKeys[canonical]; found {
			m.svcLocked[r.Key()] = true
		}
	}
}

func (m *GenerateModel) handleServiceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.svcFiltering {
		switch msg.Type {
		case tea.KeyEnter, tea.KeyEsc:
			m.svcFiltering = false
			if msg.Type == tea.KeyEsc {
				m.svcFilter = ""
				m.recomputeServiceFilter()
			}
			return m, nil
		case tea.KeyBackspace:
			if len(m.svcFilter) > 0 {
				m.svcFilter = m.svcFilter[:len(m.svcFilter)-1]
				m.recomputeServiceFilter()
			}
			return m, nil
		case tea.KeyRunes, tea.KeySpace:
			m.svcFilter += string(msg.Runes)
			m.recomputeServiceFilter()
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "esc":
		m.step = GenerateStepCancelled
		m.result.Cancelled = true
		return m, tea.Quit
	case "b":
		m.step = GenerateStepNamespaces
		m.loadErr = ""
	case "up", "k":
		m.moveCursor(&m.svcCursor, &m.svcScroll, len(m.svcFilteredView), -1)
	case "down", "j":
		m.moveCursor(&m.svcCursor, &m.svcScroll, len(m.svcFilteredView), 1)
	case "pgup":
		m.moveCursor(&m.svcCursor, &m.svcScroll, len(m.svcFilteredView), -10)
	case "pgdown":
		m.moveCursor(&m.svcCursor, &m.svcScroll, len(m.svcFilteredView), 10)
	case " ":
		if len(m.svcFilteredView) > 0 {
			c := m.svcFilteredView[m.svcCursor]
			if !m.svcLocked[c.Key()] && c.Protocol == "TCP" {
				m.svcSelected[c.Key()] = !m.svcSelected[c.Key()]
			}
		}
	case "a":
		m.toggleAllServices()
	case "/":
		m.svcFiltering = true
	case "enter":
		selected := m.selectedCandidates()
		if len(selected) == 0 {
			return m, nil
		}
		m.step = GenerateStepPortAssign
		m.portError = ""
	}
	return m, nil
}

func (m *GenerateModel) recomputeServiceFilter() {
	if m.svcFilter == "" {
		m.svcFilteredView = m.svcOrder
	} else {
		needle := strings.ToLower(m.svcFilter)
		out := make([]ServiceCandidate, 0, len(m.svcOrder))
		for _, c := range m.svcOrder {
			label := fmt.Sprintf("%s/%s:%d", c.Namespace, c.Service, c.Port)
			if strings.Contains(strings.ToLower(label), needle) {
				out = append(out, c)
			}
		}
		m.svcFilteredView = out
	}
	if m.svcCursor >= len(m.svcFilteredView) {
		m.svcCursor = max(0, len(m.svcFilteredView)-1)
	}
	if m.svcScroll > m.svcCursor {
		m.svcScroll = m.svcCursor
	}
}

func (m *GenerateModel) toggleAllServices() {
	allSelected := true
	for _, c := range m.svcFilteredView {
		if m.svcLocked[c.Key()] || c.Protocol != "TCP" {
			continue
		}
		if !m.svcSelected[c.Key()] {
			allSelected = false
			break
		}
	}
	for _, c := range m.svcFilteredView {
		if m.svcLocked[c.Key()] || c.Protocol != "TCP" {
			continue
		}
		m.svcSelected[c.Key()] = !allSelected
	}
}

func (m *GenerateModel) selectedCandidates() []ServiceCandidate {
	out := make([]ServiceCandidate, 0)
	for _, c := range m.svcOrder {
		if m.svcLocked[c.Key()] || c.Protocol != "TCP" {
			continue
		}
		if m.svcSelected[c.Key()] {
			out = append(out, c)
		}
	}
	return out
}

// ---------- Port assignment step ----------

func (m *GenerateModel) handlePortKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.step = GenerateStepCancelled
		m.result.Cancelled = true
		return m, tea.Quit
	case "esc", "b":
		m.step = GenerateStepServices
		m.portError = ""
		return m, nil
	case "backspace":
		if len(m.startingPortStr) > 0 {
			m.startingPortStr = m.startingPortStr[:len(m.startingPortStr)-1]
			m.portError = ""
		}
		return m, nil
	case "enter":
		start, ok := m.parseStartingPort()
		if !ok {
			return m, nil
		}
		forwards := m.assignPorts(start)
		m.result.PlannedForwards = forwards
		m.result.SkippedNonTCP = m.countSkippedNonTCP()
		if m.dryRun {
			m.step = GenerateStepDone
			m.result.UsedDryRun = true
			m.result.Added = 0
			return m, tea.Quit
		}
		return m, m.saveCmd(forwards)
	}

	// Digit-only input
	for _, r := range msg.Runes {
		if r >= '0' && r <= '9' && len(m.startingPortStr) < 5 {
			m.startingPortStr += string(r)
			m.portError = ""
		}
	}
	return m, nil
}

func (m *GenerateModel) parseStartingPort() (int, bool) {
	if m.startingPortStr == "" {
		m.portError = "Starting port is required"
		return 0, false
	}
	v, err := strconv.Atoi(m.startingPortStr)
	if err != nil {
		m.portError = "Starting port must be a number"
		return 0, false
	}
	if v < GenerateMinLocalPort {
		m.portError = fmt.Sprintf("Starting port must be ≥ %d (privileged ports are not allowed)", GenerateMinLocalPort)
		return 0, false
	}
	if v > GenerateMaxLocalPort {
		m.portError = fmt.Sprintf("Starting port must be ≤ %d", GenerateMaxLocalPort)
		return 0, false
	}
	m.portError = ""
	return v, true
}

// assignPorts computes the planned forwards with collision-free local ports.
// Stable order: sort by namespace, then service, then port.
func (m *GenerateModel) assignPorts(start int) []config.Forward {
	candidates := m.selectedCandidates()
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Namespace != candidates[j].Namespace {
			return candidates[i].Namespace < candidates[j].Namespace
		}
		if candidates[i].Service != candidates[j].Service {
			return candidates[i].Service < candidates[j].Service
		}
		return candidates[i].Port < candidates[j].Port
	})

	taken := make(map[int]struct{}, len(m.existingLocalPorts))
	for p := range m.existingLocalPorts {
		taken[p] = struct{}{}
	}

	out := make([]config.Forward, 0, len(candidates))
	candidate := start
	for _, c := range candidates {
		// Walk forward while the port is taken. Stop if we run out of ports.
		for _, used := taken[candidate]; used && candidate <= GenerateMaxLocalPort; _, used = taken[candidate] {
			candidate++
		}
		if candidate > GenerateMaxLocalPort {
			// Out of ports — bail; the save step will fail with a clear validation error.
			break
		}
		f := config.Forward{
			Resource:  "service/" + c.Service,
			Port:      int(c.Port),
			LocalPort: candidate,
			Protocol:  "tcp",
			Alias:     c.Service,
		}
		f.SetContext(m.contextName, c.Namespace)
		out = append(out, f)
		taken[candidate] = struct{}{}
		candidate++
	}
	return out
}

func (m *GenerateModel) countSkippedNonTCP() int {
	n := 0
	for _, c := range m.svcOrder {
		if c.Protocol != "TCP" {
			n++
		}
	}
	return n
}

// ---------- View ----------

// View implements tea.Model.
func (m *GenerateModel) View() string {
	var b strings.Builder
	b.WriteString(wizardHeaderStyle.Render(fmt.Sprintf("kportal generate · context: %s", m.contextName)))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("config: %s", m.configPath)))
	if m.dryRun {
		b.WriteString("  ")
		b.WriteString(warningStyle.Render("[dry-run]"))
	}
	b.WriteString("\n\n")

	if m.loading {
		b.WriteString(spinnerStyle.Render(spinnerFrame(m.spinnerFrame)))
		b.WriteString(" Loading from cluster…\n")
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("esc: cancel"))
		return b.String()
	}

	if m.loadErr != "" && m.step == GenerateStepNamespaces {
		b.WriteString(errorStyle.Render("Error: "))
		b.WriteString(m.loadErr)
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("esc/ctrl+c: exit"))
		return b.String()
	}

	switch m.step {
	case GenerateStepNamespaces:
		b.WriteString(m.renderNamespaceStep())
	case GenerateStepServices:
		b.WriteString(m.renderServiceStep())
	case GenerateStepPortAssign:
		b.WriteString(m.renderPortStep())
	}
	return b.String()
}

func (m *GenerateModel) renderNamespaceStep() string {
	var b strings.Builder
	b.WriteString(breadcrumbStyle.Render("Step 1 / 3 · Select namespaces"))
	b.WriteString("\n")
	if m.nsFiltering {
		b.WriteString(mutedStyle.Render("filter: "))
		b.WriteString(inputStyle.Render(m.nsFilter + "█"))
		b.WriteString("\n")
	} else if m.nsFilter != "" {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("filter: %q (press / to edit, esc to clear)", m.nsFilter)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(m.nsFilteredView) == 0 {
		b.WriteString(mutedStyle.Render("(no namespaces match)\n"))
	} else {
		end := m.nsScroll + ViewportHeight
		if end > len(m.nsFilteredView) {
			end = len(m.nsFilteredView)
		}
		for i := m.nsScroll; i < end; i++ {
			ns := m.nsFilteredView[i]
			cursor := "  "
			if i == m.nsCursor {
				cursor = selectedStyle.Render("▸ ")
			}
			box := uncheckedBoxStyle.Render("[ ]")
			if m.nsSelected[ns] {
				box = checkedBoxStyle.Render("[x]")
			}
			line := fmt.Sprintf("%s%s %s", cursor, box, ns)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	selected := m.selectedNamespaces()
	b.WriteString(mutedStyle.Render(fmt.Sprintf("%d selected", len(selected))))
	b.WriteString("\n")
	help := "↑/↓: move  space: toggle  a: toggle-all  /: filter  enter: continue  esc: cancel"
	b.WriteString(wrapHelpText(help, wizardHelpWidth(m.termWidth)))
	return b.String()
}

func (m *GenerateModel) renderServiceStep() string {
	var b strings.Builder
	b.WriteString(breadcrumbStyle.Render("Step 2 / 3 · Select services"))
	b.WriteString("\n")
	if m.loadErr != "" {
		b.WriteString(warningStyle.Render("warning: " + m.loadErr))
		b.WriteString("\n")
	}
	if m.svcFiltering {
		b.WriteString(mutedStyle.Render("filter: "))
		b.WriteString(inputStyle.Render(m.svcFilter + "█"))
		b.WriteString("\n")
	} else if m.svcFilter != "" {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("filter: %q", m.svcFilter)))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(m.svcFilteredView) == 0 {
		b.WriteString(mutedStyle.Render("(no services found)\n"))
	} else {
		end := m.svcScroll + ViewportHeight
		if end > len(m.svcFilteredView) {
			end = len(m.svcFilteredView)
		}
		for i := m.svcScroll; i < end; i++ {
			c := m.svcFilteredView[i]
			cursor := "  "
			if i == m.svcCursor {
				cursor = selectedStyle.Render("▸ ")
			}
			locked := m.svcLocked[c.Key()]
			nonTCP := c.Protocol != "TCP"
			box := uncheckedBoxStyle.Render("[ ]")
			switch {
			case locked:
				box = mutedStyle.Render("[~]")
			case nonTCP:
				box = mutedStyle.Render("[!]")
			case m.svcSelected[c.Key()]:
				box = checkedBoxStyle.Render("[x]")
			}
			label := fmt.Sprintf("%s/%s:%d", c.Namespace, c.Service, c.Port)
			if c.Protocol != "TCP" {
				label += fmt.Sprintf(" (%s)", c.Protocol)
			}
			suffix := ""
			if locked {
				suffix = " " + mutedStyle.Render("(already configured)")
			} else if nonTCP {
				suffix = " " + mutedStyle.Render("(non-TCP, skipped)")
			}
			line := fmt.Sprintf("%s%s %s%s", cursor, box, label, suffix)
			if locked || nonTCP {
				line = mutedStyle.Render(fmt.Sprintf("%s%s %s%s", cursor, box, label, suffix))
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	sel := m.selectedCandidates()
	b.WriteString(mutedStyle.Render(fmt.Sprintf("%d selected", len(sel))))
	b.WriteString("\n")
	help := "↑/↓: move  space: toggle  a: toggle-all  /: filter  enter: continue  b: back  esc: cancel"
	b.WriteString(wrapHelpText(help, wizardHelpWidth(m.termWidth)))
	return b.String()
}

func (m *GenerateModel) renderPortStep() string {
	var b strings.Builder
	b.WriteString(breadcrumbStyle.Render("Step 3 / 3 · Assign local ports"))
	b.WriteString("\n\n")
	b.WriteString(renderTextInput("Starting local port: ", m.startingPortStr, m.portError == ""))
	b.WriteString("\n")
	if m.portError != "" {
		b.WriteString(errorStyle.Render(m.portError))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	start, ok := m.previewStartingPort()
	if ok {
		preview := m.assignPorts(start)
		b.WriteString(mutedStyle.Render(fmt.Sprintf("Preview (%d forwards):", len(preview))))
		b.WriteString("\n")
		max := ViewportHeight
		if len(preview) < max {
			max = len(preview)
		}
		for i := 0; i < max; i++ {
			f := preview[i]
			line := fmt.Sprintf("  %d → %s/%s/%s:%d", f.LocalPort, f.GetContext(), f.GetNamespace(), f.Resource, f.Port)
			b.WriteString(line)
			b.WriteString("\n")
		}
		if len(preview) > max {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  … %d more not shown", len(preview)-max)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	help := "type digits to set port  enter: save  esc/b: back  ctrl+c: cancel"
	if m.dryRun {
		help = "type digits to set port  enter: preview & exit (dry-run)  esc/b: back  ctrl+c: cancel"
	}
	b.WriteString(wrapHelpText(help, wizardHelpWidth(m.termWidth)))
	return b.String()
}

// previewStartingPort attempts to parse the starting port for preview rendering.
// Unlike parseStartingPort, it does not mutate model state.
func (m *GenerateModel) previewStartingPort() (int, bool) {
	if m.startingPortStr == "" {
		return 0, false
	}
	v, err := strconv.Atoi(m.startingPortStr)
	if err != nil {
		return 0, false
	}
	if v < GenerateMinLocalPort || v > GenerateMaxLocalPort {
		return 0, false
	}
	return v, true
}

// ---------- Helpers ----------

func (m *GenerateModel) moveCursor(cursor, scroll *int, total, delta int) {
	if total == 0 {
		*cursor = 0
		*scroll = 0
		return
	}
	*cursor += delta
	if *cursor < 0 {
		*cursor = 0
	}
	if *cursor >= total {
		*cursor = total - 1
	}
	if *cursor < *scroll {
		*scroll = *cursor
	}
	if *cursor >= *scroll+ViewportHeight {
		*scroll = *cursor - ViewportHeight + 1
	}
}

func spinnerFrame(i int) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[i%len(frames)]
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// RunGenerate runs the generate flow as a bubbletea program and returns the
// final result. The discovery and mutator are passed as interfaces so tests
// can inject fakes.
func RunGenerate(
	discovery DiscoveryInterface,
	mutator MutatorInterface,
	contextName string,
	configPath string,
	dryRun bool,
	existingForwards []config.Forward,
) (GenerateResult, error) {
	m := NewGenerateModel(discovery, mutator, contextName, configPath, dryRun, existingForwards)
	prog := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := prog.Run()
	if err != nil {
		return GenerateResult{}, err
	}
	if gm, ok := finalModel.(*GenerateModel); ok {
		return gm.Result(), nil
	}
	return m.Result(), nil
}
