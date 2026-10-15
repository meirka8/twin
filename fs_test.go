package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// writeFiles creates each name in dir with the given content.
func writeFiles(t *testing.T, dir string, content string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// readFile returns the content of path, failing the test if it cannot be read.
func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// fileNamed returns the entry of dir called name.
func fileNamed(t *testing.T, dir, name string) file {
	t.Helper()
	files, err := readDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("%s not found in %s", name, dir)
	return file{}
}

func TestReadDirectoryMarksParent(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "a")

	files, err := readDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || !files[0].IsParent || files[0].Name != ".." {
		t.Fatalf("expected '..' marked as parent first, got %+v", files)
	}
	if files[1].IsParent {
		t.Fatalf("regular entry marked as parent: %+v", files[1])
	}
}

func TestOperationTargetsNeverIncludeParent(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "x", "a")
	files, _ := readDirectory(dir)

	p := pane{path: dir, files: files, selected: map[string]struct{}{}}
	if got := operationTargets(p); len(got) != 0 {
		t.Fatalf("cursor on '..' must give no targets, got %+v", got)
	}

	p.selected[files[0].Path] = struct{}{} // ".." smuggled into the selection
	p.cursor = 1
	if got := operationTargets(p); len(got) != 1 || got[0].Name != "a" {
		t.Fatalf("expected only 'a', got %+v", got)
	}
}

func TestPlanBatch(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, "new", "clean", "conflict")
	writeFiles(t, dst, "old", "conflict")
	if err := os.Mkdir(filepath.Join(src, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	files, _ := readDirectory(src) // .., dir, clean, conflict

	plan := planBatch(files, dst)
	if len(plan.clean) != 2 || len(plan.conflicts) != 1 || len(plan.rejected) != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.conflicts[0].Source.Name != "conflict" {
		t.Fatalf("wrong conflict: %+v", plan.conflicts)
	}
}

func TestPlanBatchRejectsSameDirectory(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "keep", "a")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}

	for _, dest := range []string{dir, link} {
		plan := planBatch([]file{fileNamed(t, dir, "a")}, dest)
		if len(plan.rejected) != 1 || len(plan.clean)+len(plan.conflicts) != 0 {
			t.Fatalf("dest %s: expected rejection, got %+v", dest, plan)
		}
	}
}

func TestPlanBatchRejectsDirectoryIntoItself(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "dir", "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}

	plan := planBatch([]file{fileNamed(t, root, "dir")}, sub)
	if len(plan.rejected) != 1 {
		t.Fatalf("expected rejection, got %+v", plan)
	}
	// A sibling whose name merely starts with the source name is fine.
	if err := os.Mkdir(filepath.Join(root, "dir2"), 0755); err != nil {
		t.Fatal(err)
	}
	plan = planBatch([]file{fileNamed(t, root, "dir")}, filepath.Join(root, "dir2"))
	if len(plan.clean) != 1 {
		t.Fatalf("expected clean, got %+v", plan)
	}
}

func TestTransferFilesRefusesUnsafeTargets(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, "keep", "a")
	a := fileNamed(t, dir, "a")

	if err := transferFiles([]file{a}, dir, false); err == nil {
		t.Fatal("copying a file onto itself must fail")
	}
	if got := readFile(t, a.Path); got != "keep" {
		t.Fatalf("source content changed to %q", got)
	}
}

func TestTransferFilesKeepsGoingAfterError(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, "data", "a", "b")
	missing := file{Name: "missing", Path: filepath.Join(src, "missing")}

	err := transferFiles([]file{missing, fileNamed(t, src, "a"), fileNamed(t, src, "b")}, dst, false)
	if err == nil {
		t.Fatal("expected an error for the missing file")
	}
	for _, name := range []string{"a", "b"} {
		if got := readFile(t, filepath.Join(dst, name)); got != "data" {
			t.Fatalf("%s: got %q", name, got)
		}
	}
}

func TestReadDirectoryResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "d"), filepath.Join(dir, "to-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "broken")); err != nil {
		t.Fatal(err)
	}

	if f := fileNamed(t, dir, "to-dir"); !f.IsDir || !f.IsSymlink {
		t.Errorf("link to a directory: %+v", f)
	}
	if f := fileNamed(t, dir, "broken"); f.IsDir || !f.IsSymlink {
		t.Errorf("broken link: %+v", f)
	}
}

func TestReadDirectoryErrorKeepsParent(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	files, err := readDirectory(missing)
	if err == nil {
		t.Fatal("expected an error")
	}
	if len(files) != 1 || !files[0].IsParent {
		t.Fatalf("expected only '..', got %+v", files)
	}
}

func TestCopySymlinksAsLinks(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(src, "d"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("d", filepath.Join(src, "top")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(src, "tree"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../d", filepath.Join(src, "tree", "nested")); err != nil {
		t.Fatal(err)
	}

	err := transferFiles([]file{fileNamed(t, src, "top"), fileNamed(t, src, "tree")}, dst, false)
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"top": "d", "tree/nested": "../d"} {
		if got, err := os.Readlink(filepath.Join(dst, path)); err != nil || got != want {
			t.Errorf("%s: got link to %q (%v), want %q", path, got, err, want)
		}
	}
}

func TestTransferRefusesOverwritingSourceUnderAnotherName(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	writeFiles(t, b, "keep", "f", "g")
	// a/f is a symlink to b/f: copying it into b would replace b/f with a link to itself.
	if err := os.Symlink(filepath.Join(b, "f"), filepath.Join(a, "f")); err != nil {
		t.Fatal(err)
	}
	// a/g is a hard link to b/g: copying it into b would truncate both.
	if err := os.Link(filepath.Join(b, "g"), filepath.Join(a, "g")); err != nil {
		t.Fatal(err)
	}

	sources := []file{fileNamed(t, a, "f"), fileNamed(t, a, "g")}
	if plan := planBatch(sources, b); len(plan.rejected) != 2 {
		t.Fatalf("expected both rejected, got %+v", plan)
	}
	for _, move := range []bool{false, true} {
		if err := transferFiles(sources, b, move); err == nil {
			t.Fatal("transferFiles accepted the batch")
		}
	}
	for _, name := range []string{"f", "g"} {
		if got := readFile(t, filepath.Join(b, name)); got != "keep" {
			t.Errorf("%s: got %q", name, got)
		}
	}
}

// simulateCrossDevice makes every rename fail as it does between two filesystems.
// It swaps the package-level rename, so tests that move files must not run in parallel.
func simulateCrossDevice(t *testing.T) {
	t.Helper()
	rename = func(oldpath, newpath string) error {
		return &os.LinkError{Op: "rename", Old: oldpath, New: newpath, Err: syscall.EXDEV}
	}
	t.Cleanup(func() { rename = os.Rename })
}

// mkdirs creates each directory path under root.
func mkdirs(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(root, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

// assertGone fails the test if path still exists.
func assertGone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("%s still exists (%v)", path, err)
	}
}

func TestCopyPreservesModeAndModTime(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	mkdirs(t, src, "dir")
	writeFiles(t, filepath.Join(src, "dir"), "data", "f")
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	f, d := filepath.Join(src, "dir", "f"), filepath.Join(src, "dir")
	if err := os.Chmod(f, 0640); err != nil {
		t.Fatal(err)
	}
	// A read-only source directory must still be copyable.
	for _, p := range []string{f, d} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(d, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(d, 0755); os.Chmod(filepath.Join(dst, "dir"), 0755) })

	if err := transferFiles([]file{fileNamed(t, src, "dir")}, dst, false); err != nil {
		t.Fatal(err)
	}
	for path, mode := range map[string]os.FileMode{"dir": 0555, "dir/f": 0640} {
		info, err := os.Stat(filepath.Join(dst, path))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode || !info.ModTime().Equal(old) {
			t.Errorf("%s: mode %v, mtime %v", path, info.Mode().Perm(), info.ModTime())
		}
	}
}

