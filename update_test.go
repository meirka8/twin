package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func init() {
	toastDuration = 0 // send runs every command synchronously, including the toast timer
	launch = func(path string) error { return fmt.Errorf("tests never start programs: %s", path) }
}

// newTestModel builds a model with the left (active) pane on left and the right pane on
// right, both already loaded.
func newTestModel(t *testing.T, left, right string) model {
	t.Helper()
	km := DefaultKeyMap()
	m := model{
		leftPane:  newPane(0, localFS{}, left),
		rightPane: newPane(1, localFS{}, right),
		keyMap:    km,
		aliasMap:  km.GetAliasMap(),
	}
	m.leftPane.active = true
	m = send(t, m, tea.WindowSizeMsg{Width: 84, Height: 28}) // Panes of 40×22
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

// key types r; alt presses Alt+r; press presses a key that types nothing, such as
// tea.KeyEnter or tea.KeyDown.
func key(r rune) tea.KeyPressMsg      { return tea.KeyPressMsg{Code: r, Text: string(r)} }
func alt(r rune) tea.KeyPressMsg      { return tea.KeyPressMsg{Code: r, Mod: tea.ModAlt} }
func press(code rune) tea.KeyPressMsg { return tea.KeyPressMsg{Code: code} }

// typeText types s one character at a time.
func typeText(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		m = send(t, m, key(r))
	}
	return m
}

