package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Helper to get file structs from selected paths
func getFilesFromSelected(p pane) []file {
	var files []file
	for _, f := range p.files {
		if f.IsParent {
			continue
		}
		if _, ok := p.selected[f.Path]; ok {
			files = append(files, f)
		}
	}
	return files
}

// operationTargets returns the selected files of p, or the file under the cursor when
// nothing is selected. The ".." entry is never a target.
func operationTargets(p pane) []file {
	files := getFilesFromSelected(p)
	if len(files) == 0 && p.cursor < len(p.files) && !p.files[p.cursor].IsParent {
		files = []file{p.files[p.cursor]}
	}
	return files
}

// batchPlan is the result of checking a copy/move batch against its destination.
type batchPlan struct {
	clean     []file         // Destination is free
	conflicts []fileConflict // Destination already exists
	rejected  []string       // Why individual files can never be transferred
}

// resolvePath resolves symlinks in the parent directories of path, so that two paths
// reaching the same location through different links compare equal. The last element
// is kept as is: a symlink being copied is the link itself, not its target.
func resolvePath(path string) string {
	dir, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, filepath.Base(path))
}

// checkTransfer returns an error if src must never be copied or moved to dst.
func checkTransfer(src file, dst string) error {
	if src.IsParent {
		return fmt.Errorf("%s: cannot operate on the parent entry", src.Name)
	}
	srcPath := resolvePath(src.Path)
	dstPath := resolvePath(dst)
	if srcPath == dstPath {
		return fmt.Errorf("%s: source and destination are the same", src.Name)
	}
	if src.IsDir && !src.IsSymlink && strings.HasPrefix(dstPath, srcPath+string(filepath.Separator)) {
		return fmt.Errorf("%s: cannot copy a directory into itself", src.Name)
	}
	if sameFile(src, dst) {
		return fmt.Errorf("%s: destination is the same file as the source", src.Name)
	}
	return nil
}

// sameFile reports whether writing src to dst would overwrite src itself under another
// name: a hard link, or a symlink on either side. A symlink source is copied as a link,
// so for it only dst being the link's own target is a problem.
func sameFile(src file, dst string) bool {
	srcInfo, err := os.Stat(src.Path)
	if err != nil {
		return false
	}
	stat := os.Stat
	if src.IsSymlink {
		stat = os.Lstat
	}
	dstInfo, err := stat(dst)
	return err == nil && os.SameFile(srcInfo, dstInfo)
}

// planBatch sorts a copy/move batch into files that can go straight to destDir, files
// that would overwrite something, and files that must be refused. It touches nothing.
func planBatch(sources []file, destDir string) batchPlan {
	var plan batchPlan
	for _, src := range sources {
		dst := filepath.Join(destDir, src.Name)
		if err := checkTransfer(src, dst); err != nil {
			plan.rejected = append(plan.rejected, err.Error())
			continue
		}
		_, err := os.Lstat(dst)
		switch {
		case err == nil:
			plan.conflicts = append(plan.conflicts, fileConflict{Source: src, Destination: dst})
		case os.IsNotExist(err):
			plan.clean = append(plan.clean, src)
		default:
			plan.rejected = append(plan.rejected, fmt.Sprintf("%s: %v", src.Name, err))
		}
	}
	return plan
}

// transferFiles copies or moves every file into destDir, overwriting existing entries.
// It keeps going after a failure and returns all errors joined.
func transferFiles(files []file, destDir string, move bool) error {
	var errs []error
	for _, src := range files {
		dst := filepath.Join(destDir, src.Name)
		if err := checkTransfer(src, dst); err != nil {
			errs = append(errs, err)
			continue
		}
		var err error
		switch {
		case move:
			err = moveFile(src.Path, dst)
		case src.IsSymlink:
			err = copySymlink(src.Path, dst)
		case src.IsDir:
			err = copyDir(src.Path, dst)
		default:
			err = copyFile(src.Path, dst)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", src.Name, err))
		}
	}
	return errors.Join(errs...)
}

// rename is os.Rename, swapped out in tests to simulate a move across filesystems.
var rename = os.Rename

