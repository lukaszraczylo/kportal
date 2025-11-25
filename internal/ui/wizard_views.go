package ui

import (
	"fmt"
	"strings"
)

// renderAddWizard renders the appropriate step of the add wizard
func (m model) renderAddWizard() string {
	if m.ui.addWizard == nil {
		return ""
	}

	wizard := m.ui.addWizard

	var content string
	switch wizard.step {
	case StepSelectContext:
		content = m.renderSelectContext()
	case StepSelectNamespace:
		content = m.renderSelectNamespace()
	case StepSelectResourceType:
		content = m.renderSelectResourceType()
	case StepEnterResource:
		content = m.renderEnterResource()
	case StepEnterRemotePort:
		content = m.renderEnterRemotePort()
	case StepEnterLocalPort:
		content = m.renderEnterLocalPort()
	case StepConfirmation:
		content = m.renderConfirmation()
	case StepSuccess:
		content = m.renderSuccess()
	default:
		content = "Unknown step"
	}

	return wizardBoxStyle.Render(content)
}

func (m model) renderSelectContext() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(renderHeader("Add Port Forward", renderProgress(1, 7)))
	b.WriteString("Select Kubernetes Context:\n\n")

	// Show search input if there's a filter active
	if wizard.searchFilter != "" {
		b.WriteString(renderTextInput("Filter: ", wizard.searchFilter, true))
		b.WriteString("\n\n")
	}

	if wizard.loading {
		b.WriteString(spinnerStyle.Render("⣾ Loading contexts..."))
	} else if wizard.error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("✗ Error: %v", wizard.error)))
	} else if len(wizard.contexts) == 0 {
		b.WriteString(mutedStyle.Render("No contexts found in kubeconfig"))
	} else {
		filteredContexts := wizard.getFilteredContexts()
		if len(filteredContexts) == 0 {
			b.WriteString(mutedStyle.Render("No matching contexts"))
		} else {
			const viewportHeight = 20
			totalItems := len(filteredContexts)

			// Show scroll up indicator if there are items above the viewport
			if wizard.scrollOffset > 0 {
				b.WriteString(mutedStyle.Render("        ↑ More above ↑") + "\n")
			}

			// Calculate visible range
			start := wizard.scrollOffset
			end := wizard.scrollOffset + viewportHeight
			if end > totalItems {
				end = totalItems
			}

			// Render visible contexts with (current) marker on first one (only if not filtered)
			for i := start; i < end; i++ {
				prefix := "  "
				text := filteredContexts[i]
				// Only show (current) marker if no filter and this is the first item in original list
				if wizard.searchFilter == "" && i == 0 {
					text += mutedStyle.Render(" (current)")
				}

				if i == wizard.cursor {
					prefix = "▸ "
					b.WriteString(selectedStyle.Render(prefix + text))
				} else {
					b.WriteString(prefix + text)
				}
				b.WriteString("\n")
			}

			// Show scroll down indicator if there are items below the viewport
			if end < totalItems {
				b.WriteString(mutedStyle.Render("        ↓ More below ↓") + "\n")
			}
		}
	}

	b.WriteString("\n")
	if wizard.searchFilter != "" {
		b.WriteString(helpStyle.Render(fmt.Sprintf("↑/↓: Navigate  Enter: Select  Backspace: Clear filter (%d/%d)  Esc: Cancel", len(wizard.getFilteredContexts()), len(wizard.contexts))))
	} else {
		b.WriteString(helpStyle.Render("Type to filter  ↑/↓: Navigate  Enter: Select  Esc/Ctrl+C: Cancel"))
	}

	return b.String()
}

func (m model) renderSelectNamespace() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(renderHeader("Add Port Forward", renderProgress(2, 7)))
	b.WriteString(fmt.Sprintf("Context: %s\n\n", breadcrumbStyle.Render(wizard.selectedContext)))

	b.WriteString("Select Namespace:\n\n")

	// Show search input if there's a filter active
	if wizard.searchFilter != "" {
		b.WriteString(renderTextInput("Filter: ", wizard.searchFilter, true))
		b.WriteString("\n\n")
	}

	if wizard.loading {
		b.WriteString(spinnerStyle.Render("⣾ Loading namespaces..."))
	} else if wizard.error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("✗ Error: %v\n", wizard.error)))
		b.WriteString(mutedStyle.Render("\nCluster may be unreachable. Check context."))
	} else if len(wizard.namespaces) == 0 {
		b.WriteString(mutedStyle.Render("No namespaces found"))
	} else {
		filteredNamespaces := wizard.getFilteredNamespaces()
		if len(filteredNamespaces) == 0 {
			b.WriteString(mutedStyle.Render("No matching namespaces"))
		} else {
			b.WriteString(renderList(filteredNamespaces, wizard.cursor, "  ", wizard.scrollOffset))
		}
	}

	b.WriteString("\n")
	if wizard.searchFilter != "" {
		b.WriteString(helpStyle.Render(fmt.Sprintf("↑/↓: Navigate  Enter: Select  Backspace: Clear filter (%d/%d)  Esc: Back", len(wizard.getFilteredNamespaces()), len(wizard.namespaces))))
	} else {
		b.WriteString(helpStyle.Render("Type to filter  ↑/↓: Navigate  Enter: Select  Esc: Back  Ctrl+C: Cancel"))
	}

	return b.String()
}