func TestCopyOverwriteLeavesNoTemporaryFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	writeFiles(t, src, "new", "f")
	writeFiles(t, dst, "old", "f")

	if err := transferFiles([]file{fileNamed(t, src, "f")}, dst, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dst, "f")); got != "new" {
		t.Fatalf("got %q", got)
	}
	if entries, _ := os.ReadDir(dst); len(entries) != 1 {
		t.Fatalf("expected only f in destination, got %v", entries)
	}
}

func TestCopyRefusesSpecialFiles(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	mkdirs(t, src, "dir")
	if err := unix.Mkfifo(filepath.Join(src, "dir", "fifo"), 0644); err != nil {
		t.Skip("cannot create a FIFO:", err)
	}

	// Opening the FIFO for reading would block forever.
	done := make(chan error, 1)
	go func() { done <- transferFiles([]file{fileNamed(t, src, "dir")}, dst, false) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("copying a FIFO must fail")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("copying a FIFO blocked")
	}
}

func TestMoveMergesIntoExistingDirectory(t *testing.T) {
	for _, crossDevice := range []bool{false, true} {
		t.Run(fmt.Sprint("crossDevice=", crossDevice), func(t *testing.T) {
			testMoveMerges(t, crossDevice)
		})
	}
}

// testMoveMerges moves a directory onto an existing one, by rename or across devices.
func testMoveMerges(t *testing.T, crossDevice bool) {
	src, dst := t.TempDir(), t.TempDir()
	if crossDevice {
		simulateCrossDevice(t)
	}
	mkdirs(t, src, "dir/sub")
	mkdirs(t, dst, "dir/sub")
	writeFiles(t, filepath.Join(src, "dir"), "new", "moved", "both")
	writeFiles(t, filepath.Join(src, "dir", "sub"), "new", "deep")
	writeFiles(t, filepath.Join(dst, "dir"), "old", "kept", "both")

	if err := transferFiles([]file{fileNamed(t, src, "dir")}, dst, true); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"moved": "new", "both": "new", "kept": "old", "sub/deep": "new"} {
		if got := readFile(t, filepath.Join(dst, "dir", path)); got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
	assertGone(t, filepath.Join(src, "dir"))
}

func TestMoveAcrossDevices(t *testing.T) {
	simulateCrossDevice(t)
	src, dst := t.TempDir(), t.TempDir()
	mkdirs(t, src, "dir/sub")
	writeFiles(t, src, "new", "f")
	writeFiles(t, filepath.Join(src, "dir", "sub"), "deep", "g")
	if err := os.Symlink("sub/g", filepath.Join(src, "dir", "link")); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dst, "old", "f") // Overwritten, as if confirmed
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(src, "f"), old, old); err != nil {
		t.Fatal(err)
	}

	sources := []file{fileNamed(t, src, "f"), fileNamed(t, src, "dir")}
	if err := transferFiles(sources, dst, true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dst, "f")); got != "new" {
		t.Errorf("f: got %q", got)
	}
	if info, err := os.Stat(filepath.Join(dst, "f")); err != nil || !info.ModTime().Equal(old) {
		t.Errorf("f: mtime not kept (%v)", err)
	}
	if got := readFile(t, filepath.Join(dst, "dir", "sub", "g")); got != "deep" {
		t.Errorf("dir/sub/g: got %q", got)
	}
	if got, err := os.Readlink(filepath.Join(dst, "dir", "link")); err != nil || got != "sub/g" {
		t.Errorf("dir/link: got %q (%v)", got, err)
	}
	assertGone(t, filepath.Join(src, "f"))
	assertGone(t, filepath.Join(src, "dir"))
}

func TestFailedMoveAcrossDevicesKeepsSource(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read the unreadable file")
	}
	simulateCrossDevice(t)
	src, dst := t.TempDir(), t.TempDir()
	mkdirs(t, src, "dir")
	writeFiles(t, filepath.Join(src, "dir"), "data", "a", "secret")
	if err := os.Chmod(filepath.Join(src, "dir", "secret"), 0); err != nil {
		t.Fatal(err)
	}

	if err := transferFiles([]file{fileNamed(t, src, "dir")}, dst, true); err == nil {
		t.Fatal("expected the unreadable file to fail the move")
	}
	if got := readFile(t, filepath.Join(src, "dir", "a")); got != "data" {
		t.Errorf("source changed: %q", got)
	}
	assertGone(t, filepath.Join(dst, "dir")) // No partial copy left behind
}