// moveFile moves src to dst. A directory moved onto an existing directory is merged into
// it entry by entry. Across filesystems, where rename fails with EXDEV, src is copied,
// the copy is flushed to disk, and only then is src removed.
func moveFile(src, dst string) error {
	err := rename(src, dst)
	if err == nil {
		return nil
	}
	srcInfo, statErr := os.Lstat(src)
	if statErr != nil {
		return err
	}
	dstInfo, statErr := os.Lstat(dst)
	dstExists := statErr == nil
	// Rename refuses to replace a non-empty directory, and cannot cross filesystems.
	mergeable := errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) || errors.Is(err, syscall.EXDEV)
	if mergeable && srcInfo.IsDir() && dstExists && dstInfo.IsDir() {
		return mergeDir(src, dst)
	}
	if !errors.Is(err, syscall.EXDEV) {
		return err
	}
	return moveAcrossDevices(src, dst, srcInfo, dstExists)
}

// mergeDir moves every entry of the directory src into the existing directory dst,
// then removes src. It stops at the first entry that fails.
func mergeDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := moveFile(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
			return err
		}
	}
	return os.Remove(src)
}

// moveAcrossDevices copies src to dst on another filesystem and removes src once the
// copy is on disk. If the copy fails, src is untouched and a dst that did not exist
// before is removed again, so no partial copy is left behind.
func moveAcrossDevices(src, dst string, srcInfo os.FileInfo, dstExists bool) error {
	var err error
	switch {
	case srcInfo.Mode()&os.ModeSymlink != 0:
		err = copySymlink(src, dst)
	case srcInfo.IsDir():
		err = copyDir(src, dst)
	default:
		err = copyFile(src, dst)
	}
	if err == nil {
		err = syncFS(filepath.Dir(dst))
	}
	if err != nil {
		if !dstExists {
			if rmErr := removeTree(dst); rmErr != nil {
				err = errors.Join(err, fmt.Errorf("removing the partial copy %s: %w", dst, rmErr))
			}
		}
		return err
	}
	if err := removeTree(src); err != nil {
		return fmt.Errorf("copied to %s, but removing the source failed: %w", dst, err)
	}
	return nil
}

// removeTree removes path like os.RemoveAll. If that fails, it makes every directory
// under path owner-writable, since read-only directories (which copyDir recreates)
// cannot be emptied, and tries again.
func removeTree(path string) error {
	if err := os.RemoveAll(path); err == nil {
		return nil
	}
	// WalkDir visits a directory before reading it, so it is readable by then.
	filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			if info, err := d.Info(); err == nil {
				os.Chmod(p, info.Mode()|0700)
			}
		}
		return nil // Best effort: RemoveAll below reports what is still in the way
	})
	return os.RemoveAll(path)
}

// syncFS flushes the filesystem holding dir to disk.
func syncFS(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return unix.Syncfs(int(f.Fd()))
}

// copyFile copies the regular file src to dst, keeping its mode and modification time.
// It writes a temporary file next to dst and renames it into place, so an existing dst
// is either replaced whole or left as it was, never half-written. Replacing means an
// existing dst is a new file: it is owned by the user, loses its ACLs and xattrs, a
// symlink there is replaced rather than written through, other hard links to it keep
// the old content, and the destination directory must be writable.
func copyFile(src, dst string) (err error) {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() { // Opening a FIFO would block, a device would never end
		return fmt.Errorf("%s: not a regular file", src)
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Not named after dst: a long name plus a suffix could exceed NAME_MAX.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".twin-copy-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			if rmErr := os.Remove(tmp.Name()); rmErr != nil {
				err = errors.Join(err, rmErr)
			}
		}
	}()

	if _, err = io.Copy(tmp, in); err != nil {
		return err
	}
	if err = tmp.Chmod(info.Mode()); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil { // Delayed write errors surface here
		return err
	}
	if err = os.Chtimes(tmp.Name(), time.Time{}, info.ModTime()); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// copyDir recursively copies a directory from src to dst, merging into dst if it exists.
