# Twin — Roadmap

Oct 4, 2026 · @Meir Kanevskiy

Twin is an orthodox two-pane commander at roughly mc-1995 feature level, wrapped in a clean Bubble Tea MVU codebase. The skeleton is good enough to build on; the file-operation layer is not yet safe to trust with real data.

## Where Twin stands today

**What works:** two panes with Tab switching; arrow/Home/End/PgUp/PgDn navigation; `..` parent entry with cursor restored on the way up; Insert to select; copy, move, delete, mkdir with a y/n/All/skip-all overwrite prompt; copy path to clipboard with an OSC 52 fallback; a plain-text preview (100 KB cap, binary detection); a favorites + mounted-drives popup with unmount; sync panes (Alt+I) and open-dir-in-other-pane (Alt+O); xdg-open on Enter; prefix-then-fuzzy quick search; an Alt hint bar.

**What's good about the architecture:**

- The file split by responsibility (model / update / commands / msg / fs / keys / view / styles) is clean and easy to navigate.
- All I/O already lives in `tea.Cmd` closures that return messages, so the UI never blocks and there is a natural seam for a job engine.
- Keys go through a `KeyMap` with canonical Alt bindings plus F-key aliases, so remapping and presets are a small step away.
- `focusPath` on reload is the right primitive for keeping the cursor where the user expects it.

**What will hurt as it grows:**

- Modal state is a chain of mutually exclusive booleans in one \~600-line `Update`. Every new dialog adds a branch and a chance to forget a guard (one is already missing; see below).
- `pane` is a plain struct with no notion of a data source, so archives, SFTP, search results or a tree view would each need special cases.
- No config, no persistence (favorites vanish on exit), no tests, and `m.err` is set in many places but never rendered.
- It's on Bubble Tea v1 while emitting a raw Kitty keyboard escape in `main.go`. Charm shipped v2 of Bubble Tea, Lip Gloss and Bubbles in February 2026 ([release notes](https://github.com/charmbracelet/bubbletea/releases)), and v2 handles keyboard enhancements, modifier events and OSC 52 natively.
- DOCUMENTATION.md has drifted: it says Alt+I / Ctrl+I select files, but Alt+I is Sync Panes, Ctrl+I is indistinguishable from Tab in most terminals, and selection is actually Insert.

## Fix first: bugs found reading the code

Four of these can destroy data today, so they come before any feature work. All were found by reading the source, not by running it; each deserves a regression test when fixed.

