package main

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// logModal shows the error log in place of the active pane, newest entry at the bottom.
type logModal struct {
	scrollY int // Lines scrolled up from the newest
}

func (l logModal) update(m *model, msg tea.Msg) (modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return l, nil
	}
	switch key.String() {
	case "esc", "q", m.keyMap.ErrorLog.Key:
		return nil, nil
	case "up", "k":
		l.scrollY++
	case "down", "j":
		l.scrollY--
	case "pgup":
		l.scrollY += m.logRows()
	case "pgdown":
		l.scrollY -= m.logRows()
	case "home", "g":
		l.scrollY = m.maxLogScroll()
	case "end", "G":
		l.scrollY = 0
	}
	l.scrollY = max(min(l.scrollY, m.maxLogScroll()), 0)
	return l, nil
}

// logRows is how many log lines fit in the log overlay.
func (m *model) logRows() int {
	return max(m.leftPane.height-2, 1) // Title and hint lines; both panes share a height
}

// maxLogScroll is the furthest the log can scroll up from its newest line.
func (m *model) maxLogScroll() int {
	return max(len(m.logLines())-m.logRows(), 0)
}

// logLines is the error log wrapped to the width of the log overlay, oldest first.
func (m *model) logLines() []string {
	var lines []string
	for _, e := range m.errLog {
		lines = append(lines, calculateWrappedLines(e.time.Format("15:04:05 ")+e.text, m.leftPane.width-2)...)
	}
	return lines
}

func (l logModal) paneView(m *model, width, height int) string {
	lines := m.logLines()
	end := len(lines) - min(l.scrollY, m.maxLogScroll())
	rows := append([]string{}, lines[max(end-m.logRows(), 0):end]...)
	if len(lines) == 0 {
		rows = []string{"No errors so far."}
	}
	for len(rows) < m.logRows() {
		rows = append(rows, "")
	}

	title := logTitleStyle.Render(fmt.Sprintf("Error log (%d)", len(m.errLog)))
	hint := "[↑/↓/PgUp/PgDn] Scroll  [Esc] Close"
	content := lipgloss.JoinVertical(lipgloss.Left, title, strings.Join(rows, "\n"), hint)
	return logStyle.Width(width).Height(height).Render(content)
}

func (l logModal) statusView(*model) string { return "" }