// A directory it creates gets the mode and modification time of src.
func copyDir(src, dst string) error {
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	dstInfo, err := os.Lstat(dst)
	created := os.IsNotExist(err)
	if err == nil && dstInfo.Mode()&os.ModeSymlink != 0 {
		// MkdirAll would follow it and merge into whatever the link points at.
		return fmt.Errorf("%s: destination is a symlink, not a directory", dst)
	}

	// Owner-writable while it is filled, so a read-only source can still be copied.
	if err := os.MkdirAll(dst, 0700); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		switch {
		case entry.Type()&os.ModeSymlink != 0:
			err = copySymlink(srcPath, dstPath)
		case entry.IsDir():
			err = copyDir(srcPath, dstPath)
		default:
			err = copyFile(srcPath, dstPath)
		}
		if err != nil {
			return err
		}
	}
	if !created {
		return nil // Merged into an existing directory: leave its own attributes alone
	}
	if err := os.Chmod(dst, sourceInfo.Mode()); err != nil {
		return err
	}
	// Last, since adding the entries above updated the modification time.
	return os.Chtimes(dst, time.Time{}, sourceInfo.ModTime())
}

// copySymlink recreates the symlink src at dst with the same target, replacing an
// existing file, link or empty directory at dst.
func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, dst)
}

// readDirectory reads the contents of a directory and returns a sorted list of file structs.
// On error it still returns the ".." entry, so a pane is never stranded without a way up.
func readDirectory(dirPath string) ([]file, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return withParentEntry(nil, dirPath), err
	}

	var files []file
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			log.Printf("Error getting file info for %s: %v", filepath.Join(dirPath, entry.Name()), err)
			continue
		}

		path := filepath.Join(dirPath, entry.Name())
		isSymlink := info.Mode()&os.ModeSymlink != 0
		isDir := entry.IsDir()
		if isSymlink {
			// A link to a directory is entered like one; a broken link stays a file.
			if target, err := os.Stat(path); err == nil {
				isDir = target.IsDir()
			}
		}

		files = append(files, file{
			Name:      entry.Name(),
			Path:      path,
			Size:      info.Size(),
			Mode:      info.Mode(),
			ModTime:   info.ModTime(),
			IsDir:     isDir,
			IsSymlink: isSymlink,
		})
	}

	// Sort files: directories first, then alphabetically
	sort.Slice(files, func(i, j int) bool {
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir // Directories come before files
		}
		return files[i].Name < files[j].Name
	})

	return withParentEntry(files, dirPath), nil
}

// withParentEntry prepends the synthetic ".." entry to the listing of dirPath, unless
// dirPath is the root.
func withParentEntry(files []file, dirPath string) []file {
	if filepath.Dir(dirPath) == dirPath {
		return files
	}
	parent := file{
		Name:     "..",
		Path:     filepath.Dir(dirPath),
		IsDir:    true,
		IsParent: true,
		Mode:     os.ModeDir,
		ModTime:  time.Now(), // Dummy time
	}
	return append([]file{parent}, files...)
}

// getMountedDrives returns a list of mounted drive paths on Linux systems.
func getMountedDrives() []string {
	content, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}

	var drives []string
	lines := strings.Split(string(content), "\n")
	seen := make(map[string]struct{})

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		mountPoint := fields[1]
		// Unescape octal sequences (like \040 for space)
		mountPoint = strings.ReplaceAll(mountPoint, "\\040", " ")
		mountPoint = strings.ReplaceAll(mountPoint, "\\011", "\t")

		// Filter for common external drive mount points
		if strings.HasPrefix(mountPoint, "/media") ||
			strings.HasPrefix(mountPoint, "/mnt") ||
			strings.HasPrefix(mountPoint, "/run/media") {
			if _, ok := seen[mountPoint]; !ok {
				seen[mountPoint] = struct{}{}
				drives = append(drives, mountPoint)
			}
		}
	}
	sort.Strings(drives)
	return drives
}

// getStandardPaths returns common user directories.
func getStandardPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return []string{}
	}

	paths := []string{home}
	dirs := []string{"Documents", "Downloads", "Desktop", "Pictures", "Music", "Videos"}

	for _, d := range dirs {
		p := filepath.Join(home, d)
		if _, err := os.Stat(p); err == nil {
			paths = append(paths, p)
		}
	}
	return paths
}
