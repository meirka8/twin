package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestActionsAreComplete(t *testing.T) {
	names := make(map[string]bool)
	keys := make(map[string]string) // Key -> the action it runs
	for _, a := range defaultActions() {
		if a.name == "" || a.title == "" || a.help == "" || len(a.keys) == 0 || a.run == nil {
			t.Errorf("%q lacks a name, title, help line, key or run function", a.name)
		}
		if names[a.name] {
			t.Errorf("two actions are called %q", a.name)
		}
		names[a.name] = true
		for _, k := range a.keys {
			if other, ok := keys[k]; ok {
				t.Errorf("%s runs both %s and %s", k, other, a.name)
			}
			keys[k] = a.name
		}
	}
	for _, name := range []string{"error_log", "favorites"} { // Named by keyOf and keyRuns calls
		if !names[name] {
			t.Errorf("no action is called %q", name)
		}
	}
}

func TestKeysReachTheirActions(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "a")
	for _, tt := range []struct {
		key  rune
		want modal
	}{
		{tea.KeyF3, previewModal{}},
		{tea.KeyF7, inputModal{}},
		{tea.KeyF8, confirmModal{}},
	} {
		m := cursorTo(t, newTestModel(t, dir, dir), "a")
		m = send(t, m, press(tt.key))
		if got, want := fmt.Sprintf("%T", top(m)), fmt.Sprintf("%T", tt.want); got != want {
			t.Errorf("F%d opened %s, want %s", tt.key-tea.KeyF1+1, got, want)
		}
	}

	// F5 copies a to the other pane, F6 moves b there.
	src, other := t.TempDir(), t.TempDir()
	writeFiles(t, src, "x", "a", "b")
	m := send(t, cursorTo(t, newTestModel(t, src, other), "a"), press(tea.KeyF5))
	m = send(t, cursorTo(t, m, "b"), press(tea.KeyF6))
	for _, name := range []string{"a", "b"} {
		if _, err := os.Stat(filepath.Join(other, name)); err != nil {
			t.Errorf("%s not in the other pane: %v", name, err)
		}
	}
	_, errA := os.Stat(filepath.Join(src, "a"))
	_, errB := os.Stat(filepath.Join(src, "b"))
	if errA != nil || !errors.Is(errB, os.ErrNotExist) {
		t.Errorf("after F5 on a and F6 on b, the source has a: %v, b: %v", errA, errB)
	}
	if m = send(t, m, press(tea.KeyTab)); !m.rightPane.active || m.leftPane.active {
		t.Error("Tab did not switch panes")
	}

	// F9 copies the path; its command is not run, so the test leaves the clipboard alone.
	if _, cmd := cursorTo(t, newTestModel(t, dir, dir), "a").Update(press(tea.KeyF9)); cmd == nil {
		t.Error("F9 did not copy the path")
	}
	for _, k := range []tea.KeyPressMsg{press(tea.KeyF10), {Code: 'c', Mod: tea.ModCtrl}} {
		if m := send(t, newTestModel(t, dir, dir), k); !m.quitting {
			t.Errorf("%s did not quit", k)
		}
	}
}

func TestDisabledActionsDoNothing(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "a")
	m := newTestModel(t, dir, dir) // The cursor is on ".."

	check := func(m model, keys ...tea.KeyPressMsg) {
		t.Helper()
		for _, k := range keys {
			next, cmd := m.Update(k)
			if next := next.(model); cmd != nil || top(next) != nil || next.toast != "" || len(next.errLog) != len(m.errLog) ||
				len(next.leftPane.selected) != 0 || next.leftPane.cursor != m.leftPane.cursor {
				t.Errorf("%s: command %v, %T opened, toast %q, selection %v, cursor %d",
					k, cmd != nil, top(next), next.toast, next.leftPane.selected, next.leftPane.cursor)
			}
		}
	}
	// Nothing to view, copy, move or delete on "..".
	check(m, alt('v'), alt('c'), alt('m'), alt('d'))
	// No directory to open in the other pane on a file.
	check(cursorTo(t, m, "a"), alt('o'))
	// Nothing at all in an empty listing.
	empty := send(t, m, m.leftPane.loadDirectoryCmd(&memSource{files: map[string]string{}}, "/", "")())
	if len(empty.leftPane.files) != 0 {
		t.Fatalf("listing of an empty source: %q", names(empty.leftPane.files))
	}
	check(empty, alt('v'), alt('c'), alt('m'), alt('d'), alt('p'), press(tea.KeyF9), press(tea.KeyInsert), alt('o'))
	// On ".." of another source there is nothing to act on, so nothing to refuse either.
	mem := &memSource{files: map[string]string{"/docs/a.txt": "x"}}
	check(send(t, m, m.leftPane.loadDirectoryCmd(mem, "/docs", "")()), alt('c'), alt('m'), alt('d'))
}

func TestTheKeyThatOpenedAModalClosesIt(t *testing.T) {
	m := newTestModel(t, t.TempDir(), t.TempDir())
	for _, k := range []tea.KeyPressMsg{alt('f'), alt('l')} {
		m = send(t, m, k)
		if top(m) == nil {
			t.Fatalf("%s opened nothing", k)
		}
		m = send(t, m, k)
		if top(m) != nil {
			t.Fatalf("%s left %T open", k, top(m))
		}
	}
}

func TestToastNamesTheLogKey(t *testing.T) {
	m := newTestModel(t, t.TempDir(), t.TempDir())
	m.reportError(errors.Join(os.ErrPermission, os.ErrNotExist))
	if !strings.Contains(m.toast, "(+1 more, alt+l for the log)") {
		t.Fatalf("toast %q does not point to the log", m.toast)
	}

	// Without a key for the log, the toast still says there is more.
	actions := defaultActions()
	for i := range actions {
		if actions[i].name == "error_log" {
			actions[i].keys = nil
		}
	}
	m.setActions(actions)
	m.reportError(errors.Join(os.ErrPermission, os.ErrNotExist))
	if !strings.HasSuffix(m.toast, "(+1 more in the log)") {
		t.Fatalf("toast %q with no key for the log", m.toast)
	}
}

func TestHintBarComesFromTheActions(t *testing.T) {
	m := newTestModel(t, t.TempDir(), t.TempDir())
	const want = "Alt q Quit v View c Copy m Move n MkDir d Delete p Copy Path f Favorites a Add Fav i Sync Panes o Open in Other l Log"
	if got := strings.Join(strings.Fields(ansi.Strip(m.hintsView())), " "); got != want {
		t.Errorf("hint bar:\n%s\nwant:\n%s", got, want)
	}

	m.setActions([]action{
		{name: "x", title: "Xylophone", keys: []string{"alt+x"}},
		{name: "y", title: "Yodel", keys: []string{"ctrl+y", "alt+y"}}, // Shown by its first key
		{name: "z", title: "Zither"},                                   // No key at all
	})
	if got := strings.Join(strings.Fields(ansi.Strip(m.hintsView())), " "); got != "Alt x Xylophone" {
		t.Errorf("hint bar for Xylophone on Alt+X, Yodel on Ctrl+Y and Zither on no key: %q", got)
	}
}