// top is the modal that owns the keyboard, nil when the panes do.
func top(m model) modal {
	if len(m.modals) == 0 {
		return nil
	}
	return m.modals[len(m.modals)-1]
}

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
	if top(m) != nil {
		t.Fatalf("%T opened for '..'", top(m))
	}
	send(t, m, key('y')) // Would answer a prompt, had one opened
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

	m = send(t, m, press(tea.KeyInsert))
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
		if _, ok := top(m).(conflictModal); ok {
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
	if c, ok := top(m).(conflictModal); !ok || len(c.conflicts) != 2 {
		t.Fatalf("expected two conflicts, got %+v", top(m))
	}
	if _, err := os.Stat(filepath.Join(dst, "a")); err == nil {
		t.Fatal("batch ran before conflicts were resolved")
	}

	m = send(t, m, key('n')) // keep b
	if _, ok := top(m).(conflictModal); !ok {
		t.Fatal("prompt closed after the first answer")
	}
	m = send(t, m, key('y')) // overwrite c
	if top(m) != nil {
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
	m = send(t, m, press(tea.KeyEscape))

	if top(m) != nil {
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
	t.Cleanup(func() { _ = os.Chmod(locked, 0755) })
	m := newTestModel(t, dir, dir)
	before := len(m.leftPane.files)

	m = cursorTo(t, m, "locked")
	m = send(t, m, press(tea.KeyEnter))

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

	m = send(t, m, m.leftPane.reloadCmd("")())
	if len(m.leftPane.files) != 1 || !m.leftPane.files[0].IsParent || m.leftPane.cursor != 0 {
		t.Fatalf("expected only '..' under the cursor, got %+v at %d", m.leftPane.files, m.leftPane.cursor)
	}
	m = send(t, m, press(tea.KeyEnter))
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
	m = send(t, m, press(tea.KeyEnter))
	if m.leftPane.path != filepath.Join(dir, "link") {
		t.Fatalf("Enter did not enter the link: pane at %s", m.leftPane.path)
	}
	m = send(t, m, press(tea.KeyEnter)) // ".." back out, focused on the link
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

func TestModalsOwnTheKeyboard(t *testing.T) {
	// Each of these would act on the panes: move the cursor, delete, quick search, switch
	// panes, add a favorite.
	leaks := []tea.Msg{press(tea.KeyDown), alt('d'), key('b'), press(tea.KeyTab), alt('a'), tea.PasteMsg{Content: "b"}}

	tests := []struct {
		name string
		keys []tea.Msg // Open the modal, with the cursor on "a"
	}{
		{"new folder", []tea.Msg{alt('n')}},
		{"delete", []tea.Msg{alt('d')}},
		{"overwrite", []tea.Msg{alt('c')}},
		{"preview", []tea.Msg{alt('v')}},
		{"favorites", []tea.Msg{alt('f')}},
		{"remove favorite", []tea.Msg{alt('f'), key('d')}},
		{"error log", []tea.Msg{alt('l')}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, dst := t.TempDir(), t.TempDir()
			writeFiles(t, src, "x", "a", "b")
			writeFiles(t, dst, "x", "a")
			m := cursorTo(t, newTestModel(t, src, dst), "a")
			m.favorites = []string{src, dst}
			for _, msg := range tt.keys {
				m = send(t, m, msg)
			}
			opened, depth, cursor := top(m), len(m.modals), m.leftPane.cursor
			if opened == nil {
				t.Fatal("no modal opened")
			}

			for _, msg := range leaks {
				m = send(t, m, msg)
			}
			if got := top(m); len(m.modals) != depth || fmt.Sprintf("%T", got) != fmt.Sprintf("%T", opened) {
				t.Fatalf("stack changed: %d modals, %T on top", len(m.modals), got)
			}
			if m.leftPane.cursor != cursor || m.leftPane.search.Value() != "" || !m.leftPane.active || len(m.favorites) != 2 {
				t.Fatalf("keys leaked to the panes: cursor %d, search %q, favorites %v", m.leftPane.cursor, m.leftPane.search.Value(), m.favorites)
			}
			if _, err := os.Stat(filepath.Join(src, "a")); err != nil {
				t.Fatalf("a gone: %v", err)
			}
		})
	}
}

func TestShortcutsWorkInAnyLayout(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, dir, dir)
	if !m.View().KeyboardEnhancements.ReportAlternateKeys {
		t.Fatal("the view does not ask the terminal for alternate keys")
	}

	// With the Kitty keyboard protocol and alternate keys, a key reports what it types and
	// the key at the same place on a US layout. Alt+A on AZERTY sits where US has Q.
	m = send(t, m, tea.KeyPressMsg{Code: 'a', BaseCode: 'q', Mod: tea.ModAlt})
	if m.quitting || len(m.favorites) != 1 {
		t.Fatalf("AZERTY Alt+A: quitting %v, favorites %v; want Add Fav", m.quitting, m.favorites)
	}
	// Alt+F on a Hebrew layout types כ.
	m = send(t, m, tea.KeyPressMsg{Code: 'כ', BaseCode: 'f', Mod: tea.ModAlt})
	if _, ok := top(m).(favoritesModal); !ok {
		t.Fatalf("Hebrew Alt+F opened %T, want the favorites", top(m))
	}
	m = send(t, m, press(tea.KeyEscape))
	m = send(t, m, tea.KeyPressMsg{Code: 'ב', BaseCode: 'c', Mod: tea.ModCtrl})
	if !m.quitting {
		t.Fatal("Hebrew Ctrl+C did not quit")
	}
}

func TestDialogOverFavoritesReturnsToTheList(t *testing.T) {
	dir := t.TempDir()
	other := filepath.Join(dir, "other")
	m := newTestModel(t, dir, dir)
	m.favorites = []string{dir, other}

	m = send(t, m, alt('f'))
	m = send(t, m, press(tea.KeyDown))
	m = send(t, m, key('d'))
	if _, ok := top(m).(confirmModal); !ok || len(m.modals) != 2 {
		t.Fatalf("expected a confirm over the favorites, got %d modals, %T on top", len(m.modals), top(m))
	}
	if screen := m.render(); !strings.Contains(screen, "Favorites") || !strings.Contains(screen, "Remove '"+other) {
		t.Fatalf("the list and the question should both show:\n%s", screen)
	}

	m = send(t, m, press(tea.KeyEscape)) // No: back to the list, where it was
	if f, ok := top(m).(favoritesModal); !ok || f.cursor != 1 || len(m.favorites) != 2 {
		t.Fatalf("expected the list on %s, got %+v with %v", other, top(m), m.favorites)
	}

	m = send(t, m, key('d'))
	m = send(t, m, key('y'))
	if _, ok := top(m).(favoritesModal); !ok || len(m.favorites) != 1 || m.favorites[0] != dir {
		t.Fatalf("expected the list without %s, got %T with %v", other, top(m), m.favorites)
	}
	m = send(t, m, press(tea.KeyEnter)) // The cursor followed the shorter list
	if top(m) != nil || m.leftPane.path != dir {
		t.Fatalf("Enter on %s: %T open, pane at %s", dir, top(m), m.leftPane.path)
	}
}

func TestNewFolderTakesAnyScriptAndPaste(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, dir, dir)

	m = send(t, m, alt('n'))
	m = typeText(t, m, "תיקיה")
	m = send(t, m, press(tea.KeyBackspace))           // One letter, not one byte
	m = send(t, m, tea.PasteMsg{Content: " новая\n"}) // Copied with its line break
	m = send(t, m, press(tea.KeyEnter))

	const want = "תיקי новая"
	if info, err := os.Stat(filepath.Join(dir, want)); err != nil || !info.IsDir() {
		t.Fatalf("%q not created: %v", want, err)
	}
	if f := m.leftPane.files[m.leftPane.cursor]; f.Name != want {
		t.Fatalf("cursor on %q", f.Name)
	}
}