// TestMoveAcrossRealDevices moves into $TWIN_XDEV_DIR, which must be on another
// filesystem than the system temp directory (e.g. a tmpfs mount in CI).
func TestMoveAcrossRealDevices(t *testing.T) {
	root := os.Getenv("TWIN_XDEV_DIR")
	if root == "" {
		t.Skip("TWIN_XDEV_DIR not set")
	}
	src := t.TempDir()
	dst, err := os.MkdirTemp(root, "twin-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dst) })
	var srcStat, dstStat unix.Stat_t
	if unix.Stat(src, &srcStat) != nil || unix.Stat(dst, &dstStat) != nil || srcStat.Dev == dstStat.Dev {
		t.Skip("TWIN_XDEV_DIR is on the same filesystem as the temp directory")
	}
	mkdirs(t, src, "dir")
	writeFiles(t, filepath.Join(src, "dir"), "data", "f")

	if err := transferFiles([]file{fileNamed(t, src, "dir")}, dst, true); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dst, "dir", "f")); got != "data" {
		t.Errorf("got %q", got)
	}
	assertGone(t, filepath.Join(src, "dir"))
}

func TestMoveAcrossDevicesOntoSymlinkDoesNotWriteThroughIt(t *testing.T) {
	simulateCrossDevice(t)
	src, dst, other := t.TempDir(), t.TempDir(), t.TempDir()
	mkdirs(t, src, "proj")
	writeFiles(t, filepath.Join(src, "proj"), "new", "f")
	writeFiles(t, other, "precious", "f")
	if err := os.Symlink(other, filepath.Join(dst, "proj")); err != nil {
		t.Fatal(err)
	}

	if err := transferFiles([]file{fileNamed(t, src, "proj")}, dst, true); err == nil {
		t.Fatal("expected the move onto a symlink to fail")
	}
	if got := readFile(t, filepath.Join(other, "f")); got != "precious" {
		t.Errorf("link target overwritten: %q", got)
	}
	if got := readFile(t, filepath.Join(src, "proj", "f")); got != "new" {
		t.Errorf("source changed: %q", got)
	}
}

func TestMoveAcrossDevicesWithReadOnlyDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	simulateCrossDevice(t)
	src, dst := t.TempDir(), t.TempDir()
	mkdirs(t, src, "ok/ro", "bad/ro")
	writeFiles(t, filepath.Join(src, "ok", "ro"), "data", "f")
	writeFiles(t, filepath.Join(src, "bad", "ro"), "data", "f")
	writeFiles(t, filepath.Join(src, "bad"), "data", "secret") // Copied after ro/
	if err := os.Chmod(filepath.Join(src, "bad", "secret"), 0); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"ok/ro", "bad/ro"} {
		if err := os.Chmod(filepath.Join(src, d), 0555); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { removeTree(src); removeTree(dst) })

	// The source is removed even though it holds a read-only directory.
	if err := transferFiles([]file{fileNamed(t, src, "ok")}, dst, true); err != nil {
		t.Fatal(err)
	}
	assertGone(t, filepath.Join(src, "ok"))
	if got := readFile(t, filepath.Join(dst, "ok", "ro", "f")); got != "data" {
		t.Errorf("ok/ro/f: got %q", got)
	}

	// A failed copy is cleaned up even though it holds a read-only directory.
	if err := transferFiles([]file{fileNamed(t, src, "bad")}, dst, true); err == nil {
		t.Fatal("expected the unreadable file to fail the move")
	}
	assertGone(t, filepath.Join(dst, "bad"))
}

func TestCopyLongFileName(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	name := strings.Repeat("n", 250)
	writeFiles(t, src, "data", name)
	writeFiles(t, dst, "old", name)

	if err := transferFiles([]file{fileNamed(t, src, name)}, dst, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dst, name)); got != "data" {
		t.Errorf("got %q", got)
	}
}
