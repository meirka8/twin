package main

import (
	"io/fs"
	"log"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	id          int
	path        string
	files       []file
	selected    map[string]struct{} // Paths of selected files
	cursor      int
	active      bool
	viewportY   int // Top of the visible area in the file list
	height      int // Height of the pane's display area
	width       int // Width of the pane's display area
	searchQuery string
}

// logEntry is one line of the error log.
type logEntry struct {
	time time.Time
	text string
}

// model is the main application model.
type model struct {
	leftPane              pane
	rightPane             pane
	quitting              bool
	toast                 string     // Latest error, shown in the status bar until it expires
	toastID               int        // Identifies the toast a toastExpiredMsg is meant for
	errLog                []logEntry // Every error reported, oldest first
	isLogOpen             bool
	logScrollY            int // Lines scrolled up from the newest log entry
	isCreatingFolder      bool
	folderNameInput       string
	isDeleting            bool
	filesToDelete         []file
	isConfirmingOverwrite bool
	overwriteConflicts    []fileConflict // Conflicts still waiting for a decision
	pendingClean          []file         // Files of the batch with no conflict, executed with the resolved ones
	toOverwrite           []file         // Conflicting files the user chose to overwrite
	operationDest         string         // Destination directory of the pending copy/move
	isMoving              bool           // To know if the operation is a move or copy
	isPreviewing          bool
	previewContent        string
	previewFilePath       string
	previewWidth          int
	previewHeight         int
	previewScrollY        int
	keyMap                KeyMap
	modifierState         ModifierState
	aliasMap              map[string]string
	favorites             []string
	drives                []string
	isFavoritesOpen       bool
	favoritesCursor       int
	isConfirmingRemoveFav bool
	favToRemove           int
	isConfirmingUnmount   bool
	driveToUnmount        string
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

	favorites := getStandardPaths()
	drives := getMountedDrives()

	km := DefaultKeyMap()
	return model{
		leftPane: pane{
			id:       0,
			path:     cwd,
			active:   true,
			selected: make(map[string]struct{}),
		},
		rightPane: pane{
			id:       1,
			path:     cwd,
			active:   false,
			selected: make(map[string]struct{}),
		},
		keyMap:    km,
		aliasMap:  km.GetAliasMap(),
		favorites: favorites,
		drives:    drives,
	}
}

// Init initializes the application.
func (m model) Init() tea.Cmd {
	return tea.Batch(m.leftPane.loadDirectoryCmd(m.leftPane.path, ""), m.rightPane.loadDirectoryCmd(m.rightPane.path, ""))
}