func TestNewFolderWithoutNameDoesNothing(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, dir, dir)

	m = send(t, m, alt('n'))
	m = send(t, m, press(tea.KeyEnter))
	if top(m) != nil || len(m.errLog) != 0 {
		t.Fatalf("empty name: %T open, log %+v", top(m), m.errLog)
	}
}

func TestQuickSearchTakesAnyScript(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "abc", "шум", "שלום")
	m := newTestModel(t, dir, dir)
	cursorName := func() string { return m.leftPane.files[m.leftPane.cursor].Name }

	m = typeText(t, m, "של")
	if cursorName() != "שלום" {
		t.Fatalf("של: cursor on %q", cursorName())
	}
	m = send(t, m, press(tea.KeyBackspace))
	m = send(t, m, press(tea.KeyBackspace))
	if q := m.leftPane.search.Value(); q != "" {
		t.Fatalf("query %q after deleting both letters", q)
	}
	m = send(t, m, tea.PasteMsg{Content: "шу\n"})
	if q := m.leftPane.search.Value(); q != "шу" || cursorName() != "шум" {
		t.Fatalf("pasted шу: query %q, cursor on %q", q, cursorName())
	}
	m = send(t, m, press(tea.KeyEscape))
	if q := m.leftPane.search.Value(); q != "" {
		t.Fatalf("Esc left the query %q", q)
	}
}

func TestStalePreviewIsIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "first", "a")
	writeFiles(t, dir, "second", "b")
	m := newTestModel(t, dir, dir)

	m = send(t, m, alt('v')) // On "..": nothing to show
	if top(m) != nil {
		t.Fatalf("%T opened on '..'", top(m))
	}
	m = send(t, cursorTo(t, m, "b"), alt('v'))
	// The preview of a, closed before its file was read, reports back late.
	m = send(t, m, previewReadyMsg{source: localFS{}, path: filepath.Join(dir, "a"), content: "first"})
	if p, ok := top(m).(previewModal); !ok || p.content != "second" {
		t.Fatalf("expected the preview of b, got %+v", top(m))
	}
	m = send(t, m, key('q'))
	if top(m) != nil {
		t.Fatal("preview did not close")
	}
}

// The screen must fill the terminal exactly: Lip Gloss v2 counts borders in a style's width
// and height, v1 did not. (The hint bar may be wider; the terminal clips it.)
func TestLayoutFillsTheTerminal(t *testing.T) {
	const width, height = 84, 28
	dir := t.TempDir()
	writeFiles(t, dir, "line\n", "a")
	for _, keys := range [][]tea.Msg{nil, {alt('f')}, {alt('l')}, {alt('v')}, {alt('n')}, {alt('d')}} {
		m := cursorTo(t, newTestModel(t, dir, dir), "a")
		m.favorites = []string{dir}
		for _, msg := range keys {
			m = send(t, m, msg)
		}
		for _, active := range []bool{true, false} {
			m.leftPane.active, m.rightPane.active = active, !active
			if h := lipgloss.Height(m.render()); h != height {
				t.Errorf("%T: %d lines on a %d-line terminal", top(m), h, height)
			}
			panes := m.panesView()
			if w, h := lipgloss.Width(panes), lipgloss.Height(panes); w != width || h != m.leftPane.height+2 {
				t.Errorf("%T: panes are %d×%d, want %d×%d", top(m), w, h, width, m.leftPane.height+2)
			}
		}
	}
}

