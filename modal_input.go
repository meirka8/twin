package main

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// inputModal asks for a line of text in the status bar: Enter passes it to onSubmit, Esc
// cancels. The field takes text in any script, the terminal's paste, and the usual editing
// keys (←/→, Home/End, Backspace/Delete, Ctrl+W, Ctrl+U).
type inputModal struct {
	input    textinput.Model
	onSubmit func(m *model, value string) tea.Cmd
}

func newInputModal(prompt string, onSubmit func(m *model, value string) tea.Cmd) inputModal {
	input := textinput.New()
	input.Prompt = prompt
	input.SetStyles(statusInputStyles)
	// Its own Ctrl+V reads the clipboard behind oneLinePaste's back and keeps a failure to
	// itself; the terminal's paste arrives as a tea.PasteMsg anyway.
	input.KeyMap.Paste.SetEnabled(false)
	input.Focus()
	return inputModal{input: input, onSubmit: onSubmit}
}

func (in inputModal) update(m *model, msg tea.Msg) (modal, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "enter":
			return nil, in.onSubmit(m, in.input.Value())
		case "esc":
			return nil, nil
		}
	case tea.PasteMsg:
		return in.edit(oneLinePaste(msg))
	}
	return in.edit(msg)
}

// edit passes msg on to the text field.
func (in inputModal) edit(msg tea.Msg) (modal, tea.Cmd) {
	var cmd tea.Cmd
	in.input, cmd = in.input.Update(msg)
	return in, cmd
}

func (in inputModal) paneView(*model, int, int) string { return "" }

func (in inputModal) statusView(*model) string {
	return inputPromptStyle.Render(in.input.View())
}