| Severity | Where | What happens | Fix |
| --- | --- | --- | --- |
| Critical · ✅ fixed | `fs.go` `copyFile` / `copyDir`, `update.go` Copy | **Copying with both panes on the same directory truncates the source.** Both panes start in `cwd`; F5 reports a conflict, `y` runs `copyFile(src, src)`, and `os.Create` truncates the file before `io.Copy` reads it. On a directory, every file underneath is zeroed. | Reject src == dst (and dest-inside-src) when planning the batch, before any I/O. |
| Critical · ✅ fixed | `update.go` Delete / Copy / Move, `fs.go` `readDirectory` | The synthetic `..` entry is a real target. The cursor sits on `..` (index 0) after every directory change, so Alt+D then `y` runs `os.RemoveAll` on the **parent directory**. Insert can also select `..`. Copy/move of `..` targets `filepath.Join(dest, "..")` (the *other pane's parent*), always conflicts, and `y` copies the whole parent tree there; with both panes in one directory, combined with the row above, it zeroes every file under the parent. | Mark `..` as a pseudo-entry (`IsParent bool`), exclude it from selection and from every operation's fallback-to-cursor. |
| Critical · ✅ fixed | `update.go` overwrite loop, `processOverwriteConflicts` | Pressing `y` (or `n`) on the first conflict calls `processOverwriteConflicts()` immediately, which force-copies **all remaining** conflicts. Answering `n` to file 1 silently overwrites files 2..N, while the prompt keeps asking about file 2 as if it were untouched. `overwriteAll` is set but never read. | Turn conflict resolution into an explicit queue: one decision per file, apply only decided items, then advance. |
| Critical · ✅ fixed | `commands.go` `copyFilesCmd` / `moveFilesCmd` | If any file conflicts, the command returns before copying anything; resolution only re-runs the conflicting files. Non-conflicting files in the batch are silently dropped. | Plan the whole batch first (conflicts + clean items), resolve, then execute everything. |
| High · ✅ fixed (modal stack) | `update.go` favorites branch + final delegation guard | The guard before `pane.update` omits `isFavoritesOpen`, so up/down/Home/End move both the popup and the pane, and `d`, `j`, `k` leak into quick search (`q` returns early and does not). Any unmatched key in the unmount / remove-favorite confirms also reaches the pane. | Replace the boolean chain with a modal stack whose top always consumes input. |
| High · ✅ fixed | `update.go` `directoryLoadedMsg`, `fs.go` `readDirectory` | Entering an unreadable directory (e.g. a root-owned `lost+found` on a `/run/media` drive) sets the path, gets `files == nil` and no `..`, and the error is not shown: the pane is stranded with no way up. | On load error keep the previous path and listing and surface the error; always offer a parent entry. |
| High · ✅ fixed | `commands.go` `moveFilesCmd` | `os.Rename` fails with EXDEV across filesystems, so moving to a USB drive (a featured use case) always errors. Renaming onto a non-empty dir also fails. | Fallback to copy + verify + delete on EXDEV; merge semantics for directories. |
| High · ✅ fixed | `fs.go` `copyDir` | Copying a directory into itself recurses forever. `DirEntry.IsDir()` is false for symlinks, so a link to a directory goes to `copyFile` and fails with "is a directory", leaving a partial copy; a link to a file becomes a full copy. mtimes are not preserved. | Reject dest-inside-source; copy symlinks as symlinks by default; preserve mode + mtime. |
| High · ✅ fixed | everywhere `m.err` is set | Errors are stored but never rendered, so a failed copy or delete looks like success. | Toast/notification area plus an error log (`Ctrl+L` or similar). |
| Medium · ✅ fixed | mkdir input, quick search | `len(msg.String()) == 1` counts bytes, so Hebrew, Russian or any non-ASCII letter is dropped; backspace slices bytes and can corrupt UTF-8. Paste is ignored. | Use `bubbles/textinput` for every text field; match on runes. |
| Medium · ✅ fixed | `fs.go` `readDirectory` | Symlinked directories are listed as files (`entry.IsDir()` is false for links), so Enter runs `xdg-open` instead of entering them. | `os.Stat` symlinks to resolve the target type; keep a `IsSymlink` flag for display. |
| Low · ✅ fixed | `main.go` | Kitty keyboard flags `15` are pushed by hand, but Bubble Tea v1 can't parse CSI-u. The push happens on the main screen before alt-screen is entered, and the kitty spec keeps separate flag stacks per screen, so it is likely a no-op; `os.Exit(1)` also skips the deferred pop. | Remove now; let Bubble Tea v2 negotiate keyboard enhancements. |
| Medium · ✅ fixed | `commands.go` `previewFileCmd` | `os.ReadFile` loads the whole file before the 100 KB check, so previewing a multi-GB file reads it all. The cut can land mid-rune. | Open, read the first N KB, trim to a rune boundary. |
| Low · status bar and preview ✅ fixed; pane names open | `view.go` `statusBarView`, preview | `status[:availableWidth]` slices bytes and panics if the width goes negative on a tiny terminal; the preview's `contentLines[start:end]` panics the same way when `innerHeight` is negative. Long names and paths are never truncated in panes. | Width-aware truncation with an ellipsis (`ansi.Truncate`); clamp all computed dimensions at 0. |
| Low | `update.go` `fileDeletedMsg` | Cursor is adjusted against the old file list before the reload arrives. | Restore position by neighbouring path via `focusPath`. |
| Low · ✅ fixed | `commands.go`, `main.go` | Dead code: `deleteFileCmd` is unused; `tea.WithInput(os.Stdin)` is the default. | Remove. |

## Vision: the orthodox commander, modernized

Twin should be to mc what eza is to ls: the same mental model and muscle memory, with good defaults, rich information and zero required configuration.

**Where it sits.** The modern TUI file managers mostly abandoned the two-pane model: yazi, lf, ranger and nnn are single-pane or Miller-column browsers built for viewing and hopping. superfile is the closest Go/Bubble Tea relative and is polished, but it isn't built around the Norton workflow of "select here, act onto there". On the orthodox side, mc and far2l are faithful and capable but look and feel like 1998. The open gap is a two-pane commander with mc/TC power and starship/zellij-level polish.

**Design principles** (use these to settle arguments later):

1. **Beautiful with zero config.** True colour, Nerd Font icons and rounded borders out of the box; detect capabilities and degrade to 256/16 colours and ASCII glyphs automatically.
2. **Density with hierarchy.** Show more than mc (git state, size scale, relative age), but make the eye land on the name first. Colour encodes meaning, never decoration.
3. **Discoverable, never memorized.** Everything is reachable from a command palette and a which-key hint bar that changes when you hold a modifier. This answers your "remember the long flag sequence" complaint directly.
4. **Respect muscle memory.** mc/TC F-key layout by default (F3 view, F4 edit, F5 copy, F6 move, F7 mkdir, F8 delete, F10 quit, Ctrl+O shell, Ctrl+\\ hotlist). The default is a TC/mc hybrid: mc's F-keys, command line and Ctrl+O, plus TC's tabs (Ctrl+T new, Ctrl+W close, Ctrl+Tab cycle) and multi-rename on Ctrl+M. Strict mc, TC and vim presets stay available.
   **Terminal caveat:** Ctrl+M is the same byte as Enter and Ctrl+Tab is indistinguishable from Tab in legacy terminals (the same problem as Ctrl+I/Tab), so they only work with the Kitty keyboard protocol and each needs a legacy-safe fallback binding. Alt+F1/F2/F7 are commonly grabbed by GNOME/KDE.

**Key allocation to settle before Phase 2** (the draft below double-books several keys):

| Key | Claimed by | Proposed resolution |
| --- | --- | --- |
| F2 | key-bar "menu", rename in place, user menu | F2 = user menu (mc); rename = Shift+F6 only; F9 = top menu (mc) |
| F9 | command palette, today's Copy Path | Palette on Ctrl+P only; move Copy Path off F9 |
| Ctrl+L | error log, info panel | Info panel = Ctrl+L (mc); error log = Alt+L or palette-only |
| Ctrl+M | multi-rename | Kitty-only; on legacy terminals reachable from the palette (Alt+M is taken by Move today) |
| Ctrl+Tab | cycle tabs | Kitty-only; fallback Alt+, / Alt+. |
5. **Never block, never lose data.** Every operation is a cancellable background job with progress. Delete goes to trash by default. Rename, move and trash are undoable.
6. **The shell is a first-class citizen.** mc's command line and Ctrl+O subshell are its most underrated features and most modern managers dropped them. Twin keeps them and makes them smarter.
7. **Fast on huge directories.** Stream listings, render only the viewport, compute sizes and git status lazily in the background.

## Target look for v1.0

The v1.0 screen keeps mc's layout (two panes, command line, F-key bar) and adds a starship-style header, eza-style columns, a job strip and a live hint bar.

```text
╭─ ~/src/twin   main ↑1 ●3 ─────────────╮╭─ /mnt/usb/backup ─────── 41G free ─╮
│ ⌕ .go                                 ││                                    │
│   ..                                  ││   ..                               │
│ ▸ .github/                   -     2d ││ ▸ photos/              12G    3mo  │
│ ▸ cmd/                  M    -     1h ││ ▸ twin-2025/           88M    1y   │
│▌▸ internal/             ●    -     5m ││   notes.md              4K    2w   │
│   commands.go           M   12K    5m ││                                    │
│ ✓ fs.go                      4K    2d ││                                    │
│ ✓ update.go             M   21K    1h ││                                    │
│   go.mod                     1K    3w ││                                    │
╰─ 2 selected · 25K ─────── 9 items ────╯╰─────────────────────── 4 items ────╯
 ⟳ copy 3 → /mnt/usb/backup  ━━━━━━━━━━━━╺━━━━━━━━  58%  42 MB/s  12s left
 ~/src/twin ❯ go build ./... █
 F1 help  F2 menu  F3 view  F4 edit  F5 copy  F6 move  F7 mkdir  F8 trash  F10 quit
```

Reading it top to bottom:

- **Pane header** = breadcrumb path, git branch with ahead/behind and dirty count, free space for mount points. Path segments collapse (`~/s/twin`) when narrow.
- **Filter line** (`⌕ .go`) appears only while a quick filter is active; quick *search* stays cursor-jumping like mc.
- **Columns** = icon, name, git status, size (coloured on a scale), relative age (fresher = brighter). Columns drop off right-to-left as width shrinks: age, size, git.
- **Footer** = selection count and total size, item count; in a git repo, staged/unstaged totals.
- **Job strip** = one line per running operation, collapsible; disappears when idle.
- **Command line** = mc-style, always present, runs in the active pane's directory.
- **Key bar** = F-keys by default; holding Alt or Ctrl swaps it to that layer (your removed `ModifierState` idea, made possible by key-release events in Bubble Tea v2 on Kitty-protocol terminals).

## Roadmap in nine phases

Phases 0 and 1 are sequential and unglamorous; they make everything after them cheap. From Phase 4 on, preview, shell and VFS work can run in parallel.

### Phase 0 — Stabilize (v0.1)

Goal: Twin never loses a file and never fails silently.

- [x] Fix every Critical/High item in the bug table above.
- [x] Render errors: a toast line that fades, plus a scrollable message log (Alt+L).
- [x] Extract a pure `planBatch(sources, dest) → {clean, conflicts, rejected}` so the safety rules (`..`, src == dst, dest-inside-src) live in one testable place, not in `Update`.
- [x] `fs_test.go` with `t.TempDir()` covering copy, move, delete, conflicts, `..`, same-directory copy, symlinks, dir-into-itself, unreadable dirs, cross-device (via a tmpfs mount in CI where possible). *(cross-device is simulated by failing rename with EXDEV; `TestMoveAcrossRealDevices` runs a real one when `TWIN_XDEV_DIR` points at another filesystem, for the CI tmpfs)*
- [x] Model-level tests that drive `Update` with key messages and run the returned commands: two of the Critical bugs live in `update.go`, which `fs_test.go` alone can't catch.
- [x] CI: `go vet`, `gofmt`, `golangci-lint`, tests on every push. *(`.github/workflows/ci.yml`; tests run with `-race` and `TWIN_XDEV_DIR` on a tmpfs, so the real cross-device move runs too, and the job fails if it skips)*
- [x] Rewrite DOCUMENTATION.md from `keys.go` (later: generate it). *(until the actions registry generates it, `docs_test.go` fails when its shortcut table and `keys.go` disagree)*

Exit: all fs and model tests green; deleting `..` and copying a file onto itself are impossible.

### Phase 1 — Foundation (v0.2)

Goal: an architecture that can absorb fifty features without `Update` becoming 3,000 lines.

- [x] **Migrate to Bubble Tea v2 + Lip Gloss v2 + Bubbles v2** (`charm.land/...`). Gains: native keyboard enhancements and key-release events, split mouse messages, declarative alt-screen, built-in OSC 52, faster renderer. Drop the hand-rolled Kitty escape. *(the view also asks for alternate keys, so Alt/Ctrl shortcuts work in a Hebrew or Russian layout on Kitty-protocol terminals; the OSC 52 clipboard fallback goes through Bubble Tea)*
- [x] **Modal stack** replacing the boolean chain: `[]Modal` where each modal implements `Update/View` and the top one owns input. Dialogs become small files (`modal_confirm.go`, `modal_input.go`, `modal_conflict.go`). *(modals are values; each draws a status-bar line or a box in place of the active pane, so a confirm over the favorites keeps the list visible. Also `modal_favorites.go`, `modal_log.go`, `modal_preview.go`)*
- [x] **Pane as a component** with a `Source` interface (`List`, `Stat`, `Open`, `Ops`): local FS first, later archives, SFTP, search results, "panelize" lists. This is the single most important abstraction. *(`pane.go` and `source.go`. The interface is `List`, `Parent`, `Open`: a source supplies raw entries, and the pane sorts them and adds `..`, so Phase 4's sort modes live in one place. `Stat` and `Ops` are left for the sources that need them: copy, move, delete, mkdir, xdg-open and favorites are refused outside the local filesystem, so a path of another source never reaches `os`. Tests run a pane on an in-memory read-only source)*
- [ ] **Actions registry**: every command is a named action (`copy`, `toggle_hidden`, ...) with title, default keys and an enabled-predicate. Keymaps, the hint bar, the help screen and the command palette are all generated from it.
- [ ] **Config** at `$XDG_CONFIG_HOME/twin/config.toml` (keymap preset, theme, columns, opener rules, custom commands); **state** at `$XDG_STATE_HOME/twin/` (favorites, history, last dirs). Hot reload.
- [x] `bubbles/textinput` for every text field (Unicode, paste, cursor movement). *(mkdir and quick search; quick search takes typed text, Backspace and pastes only, so arrows keep moving the cursor in the pane)*
- [ ] Golden-file UI tests with `teatest`.

Exit: adding a dialog touches one new file and one registry line.

### Phase 2 — The look (v0.3)

Goal: the "eza moment". First screenshot people want to share.

- [ ] Theme system built on semantic tokens (`accent`, `dir`, `exec`, `symlink`, `git.modified`...). Ship Catppuccin, Tokyo Night, Gruvbox, Nord; auto light/dark via OSC 11 background query.
- [ ] Read `LS_COLORS` and eza's `theme.yml` so Twin matches the user's existing `ls` colours.
- [ ] Nerd Font icons by file type with automatic ASCII fallback (config flag + heuristic).
- [ ] Responsive columns: name, git status, size (heat-scaled colour), relative mtime (age gradient), permissions in full mode. Brief (multi-column names) and Full modes like mc's Alt+T.
- [ ] Starship-style pane header: collapsible breadcrumb, git branch, ahead/behind, dirty count, free space on mounts.
- [ ] Footer: selection count + total size, item count; mini status line for the cursor entry (size, owner, mtime, symlink target).
- [ ] Key bar with modifier layers; command palette (Ctrl+P / F9) with fuzzy match over every action.
- [ ] Proper truncation everywhere (middle-ellipsis for names: `very-long…name.go`).

Exit: looks right in Kitty, Ghostty, WezTerm, Alacritty, GNOME Terminal and a plain 16-colour tty.

### Phase 3 — Job engine and safety (v0.4)

Goal: TC-grade file operations.

- [ ] Job manager: queue, concurrency limit, progress (bytes, files, throughput, ETA), pause/cancel, background (job strip + a jobs panel).
- [ ] Plan → resolve → execute pipeline: scan the whole batch, show one conflict dialog with side-by-side size/date and options TC users expect: overwrite, skip, rename, overwrite-if-older, apply-to-all.
- [ ] Cross-device move, symlink handling options, preserve mode/mtime/ownership where permitted, optional checksum verify.
- [ ] Trash via the freedesktop spec (F8 trashes, Shift+F8 deletes permanently); trash browser.
- [ ] Undo stack for rename, move, trash, mkdir.
- [ ] `fsnotify` auto-refresh of visible panes.

Exit: copying 50 GB to a USB stick shows progress, can be cancelled, and leaves nothing half-written.

### Phase 4 — Commander power (v0.5)

Goal: functional parity with what you reach for daily in mc and Total Commander.

- [ ] Selection: by pattern (mc `+`, `-`, `*`), by extension, invert, select same as other pane, Shift+arrows range.
- [ ] Sorting (name, ext, size, mtime, natural, reverse, dirs-first) and filters (hidden toggle, glob filter).
- [ ] Rename in place (F2 / Shift+F6); **multi-rename tool** (TC Ctrl+M) with patterns, regex, counters, case and a live before/after preview. It opens as a modal over the panes and adds nothing to the main screen.
- [ ] F4 edit and Shift+F4 new file via `$EDITOR` using `tea.ExecProcess`; opener rules by MIME/extension.
- [ ] Directory history per pane (Alt+Left/Right), hotlist (Ctrl+\\) with frecency and optional zoxide import, persistent favorites.
- [ ] Tabs per pane (TC Ctrl+T) with names restored across sessions. The tab strip folds into the pane header and only appears once a pane has two or more tabs, so the default screen stays as clean as today.
- [ ] Quick view (Ctrl+Q): the inactive pane previews whatever the active cursor is on. Info panel (Ctrl+L) with dir size, filesystem, owner.
- [ ] Directory size on Space (mc/TC behaviour), async.
- [ ] Compare directories and **synchronize dirs** (TC's killer feature): diff by name/size/date/content with an action plan.
- [ ] Find files (Alt+F7) via `fd`/`rg` when installed, results as a virtual panel you can operate on (mc "panelize").
- [ ] chmod/chown, symlink/hardlink creation, user menu (F2) of custom commands from config with `%f`, `%s`, `%d` macros.
- [ ] Mouse: click, double-click, wheel, click column header to sort.

Exit: a week of daily use without opening mc.

### Phase 5 — Rich preview (v0.6)

- [ ] Syntax highlighting (chroma), Markdown (glamour), pretty JSON/YAML, hex view for binaries.
- [ ] Images via Kitty graphics / Sixel with half-block fallback; PDF via `pdftotext`; archive listings; media metadata.
- [ ] Directory preview: tree + size breakdown; git diff for modified files.
- [ ] Streamed, cached, cancellable previews so scrolling a big dir stays instant.

### Phase 6 — Shell integration (v0.7)

- [ ] mc-style command line with path completion and history (atuin import optional).
- [ ] Ctrl+O drops to `$SHELL` in the active pane's dir; on return, both panes resync.
- [ ] Insert cursor name / selection / other pane path into the command line (mc Alt+Enter, Ctrl+X P).
- [ ] cd-on-exit shell wrapper (`tw` function for bash/zsh/fish) so quitting leaves you in the last directory.
- [ ] Optional zellij/tmux awareness: open a file or a shell in a new multiplexer pane instead of suspending Twin.

### Phase 7 — Virtual filesystems (v0.8)

- [ ] Archives as directories (zip, tar.\*, 7z): browse, extract with F5, create with a pack dialog.
- [ ] SFTP/SSH panel (mc shell link), honouring `~/.ssh/config` hosts.
- [ ] Drives panel upgrade: `udisksctl` mount/unmount/power-off without root; Alt+F1/F2 drive picker per pane.

### Phase 8 — Ecosystem and 1.0

- [ ] Plugin hook: external commands with a JSON contract first; embedded Lua later only if demand is real.
- [ ] Packaging with goreleaser: GitHub releases, AUR, Homebrew, Nix, .deb/.rpm.
- [ ] README demos recorded with `vhs`; docs site generated from the actions registry.
- [ ] Performance pass: 100k-entry directory under 100 ms to first paint.

## Libraries and tooling

Stay in the Charm ecosystem for UI and lean on small, well-known Go packages elsewhere; shell out to fast system tools (`git`, `fd`, `rg`, `udisksctl`) when present instead of reimplementing them.

| Need | Choice | Note |
| --- | --- | --- |
| TUI runtime, styling, widgets | Bubble Tea v2, Lip Gloss v2, Bubbles v2 | textinput, viewport, progress, help |
| Width and truncation | `charmbracelet/x/ansi`, `rivo/uniseg` | grapheme-correct widths for Hebrew/Russian/CJK/emoji |
| Git status | `git status --porcelain=v2 -z` via exec | much faster than go-git on large repos; cache per repo root |
| Config / paths | `BurntSushi/toml` (or `pelletier/go-toml/v2`), `adrg/xdg` | XDG-correct config and state dirs |
| File watching | `fsnotify/fsnotify` | debounce bursts before reloading |
| Trash | small in-repo implementation of the freedesktop spec | `.trashinfo` + `files/`, per-mount trash dirs |
| Syntax / Markdown preview | `alecthomas/chroma/v2`, `charm.land/glamour` | theme mapped to Twin tokens |
| MIME detection | `gabriel-vasile/mimetype` | for icons, openers, preview routing |
| Archives | `mholt/archives` | zip, tar.\*, 7z read; exposes `io/fs` |
| SFTP | `pkg/sftp` + `golang.org/x/crypto/ssh` | reads `~/.ssh/config` via `kevinburke/ssh_config` |
| Fuzzy matching | `sahilm/fuzzy` | ranked matches for palette and filters |
| UI tests | `charmbracelet/x/exp/teatest` | golden snapshots of rendered frames |
| Release / demos | goreleaser, `charmbracelet/vhs` | reproducible GIFs for README |

## Milestones and what to build next

Each phase ships as a tagged release, so Twin is usable (and dogfoodable) from v0.1 onward.

&#91;embedded content: release path · 9 phases, v0.1 to v1.0\]

Phases 0 to 4 run in order; preview, shell and virtual filesystems can be built in any order once the commander core lands, and all three feed 1.0.

**The first PRs:**

1. ✅ **Done** (branch `fix-data-safety`). Make `..` a pseudo-entry excluded from all operations; add `planBatch` that rejects src == dst and dest-inside-src; replace the conflict loop with a per-file queue and execute whole batches (clean + resolved) together. Add `fs_test.go` and model-level tests covering each case.
2. ✅ **Done** (branch `render-errors`). Render errors (toast + Alt+L log); keep the previous listing when a directory fails to load; resolve symlinked directories. Also: copy symlinks as links, and refuse to overwrite a file with itself through a hard link or symlink.
3. ✅ **Done** (branch `finish-phase0-high`). Close the remaining High bugs: moves fall back to copy + flush + delete across filesystems and merge into existing directories; copies keep mode and mtime, replace files atomically via a temp file (so an overwritten file becomes a new file: owned by the user, without its ACLs/xattrs, a symlink there is replaced instead of written through, other hard links keep the old content) and refuse FIFOs/devices; a directory is never copied or moved through a symlink at the destination; the favorites popup no longer leaks keys to the pane (a guard for now, the modal stack replaces it).
4. ✅ **Done** (branch `finish-phase0`). Close Phase 0: CI runs gofmt, go vet, golangci-lint (standard linters) and the tests on every push and on pull requests from forks; DOCUMENTATION.md is rewritten from `keys.go` and the code, with a test keeping its shortcut table in sync. Clearing the lint findings also made the clipboard fallback report a failed OSC 52 write instead of dropping it.
5. ✅ **Done** (branch `bubbletea-v2-modal-stack`). Bubble Tea, Lip Gloss and Bubbles v2, with the hand-rolled Kitty escape gone; a modal stack replaces the boolean chain, one file per dialog (generic confirm and text prompt, conflicts, favorites, log, preview); mkdir and quick search use `textinput`, so Hebrew, Russian and pasted text work and Backspace edits the search. Overwrite questions for a batch that was still being checked when another dialog opened wait under it, so keys typed into that dialog can't answer them; Alt/Ctrl shortcuts in a Hebrew or Russian layout use the key's US-layout letter, while ASCII layouts (AZERTY, Dvorak) keep the letter typed. Along the way: a preview result that arrives after its preview closed is ignored, the preview follows the pane when the terminal is resized, a modal can no longer push the other pane aside, and the status bar truncates by width instead of slicing bytes.
6. ✅ **Done** (branch `pane-source`). The pane is a component in `pane.go`, reading through a `Source` (`source.go`: `List`, `Parent`, `Open`) with the local filesystem as the only one so far. Sync Panes carries the source across; favorites return to the local filesystem; actions that change files or hand them to other programs are refused on any other source. The preview reads through the source and only its first 100 KB, cut at a whole character, and refuses FIFOs and devices instead of hanging on them. A pane that changes source drops its selection, and the commands that touch the disk take their sources and refuse non-local ones themselves.
