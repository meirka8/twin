# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

Twin Manager: a minimalistic two-pane TUI file manager written in Go, built with
[Bubble Tea v2](https://github.com/charmbracelet/bubbletea) (MVU framework),
[Lipgloss v2](https://github.com/charmbracelet/lipgloss) (styling) and `textinput` from
[Bubbles v2](https://github.com/charmbracelet/bubbles), imported as `charm.land/.../v2`.
Linux-focused (drive detection reads `/proc/mounts`, unmounting shells out to `umount`).

## Commands

The package is split across multiple files in `package main` at the repo root, so always
build/run the package (`.`), not an individual file.

```bash
go run .              # run the app
go build .            # build the ./twin binary
go vet ./...          # static checks
gofmt -l .            # check formatting
golangci-lint run     # standard linters, configured in .golangci.yml
go test ./...         # run the tests
```

CI (`.github/workflows/ci.yml`) runs all of these on every push and must stay green: fix lint
findings rather than adding exclusions. It also sets `TWIN_XDEV_DIR` to a tmpfs so
`TestMoveAcrossRealDevices` runs a real cross-filesystem move, and fails if that test skips.

`fs_test.go` covers the filesystem helpers with `t.TempDir()`. `update_test.go` drives
`Update` with key messages and runs the returned commands (see its `send` helper), which is
how to test anything that goes through the modal flow. `docs_test.go` checks that the
shortcut table in DOCUMENTATION.md matches `keys.go`.

## Architecture

The app follows Bubble Tea's Model-View-Update (MVU) pattern, split across files by
responsibility rather than by feature:

- `model.go` — the `model` struct (whole app state: both panes, the modal stack `modals`, the
  error log, favorites). `initialModel()` builds the starting state; `Init()` kicks off the
  initial directory loads for both panes. `activePane()` / `inactivePane()` return pointers
  to the panes.
- `pane.go` — the `pane` component: one directory of a `Source` (`source`, `path`), its
  listing, cursor, viewport, selection and quick-search `textinput`. `loadDirectoryCmd` /
  `reloadCmd` list a directory through `listDir`, which sorts it (directories first) and puts
  `..` from `Source.Parent` in front; `applyListing` installs the result. `targets()` /
  `selectedFiles()` pick what an operation acts on; `update()` handles navigation and quick
  search; `view()` renders the pane.
- `source.go` — the `Source` interface (`List`, `Parent`, `Open`) and `localFS`, the only
  implementation so far. A pane reads only through its source, so archives, SFTP or search
  results can plug in later. Everything that changes files or hands them to another program
  (copy, move, delete, mkdir, xdg-open, favorites) runs only on `localFS`, so a path of another
  source never reaches `os`: the shortcut refuses with `localOnly` for the user's sake, and the
  command, which takes the source(s), checks again. Copy Path is the exception: it copies text.
  A pane that changes source drops its selection, since paths of different sources can look
  alike. Sources are compared with `==`; add `var _ = assertComparable[T]` for a new one.
- `update.go` — `Update()`, the core control flow. It first switches on async result messages
  (`directoryLoadedMsg`, `fileOperationMsg`, `transferPlannedMsg`, etc.), which apply whatever
  modal is open. Every other message (keys, pastes) goes to the modal on top of the stack if
  there is one, otherwise to `shortcut()` (the `KeyMap` actions) and then to the active
  pane's `update()`.
- `modal.go` — the `modal` interface and the stack (`openModal`, `setModal`,
  `updateTopModal`). Modals are values: `update(m *model, msg)` returns the modal's new state,
  or nil to close it, and may open another modal over itself with `m.openModal`. A modal
  renders either `statusView` (a line in the status bar) or `paneView` (a box in place of the
  active pane). The panes show the topmost modal with a pane view, so a confirm over the
  favorites leaves the list visible; the status bar shows only the top modal's line. A modal
  that arrives from a command result rather than a key (the conflict modal, on
  `transferPlannedMsg`) goes to the bottom of the stack, so keys typed into a dialog that was
  opened in the meantime never answer it. One file per dialog: `modal_confirm.go` and `modal_input.go` (generic y/n
  question and text prompt, each taking a callback), `modal_conflict.go` (per-file overwrite
  questions; it runs the copy/move batch), `modal_favorites.go`, `modal_log.go`,
  `modal_preview.go`.
- `commands.go` — `tea.Cmd` constructors that perform actual I/O (filesystem ops, `xdg-open`
  through the `launch` variable, which tests stub, `umount`, clipboard) inside a returned
  closure and report results via a message type. Follow
  this pattern for new I/O: never mutate model state directly from a command closure — return
  a message and handle it in `Update`.
- `msg.go` — message types returned by commands (e.g. `directoryLoadedMsg`, `fileDeletedMsg`,
  `fileConflictMsg`, `previewReadyMsg`) that `Update` switches on.
- `fs.go` — local filesystem helpers with no Bubble Tea dependency: `readDirectory` (the raw
  entries of `localFS.List`, symlinks resolved), the copy/move plan (`planBatch`) and
  `transferFiles`, recursive `copyFile`/`copyDir`, `getMountedDrives`
  (parses `/proc/mounts`, filters to `/media`, `/mnt`, `/run/media`), `getStandardPaths` (home
  + XDG-style user dirs for the favorites list).
- `keys.go` — `KeyMap`/`Shortcut`: every action has a canonical `alt+<letter>` binding plus an
  optional F-key alias. `GetAliasMap()` maps F-key aliases back to their canonical key string;
  `shortcut()` resolves incoming key events through this map before switching on `m.keyMap.X.Key`,
  so a new shortcut must be added to `KeyMap` (and `GetShortcuts()`) rather than matched as a
  raw string, and to the shortcut table in DOCUMENTATION.md.
- `view.go` — `View()` (a `tea.View`: alt screen and keyboard enhancements are declared there)
  and the layout: the two panes (each from `pane.view()`), status bar and hints bar. A modal's pane view is
  cut to the pane's box, so it never shifts the other pane. Lip Gloss v2 counts borders and
  padding in `Width`/`Height`: `pane.width`/`height` are the content size inside the border,
  `pane.boxSize()` the size on screen.
- `styles.go` — all `lipgloss.Style` definitions used by `view.go`.
- `utils.go` — small pure helpers (`fuzzyMatch` for quick search, `calculateWrappedLines` for
  preview text wrapping, `clampIndex`, `oneLinePaste`).

### Key conventions

- File operations (copy/move) always operate from the *active* pane to the *inactive* pane's
  current path. Targets come from `pane.targets()`: the selection, or the file under the
  cursor when nothing is selected. Selections are cleared on copy/move/delete.
- The synthetic `..` entry has `IsParent` set and is never a target: it can't be selected,
  deleted, copied or moved. Use `IsParent`, not `Name == ".."`.
- Copy/move is plan → resolve → execute. `planTransferCmd` runs `planBatch`, which sorts the
  batch into clean files, conflicts and rejected files (`..`, source == destination, a
  directory into itself) without touching anything. `Update` then asks y/n/A(ll)/s(kip all)
  per conflict (`resolveConflicts`) and only when every conflict is decided runs
  `transferFilesCmd` once for the clean files plus the ones chosen for overwriting. Esc
  cancels the whole batch. `transferFiles` re-checks the same safety rules.
- Errors go through `m.reportError(err)`, which appends to the error log (Alt+L) and shows
  a status-bar toast; return the command it gives back so the toast expires. Never drop an
  error or store it without reporting it.
- `loadDirectoryCmd(source, path, focusPath)` both reloads (the pane's own location; use
  `reloadCmd(focusPath)`) and navigates (another path, or another source: Sync Panes copies
  the active pane's source, favorites go back to `localFS{}`). The pane only moves once the
  listing succeeds, so don't set `pane.source` or `pane.path` yourself before loading.
- Directory reloads pass an optional `focusPath` so the cursor can be restored to a specific
  entry after the reload (e.g. focus the newly created folder, or the parent's own path when
  navigating up via `..`).
- Dialogs are modals on the stack, never flags on the model. A yes/no question or a text prompt
  is `newConfirmModal` / `newInputModal` with a callback; anything else is a new type
  implementing `modal` in its own `modal_*.go` file, opened with `m.openModal`. Results that
  arrive for a modal later (like `previewReadyMsg`) are matched to it in `Update`'s result
  switch, so a stale one is ignored.
- Keys are `tea.KeyPressMsg`. Match `msg.String()` against `KeyMap` keys; a key that types
  something has `msg.Text` set (any script; `String()` then returns the text, and `"space"`
  for a space). Text fields are `bubbles/textinput`; pastes arrive as `tea.PasteMsg`.

## CVC (Continuity/Version Context) MCP

If the `cvc` MCP server is installed and available (tools named `mcp__cvc__*`), use it to track
reasoning across the session:

- Call `mcp__cvc__get_context` for a file before editing it, to see whether it's clean or has an
  outstanding dirty/patch state from a prior session.
- After completing a non-trivial reasoning step, investigation, or fix, call
  `mcp__cvc__commit_thought` with the task, your reasoning (including what you ruled out and
  why), and the response/action taken. Do this especially when the fix wasn't obvious from
  reading the current code alone (e.g. required digging through `git log`/`git blame`/merge
  history), so the "why" is preserved for future sessions.
- Use `mcp__cvc__read_history` when picking up unfamiliar or in-progress work to see what
  reasoning already happened before re-deriving it.
- If the `cvc` MCP tools are not present in the tool list, skip this section entirely — do not
  attempt to install or configure it unprompted.
