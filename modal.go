package main

import (
	"slices"

	tea "charm.land/bubbletea/v2"
)

// A modal is a dialog over the panes: a prompt in the status bar, or a panel in place of
// the active pane. Open modals form a stack; the one on top gets every key press, paste and
// any other message Update does not handle itself, so nothing reaches the panes or the
// modals below it until it closes. The panes show the topmost modal with a pane view, so a
// question asked from a panel leaves the panel visible; the status bar shows only the top
// modal's line.
//
// Modals are values, like the model: update returns the modal's new state.
type modal interface {
	// update handles a message while the modal is on top. It returns the modal's new state,
	// or nil to close it. It can open another modal over itself with m.openModal.
	update(m *model, msg tea.Msg) (modal, tea.Cmd)
	// paneView renders the modal in place of the active pane, in a box of width×height
	// cells border included, or returns "" to leave the pane to the modals below it.
	paneView(m *model, width, height int) string
	// statusView renders the modal's line in the status bar while it is on top, or returns
	// "" to leave the status bar to the file under the cursor and the error toast.
	statusView(m *model) string
}

// openModal puts md on top of the stack.
func (m *model) openModal(md modal) {
	m.modals = append(slices.Clip(m.modals), md) // Never writes into a stack another model shares
}

// setModal replaces the modal at index i of the stack with md, or removes it if md is nil.
func (m *model) setModal(i int, md modal) {
	m.modals = slices.Clone(m.modals)
	if md == nil {
		m.modals = slices.Delete(m.modals, i, i+1)
	} else {
		m.modals[i] = md
	}
}

// updateTopModal hands msg to the modal on top of the stack. Any modal it opens stays above
// it, or takes its place if it closed.
func (m *model) updateTopModal(msg tea.Msg) tea.Cmd {
	i := len(m.modals) - 1
	next, cmd := m.modals[i].update(m, msg)
	m.setModal(i, next)
	return cmd
}
