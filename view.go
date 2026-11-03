package main

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// View renders the application UI.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// With the Kitty keyboard protocol, Ctrl and Alt shortcuts also report the key's letter
	// on a US layout, so they work while typing in another script (Hebrew, Russian, ...).
	v.KeyboardEnhancements.ReportAlternateKeys = true
	return v
}

// render lays out the panes, the status bar and the hint bar.
func (m model) render() string {
	if m.quitting {
		return "Exiting Twin Manager. Goodbye!\n"
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.panesView(), m.statusBarView(), m.hintsView())
}

// panesView renders the two panes side by side. The topmost modal with a pane view takes
// the active pane's place, cut to the pane's box so that it never pushes the other pane
// aside.
func (m model) panesView() string {
	left, right := m.leftPane.view(), m.rightPane.view()
	width, height := m.activePane().boxSize()
	for _, md := range slices.Backward(m.modals) {
		if s := md.paneView(&m, width, height); s != "" {
			s = lipgloss.NewStyle().MaxWidth(width).MaxHeight(height).Render(s)
			if m.leftPane.active {
				left = s
			} else {
				right = s
			}
			break
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// statusBarView shows the line of the modal on top, if it has one: a question from a modal
// lower down is not shown, since its keys would not reach it.
func (m model) statusBarView() string {
	if n := len(m.modals); n > 0 {
		if s := m.modals[n-1].statusView(&m); s != "" {
			return s
		}
	}

	screenWidth := m.leftPane.width + m.rightPane.width + 4 // Both panes with their borders
	if m.toast != "" {
		width := max(screenWidth-errorToastStyle.GetHorizontalFrameSize(), 0)
		return errorToastStyle.Render(ansi.Truncate("! "+m.toast, width, "…"))
	}

	activePane := m.activePane()
	var search string
	if q := activePane.search.Value(); q != "" {
		search = statusBarActive.Render("Search: " + q)
	}

	f, ok := activePane.cursorFile()
	if !ok {
		return search
	}
	status := fmt.Sprintf("%s | %s | %s", f.Name, f.Mode.String(), f.ModTime.Format("2006-01-02 15:04:05"))
	width := max(screenWidth-lipgloss.Width(search)-statusBar.GetHorizontalFrameSize(), 0)
	return search + statusBar.Render(ansi.Truncate(status, width, "…"))
}

func (m model) hintsView() string {
	// Modifiers
	// We primarily care about Alt as per user request
	altStyle := altChipInactiveStyle
	if m.modifierState.Alt {
		altStyle = altChipStyle
	}

	// Check other modifiers just in case we need to show them or they affect hints
	// But user asked to "leave only alt".
	// We will just show "Alt" chip, highlighted if pressed.

	modifiers := altStyle.Render("Alt")

	// Hints
	var hints []string
	targetModifier := "alt" // Default fallback
	if m.modifierState.Ctrl {
		targetModifier = "ctrl"
	} else if m.modifierState.Alt {
		targetModifier = "alt"
	} else if m.modifierState.Shift {
		targetModifier = "shift"
	}

	for _, shortcut := range m.keyMap.GetShortcuts() {
		if shortcut.Modifier == targetModifier {
			hint := hintCardStyle.Render(
				lipgloss.JoinHorizontal(lipgloss.Left,
					hintKeyStyle.Render(shortcut.DisplayKey),
					hintDescStyle.Render(shortcut.Action),
				),
			)
			hints = append(hints, hint)
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, // Alignment check
		modifiers,
		// No spacer needed if margins are handled by styles
		lipgloss.JoinHorizontal(lipgloss.Left, hints...),
	)
}
