package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

// toastDuration is how long an error stays in the status bar; the log keeps it for good.
var toastDuration = 5 * time.Second

// maxLogEntries caps the error log, dropping the oldest entries.
const maxLogEntries = 500

// reportError adds err to the error log and shows it in the status bar until the returned
// command expires it. Joined errors (one per failed file) become one log entry each.
func (m *model) reportError(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	lines := strings.Split(err.Error(), "\n")
	now := time.Now()
	for _, line := range lines {
		m.errLog = append(m.errLog, logEntry{time: now, text: line})
	}
	if len(m.errLog) > maxLogEntries {
		m.errLog = m.errLog[len(m.errLog)-maxLogEntries:]
	}
	m.toast = lines[0]
	if key := m.keyOf("error_log"); len(lines) > 1 && key != "" {
		m.toast += fmt.Sprintf(" (+%d more, %s for the log)", len(lines)-1, key)
	} else if len(lines) > 1 {
		m.toast += fmt.Sprintf(" (+%d more in the log)", len(lines)-1)
	}
	m.toastID++
	id := m.toastID
	return tea.Tick(toastDuration, func(time.Time) tea.Msg { return toastExpiredMsg{id: id} })
}

// Update handles messages and updates the model. Results of commands are applied first,
// whatever dialog is open; everything else (keys, pastes) goes to the modal on top of the
// stack if there is one, and otherwise to the actions (see actions.go) and then to the
// active pane.
//
// Where a command comes from a method of m, it is assigned before returning because Go does
// not order the evaluation of m against the method call in `return m, m.f()`.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	if key, ok := msg.(tea.KeyPressMsg); ok {
		msg = layoutKey(key)
	}

	switch msg := msg.(type) {
	case directoryLoadedMsg:
		switch msg.paneID {
		case m.leftPane.id:
			m.leftPane.applyListing(msg)
		case m.rightPane.id:
			m.rightPane.applyListing(msg)
		}
		cmd = m.reportError(msg.err)
		return m, cmd
	case toastExpiredMsg:
		if msg.id == m.toastID { // A newer error restarted the timer
			m.toast = ""
		}
		return m, nil
	case tea.WindowSizeMsg:
		// Below the panes: the status bar (1 line) and the hint bar (3: text and padding).
		// The pane borders (2) are added around the height when rendering.
		paneHeight := msg.Height - 1 - 3 - 2
		paneWidth := msg.Width/2 - 2
		m.leftPane.height = paneHeight
		m.rightPane.height = paneHeight
		m.leftPane.width = paneWidth
		m.rightPane.width = paneWidth
		return m, nil
	case fileOpenedMsg:
		cmd = m.reportError(msg.err)
		return m, cmd
	case folderCreatedMsg:
		if msg.err != nil {
			cmd = m.reportError(msg.err)
			return m, cmd
		}
		// Reload the active pane with the cursor on the new folder
		return m, m.activePane().reloadCmd(msg.folderPath)
	case fileDeletedMsg:
		// Reload even after a failure: some of the files may be gone.
		cmd = m.reportError(msg.err)
		p := m.activePane()
		// Adjust cursor if it's out of bounds after deletion
		if p.cursor >= len(p.files)-1 && p.cursor > 0 {
			p.cursor--
		}
		return m, tea.Batch(cmd, p.reloadCmd(""))
	case transferPlannedMsg:
		if len(msg.plan.rejected) > 0 {
			cmd = m.reportError(errors.New(strings.Join(msg.plan.rejected, "\n")))
		}
		c := conflictModal{conflicts: msg.plan.conflicts, clean: msg.plan.clean, src: msg.src, dst: msg.dst, dest: msg.destPath, move: msg.move}
		if len(c.conflicts) > 0 {
			// Under any dialog opened while the batch was being planned: keys typed into that
			// dialog must not answer the overwrite questions. They come once it closes, and the
			// modal runs the batch when every conflict is decided.
			m.modals = slices.Insert(slices.Clone(m.modals), 0, modal(c))
			return m, cmd
		}
		return m, tea.Batch(cmd, c.run())
	case fileOperationMsg: // For copy/move operations
		// Reload both panes even after a failure: part of the batch may have gone through.
		cmd = m.reportError(msg.err)
		return m, tea.Batch(cmd, m.leftPane.reloadCmd(""), m.rightPane.reloadCmd(""))
	case driveUnmountedMsg:
		if msg.err != nil {
			cmd = m.reportError(msg.err)
			return m, cmd
		}
		m.drives = getMountedDrives() // The favorites list clamps its cursor to the new list
		return m, nil
	case previewReadyMsg:
		for i, md := range m.modals {
			if p, ok := md.(previewModal); ok && p.source == msg.source && p.path == msg.path {
				if msg.err != nil {
					m.setModal(i, nil) // Nothing to show
				} else {
					p.content = msg.content
					m.setModal(i, p)
				}
				break
			}
		}
		cmd = m.reportError(msg.err)
		return m, cmd
	}

	// The modal on top owns the keyboard: nothing reaches the panes while one is open.
	if len(m.modals) > 0 {
		cmd = m.updateTopModal(msg)
		return m, cmd
	}

	// A key of an action runs it; the panes never see it, even while the action is disabled.
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		if cmd, ok := m.runKey(msg); ok {
			return m, cmd
		}
	}

	// Anything else is navigation or quick search in the active pane.
	p := m.activePane()
	*p, cmd = p.update(msg)
	return m, cmd
}

// layoutKey makes a key read the same in any keyboard layout. A terminal with the Kitty
// keyboard protocol, asked for alternate keys (see View), also reports the key at the same
// place on a US layout, and Key.String prefers that one. It is kept only for a key that types
// something outside ASCII, so that Alt+F and Ctrl+C work in a Hebrew or Russian layout; a key
// that types ASCII reads as typed, so AZERTY and Dvorak keep the letters printed on the keys.
func layoutKey(key tea.KeyPressMsg) tea.KeyPressMsg {
	if key.Code <= unicode.MaxASCII {
		key.BaseCode = 0
	}
	return key
}
