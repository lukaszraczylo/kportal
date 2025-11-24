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
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Resource: %s\n\n", resourceInfo)))

	// If we have detected ports and in list mode, show them as a list
	if len(wizard.detectedPorts) > 0 && wizard.inputMode == InputModeList {
		b.WriteString("Select remote port:\n\n")

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
			portDesc := fmt.Sprintf("%d", port.Port)
			if port.Name != "" {
				portDesc += fmt.Sprintf(" (%s)", port.Name)
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
				b.WriteString(mutedStyle.Render(prefix + manualOption))
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
				portDesc := fmt.Sprintf("%d", port.Port)
				if port.Name != "" {
					portDesc += fmt.Sprintf(" (%s)", port.Name)
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
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Resource: %s\n", resourceInfo)))
	b.WriteString(mutedStyle.Render(fmt.Sprintf("Remote port: %d\n\n", wizard.remotePort)))

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
	} else if wizard.textInput != "" && wizard.localPort > 0 {
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
