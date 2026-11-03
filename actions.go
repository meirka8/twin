package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// An action is one of Twin's commands. Its keys run it, and the hint bar and the shortcut
// table in DOCUMENTATION.md are generated from the list of actions; so are the help screen,
// the command palette and the keymaps of the config to be.
type action struct {
	name  string   // Identifier, for keymaps in the config: "copy"
	title string   // Label in the hint bar: "Copy"
	help  string   // What it does, for the documentation
	keys  []string // Keys that run it, as tea.Key.String spells them; the hint bar shows the first
	// enabled reports whether the action has anything to act on now; nil means it always
	// has. The keys of a disabled action do nothing.
	enabled func(m *model) bool
	// run performs the action. Call do instead, which runs it only while it is enabled.
	run func(m *model) tea.Cmd
}

// do runs a on m if it is enabled.
func (a action) do(m *model) tea.Cmd {
	if a.enabled != nil && !a.enabled(m) {
		return nil
	}
	return a.run(m)
}

// defaultActions returns every action with its default keys, in the order of the hint bar
// and of the documentation. Quit comes first, so that it still shows on a narrow terminal.
func defaultActions() []action {
	return []action{
		{name: "quit", title: "Quit", help: "Quit, without asking", keys: []string{"alt+q", "f10", "ctrl+c"}, run: (*model).quit},
		{name: "switch_pane", title: "Switch Pane", help: "Switch the active pane", keys: []string{"tab"}, run: (*model).switchPane},
		{name: "preview", title: "View", help: "View the file under the cursor", keys: []string{"alt+v", "f3"}, enabled: onFile, run: (*model).preview},
		{name: "copy", title: "Copy", help: "Copy to the other pane", keys: []string{"alt+c", "f5"}, enabled: hasTargets, run: func(m *model) tea.Cmd { return m.transfer(false) }},
		{name: "move", title: "Move", help: "Move to the other pane", keys: []string{"alt+m", "f6"}, enabled: hasTargets, run: func(m *model) tea.Cmd { return m.transfer(true) }},
		{name: "mkdir", title: "MkDir", help: "Create a folder in the active pane", keys: []string{"alt+n", "f7"}, run: (*model).newFolder},
		{name: "delete", title: "Delete", help: "Delete (permanently)", keys: []string{"alt+d", "f8"}, enabled: hasTargets, run: (*model).deleteTargets},
		{name: "copy_path", title: "Copy Path", help: "Copy the full path to the clipboard", keys: []string{"alt+p", "f9"}, enabled: onEntry, run: (*model).copyPath},
		{name: "select", title: "Select", help: "Select or deselect the entry under the cursor, then move down", keys: []string{"insert"}, enabled: onEntry, run: (*model).toggleSelection},
		{name: "favorites", title: "Favorites", help: "Favorites and drives", keys: []string{"alt+f"}, run: opens(favoritesModal{})},
		{name: "add_favorite", title: "Add Fav", help: "Add a directory to the favorites", keys: []string{"alt+a"}, run: (*model).addFavorite},
		{name: "sync_panes", title: "Sync Panes", help: "Sync panes: open the active pane's directory in the other pane", keys: []string{"alt+i"}, run: (*model).syncPanes},
		{name: "open_in_other", title: "Open in Other", help: "Open the directory under the cursor in the other pane", keys: []string{"alt+o"}, enabled: onDirectory, run: (*model).openInOther},
		{name: "error_log", title: "Log", help: "Error log", keys: []string{"alt+l"}, run: opens(logModal{})},
	}
}

// setActions gives m its actions, and the keymap that maps each of their keys to the action
// it runs. The two are only ever set together, so they always agree.
func (m *model) setActions(actions []action) {
	m.actions = actions
	m.keymap = make(map[string]action)
	for _, a := range actions {
		for _, k := range a.keys {
			m.keymap[k] = a
		}
	}
}

// keyOf returns the key the hint bar shows for the action called name, or "" if it has no
// key.
func (m *model) keyOf(name string) string {
	for _, a := range m.actions {
		if a.name == name && len(a.keys) > 0 {
			return a.keys[0]
		}
	}
	return ""
}

// keyRuns reports whether key runs the action called name.
func (m *model) keyRuns(key tea.KeyPressMsg, name string) bool {
	a, ok := m.keymap[key.String()]
	return ok && a.name == name
}