func (m model) renderSelectResourceType() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(renderHeader("Add Port Forward", renderProgress(3, 7)))
	b.WriteString(renderBreadcrumb(wizard.selectedContext, wizard.selectedNamespace))
	b.WriteString("\n\n")

	b.WriteString("Select Resource Type:\n\n")

	resourceTypes := []ResourceType{
		ResourceTypePodPrefix,
		ResourceTypePodSelector,
		ResourceTypeService,
	}

	for i, rt := range resourceTypes {
		prefix := "  "
		if i == wizard.cursor {
			prefix = "▸ "
			b.WriteString(selectedStyle.Render(prefix + rt.String()))
			b.WriteString("\n")
			b.WriteString(mutedStyle.Render("  " + rt.Description()))
		} else {
			b.WriteString(prefix + rt.String())
		}
		b.WriteString("\n")
		if i < len(resourceTypes)-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: Navigate  Enter: Select  Esc: Back  Ctrl+C: Cancel"))

	return b.String()
}

func (m model) renderEnterResource() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(renderHeader("Add Port Forward", renderProgress(4, 7)))
	b.WriteString(renderBreadcrumb(wizard.selectedContext, wizard.selectedNamespace))
	b.WriteString("\n\n")

	switch wizard.selectedResourceType {
	case ResourceTypePodPrefix:
		b.WriteString("Enter pod name prefix:\n\n")

		// Show running pods for reference
		if wizard.loading {
			b.WriteString(spinnerStyle.Render("⣾ Loading pods..."))
		} else if len(wizard.pods) > 0 {
			b.WriteString(mutedStyle.Render("Running pods:\n"))
			showCount := 0
			for _, pod := range wizard.pods {
				if strings.HasPrefix(pod.Name, wizard.textInput) || wizard.textInput == "" {
					if showCount < 5 { // Limit to 5 pods
						b.WriteString(mutedStyle.Render(fmt.Sprintf("  • %s\n", pod.Name)))
						showCount++
					}
				}
			}
			if showCount == 0 && wizard.textInput != "" {
				b.WriteString(mutedStyle.Render("  (no matching pods)\n"))
			} else if len(wizard.pods) > showCount {
				b.WriteString(mutedStyle.Render(fmt.Sprintf("  ... and %d more\n", len(wizard.pods)-showCount)))
			}
			b.WriteString("\n")
		}

		// Text input
		b.WriteString(renderTextInput("Prefix: ", wizard.textInput, true))
		b.WriteString("\n\n")

		// Show match count
		if wizard.textInput != "" {
			matchCount := 0
			for _, pod := range wizard.pods {
				if strings.HasPrefix(pod.Name, wizard.textInput) {
					matchCount++
				}
			}

			if matchCount > 0 {
				b.WriteString(successStyle.Render(fmt.Sprintf("✓ Matches %d pod(s)", matchCount)))
			} else {
				b.WriteString(warningStyle.Render("⚠ No matching pods (you can still proceed)"))
			}
		}

	case ResourceTypePodSelector:
		b.WriteString("Enter label selector:\n")
		b.WriteString(mutedStyle.Render("Format: key=value,key2=value2\n\n"))

		b.WriteString(renderTextInput("Selector: ", wizard.textInput, true))
		b.WriteString("\n\n")

		if wizard.loading {
			b.WriteString(spinnerStyle.Render("⣾ Validating selector..."))
		} else if len(wizard.matchingPods) > 0 {
			b.WriteString(successStyle.Render(fmt.Sprintf("✓ Found %d matching pod(s):\n", len(wizard.matchingPods))))
			showCount := 0
			for _, pod := range wizard.matchingPods {
				if showCount < 3 {
					b.WriteString(mutedStyle.Render(fmt.Sprintf("  • %s\n", pod.Name)))
					showCount++
				}
			}
			if len(wizard.matchingPods) > 3 {
				b.WriteString(mutedStyle.Render(fmt.Sprintf("  ... and %d more\n", len(wizard.matchingPods)-3)))
			}
		} else if wizard.error != nil {
			b.WriteString(errorStyle.Render(fmt.Sprintf("✗ Invalid selector: %v", wizard.error)))
		}

	case ResourceTypeService:
		b.WriteString("Select service:\n\n")

		// Show search input if there's a filter active
		if wizard.searchFilter != "" {
			b.WriteString(renderTextInput("Filter: ", wizard.searchFilter, true))
			b.WriteString("\n\n")
		}

		if wizard.loading {
			b.WriteString(spinnerStyle.Render("⣾ Loading services..."))
		} else if len(wizard.services) == 0 {
			b.WriteString(mutedStyle.Render("No services found"))
		} else {
			filteredServices := wizard.getFilteredServices()
			if len(filteredServices) == 0 {
				b.WriteString(mutedStyle.Render("No matching services"))
			} else {
				serviceNames := make([]string, len(filteredServices))
				for i, svc := range filteredServices {
					serviceNames[i] = svc.Name
				}
				b.WriteString(renderList(serviceNames, wizard.cursor, "  ", wizard.scrollOffset))
			}
		}
	}

	b.WriteString("\n")
	// Show appropriate help text based on resource type and filter state
	if wizard.selectedResourceType == ResourceTypeService {
		if wizard.searchFilter != "" {
			b.WriteString(helpStyle.Render(fmt.Sprintf("↑/↓: Navigate  Enter: Select  Backspace: Clear filter (%d/%d)  Esc: Back", len(wizard.getFilteredServices()), len(wizard.services))))
		} else {
			b.WriteString(helpStyle.Render("Type to filter  ↑/↓: Navigate  Enter: Select  Esc: Back  Ctrl+C: Cancel"))
		}
	} else {
		b.WriteString(helpStyle.Render("Enter: Continue  Esc: Back  Ctrl+C: Cancel"))
	}

	return b.String()
}