func TestConflictsWaitForTheDialogOpenWhilePlanning(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, "new", "b")
	writeFiles(t, dst, "old", "b")
	m := cursorTo(t, newTestModel(t, src, dst), "b")

	next, plan := m.Update(alt('c')) // The batch is still being checked...
	m = send(t, next.(model), alt('n'))
	m = send(t, m, plan()) // ...and the plan comes back while a folder name is typed
	m = typeText(t, m, "archive")

	if got := readFile(t, filepath.Join(dst, "b")); got != "old" {
		t.Fatalf("the folder name answered the overwrite question: b is %q", got)
	}
	if in, ok := top(m).(inputModal); !ok || in.input.Value() != "archive" {
		t.Fatalf("expected the folder prompt with archive, got %+v", top(m))
	}
	if strings.Contains(m.render(), "Overwrite") {
		t.Fatal("the overwrite question shows while its keys go to the folder prompt")
	}

	m = send(t, m, press(tea.KeyEnter))
	if _, err := os.Stat(filepath.Join(src, "archive")); err != nil {
		t.Fatalf("folder not created: %v", err)
	}
	if _, ok := top(m).(conflictModal); !ok || !strings.Contains(m.render(), "Overwrite b?") {
		t.Fatalf("expected the overwrite question once the prompt closed, got %T", top(m))
	}
	m = send(t, m, key('n'))
	if top(m) != nil || readFile(t, filepath.Join(dst, "b")) != "old" {
		t.Fatalf("after n: %T open, b is %q", top(m), readFile(t, filepath.Join(dst, "b")))
	}
}

func TestOverwriteAll(t *testing.T) {
	for _, move := range []bool{false, true} {
		src, dst := t.TempDir(), t.TempDir()
		writeFiles(t, src, "new", "a", "b", "c")
		writeFiles(t, dst, "old", "b", "c")
		m := newTestModel(t, src, dst)
		for _, f := range m.leftPane.files[1:] {
			m.leftPane.selected[f.Path] = struct{}{}
		}

		op := alt('c')
		if move {
			op = alt('m')
		}
		m = send(t, m, op)
		m = send(t, m, key('n')) // Keep b, then overwrite c and anything after it
		m = send(t, m, key('A'))

		if top(m) != nil {
			t.Fatalf("move %v: %T still open", move, top(m))
		}
		for name, want := range map[string]string{"a": "new", "b": "old", "c": "new"} {
			if got := readFile(t, filepath.Join(dst, name)); got != want {
				t.Errorf("move %v: %s is %q, want %q", move, name, got, want)
			}
		}
		for name, gone := range map[string]bool{"a": move, "b": false, "c": move} {
			if _, err := os.Stat(filepath.Join(src, name)); os.IsNotExist(err) != gone {
				t.Errorf("move %v: %s left in the source: %v", move, name, !gone)
			}
		}
	}
}

func TestFavoritesDrives(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel(t, dir, dir)
	m.favorites = []string{dir}
	for i := range 20 {
		m.drives = append(m.drives, fmt.Sprintf("/twin-test/drive%d", i))
	}

	m = send(t, m, alt('f'))
	m = send(t, m, press(tea.KeyEnd))
	m = send(t, m, key('d'))
	if !strings.Contains(m.render(), "Unmount '/twin-test/drive19'? (y/n)") {
		t.Fatalf("no unmount question:\n%s", m.render())
	}
	m = send(t, m, key('n'))
	if f, ok := top(m).(favoritesModal); !ok || f.cursor != 20 {
		t.Fatalf("n did not return to the list on the drive: %+v", top(m))
	}

	// An unmount rereads the drives, here the real ones: fewer than 20.
	m = send(t, m, driveUnmountedMsg{drivePath: "/twin-test/drive19"})
	if n := strings.Count(m.render(), "> "); n != 1 {
		t.Fatalf("%d entries under the cursor after the list shrank", n)
	}
}

func TestPreviewOfUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any file")
	}
	dir := t.TempDir()
	writeFiles(t, dir, "secret", "locked")
	if err := os.Chmod(filepath.Join(dir, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	m := cursorTo(t, newTestModel(t, dir, dir), "locked")

	m = send(t, m, alt('v'))
	if top(m) != nil || len(m.errLog) != 1 {
		t.Fatalf("expected the preview closed and the error logged: %T open, log %+v", top(m), m.errLog)
	}
}
