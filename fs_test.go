package main

import (
	"os"
	"path/filepath"
	"testing"
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
