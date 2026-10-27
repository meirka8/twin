package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// A Source is where a pane's entries come from. A pane lists, enters and previews only
// through its source, so it works the same on any of them: only the local filesystem
// exists so far; archives, SFTP and search results ("panelize") are to come. Paths are the
// source's own; on the local filesystem they are absolute OS paths.
//
// Changing files (copy, move, delete, mkdir) and handing them to other programs work only
// on the local filesystem for now (see localOnly); the first writable source of another
// kind will shape an interface for them.
//
// Panes compare sources with ==, so an implementation must be comparable: a pointer, or a
// struct of comparable fields. Check it with assertComparable.
type Source interface {
	// List returns the entries of the directory dir, in any order and without "..".
	List(dir string) ([]file, error)
	// Parent returns the directory that ".." in dir leads to; ok is false at the top.
	Parent(dir string) (parent string, ok bool)
	// Open opens the regular file at path for reading.
	Open(path string) (io.ReadCloser, error)
}

// assertComparable does not compile for a type that is not comparable.
func assertComparable[T comparable]() {}

// localFS is the local filesystem.
type localFS struct{}

var _ = assertComparable[localFS]

func (localFS) List(dir string) ([]file, error) { return readDirectory(dir) }

func (localFS) Parent(dir string) (string, bool) {
	parent := filepath.Dir(dir)
	return parent, parent != dir
}

// Open opens the regular file at path, following a symlink. Anything else is refused:
// opening a FIFO for reading blocks until a writer comes, and a device may never end. The
// file is opened non-blocking and checked again, in case it was replaced in between.
func (localFS) Open(path string) (io.ReadCloser, error) {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
		return nil, notRegular(path, err)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	if info, err := f.Stat(); err != nil || !info.Mode().IsRegular() {
		f.Close()
		return nil, notRegular(path, err)
	}
	return f, nil
}

// notRegular is err, or if there is none, the error for a path that is not a regular file.
func notRegular(path string, err error) error {
	if err != nil {
		return err
	}
	return fmt.Errorf("%s: not a regular file", path)
}

// localOnly returns an error naming action unless every source is the local filesystem,
// the only place where Twin changes files or hands them to other programs for now.
func localOnly(action string, sources ...Source) error {
	for _, src := range sources {
		if _, ok := src.(localFS); !ok {
			return fmt.Errorf("%s works on the local filesystem only", action)
		}
	}
	return nil
}
