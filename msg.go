package main

// Messages
type directoryLoadedMsg struct {
	paneID    int
	path      string // Directory that was loaded; the pane moves there only on success
	files     []file
	err       error
	focusPath string
}

type fileOpenedMsg struct {
	err error
}

type folderCreatedMsg struct {
	err        error
	folderPath string
}

type fileDeletedMsg struct {
	err error
}

type fileOperationMsg struct { // For copy/move
	err error
}

type transferPlannedMsg struct {
	plan     batchPlan
	destPath string
	move     bool
}

type previewReadyMsg struct {
	path    string // File read, so that a preview closed in the meantime ignores it
	content string
	err     error
}

type driveUnmountedMsg struct {
	err       error
	drivePath string
}

type toastExpiredMsg struct {
	id int
}
