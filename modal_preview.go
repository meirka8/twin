package main

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// previewModal shows a text file in place of the active pane, wrapped to its width.
type previewModal struct {
	path    string // File shown; previewReadyMsg for any other file is stale
	content string // Empty until previewFileCmd reports back
	scrollY int    // First line shown
}

// previewTextSize is the size of the text area of a preview drawn in a width×height box.
func previewTextSize(width, height int) (int, int) {
	return max(width-previewStyle.GetHorizontalFrameSize(), 0), max(height-previewStyle.GetVerticalFrameSize(), 0)
}

func (p previewModal) update(m *model, msg tea.Msg) (modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	width, height := previewTextSize(m.activePane().boxSize())
	maxScroll := max(len(calculateWrappedLines(p.content, width))-height, 0)
	switch key.String() {
	case "esc", "q":
		return nil, nil
	case "up", "k":
		p.scrollY--
	case "down", "j":
		p.scrollY++
	case "pgup":
		p.scrollY -= height
	case "pgdown":
		p.scrollY += height
	case "home", "g":
		p.scrollY = 0
	case "end", "G":
		p.scrollY = maxScroll
	}
	p.scrollY = max(min(p.scrollY, maxScroll), 0)
	return p, nil
}

func (p previewModal) paneView(_ *model, width, height int) string {
	textWidth, textHeight := previewTextSize(width, height)
	lines := calculateWrappedLines(p.content, textWidth)
	start := max(min(p.scrollY, len(lines)-textHeight), 0) // The pane may have grown since
	end := min(start+textHeight, len(lines))
	return previewStyle.Width(width).Height(height).Render(strings.Join(lines[start:end], "\n"))
}

func (p previewModal) statusView(*model) string { return "" }