func (m model) renderEnterRemotePort() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(renderHeader("Add Port Forward", renderProgress(5, 7)))
	b.WriteString(renderBreadcrumb(wizard.selectedContext, wizard.selectedNamespace))
	b.WriteString("\n")

	// Show resource selection
	resourceInfo := wizard.resourceValue
	if wizard.selector != "" {
		resourceInfo = fmt.Sprintf("%s [%s]", wizard.resourceValue, wizard.selector)
	}
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Resource: %s", resourceInfo)))
	b.WriteString("\n\n")

	// If we have detected ports and in list mode, show them as a list
	if len(wizard.detectedPorts) > 0 && wizard.inputMode == InputModeList {
		b.WriteString("Select remote port:")
		b.WriteString("\n\n")

		const viewportHeight = 20
		totalItems := len(wizard.detectedPorts) + 1 // +1 for manual entry option

		// Show scroll up indicator if there are items above the viewport
		if wizard.scrollOffset > 0 {
			b.WriteString(mutedStyle.Render("        ↑ More above ↑") + "\n")
		}

		// Calculate visible range
		start := wizard.scrollOffset
		end := wizard.scrollOffset + viewportHeight
		if end > totalItems {
			end = totalItems
		}

		// Render detected ports within viewport
		for i := start; i < end && i < len(wizard.detectedPorts); i++ {
			port := wizard.detectedPorts[i]
			// For services, show both service port and target port if they differ
			var portDesc string
			if port.TargetPort > 0 && port.TargetPort != port.Port {
				// Service with different target port: "80 → 8000 (http)"
				portDesc = fmt.Sprintf("%d → %d", port.Port, port.TargetPort)
				if port.Name != "" {
					portDesc += fmt.Sprintf(" (%s)", port.Name)
				}
			} else {
				// Pod port or service with same port
				portDesc = fmt.Sprintf("%d", port.Port)
				if port.Name != "" {
					portDesc += fmt.Sprintf(" (%s)", port.Name)
				}
			}

			prefix := "  "
			if i == wizard.cursor {
				prefix = "▸ "
				b.WriteString(selectedStyle.Render(prefix + portDesc))
			} else {
				b.WriteString(prefix + portDesc)
			}
			b.WriteString("\n")
		}

		// Add "Manual entry" option if within viewport
		manualIdx := len(wizard.detectedPorts)
		if manualIdx >= start && manualIdx < end {
			manualOption := "Manual entry (type port number)"
			prefix := "  "
			if wizard.cursor == manualIdx {
				prefix = "▸ "
				b.WriteString(selectedStyle.Render(prefix + manualOption))
			} else {
				b.WriteString(prefix + mutedStyle.Render(manualOption))
			}
			b.WriteString("\n")
		}

		// Show scroll down indicator if there are items below the viewport
		if end < totalItems {
			b.WriteString(mutedStyle.Render("        ↓ More below ↓") + "\n")
		}

		b.WriteString("\n")
		b.WriteString(helpStyle.Render("↑/↓: Navigate  Enter: Select  Esc: Back  Ctrl+C: Cancel"))
	} else {
		// Text input mode (no detected ports or user chose manual entry)
		if len(wizard.detectedPorts) > 0 {
			b.WriteString(mutedStyle.Render("Detected ports:\n"))
			for _, port := range wizard.detectedPorts {
				var portDesc string
				if port.TargetPort > 0 && port.TargetPort != port.Port {
					portDesc = fmt.Sprintf("%d → %d", port.Port, port.TargetPort)
					if port.Name != "" {
						portDesc += fmt.Sprintf(" (%s)", port.Name)
					}
				} else {
					portDesc = fmt.Sprintf("%d", port.Port)
					if port.Name != "" {
						portDesc += fmt.Sprintf(" (%s)", port.Name)
					}
				}
				b.WriteString(mutedStyle.Render(fmt.Sprintf("  • %s\n", portDesc)))
			}
			b.WriteString("\n")
		}

		b.WriteString(renderTextInput("Remote port: ", wizard.textInput, wizard.error == nil))
		b.WriteString("\n\n")

		if wizard.error != nil {
			b.WriteString(errorStyle.Render(fmt.Sprintf("✗ %v", wizard.error)))
		} else if wizard.textInput != "" {
			b.WriteString(mutedStyle.Render("Press Enter to continue"))
		}

		b.WriteString("\n")
		b.WriteString(helpStyle.Render("Enter: Continue  Esc: Back  Ctrl+C: Cancel"))
	}

	return b.String()
}