// runKey runs the action bound to key, if it is enabled; ok is false when no action is.
func (m *model) runKey(key tea.KeyPressMsg) (_ tea.Cmd, ok bool) {
	a, ok := m.keymap[key.String()]
	if !ok {
		return nil, false
	}
	return a.do(m), true
}

// Conditions for action.enabled, on the active pane.

func onEntry(m *model) bool {
	_, ok := m.activePane().cursorFile()
	return ok
}

func onFile(m *model) bool {
	f, ok := m.activePane().cursorFile()
	return ok && !f.IsDir
}

func onDirectory(m *model) bool {
	f, ok := m.activePane().cursorFile()
	return ok && f.IsDir
}

func hasTargets(m *model) bool { return len(m.activePane().targets()) > 0 }

// opens returns the run function of an action that opens md.
func opens(md modal) func(*model) tea.Cmd {
	return func(m *model) tea.Cmd {
		m.openModal(md)
		return nil
	}
}

// The run functions of the actions.

func (m *model) quit() tea.Cmd {
	m.quitting = true
	return tea.Quit
}

func (m *model) switchPane() tea.Cmd {
	m.leftPane.active = !m.leftPane.active
	m.rightPane.active = !m.rightPane.active
	return nil
}

func (m *model) preview() tea.Cmd {
	p := m.activePane()
	f, _ := p.cursorFile()
	m.openModal(previewModal{source: p.source, path: f.Path})
	return previewFileCmd(p.source, f.Path)
}

// transfer copies or moves the targets of the active pane to the other one.
func (m *model) transfer(move bool) tea.Cmd {
	p, dst := m.activePane(), m.inactivePane()
	if err := localOnly(transferAction(move), p.source, dst.source); err != nil {
		return m.reportError(err)
	}
	files := p.targets()
	p.selected = make(map[string]struct{}) // Clear selection
	return planTransferCmd(p.source, files, dst.source, dst.path, move)
}

func (m *model) newFolder() tea.Cmd {
	p := m.activePane()
	if err := localOnly("MkDir", p.source); err != nil {
		return m.reportError(err)
	}
	src, dir := p.source, p.path
	m.openModal(newInputModal("Create folder: ", func(_ *model, name string) tea.Cmd {
		if name == "" {
			return nil
		}
		return createFolderCmd(src, filepath.Join(dir, name))
	}))
	return nil
}

func (m *model) deleteTargets() tea.Cmd {
	p := m.activePane()
	if err := localOnly("Delete", p.source); err != nil {
		return m.reportError(err)
	}
	src, files := p.source, p.targets()
	prompt := fmt.Sprintf("Delete %d items?", len(files))
	if len(files) == 1 {
		prompt = fmt.Sprintf("Delete %s?", files[0].Name)
	}
	m.openModal(newConfirmModal(prompt, func(m *model) tea.Cmd {
		m.activePane().selected = make(map[string]struct{}) // Clear selection
		return deleteFilesCmd(src, files)
	}))
	return nil
}

func (m *model) copyPath() tea.Cmd {
	p := m.activePane()
	files := p.selectedFiles()
	if len(files) == 0 { // The entry under the cursor, ".." included: it names a directory
		f, _ := p.cursorFile()
		files = []file{f}
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return copyToClipboardCmd(strings.Join(paths, "\n"))
}

func (m *model) toggleSelection() tea.Cmd {
	m.activePane().toggleSelection()
	return nil
}

func (m *model) addFavorite() tea.Cmd {
	p := m.activePane()
	if err := localOnly("Add Fav", p.source); err != nil {
		return m.reportError(err) // Favorites are local directories
	}
	pathToAdd := p.path
	if f, ok := p.cursorFile(); ok && f.IsDir && !f.IsParent {
		pathToAdd = f.Path
	}
	if !slices.Contains(m.favorites, pathToAdd) {
		m.favorites = append(m.favorites, pathToAdd)
	}
	return nil
}

func (m *model) syncPanes() tea.Cmd {
	p := m.activePane()
	return m.inactivePane().loadDirectoryCmd(p.source, p.path, "")
}

func (m *model) openInOther() tea.Cmd {
	p := m.activePane()
	f, _ := p.cursorFile()
	return m.inactivePane().loadDirectoryCmd(p.source, f.Path, "")
}
