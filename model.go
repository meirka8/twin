package main

import (
	"io/fs"
	"log"
	"os"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// file represents a file or directory entry.
type file struct {
	Name    string
	Path    string
	Size    int64
	Mode    fs.FileMode
	ModTime time.Time
	IsDir   bool
	// IsParent marks the synthetic ".." entry. It is only for navigation and is
	// never a target of selection or file operations.
	IsParent bool
	// IsSymlink marks a symbolic link. IsDir then describes the link's target, so a link
	// to a directory is entered like one, but operations act on the link itself.
	IsSymlink bool
}

// fileConflict represents a file that already exists at the destination.
type fileConflict struct {
	Source      file
	Destination string
}

// pane represents one of the two file listing panels.
type pane struct {
	id        int
	path      string
	files     []file
	selected  map[string]struct{} // Paths of selected files
	cursor    int
	active    bool
	viewportY int             // Top of the visible area in the file list
	height    int             // Height of the pane's content, inside its border
	width     int             // Width of the pane's content, inside its border
	search    textinput.Model // Quick-search query
}

// newPane returns an empty pane; loadDirectoryCmd fills it.
func newPane(id int, path string) pane {
	search := textinput.New()
	search.Focus() // It only gets the keys pane.update passes on to it
	return pane{id: id, path: path, selected: make(map[string]struct{}), search: search}
}

// boxSize is the size of the pane on screen, border included. Modals shown in place of
// the pane fill the same box.
func (p pane) boxSize() (width, height int) {
	return p.width + 2, p.height + 2
}

// logEntry is one line of the error log.
type logEntry struct {
	time time.Time
	text string
}

// model is the main application model.
type model struct {
	leftPane      pane
	rightPane     pane
	modals        []modal // Open dialogs, the one on top last; see modal.go
	quitting      bool
	toast         string     // Latest error, shown in the status bar until it expires
	toastID       int        // Identifies the toast a toastExpiredMsg is meant for
	errLog        []logEntry // Every error reported, oldest first
	keyMap        KeyMap
	modifierState ModifierState
	aliasMap      map[string]string
	favorites     []string
	drives        []string
}

// ModifierState tracks the state of modifier keys.
type ModifierState struct {
	Ctrl  bool
	Alt   bool
	Shift bool
}

// initialModel creates a new model with default state.
func initialModel() model {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	km := DefaultKeyMap()
	m := model{
		leftPane:  newPane(0, cwd),
		rightPane: newPane(1, cwd),
		keyMap:    km,
		aliasMap:  km.GetAliasMap(),
		favorites: getStandardPaths(),
		drives:    getMountedDrives(),
	}
	m.leftPane.active = true
	return m
}

// activePane is the pane the keyboard acts on; copy and move go from it to the inactive one.
func (m *model) activePane() *pane {
	if m.rightPane.active {
		return &m.rightPane
	}
	return &m.leftPane
}

func (m *model) inactivePane() *pane {
	if m.rightPane.active {
		return &m.leftPane
	}
	return &m.rightPane
}

// Init initializes the application.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.leftPane.loadDirectoryCmd(m.leftPane.path, ""), m.rightPane.loadDirectoryCmd(m.rightPane.path, ""))
}
