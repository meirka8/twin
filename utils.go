package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// calculateWrappedLines wraps the content to fit the given width and returns the lines.
func calculateWrappedLines(content string, width int) []string {
	if width <= 0 {
		return []string{}
	}
	wrappedContent := lipgloss.NewStyle().Width(width).Render(content)
	return strings.Split(wrappedContent, "\n")
}

// fuzzyMatch checks if the query characters appear in the target string in order.
func fuzzyMatch(query, target string) bool {
	if query == "" {
		return true
	}

	queryRunes := []rune(strings.ToLower(query))

	qIdx := 0
	for _, t := range strings.ToLower(target) {
		if qIdx < len(queryRunes) && t == queryRunes[qIdx] {
			qIdx++
		}
	}
	return qIdx == len(queryRunes)
}

// clampIndex limits i to the indexes of a list of n items, or 0 when the list is empty.
func clampIndex(i, n int) int {
	return max(min(i, n-1), 0)
}

// oneLinePaste readies a paste for a one-line text field. The line break that comes along
// when a whole line is copied is dropped; textinput turns any other into a space.
func oneLinePaste(msg tea.PasteMsg) tea.PasteMsg {
	msg.Content = strings.TrimRight(msg.Content, "\r\n")
	return msg
}
