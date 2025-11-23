package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color palette for wizards
var (
	primaryColor   = lipgloss.Color("205") // Pink/Magenta
	successColor   = lipgloss.Color("42")  // Green
	errorColor     = lipgloss.Color("196") // Red
	warningColor   = lipgloss.Color("220") // Yellow
	mutedColor     = lipgloss.Color("241") // Gray
	accentColor    = lipgloss.Color("63")  // Purple
	highlightColor = lipgloss.Color("117") // Light blue
)

// Text styles
var (
	wizardHeaderStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				MarginBottom(0)

	wizardStepStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)

	breadcrumbStyle = lipgloss.NewStyle().
			Foreground(highlightColor).
			Bold(true)

	selectedStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor).
			Bold(true)

	mutedStyle = lipgloss.NewStyle().
			Foreground(mutedColor)

	helpStyle = lipgloss.NewStyle().
			Foreground(mutedColor).
			Italic(true)

	spinnerStyle = lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true)
)

// Input styles
var (
	inputStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	validInputStyle = lipgloss.NewStyle().
			Foreground(successColor)
)

// Checkbox styles
var (
	checkedBoxStyle = lipgloss.NewStyle().
			Foreground(successColor).
			Bold(true)

	uncheckedBoxStyle = lipgloss.NewStyle().
				Foreground(mutedColor)
)

// Container styles
var (
	wizardBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(1, 2).
		Width(60)
)

// Helper functions for rendering

// renderProgress returns a step indicator like "Step 2/7"
func renderProgress(current, total int) string {
	return wizardStepStyle.Render(fmt.Sprintf("Step %d/%d", current, total))
}

// renderHeader returns a formatted header with title and progress
func renderHeader(title, progress string) string {
	header := wizardHeaderStyle.Render(title)
	if progress != "" {
		header += "  " + progress
	}
	return header + "\n\n"
}

// renderBreadcrumb returns a formatted breadcrumb path
func renderBreadcrumb(parts ...string) string {
	return breadcrumbStyle.Render(strings.Join(parts, " / "))
}

// renderList renders a list of items with cursor selection and viewport scrolling
func renderList(items []string, cursor int, prefix string, scrollOffset int) string {
	var b strings.Builder

	const viewportHeight = 20
	totalItems := len(items)

	// Show scroll up indicator if there are items above the viewport
	if scrollOffset > 0 {
		b.WriteString(mutedStyle.Render("        ↑ More above ↑") + "\n")
	}

	// Calculate visible range
	start := scrollOffset
	end := scrollOffset + viewportHeight
	if end > totalItems {
		end = totalItems
	}

	// Render visible items
	for i := start; i < end; i++ {
		cursorPrefix := prefix
		if i == cursor {
			cursorPrefix = "▸ "
			b.WriteString(selectedStyle.Render(cursorPrefix + items[i]))
		} else {
			b.WriteString(cursorPrefix + items[i])
		}
		b.WriteString("\n")
	}

	// Show scroll down indicator if there are items below the viewport
	if end < totalItems {
		b.WriteString(mutedStyle.Render("        ↓ More below ↓") + "\n")
	}

	return b.String()
}

// renderTextInput renders a text input field with a cursor
func renderTextInput(label, value string, valid bool) string {
	var b strings.Builder

	b.WriteString(label)

	inputText := value + "█"
	if valid {
		b.WriteString(validInputStyle.Render(inputText))
	} else {
		b.WriteString(inputStyle.Render(inputText))
	}

	return b.String()
}

// overlayContent overlays modal content centered on the base view
func overlayContent(base, modal string, termWidth, termHeight int) string {
	baseLines := strings.Split(base, "\n")
	modalLines := strings.Split(modal, "\n")

	// Ensure base has enough lines
	for len(baseLines) < termHeight {
		baseLines = append(baseLines, "")
	}

	modalHeight := len(modalLines)
	modalWidth := 0
	for _, line := range modalLines {
		w := lipgloss.Width(line)
		if w > modalWidth {
			modalWidth = w
		}
	}

	// Calculate center position
	startRow := (termHeight - modalHeight) / 2
	if startRow < 0 {
		startRow = 0
	}

	// Create result with modal overlaid
	result := make([]string, len(baseLines))
	copy(result, baseLines)

	for i, modalLine := range modalLines {
		row := startRow + i
		if row >= 0 && row < len(result) {
			// Center the modal line
			padding := (termWidth - lipgloss.Width(modalLine)) / 2
			if padding < 0 {
				padding = 0
			}

			result[row] = strings.Repeat(" ", padding) + modalLine
		}
	}

	return strings.Join(result, "\n")
}
