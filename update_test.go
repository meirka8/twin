package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newTestModel builds a model with the left (active) pane on left and the right pane on
// right, both already loaded.
func newTestModel(t *testing.T, left, right string) model {
	t.Helper()
	km := DefaultKeyMap()
	m := model{
		leftPane:  pane{id: 0, path: left, active: true, selected: map[string]struct{}{}, height: 20, width: 40},
		rightPane: pane{id: 1, path: right, selected: map[string]struct{}{}, height: 20, width: 40},
		keyMap:    km,
		aliasMap:  km.GetAliasMap(),
	}
	return send(t, m, m.Init()())
}

// send feeds msg to Update and keeps running the returned commands, as the Bubble Tea
// runtime would, until nothing is left to do.
func send(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	queue := []tea.Msg{msg}
	for len(queue) > 0 {
		msg, queue = queue[0], queue[1:]
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil {
					queue = append(queue, c())
				}
			}
			continue
		}
		next, cmd := m.Update(msg)
		m = next.(model)
		if cmd != nil {
			queue = append(queue, cmd())
		}
	}
	return m
}

func key(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }
func alt(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}, Alt: true} }

// cursorTo puts the left pane's cursor on name.
func cursorTo(t *testing.T, m model, name string) model {
	t.Helper()
	for i, f := range m.leftPane.files {
		if f.Name == name {
			m.leftPane.cursor = i
			return m
		}
	}
	t.Fatalf("%s not in left pane", name)
	return m
}

func TestDeleteOnParentEntryDoesNothing(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, child, child) // cursor starts on ".."

	m = send(t, m, alt('d'))
	if m.isDeleting {
		t.Fatal("delete prompt opened for '..'")
	}
	m = send(t, m, key('y'))
	if _, err := os.Stat(parent); err != nil {
		t.Fatalf("parent directory gone: %v", err)
	}
}

func TestInsertDoesNotSelectParent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir, dir)

	m = send(t, m, tea.KeyMsg{Type: tea.KeyInsert})
	if len(m.leftPane.selected) != 0 {
		t.Fatalf("'..' got selected: %v", m.leftPane.selected)
	}
}

func TestCopyWithinSameDirectoryKeepsContent(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "d")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, "keep", "a")
	writeFiles(t, parent, "keep", "p")
	m := newTestModel(t, dir, dir)

	for _, name := range []string{"a", ".."} {
		m = cursorTo(t, m, name)
		m = send(t, m, alt('c'))
		if m.isConfirmingOverwrite {
			t.Fatalf("%s: offered to overwrite a file with itself", name)
		}
		m = send(t, m, key('y'))
	}
	if got := readFile(t, filepath.Join(dir, "a")); got != "keep" {
		t.Fatalf("a truncated to %q", got)
	}
	if got := readFile(t, filepath.Join(parent, "p")); got != "keep" {
		t.Fatalf("p truncated to %q", got)
	}
}

func TestConflictsAreDecidedPerFile(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, "new", "a", "b", "c")
	writeFiles(t, dst, "old", "b", "c")
	m := newTestModel(t, src, dst)
	for _, f := range m.leftPane.files[1:] {
		m.leftPane.selected[f.Path] = struct{}{}
	}

	m = send(t, m, alt('c'))
	if !m.isConfirmingOverwrite || len(m.overwriteConflicts) != 2 {
		t.Fatalf("expected two conflicts, got %+v", m.overwriteConflicts)
	}
	if _, err := os.Stat(filepath.Join(dst, "a")); err == nil {
		t.Fatal("batch ran before conflicts were resolved")
	}

	m = send(t, m, key('n')) // keep b
	if !m.isConfirmingOverwrite {
		t.Fatal("prompt closed after the first answer")
	}
	m = send(t, m, key('y')) // overwrite c
	if m.isConfirmingOverwrite {
		t.Fatal("prompt still open after the last answer")
	}

	want := map[string]string{"a": "new", "b": "old", "c": "new"}
	for name, content := range want {
		if got := readFile(t, filepath.Join(dst, name)); got != content {
			t.Errorf("%s: got %q, want %q", name, got, content)
		}
	}
}

func TestMoveSkipAllStillMovesCleanFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, "new", "a", "b", "c")
	writeFiles(t, dst, "old", "b", "c")
	m := newTestModel(t, src, dst)
	for _, f := range m.leftPane.files[1:] {
		m.leftPane.selected[f.Path] = struct{}{}
	}

	m = send(t, m, alt('m'))
	m = send(t, m, key('s'))

	if got := readFile(t, filepath.Join(dst, "a")); got != "new" {
		t.Fatalf("a not moved: %q", got)
	}
	for _, name := range []string{"b", "c"} {
		if got := readFile(t, filepath.Join(src, name)); got != "new" {
			t.Errorf("%s should stay in source: %q", name, got)
		}
		if got := readFile(t, filepath.Join(dst, name)); got != "old" {
			t.Errorf("%s overwritten in destination: %q", name, got)
		}
	}
}

func TestEscCancelsWholeBatch(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, "new", "a", "b")
	writeFiles(t, dst, "old", "b")
	m := newTestModel(t, src, dst)
	for _, f := range m.leftPane.files[1:] {
		m.leftPane.selected[f.Path] = struct{}{}
	}

	m = send(t, m, alt('c'))
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	if m.isConfirmingOverwrite || m.pendingClean != nil {
		t.Fatal("pending operation not cleared")
	}
	if _, err := os.Stat(filepath.Join(dst, "a")); err == nil {
		t.Fatal("a copied despite cancel")
	}
}
