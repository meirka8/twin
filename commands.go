package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/atotto/clipboard"
	"github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"
)

// Commands
func (p pane) loadDirectoryCmd(focusPath string) tea.Cmd {
	return func() tea.Msg {
		files, err := readDirectory(p.path)
		return directoryLoadedMsg{paneID: p.id, files: files, err: err, focusPath: focusPath}
	}
}

func openFileCmd(path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("xdg-open", path)
		err := cmd.Run()
		return fileOpenedMsg{err: err}
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
			if f.IsDir {
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
			return previewReadyMsg{Err: fmt.Errorf("could not read file: %w", err)}
		}

		// Basic check for binary content
		if !utf8.Valid(content) || bytes.Contains(content, []byte{0}) {
			return previewReadyMsg{Content: fmt.Sprintf("--- Binary file: %s ---", filepath.Base(path))}
		}

		// Limit preview size
		const maxPreviewSize = 1024 * 100 // 100KB
		if len(content) > maxPreviewSize {
			return previewReadyMsg{Content: fmt.Sprintf("--- File too large for preview (%s), showing first %d bytes ---\n%s", filepath.Base(path), maxPreviewSize, content[:maxPreviewSize])}
		}

		return previewReadyMsg{Content: string(content)}
	}
}

func unmountDriveCmd(path string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("umount", path)
		err := cmd.Run()
		return driveUnmountedMsg{err: err, drivePath: path}
	}
}

func copyToClipboardCmd(text string) tea.Cmd {
	return func() tea.Msg {
		err := clipboard.WriteAll(text)
		if err != nil {
			// Fallback to OSC 52
			osc52.New(text).WriteTo(os.Stderr)
			// We don't return the error if OSC 52 "succeeds" (it just writes to stderr)
			// But strictly speaking we don't know if the terminal handled it.
			// However, it's better than failing.
			return clipboardCopiedMsg{err: nil}
		}
		return clipboardCopiedMsg{err: err}
	}
}
