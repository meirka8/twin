# Twin Manager — Documentation

Twin is a two-pane file manager for the terminal in the Norton / Midnight Commander tradition: you
pick files in one pane and act on them towards the other. It is Linux-focused.

## Requirements

- Linux. Mounted drives are read from `/proc/mounts`, and unmounting runs `umount`.
- `xdg-open` to open files with their default application.
- For Copy Path: `wl-copy` (Wayland), `xclip` or `xsel`. Without any of them Twin sends the path
  to the terminal with the OSC 52 escape sequence, which most current terminals put on the
  clipboard.

## Running

```bash
go run .
```

or build the `twin` binary first:

```bash
go build .
./twin
```

Both panes start in the current directory, with the left pane active.

## The screen

- **Panes.** Each pane shows its directory path on the first line and its entries below:
  directories first (in blue), then files, each group sorted by name in byte order (capitals
  before lowercase). Hidden files are always shown. Every directory except `/` starts with a
  `..` entry that leads to the parent. Symbolic links end in `@`; a link to a directory is listed
  with the directories and is entered like one.
- **Active pane.** The active pane has a blue border, the other a grey one. Copy and move always
  go from the active pane into the other pane's directory.
- **Status bar.** Shows the name, permissions and modification time of the entry under the
  cursor, the quick-search query, the questions Twin asks (overwrite? delete? unmount?) and, in
  red, the latest error.
- **Hint bar.** Lists the Alt shortcuts.

## Keys

### Shortcuts

Most shortcuts are Alt+letter; the classic ones also answer to the Midnight Commander function
key. In a terminal with the Kitty keyboard protocol (Kitty, Ghostty and foot, for example) the
Alt and Ctrl shortcuts also work while the keyboard is in another layout, such as Hebrew or
Russian; in other terminals they need a Latin layout.

<!-- shortcuts: kept in sync with keys.go by docs_test.go -->
| Key | F-key | Action |
| --- | --- | --- |
| `Alt+V` | `F3` | View the file under the cursor |
| `Alt+C` | `F5` | Copy to the other pane |
| `Alt+M` | `F6` | Move to the other pane |
| `Alt+N` | `F7` | Create a folder in the active pane |
| `Alt+D` | `F8` | Delete (permanently) |
| `Alt+P` | `F9` | Copy the full path to the clipboard |
| `Alt+Q` | `F10` | Quit, without asking |
| `Ctrl+C` | | Quit (same as Alt+Q) |
| `Tab` | | Switch the active pane |
| `Insert` | | Select or deselect the entry under the cursor, then move down |
| `Alt+F` | | Favorites and drives |
| `Alt+A` | | Add a directory to the favorites |
| `Alt+I` | | Sync panes: open the active pane's directory in the other pane |
| `Alt+O` | | Open the directory under the cursor in the other pane |
| `Alt+L` | | Error log |

### Moving around

| Key | Action |
| --- | --- |
| `↑` `↓` | Move the cursor |
| `PgUp` `PgDn` | Move the cursor by a pane's height |
| `Home` `End` | First or last entry |
| `Enter` | Enter a directory, or open a file with `xdg-open`. On `..`, go up with the cursor on the directory you came from |
| any letter, digit or symbol, in any script | Quick search |
| `Esc` | Clear the quick search |

### Quick search

Typing in a pane jumps to a match; there is nothing to open first. Each character extends the
query, which is shown in the status bar; letters of any script count, and a paste adds to the
query as if typed. The cursor jumps to the first entry whose name starts with the query,
ignoring case; if there is none, to the first entry that contains the query's characters in
order (`cmg` finds `commands.go`). Backspace deletes the last character of the query. Esc,
↑/↓, PgUp/PgDn and Enter clear the query.

## Working with files

### Which files an operation acts on

Copy, Move, Delete and Copy Path act on the selected entries of the active pane, or on the entry
under the cursor when nothing is selected. Select with Insert. The `..` entry cannot be selected
and is never copied, moved or deleted. Starting a copy or move, or confirming a delete, clears
the selection.

### Copy and Move