func (m model) renderEnterLocalPort() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(renderHeader("Add Port Forward", renderProgress(6, 7)))
	b.WriteString(renderBreadcrumb(wizard.selectedContext, wizard.selectedNamespace))
	b.WriteString("\n")

	resourceInfo := wizard.resourceValue
	if wizard.selector != "" {
		resourceInfo = fmt.Sprintf("%s [%s]", wizard.resourceValue, wizard.selector)
	}
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Resource: %s", resourceInfo)))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Remote port: %d", wizard.remotePort)))
	b.WriteString("\n\n")

	b.WriteString(renderTextInput("Local port: ", wizard.textInput, wizard.error == nil))
	b.WriteString("\n\n")

	if wizard.loading {
		b.WriteString(spinnerStyle.Render("⣾ Checking availability..."))
	} else if wizard.error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("✗ %v", wizard.error)))
	} else if wizard.portCheckMsg != "" {
		if wizard.portAvailable {
			b.WriteString(successStyle.Render(wizard.portCheckMsg))
		} else {
			b.WriteString(errorStyle.Render(wizard.portCheckMsg))
		}
	} else if wizard.textInput != "" {
		b.WriteString(mutedStyle.Render("Press Enter to check availability"))
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Enter: Continue  Esc: Back  Ctrl+C: Cancel"))

	return b.String()
}

