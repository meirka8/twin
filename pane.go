package main

import (
	"cmp"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// pane is one of the two file listing panels: a directory of a Source, with a cursor, a
// selection and a quick search. Update hands it the keys no shortcut or modal takes.
type pane struct {
	id        int
	source    Source // Where the entries come from
	path      string // Directory shown, in the source's own terms
	files     []file
	selected  map[string]struct{} // Paths of selected files
	cursor    int
	active    bool
	viewportY int             // Top of the visible area in the file list
	height    int             // Height of the pane's content, inside its border
	width     int             // Width of the pane's content, inside its border
	search    textinput.Model // Quick-search query
}

// newPane returns an empty pane on dir of src; loadDirectoryCmd fills it.
func newPane(id int, src Source, dir string) pane {
	search := textinput.New()
	search.Focus() // It only gets the keys pane.update passes on to it
	return pane{id: id, source: src, path: dir, selected: make(map[string]struct{}), search: search}
}

// boxSize is the size of the pane on screen, border included. Modals shown in place of
// the pane fill the same box.
func (p pane) boxSize() (width, height int) {
	return p.width + 2, p.height + 2
}

// loadDirectoryCmd lists dir of src for the pane: its own location to reload, another one
// to navigate. The pane moves there only once the listing succeeds (see applyListing).
func (p pane) loadDirectoryCmd(src Source, dir, focusPath string) tea.Cmd {
	return func() tea.Msg {
		files, err := listDir(src, dir)
		return directoryLoadedMsg{paneID: p.id, source: src, path: dir, files: files, err: err, focusPath: focusPath}
	}
}

// reloadCmd lists the pane's own directory again, with the cursor on focusPath if given.
func (p pane) reloadCmd(focusPath string) tea.Cmd {
	return p.loadDirectoryCmd(p.source, p.path, focusPath)
}

// listDir lists dir of src the way a pane shows it: "..", if dir has a parent, then the
// directories, then the files, each sorted by name. On error the listing still has "..",
// so a pane is never stranded without a way up.
func listDir(src Source, dir string) ([]file, error) {
	files, err := src.List(dir)
	if err != nil {
		files = nil
	}
	slices.SortFunc(files, func(a, b file) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Name, b.Name)
	})
	if parent, ok := src.Parent(dir); ok {
		dotdot := file{Name: "..", Path: parent, IsDir: true, IsParent: true, Mode: os.ModeDir, ModTime: time.Now()}
		files = append([]file{dotdot}, files...)
	}
	return files, err
}

// applyListing installs a listing from loadDirectoryCmd. A listing of another location
// moves the pane there; if loading it failed, the pane stays where it was with its old
// listing. A failed reload of the pane's own directory still carries "..", so there is a
// way out.
func (p *pane) applyListing(msg directoryLoadedMsg) {
	if msg.source != p.source || msg.path != p.path {
		if msg.err != nil {
			return
		}
		if msg.source != p.source {
			// The paths of another source are not this one's: a selection kept across would
			// make different files the targets of the next operation.
			p.selected = make(map[string]struct{})
		}
		p.source, p.path = msg.source, msg.path
		p.cursor = 0
		p.viewportY = 0
	}
	p.files = msg.files
	if msg.focusPath != "" {
		for i, f := range p.files {
			if f.Path == msg.focusPath {
				p.cursor = i
				// Adjust viewport to make cursor visible
				if p.cursor >= p.viewportY+p.height-2 {
					p.viewportY = p.cursor - p.height + 3
				}
				break
			}
		}
	}
	// Ensure cursor and viewport are within bounds after the listing shrank
	p.cursor = clampIndex(p.cursor, len(p.files))
	p.viewportY = min(p.viewportY, p.cursor)
}

// cursorFile is the entry under the cursor; ok is false in an empty listing.
func (p pane) cursorFile() (_ file, ok bool) {
	if p.cursor >= len(p.files) {
		return file{}, false
	}
	return p.files[p.cursor], true
}

// selectedFiles returns the selected entries, in listing order. ".." is never one of them.
func (p pane) selectedFiles() []file {
	var files []file
	for _, f := range p.files {
		if _, ok := p.selected[f.Path]; ok && !f.IsParent {
			files = append(files, f)
		}
	}
	return files
}

