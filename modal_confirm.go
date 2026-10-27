package main

import tea "charm.land/bubbletea/v2"

// confirmModal asks a yes/no question in the status bar: y runs onYes, n or Esc closes it,
// and any other key is ignored.
type confirmModal struct {
	prompt string // The question, without the (y/n)
	onYes  func(m *model) tea.Cmd
}

func newConfirmModal(prompt string, onYes func(m *model) tea.Cmd) confirmModal {
	return confirmModal{prompt: prompt, onYes: onYes}
}

func (c confirmModal) update(m *model, msg tea.Msg) (modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch key.String() {
	case "y", "Y":
		return nil, c.onYes(m)
	case "n", "N", "esc":
		return nil, nil
	}
	return c, nil
}

func (c confirmModal) paneView(*model, int, int) string { return "" }

func (c confirmModal) statusView(*model) string {
	return confirmPromptStyle.Render(c.prompt + " (y/n)")
}
