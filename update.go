package main

import (
	"errors"
	"fmt"
	"path/filepath"
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
	if len(lines) > 1 {
		m.toast += fmt.Sprintf(" (+%d more, %s for the log)", len(lines)-1, m.keyMap.ErrorLog.Key)
	}
	m.toastID++
	id := m.toastID
	return tea.Tick(toastDuration, func(time.Time) tea.Msg { return toastExpiredMsg{id: id} })
}

// Update handles messages and updates the model. Results of commands are applied first,
// whatever dialog is open; everything else (keys, pastes) goes to the modal on top of the
// stack if there is one, and otherwise to the shortcuts and then to the active pane.
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

	if msg, ok := msg.(tea.KeyPressMsg); ok {
		if cmd, ok := m.shortcut(msg); ok {
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

// shortcut runs the KeyMap action bound to key, if any; ok is false for any other key.
func (m *model) shortcut(key tea.KeyPressMsg) (_ tea.Cmd, ok bool) {
	k := key.String()
	if mapKey, ok := m.aliasMap[k]; ok {
		k = mapKey
	}
	p := m.activePane()
	switch k {
	case m.keyMap.Quit.Key, m.keyMap.ForceQuit.Key:
		m.quitting = true
		return tea.Quit, true
	case m.keyMap.SwitchPane.Key:
		m.leftPane.active = !m.leftPane.active
		m.rightPane.active = !m.rightPane.active
		return nil, true
	case m.keyMap.Preview.Key:
		if f, ok := p.cursorFile(); ok && !f.IsDir {
			m.openModal(previewModal{source: p.source, path: f.Path})
			return previewFileCmd(p.source, f.Path), true
		}
		return nil, true
	case m.keyMap.Copy.Key, m.keyMap.Move.Key:
		files := p.targets()
		if len(files) == 0 {
			return nil, true
		}
		move, dst := k == m.keyMap.Move.Key, m.inactivePane()
		if err := localOnly(transferAction(move), p.source, dst.source); err != nil {
			return m.reportError(err), true
		}
		p.selected = make(map[string]struct{}) // Clear selection
		return planTransferCmd(p.source, files, dst.source, dst.path, move), true
	case m.keyMap.NewFolder.Key:
		if err := localOnly("MkDir", p.source); err != nil {
			return m.reportError(err), true
		}
		src, dir := p.source, p.path
		m.openModal(newInputModal("Create folder: ", func(_ *model, name string) tea.Cmd {
			if name == "" {
				return nil
			}
			return createFolderCmd(src, filepath.Join(dir, name))
		}))
		return nil, true
	case m.keyMap.Delete.Key:
		files := p.targets()
		if len(files) == 0 {
			return nil, true
		}
		if err := localOnly("Delete", p.source); err != nil {
			return m.reportError(err), true
		}
		src := p.source
		prompt := fmt.Sprintf("Delete %d items?", len(files))
		if len(files) == 1 {
			prompt = fmt.Sprintf("Delete %s?", files[0].Name)
		}
		m.openModal(newConfirmModal(prompt, func(m *model) tea.Cmd {
			m.activePane().selected = make(map[string]struct{}) // Clear selection
			return deleteFilesCmd(src, files)
		}))
		return nil, true
	case m.keyMap.CopyPath.Key:
		files := p.selectedFiles()
		if f, ok := p.cursorFile(); ok && len(files) == 0 { // ".." included: it names a directory
			files = []file{f}
		}
		if len(files) == 0 {
			return nil, true
		}
		var paths []string
		for _, f := range files {
			paths = append(paths, f.Path)
		}
		return copyToClipboardCmd(strings.Join(paths, "\n")), true
	case m.keyMap.ErrorLog.Key:
		m.openModal(logModal{})
		return nil, true
	case m.keyMap.Favorites.Key:
		m.openModal(favoritesModal{})
		return nil, true
	case m.keyMap.AddToFavorites.Key:
		if err := localOnly("Add Fav", p.source); err != nil {
			return m.reportError(err), true // Favorites are local directories
		}
		pathToAdd := p.path
		if f, ok := p.cursorFile(); ok && f.IsDir && !f.IsParent {
			pathToAdd = f.Path
		}
		if !slices.Contains(m.favorites, pathToAdd) {
			m.favorites = append(m.favorites, pathToAdd)
		}
		return nil, true
	case m.keyMap.SyncPanes.Key:
		return m.inactivePane().loadDirectoryCmd(p.source, p.path, ""), true
	case m.keyMap.OpenInOther.Key:
		if f, ok := p.cursorFile(); ok && f.IsDir {
			return m.inactivePane().loadDirectoryCmd(p.source, f.Path, ""), true
		}
		return nil, true
	}
	return nil, false
}
