package main

// Shortcut represents a keyboard shortcut with a description.
type Shortcut struct {
	Key      string // The actual key to press (e.g., "c", "q", "f10")
	Modifier string // "ctrl", "alt", "shift", or empty
	Action   string // Description for the UI
	Cmd      string // Internal command identifier
}

// KeyMap holds all application shortcuts.
type KeyMap struct {
	Quit            Shortcut
	ForceQuit       Shortcut
	SwitchPane      Shortcut
	Preview         Shortcut
	Copy            Shortcut
	Move            Shortcut
	NewFolder       Shortcut
	Delete          Shortcut
	CopyPath        Shortcut
	ToggleSelection Shortcut
}

// DefaultKeyMap returns the default key mapping.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit:            Shortcut{Key: "q", Modifier: "alt", Action: "Quit", Cmd: "quit"},
		ForceQuit:       Shortcut{Key: "c", Modifier: "ctrl", Action: "Force Quit", Cmd: "force_quit"},
		SwitchPane:      Shortcut{Key: "tab", Modifier: "", Action: "Switch Pane", Cmd: "switch_pane"},
		Preview:         Shortcut{Key: "v", Modifier: "alt", Action: "View", Cmd: "preview"},
		Copy:            Shortcut{Key: "c", Modifier: "alt", Action: "Copy", Cmd: "copy"},
		Move:            Shortcut{Key: "m", Modifier: "alt", Action: "Move", Cmd: "move"},
		NewFolder:       Shortcut{Key: "n", Modifier: "alt", Action: "MkDir", Cmd: "mkdir"},
		Delete:          Shortcut{Key: "d", Modifier: "alt", Action: "Delete", Cmd: "delete"},
		CopyPath:        Shortcut{Key: "p", Modifier: "alt", Action: "Copy Path", Cmd: "copy_path"},
		ToggleSelection: Shortcut{Key: "i", Modifier: "alt", Action: "Select", Cmd: "select"},
	}
}

// GetShortcuts returns a slice of all shortcuts for iteration.
func (k KeyMap) GetShortcuts() []Shortcut {
	return []Shortcut{
		k.Quit,
		k.ForceQuit,
		k.SwitchPane,
		k.Preview,
		k.Copy,
		k.Move,
		k.NewFolder,
		k.Delete,
		k.CopyPath,
		k.ToggleSelection,
	}
}
