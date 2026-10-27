package main

import (
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/unix"
)

// memSource is a read-only Source kept in memory, standing in for the archives and remote
// filesystems to come: a pane on it must work without touching the disk.
type memSource struct {
	files map[string]string // Path → content; directories are implied by the paths
}

func (s *memSource) List(dir string) ([]file, error) {
	var files []file
	seen := map[string]bool{}
	for p := range s.files {
		rel, ok := strings.CutPrefix(p, strings.TrimSuffix(dir, "/")+"/")
		if !ok {
			continue
		}
		name, _, isDir := strings.Cut(rel, "/")
		if !seen[name] {
			seen[name] = true
			files = append(files, file{Name: name, Path: path.Join(dir, name), IsDir: isDir})
		}
	}
	if len(files) == 0 && dir != "/" {
		return nil, fs.ErrNotExist
	}
	return files, nil
}

func (s *memSource) Parent(dir string) (string, bool) { return path.Dir(dir), dir != "/" }

func (s *memSource) Open(p string) (io.ReadCloser, error) {
	content, ok := s.files[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func names(files []file) string {
	var s []string
	for _, f := range files {
		s = append(s, f.Name)
	}
	return strings.Join(s, " ")
}

func TestListDirPutsParentAndDirectoriesFirst(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "b", "A", "a")
	if err := os.Mkdir(filepath.Join(dir, "z"), 0755); err != nil {
		t.Fatal(err)
	}

	files, err := listDir(localFS{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(files); got != ".. z A a b" {
		t.Fatalf("listing %q, want \".. z A a b\"", got)
	}
	if !files[0].IsParent || files[0].Path != filepath.Dir(dir) || files[1].IsParent {
		t.Fatalf("only '..' must be the parent: %+v", files[:2])
	}
	if root, _ := listDir(localFS{}, "/"); len(root) > 0 && root[0].IsParent {
		t.Fatal("'/' has no parent")
	}
}

func TestListDirErrorKeepsParent(t *testing.T) {
	files, err := listDir(localFS{}, filepath.Join(t.TempDir(), "missing"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(files) != 1 || !files[0].IsParent {
		t.Fatalf("expected only '..', got %+v", files)
	}
}

func TestTargetsNeverIncludeParent(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "a")
	files, _ := listDir(localFS{}, dir)

	p := pane{source: localFS{}, path: dir, files: files, selected: map[string]struct{}{}}
	if got := p.targets(); len(got) != 0 {
		t.Fatalf("cursor on '..' must give no targets, got %+v", got)
	}

	p.selected[files[0].Path] = struct{}{} // ".." smuggled into the selection
	p.cursor = 1
	if got := p.targets(); len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("expected only 'a', got %+v", got)
	}
}

func TestPaneOnAnotherSource(t *testing.T) {
	local := t.TempDir()
	mem := &memSource{files: map[string]string{
		"/docs/a.txt":     "from memory",
		"/docs/sub/b.txt": "deeper",
	}}
	m := newTestModel(t, local, local)
	m.favorites = []string{local}
	m = send(t, m, m.leftPane.loadDirectoryCmd(mem, "/docs", "")())
	if m.leftPane.source != Source(mem) || names(m.leftPane.files) != ".. sub a.txt" {
		t.Fatalf("pane on %v %s: %q", m.leftPane.source, m.leftPane.path, names(m.leftPane.files))
	}

	m = send(t, cursorTo(t, m, "sub"), press(tea.KeyEnter))
	if m.leftPane.path != "/docs/sub" || names(m.leftPane.files) != ".. b.txt" {
		t.Fatalf("Enter on sub: %s %q", m.leftPane.path, names(m.leftPane.files))
	}
	m = send(t, m, press(tea.KeyEnter)) // ".." back up, focused on sub
	if f, _ := m.leftPane.cursorFile(); m.leftPane.path != "/docs" || f.Name != "sub" {
		t.Fatalf("'..': pane at %s, cursor on %s", m.leftPane.path, f.Name)
	}

	m = send(t, cursorTo(t, m, "a.txt"), alt('v'))
	if p, ok := top(m).(previewModal); !ok || p.content != "from memory" {
		t.Fatalf("preview through the source: %+v", top(m))
	}
	m = send(t, m, key('q'))

	// Nothing that changes files or hands them to another program may run outside the local
	// filesystem: the paths are the source's, not the disk's.
	for _, k := range []tea.KeyPressMsg{alt('d'), alt('n'), alt('c'), alt('m'), alt('a'), press(tea.KeyEnter)} {
		logged := len(m.errLog)
		m = send(t, m, k)
		if top(m) != nil || len(m.errLog) != logged+1 || !strings.Contains(m.errLog[logged].text, "local filesystem only") {
			t.Fatalf("%s on a memory source: %T open, log %+v", k, top(m), m.errLog[logged:])
		}
	}
	if entries, _ := os.ReadDir(local); len(entries) != 0 || len(m.favorites) != 1 {
		t.Fatalf("the local side changed: %v, favorites %v", entries, m.favorites)
	}

	m = send(t, cursorTo(t, m, "sub"), alt('o')) // Open in other: the other pane follows the source
	if m.rightPane.source != Source(mem) || m.rightPane.path != "/docs/sub" {
		t.Fatalf("opened in the other pane on %v %s", m.rightPane.source, m.rightPane.path)
	}
	m = send(t, m, alt('i')) // Sync panes: likewise
	if m.rightPane.source != Source(mem) || m.rightPane.path != "/docs" {
		t.Fatalf("synced pane on %v %s", m.rightPane.source, m.rightPane.path)
	}
	m = send(t, m, alt('f'))
	m = send(t, m, press(tea.KeyEnter)) // A favorite is a local directory
	if m.leftPane.source != Source(localFS{}) || m.leftPane.path != local {
		t.Fatalf("favorite: pane on %v %s", m.leftPane.source, m.leftPane.path)
	}
}

// endlessSource has one file, /endless, that never ends: "€€€…", three bytes each.
type endlessSource struct{}

func (endlessSource) List(string) ([]file, error) {
	return []file{{Name: "endless", Path: "/endless"}}, nil
}
func (endlessSource) Parent(string) (string, bool) { return "", false }
func (endlessSource) Open(string) (io.ReadCloser, error) {
	return io.NopCloser(&endlessReader{}), nil
}

type endlessReader struct{ n int }

func (r *endlessReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = "€"[(r.n+i)%3]
	}
	r.n += len(p)
	return len(p), nil
}

func TestPreviewReadsOnlyTheStart(t *testing.T) {
	msg := previewFileCmd(endlessSource{}, "/endless")().(previewReadyMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	header, text, _ := strings.Cut(msg.content, "\n")
	if !strings.Contains(header, "too large") {
		t.Fatalf("no notice that the file goes on: %q", header)
	}
	// 100 KB is not a whole number of €: the character cut in half is dropped.
	if !utf8.ValidString(text) || len(text) != previewLimit/3*3 {
		t.Fatalf("got %d bytes, valid UTF-8 %v; want %d", len(text), utf8.ValidString(text), previewLimit/3*3)
	}
}

func TestCopyIntoAnotherSourceIsRefused(t *testing.T) {
	local := t.TempDir()
	writeFiles(t, local, "keep", "a")
	mem := &memSource{files: map[string]string{"/docs/b.txt": "b"}}
	m := cursorTo(t, newTestModel(t, local, local), "a")
	m = send(t, m, m.rightPane.loadDirectoryCmd(mem, "/docs", "")())

	for _, k := range []tea.KeyPressMsg{alt('c'), alt('m')} {
		logged := len(m.errLog)
		m = send(t, m, k)
		if len(m.errLog) != logged+1 || !strings.Contains(m.errLog[logged].text, "local filesystem only") {
			t.Fatalf("%s into a memory source: log %+v", k, m.errLog[logged:])
		}
	}
	if got := readFile(t, filepath.Join(local, "a")); got != "keep" {
		t.Fatalf("a changed: %q", got)
	}
	// The commands refuse too, should a caller ever skip the check.
	a := m.leftPane.files[m.leftPane.cursor]
	for _, move := range []bool{false, true} {
		if msg := planTransferCmd(localFS{}, []file{a}, mem, "/docs", move)().(transferPlannedMsg); len(msg.plan.rejected) != 1 || len(msg.plan.clean) != 0 {
			t.Fatalf("planned into a memory source: %+v", msg.plan)
		}
		if msg := transferFilesCmd(localFS{}, []file{a}, mem, "/docs", move)().(fileOperationMsg); msg.err == nil {
			t.Fatal("transferred into a memory source")
		}
	}
	if msg := deleteFilesCmd(mem, []file{a})().(fileDeletedMsg); msg.err == nil || readFile(t, a.Path) != "keep" {
		t.Fatalf("deleted through a memory source: %v", msg.err)
	}
	if msg := createFolderCmd(mem, filepath.Join(local, "new"))().(folderCreatedMsg); msg.err == nil {
		t.Fatal("created a folder through a memory source")
	}
}

func TestSelectionDoesNotCrossSources(t *testing.T) {
	local := t.TempDir()
	writeFiles(t, local, "keep", "a.txt")
	// A source whose paths look like the local ones, as a remote home directory would.
	mem := &memSource{files: map[string]string{filepath.Join(local, "a.txt"): "remote"}}
	m := newTestModel(t, local, local)
	m.favorites = []string{local}
	m = send(t, m, m.leftPane.loadDirectoryCmd(mem, local, "")())

	m = send(t, cursorTo(t, m, "a.txt"), press(tea.KeyInsert)) // Selected on the memory source
	m = send(t, m, alt('f'))
	m = send(t, m, press(tea.KeyEnter)) // Back to the local directory, cursor on ".."
	if len(m.leftPane.selected) != 0 {
		t.Fatalf("selection kept across sources: %v", m.leftPane.selected)
	}
	m = send(t, m, alt('d'))
	send(t, m, key('y')) // Would confirm a delete, had one been asked
	if got := readFile(t, filepath.Join(local, "a.txt")); got != "keep" {
		t.Fatalf("local a.txt changed: %q", got)
	}
}

// errReader fails every read.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type errSource struct{ memSource }

func (*errSource) Open(string) (io.ReadCloser, error) { return io.NopCloser(errReader{}), nil }

func TestReadHead(t *testing.T) {
	const limit = 8
	tests := []struct {
		content, want string
		more          bool
	}{
		{"short", "short", false},
		{"12345678", "12345678", false}, // Exactly the limit: nothing more
		{"123456789", "12345678", true},
		{"123456é", "123456é", false},  // A two-byte character ends at the limit
		{"1234567é9", "1234567", true}, // ...and cut in half
		{"1234😀x", "1234😀", true},      // A four-byte character ends at the limit
		{"12345😀", "12345", true},      // ...and cut short
		{"€€€", "€€", true},            // Three-byte characters, one cut after a byte
	}
	for _, tt := range tests {
		src := &memSource{files: map[string]string{"/f": tt.content}}
		got, more, err := readHead(src, "/f", limit)
		if err != nil || string(got) != tt.want || more != tt.more {
			t.Errorf("%q: got %q, more %v, %v; want %q, more %v", tt.content, got, more, err, tt.want, tt.more)
		}
	}
	if _, _, err := readHead(&errSource{}, "/f", limit); err == nil {
		t.Error("a read error was dropped")
	}
	if _, _, err := readHead(&memSource{}, "/missing", limit); err == nil {
		t.Error("opening a missing file did not fail")
	}
}

func TestPreviewRefusesSpecialFiles(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "fifo")
	if err := unix.Mkfifo(fifo, 0644); err != nil {
		t.Skip("cannot create a FIFO:", err)
	}
	// Reading a FIFO would wait for a writer forever.
	done := make(chan previewReadyMsg, 1)
	go func() { done <- previewFileCmd(localFS{}, fifo)().(previewReadyMsg) }()
	select {
	case msg := <-done:
		if msg.err == nil {
			t.Fatalf("previewed a FIFO: %q", msg.content)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("previewing a FIFO blocked")
	}
}