func (m model) renderConfirmation() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(renderHeader("Add Port Forward", renderProgress(7, 7)))
	b.WriteString("\n")

	b.WriteString("Review Configuration:\n\n")

	resourceInfo := wizard.resourceValue
	if wizard.selector != "" {
		resourceInfo = fmt.Sprintf("pod (selector: %s)", wizard.selector)
	} else if wizard.selectedResourceType == ResourceTypePodPrefix {
		resourceInfo = fmt.Sprintf("pod/%s", wizard.resourceValue)
	} else if wizard.selectedResourceType == ResourceTypeService {
		resourceInfo = fmt.Sprintf("service/%s", wizard.resourceValue)
	}

	b.WriteString(fmt.Sprintf("  Context:      %s\n", wizard.selectedContext))
	b.WriteString(fmt.Sprintf("  Namespace:    %s\n", wizard.selectedNamespace))
	b.WriteString(fmt.Sprintf("  Resource:     %s\n", resourceInfo))
	b.WriteString(fmt.Sprintf("  Remote Port:  %d\n", wizard.remotePort))
	b.WriteString(fmt.Sprintf("  Local Port:   %d\n", wizard.localPort))
	b.WriteString("  Protocol:     tcp\n")

	b.WriteString("\n")

	// Show alias field with focus indicator
	if wizard.confirmationFocus == FocusAlias {
		b.WriteString(selectedStyle.Render("▸ Optional alias (friendly name):") + "\n")
		b.WriteString("    Alias: " + validInputStyle.Render(wizard.textInput+"█") + "\n")
	} else {
		b.WriteString(mutedStyle.Render("  Optional alias (friendly name):") + "\n")
		b.WriteString(mutedStyle.Render("    Alias: "+wizard.textInput) + "\n")
	}

	b.WriteString("\n")

	// Show buttons with focus indicator
	if wizard.confirmationFocus == FocusButtons {
		if wizard.cursor == 0 {
			b.WriteString(selectedStyle.Render("▸ Add to .kportal.yaml") + "\n")
			b.WriteString("  Cancel\n")
		} else {
			b.WriteString("  Add to .kportal.yaml\n")
			b.WriteString(selectedStyle.Render("▸ Cancel") + "\n")
		}
	} else {
		b.WriteString(mutedStyle.Render("  Add to .kportal.yaml") + "\n")
		b.WriteString(mutedStyle.Render("  Cancel") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓/Tab: Navigate  Enter: Confirm  Esc: Back"))

	return b.String()
}

func (m model) renderSuccess() string {
	wizard := m.ui.addWizard
	var b strings.Builder

	b.WriteString(successStyle.Render("Success! ✓"))
	b.WriteString("\n\n")

	if wizard.error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v", wizard.error)))
	} else {
		b.WriteString("Added to .kportal.yaml\n\n")

		forwardDesc := fmt.Sprintf("localhost:%d → %s:%d",
			wizard.localPort,
			wizard.resourceValue,
			wizard.remotePort)

		if wizard.alias != "" {
			forwardDesc = fmt.Sprintf("%s (%s)", wizard.alias, forwardDesc)
		}

		b.WriteString(successStyle.Render(forwardDesc))
		b.WriteString("\n\n")
		b.WriteString(mutedStyle.Render("The port forward will be active shortly."))
	}

	b.WriteString("\n\n")
	b.WriteString("Would you like to:\n")

	if wizard.cursor == 0 {
		b.WriteString(selectedStyle.Render("▸ Add another port forward") + "\n")
		b.WriteString("  Return to main view\n")
	} else {
		b.WriteString("  Add another port forward\n")
		b.WriteString(selectedStyle.Render("▸ Return to main view") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: Navigate  Enter: Select"))

	return b.String()
}

// renderRemoveWizard renders the remove wizard
func (m model) renderRemoveWizard() string {
	if m.ui.removeWizard == nil {
		return ""
	}

	wizard := m.ui.removeWizard

	var content string
	if wizard.confirming {
		content = m.renderRemoveConfirmation()
	} else {
		content = m.renderRemoveSelection()
	}

	return wizardBoxStyle.Render(content)
}

func (m model) renderRemoveSelection() string {
	wizard := m.ui.removeWizard
	var b strings.Builder

	b.WriteString(renderHeader("Remove Port Forwards", ""))
	b.WriteString("\n")

	b.WriteString("Select forwards to remove (Space to toggle):\n\n")

	for i, fwd := range wizard.forwards {
		isSelected := i == wizard.cursor
		isChecked := wizard.selected[i]

		line1 := fmt.Sprintf("%s:%d→%d", fwd.Alias, fwd.Port, fwd.LocalPort)
		line2 := fmt.Sprintf("    %s/%s/%s", fwd.Context, fwd.Namespace, fwd.Resource)

		checkbox := "[ ] "
		if isChecked {
			checkbox = "[✓] "
		}

		fullLine := checkbox + line1
		if isSelected {
			b.WriteString(selectedStyle.Render(fullLine))
		} else {
			if isChecked {
				b.WriteString(checkedBoxStyle.Render(checkbox) + line1)
			} else {
				b.WriteString(uncheckedBoxStyle.Render(checkbox) + line1)
			}
		}

		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(line2))
		b.WriteString("\n\n")
	}

	selectedCount := wizard.getSelectedCount()
	b.WriteString(fmt.Sprintf("%d of %d selected\n\n", selectedCount, len(wizard.forwards)))

	b.WriteString(helpStyle.Render("Space: Toggle  a: All  n: None  Enter: Remove  Esc: Cancel"))

	return b.String()
}

func (m model) renderRemoveConfirmation() string {
	wizard := m.ui.removeWizard
	var b strings.Builder

	b.WriteString(renderHeader("Confirm Removal", ""))
	b.WriteString("\n")

	selectedCount := wizard.getSelectedCount()
	b.WriteString(fmt.Sprintf("Remove %d port forward(s)?\n\n", selectedCount))

	selectedForwards := wizard.getSelectedForwards()
	for _, fwd := range selectedForwards {
		b.WriteString(errorStyle.Render(fmt.Sprintf("  • %s:%d→%d\n", fwd.Alias, fwd.Port, fwd.LocalPort)))
		b.WriteString(mutedStyle.Render(fmt.Sprintf("    %s/%s/%s\n", fwd.Context, fwd.Namespace, fwd.Resource)))
	}

	b.WriteString("\n")
	b.WriteString(warningStyle.Render("This action cannot be undone."))
	b.WriteString("\n\n")

	// Yes/No buttons
	if wizard.confirmCursor == 0 {
		b.WriteString(selectedStyle.Render("▸ Yes, remove them") + "\n")
		b.WriteString("  Cancel\n")
	} else {
		b.WriteString("  Yes, remove them\n")
		b.WriteString(selectedStyle.Render("▸ Cancel") + "\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("↑/↓: Navigate  Enter: Confirm  Esc: Cancel"))

	return b.String()
}

// renderBenchmark renders the benchmark wizard
func (m model) renderBenchmark() string {
	if m.ui.benchmarkState == nil {
		return ""
	}

	state := m.ui.benchmarkState

	var content string
	switch state.step {
	case BenchmarkStepConfig:
		content = m.renderBenchmarkConfig()
	case BenchmarkStepRunning:
		content = m.renderBenchmarkRunning()
	case BenchmarkStepResults:
		content = m.renderBenchmarkResults()
	default:
		content = "Unknown step"
	}

	return wizardBoxStyle.Render(content)
}

func (m model) renderBenchmarkConfig() string {
	state := m.ui.benchmarkState
	var b strings.Builder

	b.WriteString(renderHeader("HTTP Benchmark", ""))
	b.WriteString(fmt.Sprintf("Target: %s (localhost:%d)", breadcrumbStyle.Render(state.forwardAlias), state.localPort))
	b.WriteString("\n\n")

	b.WriteString("Configure benchmark parameters:")
	b.WriteString("\n\n")

	fields := []struct {
		label string
		value string
	}{
		{"URL Path", state.urlPath},
		{"Method", state.method},
		{"Concurrency", fmt.Sprintf("%d", state.concurrency)},
		{"Requests", fmt.Sprintf("%d", state.requests)},
	}

	for i, field := range fields {
		prefix := "  "
		if i == state.cursor {
			prefix = "▸ "
			b.WriteString(selectedStyle.Render(fmt.Sprintf("%s%-12s", prefix, field.label+":")))
			b.WriteString(validInputStyle.Render(field.value + "█"))
		} else {
			b.WriteString(fmt.Sprintf("%s%-12s %s", prefix, field.label+":", field.value))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Will send %d requests with %d concurrent workers", state.requests, state.concurrency)))
	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render("↑/↓/Tab: Navigate  Type to edit  Enter: Run  Esc: Cancel"))

	return b.String()
}

func (m model) renderBenchmarkRunning() string {
	state := m.ui.benchmarkState
	var b strings.Builder

	b.WriteString(renderHeader("HTTP Benchmark", ""))
	b.WriteString(fmt.Sprintf("Target: %s", breadcrumbStyle.Render(state.forwardAlias)))
	b.WriteString("\n\n")

	// Progress bar
	progress := float64(state.progress) / float64(state.total)
	if state.total == 0 {
		progress = 0
	}
	barWidth := 30
	filled := int(progress * float64(barWidth))
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
	percent := int(progress * 100)

	b.WriteString(spinnerStyle.Render("Running benchmark..."))
	b.WriteString("\n\n")

	b.WriteString(fmt.Sprintf("  [%s] %d%%", successStyle.Render(bar), percent))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("  %d / %d requests completed", state.progress, state.total)))
	b.WriteString("\n\n")

	b.WriteString(mutedStyle.Render(fmt.Sprintf("URL: http://localhost:%d%s", state.localPort, state.urlPath)))
	b.WriteString("\n")
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Method: %s  Concurrency: %d", state.method, state.concurrency)))
	b.WriteString("\n\n")

	b.WriteString(helpStyle.Render("Please wait..."))

	return b.String()
}

func (m model) renderBenchmarkResults() string {
	state := m.ui.benchmarkState
	var b strings.Builder

	b.WriteString(renderHeader("Benchmark Results", ""))
	b.WriteString(fmt.Sprintf("Target: %s", breadcrumbStyle.Render(state.forwardAlias)))
	b.WriteString("\n\n")

	if state.error != nil {
		b.WriteString(errorStyle.Render(fmt.Sprintf("✗ Error: %v", state.error)))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Press Enter or Esc to return"))
		return b.String()
	}

	if state.results == nil {
		b.WriteString(mutedStyle.Render("No results available"))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("Press Enter or Esc to return"))
		return b.String()
	}

	r := state.results

	// Summary
	successRate := float64(r.Successful) / float64(r.TotalRequests) * 100
	if r.TotalRequests == 0 {
		successRate = 0
	}

	b.WriteString(fmt.Sprintf("Total Requests:  %d", r.TotalRequests))
	b.WriteString("\n")
	if r.Failed == 0 {
		b.WriteString(successStyle.Render(fmt.Sprintf("Successful:      %d (%.1f%%)", r.Successful, successRate)))
	} else {
		b.WriteString(fmt.Sprintf("Successful:      %d (%.1f%%)", r.Successful, successRate))
	}
	b.WriteString("\n")
	if r.Failed > 0 {
		b.WriteString(errorStyle.Render(fmt.Sprintf("Failed:          %d", r.Failed)))
	} else {
		b.WriteString(fmt.Sprintf("Failed:          %d", r.Failed))
	}
	b.WriteString("\n\n")

	// Latency stats
	b.WriteString(breadcrumbStyle.Render("Latency (ms)"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Min:    %.2f", r.MinLatency))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Max:    %.2f", r.MaxLatency))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Avg:    %.2f", r.AvgLatency))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  P50:    %.2f", r.P50Latency))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  P95:    %.2f", r.P95Latency))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  P99:    %.2f", r.P99Latency))
	b.WriteString("\n\n")

	// Throughput
	b.WriteString(breadcrumbStyle.Render("Throughput"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Requests/sec:  %.2f", r.Throughput))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Bytes read:    %d", r.BytesRead))
	b.WriteString("\n")

	// Status codes if interesting
	if len(r.StatusCodes) > 0 {
		b.WriteString("\n")
		b.WriteString(breadcrumbStyle.Render("Status Codes"))
		b.WriteString("\n")
		for code, count := range r.StatusCodes {
			if code >= 200 && code < 300 {
				b.WriteString(successStyle.Render(fmt.Sprintf("  %d: %d", code, count)))
			} else if code >= 400 {
				b.WriteString(errorStyle.Render(fmt.Sprintf("  %d: %d", code, count)))
			} else {
				b.WriteString(fmt.Sprintf("  %d: %d", code, count))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Press Enter or Esc to return"))

	return b.String()
}

// renderHTTPLog renders the HTTP log viewer as a full-screen table
func (m model) renderHTTPLog() string {
	if m.ui.httpLogState == nil {
		return ""
	}

	state := m.ui.httpLogState

	// Get terminal dimensions
	termWidth := m.termWidth
	termHeight := m.termHeight
	if termWidth == 0 {
		termWidth = 120
	}
	if termHeight == 0 {
		termHeight = 40
	}

	// Get filtered entries
	filteredEntries := state.getFilteredEntries()
	totalEntries := len(filteredEntries)
	totalUnfiltered := len(state.entries)

	// Build output
	var b strings.Builder

	// Header line
	title := wizardHeaderStyle.Render("HTTP Traffic Log")
	b.WriteString(title)
	b.WriteString("  ")
	b.WriteString(breadcrumbStyle.Render(state.forwardAlias))

	// Status indicators
	b.WriteString("  ")
	filterLabel := state.getFilterModeLabel()
	if state.filterMode != HTTPLogFilterNone {
		b.WriteString(accentStyle.Render(fmt.Sprintf("[Filter: %s]", filterLabel)))
	}
	if state.filterText != "" {
		b.WriteString("  ")
		b.WriteString(accentStyle.Render(fmt.Sprintf("[Search: \"%s\"]", state.filterText)))
	}
	if state.autoScroll {
		b.WriteString("  ")
		b.WriteString(successStyle.Render("[Auto-scroll]"))
	}
	b.WriteString("\n")

	// Filter input line (if active)
	if state.filterActive {
		b.WriteString(accentStyle.Render("Search: "))
		b.WriteString(state.filterText)
		b.WriteString(accentStyle.Render("_"))
		b.WriteString("\n")
	}

	// Table or empty message
	if totalEntries == 0 {
		b.WriteString("\n")
		if totalUnfiltered == 0 {
			b.WriteString(mutedStyle.Render("  No HTTP traffic logged yet.\n"))
			b.WriteString(mutedStyle.Render("  Enable with: httpLog: true in .kportal.yaml\n"))
		} else {
			b.WriteString(mutedStyle.Render(fmt.Sprintf("  No entries match filter. (%d total entries)\n", totalUnfiltered)))
			b.WriteString(mutedStyle.Render("  Press 'c' to clear filters.\n"))
		}
		// Pad to fill screen
		for i := 0; i < termHeight-10; i++ {
			b.WriteString("\n")
		}
	} else {
		// Render simple table without lipgloss table (for better control)
		b.WriteString("\n")

		// Header
		header := fmt.Sprintf("  %-10s  %-7s  %-6s  %-8s  %s",
			"TIME", "METHOD", "STATUS", "LATENCY", "PATH")
		b.WriteString(mutedStyle.Render(header))
		b.WriteString("\n")
		b.WriteString(mutedStyle.Render(strings.Repeat("─", termWidth-2)))
		b.WriteString("\n")

		// Calculate visible range
		viewportHeight := termHeight - 8 // header, filter bar, table header, separator, footer, help
		if viewportHeight < 5 {
			viewportHeight = 5
		}

		// Ensure cursor is in valid range
		if state.cursor < 0 {
			state.cursor = 0
		}
		if state.cursor >= totalEntries {
			state.cursor = totalEntries - 1
		}

		// Calculate scroll offset to keep cursor visible
		if state.cursor < state.scrollOffset {
			state.scrollOffset = state.cursor
		}
		if state.cursor >= state.scrollOffset+viewportHeight {
			state.scrollOffset = state.cursor - viewportHeight + 1
		}
		if state.scrollOffset < 0 {
			state.scrollOffset = 0
		}

		start := state.scrollOffset
		end := start + viewportHeight
		if end > totalEntries {
			end = totalEntries
		}

		// Calculate max path width
		maxPathWidth := termWidth - 48
		if maxPathWidth < 10 {
			maxPathWidth = 10
		}

		for i := start; i < end; i++ {
			entry := filteredEntries[i]

			// Format fields
			statusStr := ""
			if entry.StatusCode > 0 {
				statusStr = fmt.Sprintf("%d", entry.StatusCode)
			}

			latencyStr := ""
			if entry.LatencyMs > 0 {
				if entry.LatencyMs >= 1000 {
					latencyStr = fmt.Sprintf("%.1fs", float64(entry.LatencyMs)/1000)
				} else {
					latencyStr = fmt.Sprintf("%dms", entry.LatencyMs)
				}
			}

			// Truncate path
			path := entry.Path
			if len(path) > maxPathWidth {
				path = path[:maxPathWidth-3] + "..."
			}

			// Build line
			line := fmt.Sprintf("%-10s  %-7s  %-6s  %-8s  %s",
				entry.Timestamp,
				entry.Method,
				statusStr,
				latencyStr,
				path)

			// Selection prefix
			prefix := "  "
			if i == state.cursor {
				prefix = "▸ "
			}

			// Apply color based on status
			// 200s = normal text, 400s = warning (orange), 500s = error (red)
			var styledLine string
			if entry.StatusCode >= 500 {
				styledLine = errorStyle.Render(line)
			} else if entry.StatusCode >= 400 {
				styledLine = warningStyle.Render(line)
			} else {
				// 200s and other codes - normal text color
				styledLine = line
			}

			if i == state.cursor {
				b.WriteString(selectedStyle.Render(prefix))
			} else {
				b.WriteString(prefix)
			}
			b.WriteString(styledLine)
			b.WriteString("\n")
		}

		// Pad remaining lines
		linesRendered := end - start
		for i := linesRendered; i < viewportHeight; i++ {
			b.WriteString("\n")
		}
	}

	// Footer with entry count
	b.WriteString("\n")
	if totalEntries != totalUnfiltered {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  %d of %d entries (filtered from %d)", totalEntries, totalEntries, totalUnfiltered)))
	} else {
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  %d entries", totalEntries)))
	}
	b.WriteString("\n")

	// Help line at bottom
	b.WriteString(helpStyle.Render("  ↑/↓/PgUp/PgDn: Navigate  g/G: Top/Bottom  a: Auto-scroll  f: Filter  /: Search  c: Clear  q: Close"))

	return b.String()
}
