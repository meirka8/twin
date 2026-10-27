package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/atotto/clipboard"
)

// Commands
// loadDirectoryCmd lists path for the pane: its own path to reload, another one to navigate.
func (p pane) loadDirectoryCmd(path, focusPath string) tea.Cmd {
	return func() tea.Msg {
		files, err := readDirectory(path)
		return directoryLoadedMsg{paneID: p.id, path: path, files: files, err: err, focusPath: focusPath}
	}
}

func openFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("xdg-open", path)
		if err := cmd.Run(); err != nil {
			return fileOpenedMsg{err: fmt.Errorf("open %s: %w", filepath.Base(path), err)}
		}
		return fileOpenedMsg{}
	}
}

func createFolderCmd(path string) tea.Cmd {
	return func() tea.Msg {
		err := os.Mkdir(path, 0755)
		return folderCreatedMsg{err: err, folderPath: path}
	}
}

func deleteFilesCmd(files []file) tea.Cmd {
	return func() tea.Msg {
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

// planTransferCmd checks a copy/move batch against the destination without touching any
// file; Update then resolves conflicts and runs transferFilesCmd for the whole batch.
func planTransferCmd(sourceFiles []file, destPath string, move bool) tea.Cmd {
	return func() tea.Msg {
		return transferPlannedMsg{plan: planBatch(sourceFiles, destPath), destPath: destPath, move: move}
	}
}

func transferFilesCmd(files []file, destPath string, move bool) tea.Cmd {
	return func() tea.Msg {
		return fileOperationMsg{err: transferFiles(files, destPath, move)}
	}
}

func previewFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		content, err := os.ReadFile(path)
		if err != nil {
			return previewReadyMsg{path: path, err: fmt.Errorf("could not read file: %w", err)}
		}

		// Basic check for binary content
		if !utf8.Valid(content) || bytes.Contains(content, []byte{0}) {
			return previewReadyMsg{path: path, content: fmt.Sprintf("--- Binary file: %s ---", filepath.Base(path))}
		}

		// Limit preview size
		const maxPreviewSize = 1024 * 100 // 100KB
		if len(content) > maxPreviewSize {
			return previewReadyMsg{path: path, content: fmt.Sprintf("--- File too large for preview (%s), showing first %d bytes ---\n%s", filepath.Base(path), maxPreviewSize, content[:maxPreviewSize])}
		}

		return previewReadyMsg{path: path, content: string(content)}
	}
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
