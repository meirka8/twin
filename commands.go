package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// Commands that do I/O off the UI thread and report back with a message. The pane's own
// listing command, loadDirectoryCmd, is in pane.go.

// The commands below that change files or hand them to another program check their
// sources with localOnly themselves, although the shortcuts refuse earlier for the user's
// sake: a path of another source must never reach the disk.

// launch opens path with its default application. Tests replace it, so that they never start
// one.
var launch = func(path string) error { return exec.Command("xdg-open", path).Run() }

// openFileCmd opens the file at path of src with its default application.
func openFileCmd(src Source, path string) tea.Cmd {
	return func() tea.Msg {
		if err := localOnly("Open", src); err != nil {
			return fileOpenedMsg{err: err}
		}
		if err := launch(path); err != nil {
			return fileOpenedMsg{err: fmt.Errorf("open %s: %w", filepath.Base(path), err)}
		}
		return fileOpenedMsg{}
	}
}

func createFolderCmd(src Source, path string) tea.Cmd {
	return func() tea.Msg {
		if err := localOnly("MkDir", src); err != nil {
			return folderCreatedMsg{err: err}
		}
		err := os.Mkdir(path, 0755)
		return folderCreatedMsg{err: err, folderPath: path}
	}
}

func deleteFilesCmd(src Source, files []file) tea.Cmd {
	return func() tea.Msg {
		if err := localOnly("Delete", src); err != nil {
			return fileDeletedMsg{err: err}
		}
		var errors []string
		for _, f := range files {
			if f.IsParent {
				continue // Never delete the parent through its ".." entry
			}
			var err error
			if f.IsDir && !f.IsSymlink { // A link is removed, never what it points to
				err = os.RemoveAll(f.Path)
			} else {
				err = os.Remove(f.Path)
			}
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", f.Name, err))
			}
		}
		if len(errors) > 0 {
			return fileDeletedMsg{err: fmt.Errorf("failed to delete: %s", strings.Join(errors, ", "))}
		}
		return fileDeletedMsg{err: nil}
	}
}

// transferAction names a copy or a move for messages.
func transferAction(move bool) string {
	if move {
		return "Move"
	}
	return "Copy"
}

// planTransferCmd checks a copy/move batch from src against destDir of dst without touching
// any file; Update then resolves conflicts and runs transferFilesCmd for the whole batch.
func planTransferCmd(src Source, files []file, dst Source, destDir string, move bool) tea.Cmd {
	return func() tea.Msg {
		msg := transferPlannedMsg{src: src, dst: dst, destPath: destDir, move: move}
		if err := localOnly(transferAction(move), src, dst); err != nil {
			msg.plan.rejected = []string{err.Error()}
			return msg
		}
		msg.plan = planBatch(files, destDir)
		return msg
	}
}

func transferFilesCmd(src Source, files []file, dst Source, destDir string, move bool) tea.Cmd {
	return func() tea.Msg {
		if err := localOnly(transferAction(move), src, dst); err != nil {
			return fileOperationMsg{err: err}
		}
		return fileOperationMsg{err: transferFiles(files, destDir, move)}
	}
}

// previewLimit is how much of a file the preview reads and shows.
const previewLimit = 100 * 1024

// previewFileCmd reads the start of the file at path of src for the preview.
func previewFileCmd(src Source, path string) tea.Cmd {
	return func() tea.Msg {
		msg := previewReadyMsg{source: src, path: path}
		content, more, err := readHead(src, path, previewLimit)
		switch {
		case err != nil:
			msg.err = fmt.Errorf("could not read file: %w", err)
		case !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0: // Binary, as far as read
			msg.content = fmt.Sprintf("--- Binary file: %s ---", filepath.Base(path))
		case more:
			msg.content = fmt.Sprintf("--- File too large for preview (%s), showing the first %d KB ---\n%s", filepath.Base(path), previewLimit/1024, content)
		default:
			msg.content = string(content)
		}
		return msg
	}
}

// readHead reads at most limit bytes from the start of the file at path of src, without
// splitting a UTF-8 character at the end; more reports whether the file goes on.
func readHead(src Source, path string, limit int) (_ []byte, more bool, err error) {
	f, err := src.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	content, err := io.ReadAll(io.LimitReader(f, int64(limit)+1)) // One byte over tells there is more
	if err != nil || len(content) <= limit {
		return content, false, err
	}
	content = content[:limit]
	for i := len(content) - 1; i >= max(len(content)-utf8.UTFMax, 0); i-- {
		if utf8.RuneStart(content[i]) {
			if !utf8.FullRune(content[i:]) {
				content = content[:i]
			}
			break
		}
	}
	return content, true, nil
}

func unmountDriveCmd(path string) tea.Cmd {
	return func() tea.Msg {
		out, err := exec.Command("umount", path).CombinedOutput()
		if err != nil {
			if detail := strings.TrimSpace(string(out)); detail != "" {
				err = errors.New(detail) // umount's own message names the reason
			}
			err = fmt.Errorf("unmount %s: %w", path, err)
		}
		return driveUnmountedMsg{err: err, drivePath: path}
	}
}

func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		if err := clipboard.WriteAll(text); err != nil {
			// No clipboard tool (wl-copy, xclip, xsel): ask the terminal to set the clipboard
			// with OSC 52. Whether it does cannot be known, so there is nothing to report.
			return tea.SetClipboard(text)()
		}
		return nil
	}
}