// targets returns the entries an operation acts on: the selection, or the entry under the
// cursor when nothing is selected. ".." is never a target.
func (p pane) targets() []file {
	files := p.selectedFiles()
	if f, ok := p.cursorFile(); ok && len(files) == 0 && !f.IsParent {
		files = []file{f}
	}
	return files
}

// update handles messages for a pane.
func (p pane) update(msg tea.Msg) (pane, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "home":
			p.cursor = 0
		case "end":
			p.cursor = max(len(p.files)-1, 0)
		case "up":
			p.search.Reset() // Clear search on navigation
			if p.cursor > 0 {
				p.cursor--
			}
		case "down":
			p.search.Reset() // Clear search on navigation
			if p.cursor < len(p.files)-1 {
				p.cursor++
			}
		case "pgup":
			p.search.Reset() // Clear search on navigation
			p.cursor = max(p.cursor-p.height, 0)
		case "pgdown":
			p.search.Reset() // Clear search on navigation
			p.cursor = clampIndex(p.cursor+p.height, len(p.files))
		case "enter":
			p.search.Reset() // Clear search on navigation
			if f, ok := p.cursorFile(); ok {
				switch {
				case f.IsParent: // Back up with the cursor on the directory we leave
					return p, p.loadDirectoryCmd(p.source, f.Path, p.path)
				case f.IsDir:
					return p, p.loadDirectoryCmd(p.source, f.Path, "")
				default:
					return p, openFileCmd(p.source, f.Path)
				}
			}
		case "esc":
			p.search.Reset() // Clear search explicitly
		case "insert":
			if f, ok := p.cursorFile(); ok {
				if !f.IsParent {
					if _, ok := p.selected[f.Path]; ok {
						delete(p.selected, f.Path)
					} else {
						p.selected[f.Path] = struct{}{}
					}
				}
				// Move cursor down after selection/deselection
				if p.cursor < len(p.files)-1 {
					p.cursor++
				}
			}
		case "backspace":
			if p.search.Value() != "" {
				p = p.quickSearch(msg)
			}
		default:
			if msg.Text != "" { // A typed character, in any script
				p = p.quickSearch(msg)
			}
		}
	case tea.PasteMsg:
		p = p.quickSearch(oneLinePaste(msg))
	}

	// Ensure viewport is within bounds
	if p.cursor < p.viewportY {
		p.viewportY = p.cursor
	}
	if p.cursor >= p.viewportY+p.height-2 {
		p.viewportY = p.cursor - p.height + 3
	}

	return p, nil
}

// quickSearch edits the quick-search query with msg (a typed character, Backspace or a
// paste) and moves the cursor to the first entry whose name starts with the query, or else
// to the first one that contains its characters in order.
func (p pane) quickSearch(msg tea.Msg) pane {
	// The query's cursor is never drawn, so the command that would make it blink is dropped.
	p.search, _ = p.search.Update(msg)
	query := strings.ToLower(p.search.Value())
	if query == "" {
		return p
	}
	if i := slices.IndexFunc(p.files, func(f file) bool { return strings.HasPrefix(strings.ToLower(f.Name), query) }); i >= 0 {
		p.cursor = i
	} else if i := slices.IndexFunc(p.files, func(f file) bool { return fuzzyMatch(query, f.Name) }); i >= 0 {
		p.cursor = i
	}
	return p
}

// view renders the pane: its path, then the entries that fit, in its border.
func (p pane) view() string {
	var s strings.Builder
	s.WriteString(p.path + "\n")

	for i := p.viewportY; i < len(p.files) && i < p.viewportY+p.height-2; i++ {
		f := p.files[i]
		name := f.Name
		if f.IsSymlink {
			name += "@" // As in ls -F
		}
		line := " " + name
		if f.IsDir {
			line = " " + dirStyle.Render(name)
		}

		_, isSelected := p.selected[f.Path]

		if i == p.cursor {
			s.WriteString(cursorStyle.Render(line))
		} else if isSelected {
			s.WriteString(selectionStyle.Render(line))
		} else {
			s.WriteString(line)
		}
		s.WriteString("\n")
	}

	style := inactiveStyle
	if p.active {
		style = activeStyle
	}

	width, height := p.boxSize()
	return style.Width(width).Height(height).Render(s.String())
}
