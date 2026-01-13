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

	// JSON syntax highlighting colors
	jsonKeyColor    = lipgloss.Color("81")  // Cyan
	jsonStringColor = lipgloss.Color("180") // Light orange/tan
	jsonNumberColor = lipgloss.Color("141") // Light purple
	jsonBoolColor   = lipgloss.Color("209") // Orange
	jsonNullColor   = lipgloss.Color("243") // Dark gray
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

	accentStyle = lipgloss.NewStyle().
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

// JSON syntax highlighting styles
var (
	jsonKeyStyle = lipgloss.NewStyle().
			Foreground(jsonKeyColor)

	jsonStringStyle = lipgloss.NewStyle().
			Foreground(jsonStringColor)

	jsonNumberStyle = lipgloss.NewStyle().
			Foreground(jsonNumberColor)

	jsonBoolStyle = lipgloss.NewStyle().
			Foreground(jsonBoolColor)

	jsonNullStyle = lipgloss.NewStyle().
			Foreground(jsonNullColor)
)

// Container styles
var (
	// wizardBoxStyle creates a bordered modal box
	wizardBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accentColor).
		Padding(1, 2)
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

	totalItems := len(items)

	// Show scroll up indicator if there are items above the viewport
	if scrollOffset > 0 {
		b.WriteString(mutedStyle.Render("        ↑ More above ↑") + "\n")
	}

	// Calculate visible range
	start := scrollOffset
	end := scrollOffset + ViewportHeight
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

// wizardHelpWidth returns an appropriate width for wizard help text
// based on terminal width. For modals, we use a sensible maximum.
func wizardHelpWidth(termWidth int) int {
	if termWidth == 0 {
		termWidth = 80
	}
	// Wizard modals shouldn't be wider than 70 chars typically
	// but on narrow terminals, use available space minus padding
	maxWidth := 70
	available := termWidth - 10 // account for modal borders and padding
	if available < maxWidth {
		return available
	}
	return maxWidth
}

// wrapHelpText wraps help text to fit within the given width.
// Help text is expected to be in the format "key: action  key: action  ..."
// separated by double spaces. On smaller screens, it wraps to multiple lines.
func wrapHelpText(text string, width int) string {
	if width <= 0 {
		width = 80 // Default width
	}

	// Account for some padding/margin
	availableWidth := width - 4
	if availableWidth < 20 {
		availableWidth = 20
	}

	// If text fits, return as-is
	if len(text) <= availableWidth {
		return helpStyle.Render(text)
	}

	// Split by double-space separator (common in help text)
	parts := strings.Split(text, "  ")
	if len(parts) <= 1 {
		// No double-space separators, just truncate
		if len(text) > availableWidth-3 {
			return helpStyle.Render(text[:availableWidth-3] + "...")
		}
		return helpStyle.Render(text)
	}

	var lines []string
	var currentLine strings.Builder

	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check if adding this part would exceed width
		addition := part
		if currentLine.Len() > 0 {
			addition = "  " + part
		}

		if currentLine.Len()+len(addition) > availableWidth && currentLine.Len() > 0 {
			// Start new line
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(part)
		} else {
			if currentLine.Len() > 0 {
				currentLine.WriteString("  ")
			}
			currentLine.WriteString(part)
		}

		// Handle last part
		if i == len(parts)-1 && currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}
	}

	// Join with newlines and apply style to each line
	var result strings.Builder
	for i, line := range lines {
		if i > 0 {
			result.WriteString("\n")
		}
		result.WriteString(helpStyle.Render(line))
	}

	return result.String()
}

// overlayContent overlays modal content centered on the base view
// Note: base parameter is kept for API compatibility but not used since
// lipgloss.Place provides cleaner centering without background artifacts
func overlayContent(_, modal string, termWidth, termHeight int) string {
	// Use lipgloss.Place to center the modal in the terminal viewport
	// This handles all alignment properly and respects ANSI styling
	return lipgloss.Place(
		termWidth,
		termHeight,
		lipgloss.Center,
		lipgloss.Center,
		modal,
		lipgloss.WithWhitespaceChars(" "),
	)
}
