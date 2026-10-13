package main

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func init() {
	toastDuration = 0 // send runs every command synchronously, including the toast timer
}

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

func TestEnteringUnreadableDirectoryKeepsPaneWhereItWas(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	dir := t.TempDir()
	locked := filepath.Join(dir, "locked")
	if err := os.Mkdir(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0755) })
	m := newTestModel(t, dir, dir)
	before := len(m.leftPane.files)

	m = cursorTo(t, m, "locked")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if m.leftPane.path != dir || len(m.leftPane.files) != before {
		t.Fatalf("pane moved to %s with %d entries", m.leftPane.path, len(m.leftPane.files))
	}
	if m.leftPane.files[m.leftPane.cursor].Name != "locked" {
		t.Fatal("cursor lost its place")
	}
	if len(m.errLog) != 1 {
		t.Fatalf("expected the failure in the log, got %+v", m.errLog)
	}
}

func TestFailedReloadStillOffersParent(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "gone")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, "x", "a", "b")
	m := newTestModel(t, dir, dir)
	m.leftPane.cursor = 2
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}

	m = send(t, m, m.leftPane.loadDirectoryCmd(dir, "")())
	if len(m.leftPane.files) != 1 || !m.leftPane.files[0].IsParent || m.leftPane.cursor != 0 {
		t.Fatalf("expected only '..' under the cursor, got %+v at %d", m.leftPane.files, m.leftPane.cursor)
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.leftPane.path != parent {
		t.Fatalf("could not go up: pane at %s", m.leftPane.path)
	}
}

func TestErrorsAreLoggedAndShownUntilExpired(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "a")
	m := newTestModel(t, dir, dir)

	m = cursorTo(t, m, "a")
	m = send(t, m, alt('c')) // Same directory: rejected
	if len(m.errLog) != 1 {
		t.Fatalf("rejected copy not logged: %+v", m.errLog)
	}
	if m.toast != "" {
		t.Fatalf("toast outlived its timer: %q", m.toast)
	}

	// An older timer must not clear a newer toast.
	m.reportError(os.ErrPermission)
	stale := toastExpiredMsg{id: m.toastID}
	m.reportError(os.ErrNotExist)
	next, _ := m.Update(stale)
	if m = next.(model); m.toast == "" {
		t.Fatal("newer toast cleared by an older timer")
	}
}

func TestErrorLogOwnsKeysWhileOpen(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "a", "b")
	m := newTestModel(t, dir, dir)

	m = send(t, m, alt('l'))
	if !m.isLogOpen {
		t.Fatal("log did not open")
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = send(t, m, alt('d'))
	m = send(t, m, key('b'))
	if m.leftPane.cursor != 0 || m.isDeleting || m.leftPane.searchQuery != "" {
		t.Fatal("keys leaked to the pane while the log was open")
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.isLogOpen {
		t.Fatal("log did not close")
	}
}

func TestSymlinkedDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, target, "keep", "inside")
	if err := os.Symlink(target, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	m := newTestModel(t, dir, dir)

	m = cursorTo(t, m, "link")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.leftPane.path != filepath.Join(dir, "link") {
		t.Fatalf("Enter did not enter the link: pane at %s", m.leftPane.path)
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // ".." back out, focused on the link
	if f := m.leftPane.files[m.leftPane.cursor]; f.Name != "link" {
		t.Fatalf("cursor on %s after going up", f.Name)
	}

	m = send(t, m, alt('d'))
	m = send(t, m, key('y'))
	if _, err := os.Lstat(filepath.Join(dir, "link")); !os.IsNotExist(err) {
		t.Fatal("link not deleted")
	}
	if got := readFile(t, filepath.Join(target, "inside")); got != "keep" {
		t.Fatalf("deleting the link touched its target: %q", got)
	}
}