Alt+C (F5) copies and Alt+M (F6) moves into the other pane's directory. Twin first checks the
whole batch without touching anything:

- **Refused entries** are reported as errors while the rest of the batch goes ahead: `..`, an
  entry copied or moved onto itself (both panes in the same directory, or the same file reached
  through a hard link or symlink), and a directory copied or moved into itself or one of its
  subdirectories.
- **Conflicts.** For every entry whose name already exists at the destination, the status bar
  asks `Overwrite NAME? (y/n/A/s)`:

  | Key | Answer |
  | --- | --- |
  | `y` | Overwrite this one |
  | `n` | Skip this one |
  | `A` | Overwrite this one and every remaining conflict |
  | `s` | Skip this one and every remaining conflict |
  | `Esc` | Cancel the whole batch: nothing is copied or moved, not even the entries without a conflict |

  Nothing is written until every conflict is answered; then the whole batch runs at once. If
  another dialog was opened while Twin was still checking the batch, the questions wait until
  it closes, so keys typed into it never answer them.

Overwriting a directory merges into it: files inside it with the same names are overwritten
without further questions, and files that exist only at the destination stay.

**Copying** keeps permissions and modification times. Each file is written to a temporary file
next to its destination and renamed into place, so an overwritten file is either replaced whole
or left as it was. Replacing makes it a new file: it is owned by you, loses its ACLs and extended
attributes, a symlink at the destination is replaced instead of written through, and other hard
links to the old file keep the old content. Symbolic links are copied as links to the same
target, never followed. FIFOs, sockets and device files are refused. A directory is never copied
or moved through a symlink at the destination.

**Moving** renames within a filesystem. Across filesystems (to a USB drive, for example) Twin
copies, flushes the copy to disk, and only then removes the source; if the copy fails, the source
is untouched and nothing half-copied is left behind. A directory moved onto an existing directory
is merged into it entry by entry, each entry moved this way. If one fails, the merge stops there:
the entries moved before it stay moved and the rest stay in the source.

When a batch finishes, both panes reload. A failed entry does not stop the rest of the batch, and
each failure goes to the error log. Within a copied directory, though, the first failure stops the
rest of that directory, and what was copied before it stays. There is no progress display yet: a
large copy runs in the background and the panes update when it is done.

### Delete

Alt+D (F8) asks `Delete NAME? (y/n)`, or `Delete N items? (y/n)`. `y` deletes; `n` or Esc
cancels. **Deleting is permanent**: there is no trash and no undo yet. Directories are deleted
with everything in them. A symbolic link is removed itself, never what it points to.

### New folder

Alt+N (F7) asks for a name in the status bar. Enter creates the folder in the active pane's
directory and puts the cursor on it; Enter with no name, or Esc, cancels. The name can be in any
script, and can be pasted with the terminal's paste (Ctrl+Shift+V in most terminals): a line
break at the end of the paste is dropped, any other becomes a space. The usual editing keys work: ←/→, Home/End, Backspace/Delete, Ctrl+W (delete a word)
and Ctrl+U (delete to the start).

### Copy Path

Alt+P (F9) puts the full path of the target entries on the clipboard, one per line. The selection
is kept.

## Views

### File preview

Alt+V (F3) shows the file under the cursor in place of the active pane; the other pane stays
visible; on a directory it does nothing. Long lines wrap. Only text is shown: a binary file (invalid UTF-8 or a NUL byte) shows a
placeholder, and only the first 100 KB of a larger file is shown.

| Key | Action |
| --- | --- |
| `↑` `↓` or `k` `j` | Scroll by a line |
| `PgUp` `PgDn` | Scroll by a page |
| `Home` or `g`, `End` or `G` | Top or bottom |
| `Esc` or `q` | Close |

### Favorites and drives

Alt+F opens a list in place of the active pane: the favorites first (your home directory and
whichever of Documents, Downloads, Desktop, Pictures, Music and Videos exist in it), then the
drives mounted under `/media`, `/mnt` and `/run/media`.

