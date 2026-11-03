package main

import (
	"io/fs"
	"log"
	"os"
	"time"

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

// logEntry is one line of the error log.
type logEntry struct {
	time time.Time
	text string
}

// model is the main application model.
type model struct {
	leftPane  pane
	rightPane pane
	modals    []modal // Open dialogs, the one on top last; see modal.go
	quitting  bool
	toast     string            // Latest error, shown in the status bar until it expires
	toastID   int               // Identifies the toast a toastExpiredMsg is meant for
	errLog    []logEntry        // Every error reported, oldest first
	actions   []action          // Every action with its keys, in hint-bar order; see actions.go
	keymap    map[string]action // The action each key runs; set with actions by setActions
	favorites []string
	drives    []string
}

// initialModel creates the model Twin starts with: both panes on the working directory.
func initialModel() model {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	m := newModel(cwd, cwd)
	m.favorites = getStandardPaths()
	m.drives = getMountedDrives()
	return m
}

// newModel returns a model with the left pane, active, on left and the right one on right,
// with the default keys and no favorites. Init loads the panes.
func newModel(left, right string) model {
	m := model{
		leftPane:  newPane(0, localFS{}, left),
		rightPane: newPane(1, localFS{}, right),
	}
	m.leftPane.active = true
	m.setActions(defaultActions())
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
	return tea.Batch(m.leftPane.reloadCmd(""), m.rightPane.reloadCmd(""))
}
