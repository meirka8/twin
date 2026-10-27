package main

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// favoritesModal lists the favorites, then the mounted drives, in place of the active pane.
// Enter opens the entry under the cursor in the active pane; d asks to remove a favorite or
// to unmount a drive.
type favoritesModal struct {
	cursor int // Index into the favorites followed by the drives
}

func (f favoritesModal) update(m *model, msg tea.Msg) (modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	entries := slices.Concat(m.favorites, m.drives)
	f.cursor = clampIndex(f.cursor, len(entries)) // An unmount or a removal shortens the list
	switch key.String() {
	case "esc", "q", m.keyMap.Favorites.Key:
		return nil, nil
	case "up", "k":
		f.cursor--
	case "down", "j":
		f.cursor++
	case "home":
		f.cursor = 0
	case "end":
		f.cursor = len(entries) - 1
	case "enter":
		if len(entries) > 0 {
			return nil, m.activePane().loadDirectoryCmd(entries[f.cursor], "")
		}
	case "delete", "d":
		if len(entries) == 0 {
			break
		}
		path := entries[f.cursor]
		if f.cursor < len(m.favorites) {
			m.openModal(newConfirmModal(fmt.Sprintf("Remove '%s' from favorites?", path), func(m *model) tea.Cmd {
				m.favorites = slices.DeleteFunc(slices.Clone(m.favorites), func(p string) bool { return p == path })
				return nil
			}))
		} else {
			m.openModal(newConfirmModal(fmt.Sprintf("Unmount '%s'?", path), func(*model) tea.Cmd {
				return unmountDriveCmd(path)
			}))
		}
	}
	f.cursor = clampIndex(f.cursor, len(entries))
	return f, nil
}

func (f favoritesModal) paneView(m *model, width, height int) string {
	cursor := clampIndex(f.cursor, len(m.favorites)+len(m.drives))
	var s strings.Builder
	s.WriteString("Favorites\n\n")
	item := func(i int, path string) {
		if i == cursor {
			s.WriteString(selectionStyle.Render("> " + path))
		} else {
			s.WriteString("  " + path)
		}
		s.WriteString("\n")
	}
	for i, fav := range m.favorites {
		item(i, fav)
	}
	if len(m.drives) > 0 {
		s.WriteString(separatorStyle.Render("──────────────────────") + "\n")
		for i, drive := range m.drives {
			item(len(m.favorites)+i, drive)
		}
	}

	deleteHint := "[Delete/d] Remove"
	if cursor >= len(m.favorites) {
		deleteHint = "[d] Unmount"
	}
	s.WriteString("\n[Enter] Go  [Esc] Close  " + deleteHint)

	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, popupStyle.Render(s.String()))
}

func (f favoritesModal) statusView(*model) string { return "" }
