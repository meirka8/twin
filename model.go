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
		leftPane:  newPane(0, localFS{}, cwd),
		rightPane: newPane(1, localFS{}, cwd),
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
	return tea.Batch(m.leftPane.reloadCmd(""), m.rightPane.reloadCmd(""))
}
