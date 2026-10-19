package main

import (
	"fmt"
	"slices"

	tea "charm.land/bubbletea/v2"
)

// conflictModal asks about each file of a copy or move batch that already exists at the
// destination, in the status bar, then runs the whole batch: the files without a conflict
// together with the ones chosen for overwriting. Nothing is copied or moved before every
// conflict is decided, and Esc cancels the whole batch.
type conflictModal struct {
	conflicts   []fileConflict // Still waiting for a decision; the first is asked about
	clean       []file         // Files of the batch with no conflict
	toOverwrite []file         // Conflicting files the user chose to overwrite
	dest        string         // Destination directory
	move        bool
}

func (c conflictModal) update(_ *model, msg tea.Msg) (modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch key.String() {
	case "y", "Y": // Overwrite this file
		c = c.decide(1, true)
	case "n", "N": // Skip this file
		c = c.decide(1, false)
	case "a", "A": // Overwrite all remaining
		c = c.decide(len(c.conflicts), true)
	case "s", "S": // Skip all remaining
		c = c.decide(len(c.conflicts), false)
	case "esc": // Cancel the whole batch
		return nil, nil
	}
	if len(c.conflicts) > 0 {
		return c, nil
	}
	return nil, c.run()
}

// decide answers the next n conflicts: overwrite them, or skip them.
func (c conflictModal) decide(n int, overwrite bool) conflictModal {
	if overwrite {
		c.toOverwrite = slices.Clip(c.toOverwrite) // Appending must not write into an older copy's array
		for _, conflict := range c.conflicts[:n] {
			c.toOverwrite = append(c.toOverwrite, conflict.Source)
		}
	}
	c.conflicts = c.conflicts[n:]
	return c
}

// run returns the command that copies or moves the batch once every conflict is decided.
func (c conflictModal) run() tea.Cmd {
	files := append(slices.Clip(c.clean), c.toOverwrite...)
	if len(files) == 0 {
		return nil
	}
	return transferFilesCmd(files, c.dest, c.move)
}

func (c conflictModal) paneView(*model, int, int) string { return "" }

func (c conflictModal) statusView(*model) string {
	return overwritePromptStyle.Render(fmt.Sprintf("Overwrite %s? (y/n/A/s)", c.conflicts[0].Source.Name))
}