| Key | Action |
| --- | --- |
| `↑` `↓` or `k` `j`, `Home` `End` | Move |
| `Enter` | Open in the active pane |
| `d` or `Delete` | On a favorite: remove it from the list. On a drive: unmount it with `umount`. Both ask `y`/`n` in the status bar first, with the list still showing; `n` or Esc returns to the list |
| `Esc`, `q` or `Alt+F` | Close |

Alt+A adds the directory under the cursor to the favorites, or the active pane's own directory
when the cursor is on a file or on `..`; a directory already in the list is not added again. Favorites are not saved yet: they reset to the defaults
when Twin restarts. The drive list is read when Twin starts and after each unmount.

### Sync panes and Open in other

Alt+I opens the active pane's directory in the other pane. Alt+O opens the directory under the
cursor (including `..`) in the other pane; on a file it does nothing. The active pane stays
active.

## Errors

A failed operation shows in red in the status bar for five seconds; when a copy or move had
several failures, the toast names the first and counts the rest. Every error is kept in the error
log (Alt+L) with the time it happened, up to the last 500, until Twin quits.

The log opens in place of the active pane, newest entry at the bottom:

| Key | Action |
| --- | --- |
| `↑` `↓` or `k` `j` | Scroll by a line |
| `PgUp` `PgDn` | Scroll by a page |
| `Home` or `g`, `End` or `G` | Oldest or newest |
| `Esc`, `q` or `Alt+L` | Close |

A directory that cannot be opened (no permission, for example) leaves the pane where it was and
reports why.

## Known limitations

These are tracked in the [roadmap](Twin%20%E2%80%94%20Roadmap.md).

- Delete is permanent; there is no trash and no undo.
- Copy and move show no progress and cannot be cancelled.
- Nothing is saved between sessions, favorites included, and there is no config file.
- Preview reads the whole file before cutting it at 100 KB, so previewing a huge file is slow.
- Long names and paths are not shortened to fit the pane.
- An entry whose details cannot be read while listing a directory is left out of the listing,
  and no error is shown.

## Under the hood

Twin is written in Go with [Bubble Tea v2](https://github.com/charmbracelet/bubbletea), an
Elm-style Model-View-Update framework, [Lip Gloss v2](https://github.com/charmbracelet/lipgloss)
for styling and the text field from [Bubbles v2](https://github.com/charmbracelet/bubbles). The
whole state is one `model`; `Update` turns key presses and results into a new model; `View`
renders it. Anything that touches the disk runs in a command off the UI thread and reports back
with a message, so the interface does not freeze.

Dialogs are modals on a stack: the one on top gets every key press and paste until it closes,
so nothing reaches the panes or the dialog below it. A modal draws either a line in the status
bar or a panel in place of the active pane, so a question asked from the favorites list leaves
the list showing.

| File | Contents |
| --- | --- |
| `model.go` | The `model` and `pane` state, and the initial state |
| `update.go` | `Update`: results of commands, the shortcuts, pane navigation and quick search |
| `modal.go` | The modal stack |
| `modal_confirm.go`, `modal_input.go` | Yes/no question and text prompt in the status bar |
| `modal_conflict.go` | Overwrite questions for a copy or move |
| `modal_favorites.go`, `modal_log.go`, `modal_preview.go` | Favorites, error log and file preview |
| `commands.go` | Commands that do I/O and return a message |
| `msg.go` | The message types |
| `fs.go` | Filesystem helpers: listing, the copy/move plan, copy, move, drives |
| `keys.go` | The key map: every shortcut, its Alt binding and F-key alias |
| `view.go`, `styles.go` | Rendering and its styles |
| `utils.go` | Fuzzy matching, text wrapping and small helpers |

Copy and move are planned, resolved and only then executed. `planBatch` sorts a batch into clean
entries, conflicts and refused entries without touching anything; a conflict modal asks about
each conflict; `transferFiles` runs the batch and re-checks the same safety rules.

The shortcut table above mirrors `keys.go`; `docs_test.go` fails when they disagree, so a new or
changed shortcut needs a matching row. For development commands and CI see the
[README](README.md).
